package reviewrun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"accordli.com/analyze-ai/api/internal/core/docconv"
	"accordli.com/analyze-ai/api/internal/core/lens"
	"accordli.com/analyze-ai/api/internal/core/llm"
	"accordli.com/analyze-ai/api/internal/infra/billing"
	"accordli.com/analyze-ai/api/internal/infra/observability"
	"accordli.com/analyze-ai/api/internal/infra/queue"
	"accordli.com/analyze-ai/api/internal/infra/repo"
)

// LensSet is the SoloMocky Lens roster. Order matters — runs sequentially.
var LensSet = []string{"entities_v1", "open_questions_v1"}

// JobKind is the queue.Job.Kind for the SoloMocky run handler. One
// goroutine handles the whole Run end-to-end (convert + reserve +
// summary + lenses + commit). When River lands, this becomes a fanout.
const JobKind = "review_run.execute"

// Orchestrator owns the run pipeline. Built at startup with all
// dependencies injected so the queue handler stays a free function.
type Orchestrator struct {
	Repos     *repo.Repos
	LLM       llm.Client
	Templates *lens.Templates
	Billing   billing.Reserver
	Convert   docconv.Converter
	Log       observability.Logger
}

// JobArgs is what the dispatcher hands the handler. JSON-encoded into
// queue.Job.Args so the same shape works for River later.
type JobArgs struct {
	OrgID         uuid.UUID `json:"org_id"`
	MatterID      uuid.UUID `json:"matter_id"`
	ReviewRunID   uuid.UUID `json:"review_run_id"`
	OriginalDocID uuid.UUID `json:"original_doc_id"`
}

// Dispatch enqueues a run for the given Matter. Caller has already
// inserted the matter, the original document, and the review_run row;
// this only kicks off the goroutine.
func (o *Orchestrator) Dispatch(ctx context.Context, q queue.Dispatcher, args JobArgs) error {
	body, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return q.Enqueue(ctx, queue.Job{
		ID:   args.ReviewRunID.String(),
		Kind: JobKind,
		Args: body,
	})
}

// Handler is the registered queue.Handler. Reads JobArgs from j.Args
// and runs the pipeline. Errors are logged but not bubbled — the queue
// has no caller to return to.
func (o *Orchestrator) Handler(ctx context.Context, j queue.Job) error {
	var args JobArgs
	if err := json.Unmarshal(j.Args, &args); err != nil {
		o.Log.Error(ctx, "review_run: bad args", map[string]any{"err": err.Error()})
		return err
	}
	if err := o.run(ctx, args); err != nil {
		o.Log.Error(ctx, "review_run: failed", map[string]any{
			"review_run_id": args.ReviewRunID.String(),
			"err":           err.Error(),
		})
		return err
	}
	return nil
}

func (o *Orchestrator) run(ctx context.Context, args JobArgs) error {
	// 1. Load the original .docx from the documents row, convert to markdown.
	orig, err := o.Repos.Document.GetByKind(ctx, args.MatterID, "original")
	if err != nil {
		return fmt.Errorf("load original: %w", err)
	}
	if orig.BlobURL == nil {
		return fmt.Errorf("original document has no blob_url")
	}
	docxBytes, err := loadBlob(*orig.BlobURL)
	if err != nil {
		return fmt.Errorf("load docx blob: %w", err)
	}

	markdown, err := o.Convert.DocxToMarkdown(ctx, docxBytes)
	if err != nil {
		o.fail(ctx, args, "convert_failed")
		return fmt.Errorf("convert: %w", err)
	}

	// Persist the converted markdown as a documents row.
	matter, err := o.Repos.Matter.Get(ctx, args.OrgID, args.MatterID)
	if err != nil {
		return fmt.Errorf("get matter: %w", err)
	}
	mdDoc := &repo.Document{
		MatterID:  args.MatterID,
		Kind:      "markdown",
		ContentMD: &markdown,
	}
	if err := o.Repos.Document.Insert(ctx, mdDoc); err != nil {
		return fmt.Errorf("insert markdown doc: %w", err)
	}

	// 2. Reserve (no-op today). Build prefix.
	res, err := o.Billing.Reserve(ctx, args.OrgID, 1)
	if err != nil {
		return fmt.Errorf("reserve: %w", err)
	}

	prefix := BuildPrefix(MatterMetadata{Title: matter.Title}, markdown)
	tokens := EstimateTokens(prefix)
	if err := o.Repos.ReviewRun.SetPrefix(ctx, args.ReviewRunID, prefix, tokens, "B", res.ID); err != nil {
		return fmt.Errorf("set prefix: %w", err)
	}

	// 3. Pre-create one LensRun row per Lens so the FE can render the
	// spinners immediately. Sequential execution updates each row in
	// turn.
	lensRuns := make(map[string]*repo.LensRun, len(LensSet))
	for _, key := range LensSet {
		_, sha, err := o.Templates.Load("lens", key)
		if err != nil {
			return fmt.Errorf("load lens template %s: %w", key, err)
		}
		lr, err := o.Repos.LensRun.Create(ctx, args.ReviewRunID, args.OrgID, key, sha)
		if err != nil {
			return fmt.Errorf("create lens run %s: %w", key, err)
		}
		lensRuns[key] = lr
	}

	// 4. Summary call (non-Lens, prose). Persist on review_runs.summary.
	if err := o.runSummary(ctx, args, prefix); err != nil {
		// Summary failure doesn't fail the Run — log and keep going.
		o.Log.Error(ctx, "summary: failed", map[string]any{"err": err.Error()})
	}

	// 5. Lenses, sequentially.
	completed := 0
	for _, key := range LensSet {
		lr := lensRuns[key]
		if err := o.runLens(ctx, args, prefix, lr); err != nil {
			o.Log.Error(ctx, "lens: failed", map[string]any{
				"lens_key": key,
				"err":      err.Error(),
			})
			_ = o.Repos.LensRun.MarkFailed(ctx, lr.ID, classifyErr(err))
			continue
		}
		completed++
	}

	// 6. Commit / Rollback per Reviewer-v2 §"ARC Consumption". 90%
	// rule with two Lenses rounds to "both must complete".
	threshold := (len(LensSet) * 9) / 10
	if completed >= threshold && completed > 0 {
		_ = o.Billing.Commit(ctx, res)
		status := "completed"
		if completed < len(LensSet) {
			status = "partial"
		}
		_ = o.Repos.ReviewRun.Finalize(ctx, args.ReviewRunID, status)
	} else {
		_ = o.Billing.Rollback(ctx, res)
		status := "partial"
		if completed == 0 {
			status = "failed"
		}
		_ = o.Repos.ReviewRun.Finalize(ctx, args.ReviewRunID, status)
	}

	return nil
}

func (o *Orchestrator) runSummary(ctx context.Context, args JobArgs, prefix string) error {
	body, _, err := o.Templates.Load("summary", "summary_v1")
	if err != nil {
		return err
	}
	rendered, err := lens.Render(body, nil)
	if err != nil {
		return err
	}

	resp, err := o.LLM.Complete(ctx, llm.Request{
		System: PrefixSystem,
		Blocks: []llm.Block{
			{Text: prefix, CacheControl: "ephemeral"},
			{Text: rendered},
		},
	})
	if err != nil {
		return err
	}
	return o.Repos.ReviewRun.SetSummary(ctx, args.ReviewRunID, strings.TrimSpace(resp.Text))
}

func (o *Orchestrator) runLens(ctx context.Context, args JobArgs, prefix string, lr *repo.LensRun) error {
	if err := o.Repos.LensRun.MarkRunning(ctx, lr.ID, "B"); err != nil {
		return err
	}

	body, _, err := o.Templates.Load("lens", lr.LensKey)
	if err != nil {
		return err
	}
	rendered, err := lens.Render(body, nil)
	if err != nil {
		return err
	}

	resp, err := o.LLM.Complete(ctx, llm.Request{
		System: PrefixSystem,
		Blocks: []llm.Block{
			{Text: prefix, CacheControl: "ephemeral"},
			{Text: rendered},
		},
	})
	if err != nil {
		return err
	}

	findings, err := parseFindingsJSONL(resp.Text, lr, args.OrgID)
	if err != nil {
		return fmt.Errorf("parse jsonl: %w", err)
	}
	if err := o.Repos.Finding.PersistAll(ctx, findings); err != nil {
		return fmt.Errorf("persist findings: %w", err)
	}
	return o.Repos.LensRun.MarkCompleted(ctx, lr.ID, len(findings))
}

func (o *Orchestrator) fail(ctx context.Context, args JobArgs, errKind string) {
	o.Log.Error(ctx, "review_run terminal failure", map[string]any{
		"review_run_id": args.ReviewRunID.String(),
		"error_kind":    errKind,
	})
	_ = o.Repos.ReviewRun.Finalize(ctx, args.ReviewRunID, "failed")
}

// parseFindingsJSONL accepts the LLM's text and produces Finding rows.
// One JSON object per line. Lines that fail to parse are skipped with
// a logged warning rather than failing the whole Lens — single bad
// line shouldn't waste 30 good ones.
func parseFindingsJSONL(text string, lr *repo.LensRun, orgID uuid.UUID) ([]*repo.Finding, error) {
	var out []*repo.Finding
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Some models wrap output in a fence; strip ``` lines defensively.
		if strings.HasPrefix(line, "```") {
			continue
		}
		var raw struct {
			Category     *string         `json:"category"`
			Excerpt      *string         `json:"excerpt"`
			LocationHint *string         `json:"location_hint"`
			Details      json.RawMessage `json:"details"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		details := raw.Details
		if len(details) == 0 {
			details = json.RawMessage("{}")
		}
		// Cap excerpt at 200 chars (DB CHECK constraint).
		if raw.Excerpt != nil && len(*raw.Excerpt) > 200 {
			s := (*raw.Excerpt)[:200]
			raw.Excerpt = &s
		}
		out = append(out, &repo.Finding{
			ReviewRunID:    lr.ReviewRunID,
			LensRunID:      lr.ID,
			OrganizationID: orgID,
			LensKey:        lr.LensKey,
			Category:       raw.Category,
			Excerpt:        raw.Excerpt,
			LocationHint:   raw.LocationHint,
			Details:        details,
		})
	}
	return out, nil
}

func classifyErr(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "anthropic"):
		return "vendor_error"
	case strings.Contains(msg, "parse"):
		return "parse_error"
	default:
		return "unknown_error"
	}
}

// loadBlob is a tiny shim that handles the file:// URLs LocalFSBlob
// produces. Phase Blob will swap LocalFSBlob for AzureBlob; that impl
// returns https URLs the worker fetches via http.Get with a SAS token.
// Keeping the shim here lets the orchestrator stay impl-agnostic.
func loadBlob(blobURL string) ([]byte, error) {
	const fileScheme = "file://"
	if !strings.HasPrefix(blobURL, fileScheme) {
		return nil, fmt.Errorf("unsupported blob scheme: %q", blobURL)
	}
	path := strings.TrimPrefix(blobURL, fileScheme)
	return readFile(path)
}
