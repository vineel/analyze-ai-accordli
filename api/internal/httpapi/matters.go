package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"accordli.com/analyze-ai/api/internal/core/reviewrun"
	"accordli.com/analyze-ai/api/internal/infra/queue"
	"accordli.com/analyze-ai/api/internal/infra/repo"
	"accordli.com/analyze-ai/api/internal/infra/storage"
	"accordli.com/analyze-ai/api/internal/solomocky"
)

// MattersDeps groups everything the matter routes need. Lives on
// Deps; named struct so the route file isn't reaching all over.
type MattersDeps struct {
	Repos        *repo.Repos
	Blob         storage.BlobStore
	Queue        queue.Dispatcher
	Orchestrator *reviewrun.Orchestrator
}

func (d *Deps) mountMatters(r chi.Router) {
	r.Get("/api/matters", d.listMatters)
	r.Post("/api/matters", d.createMatter)
	r.Get("/api/matters/{id}", d.getMatter)
	r.Get("/api/matters/{id}/document/markdown", d.downloadMarkdown)
	r.Get("/api/matters/{id}/document/original", d.downloadOriginal)
}

type matterResponse struct {
	ID         uuid.UUID  `json:"id"`
	Title      string     `json:"title"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	LockedAt   *time.Time `json:"locked_at,omitempty"`
	HasOriginal bool      `json:"has_original"`
	HasMarkdown bool      `json:"has_markdown"`
}

func (d *Deps) listMatters(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	matters, err := d.Matters.Repos.Matter.ListForOrg(r.Context(), id.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list matters: "+err.Error())
		return
	}
	out := make([]matterResponse, 0, len(matters))
	for _, m := range matters {
		out = append(out, matterResponse{
			ID:        m.ID,
			Title:     m.Title,
			Status:    m.Status,
			CreatedAt: m.CreatedAt,
			LockedAt:  m.LockedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type createMatterRequest struct {
	Title string `json:"title"`
}

// createMatter is the SoloMocky create flow. Always uses the bundled
// sample doc — there is no upload UI yet (the "Continue to use sample
// agreement" placeholder dialog from mocky-self-contained.md).
func (d *Deps) createMatter(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())

	var body createMatterRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, errors.New("EOF")) {
		// Title is optional; ignore decode errors so an empty body works.
		body.Title = ""
	}
	if strings.TrimSpace(body.Title) == "" {
		body.Title = "Mocky Matter " + time.Now().Format("Jan 2, 2006 3:04pm")
	}

	ctx := r.Context()
	matter, err := d.Matters.Repos.Matter.Create(ctx, id.OrganizationID, id.DepartmentID, id.UserID, body.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create matter: "+err.Error())
		return
	}

	// Load and store the bundled sample doc as the "original" document.
	docxBytes, err := solomocky.LoadSampleDocx()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load sample: "+err.Error())
		return
	}
	blobKey := "matters/" + matter.ID.String() + "/" + solomocky.SampleDocxFilename
	if err := d.Matters.Blob.Put(ctx, blobKey, strings.NewReader(string(docxBytes))); err != nil {
		writeError(w, http.StatusInternalServerError, "put blob: "+err.Error())
		return
	}
	blobURL, err := d.Matters.Blob.SignedURL(ctx, blobKey, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sign blob: "+err.Error())
		return
	}
	filename := solomocky.SampleDocxFilename
	size := int64(len(docxBytes))
	originalDoc := &repo.Document{
		MatterID:  matter.ID,
		Kind:      "original",
		BlobURL:   &blobURL,
		Filename:  &filename,
		SizeBytes: &size,
	}
	if err := d.Matters.Repos.Document.Insert(ctx, originalDoc); err != nil {
		writeError(w, http.StatusInternalServerError, "insert original: "+err.Error())
		return
	}

	// Lock the matter (Reviewer's "once a Run starts, the Matter is locked").
	if err := d.Matters.Repos.Matter.Lock(ctx, id.OrganizationID, matter.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "lock matter: "+err.Error())
		return
	}

	// Open a ReviewRun row, then dispatch the job.
	rr, err := d.Matters.Repos.ReviewRun.Create(ctx, matter.ID, id.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create review_run: "+err.Error())
		return
	}
	if err := d.Matters.Orchestrator.Dispatch(ctx, d.Matters.Queue, reviewrun.JobArgs{
		OrgID:         id.OrganizationID,
		MatterID:      matter.ID,
		ReviewRunID:   rr.ID,
		OriginalDocID: originalDoc.ID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "dispatch: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, matterResponse{
		ID:          matter.ID,
		Title:       matter.Title,
		Status:      "locked",
		CreatedAt:   matter.CreatedAt,
		HasOriginal: true,
	})
}

type lensRunResponse struct {
	ID            uuid.UUID `json:"id"`
	LensKey       string    `json:"lens_key"`
	Status        string    `json:"status"`
	FindingCount  *int      `json:"finding_count,omitempty"`
	ErrorKind     *string   `json:"error_kind,omitempty"`
	Findings      []finding `json:"findings"`
}

type finding struct {
	ID           uuid.UUID       `json:"id"`
	Category     *string         `json:"category,omitempty"`
	Excerpt      *string         `json:"excerpt,omitempty"`
	LocationHint *string         `json:"location_hint,omitempty"`
	Details      json.RawMessage `json:"details"`
}

type matterDetail struct {
	matterResponse
	Run     *runResponse      `json:"run,omitempty"`
	Lenses  []lensRunResponse `json:"lenses"`
	Summary *string           `json:"summary,omitempty"`
}

type runResponse struct {
	ID          uuid.UUID  `json:"id"`
	Status      string     `json:"status"`
	Vendor      *string    `json:"vendor,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (d *Deps) getMatter(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	matterID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad matter id")
		return
	}
	ctx := r.Context()
	m, err := d.Matters.Repos.Matter.Get(ctx, id.OrganizationID, matterID)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	hasMarkdown := false
	if _, err := d.Matters.Repos.Document.GetByKind(ctx, matterID, "markdown"); err == nil {
		hasMarkdown = true
	}

	out := matterDetail{
		matterResponse: matterResponse{
			ID:          m.ID,
			Title:       m.Title,
			Status:      m.Status,
			CreatedAt:   m.CreatedAt,
			LockedAt:    m.LockedAt,
			HasOriginal: true,
			HasMarkdown: hasMarkdown,
		},
		Lenses: []lensRunResponse{},
	}

	rr, err := d.Matters.Repos.ReviewRun.GetByMatter(ctx, id.OrganizationID, matterID)
	if err == nil {
		out.Run = &runResponse{
			ID:          rr.ID,
			Status:      rr.Status,
			Vendor:      rr.Vendor,
			CreatedAt:   rr.CreatedAt,
			CompletedAt: rr.CompletedAt,
		}
		out.Summary = rr.Summary

		lensRuns, err := d.Matters.Repos.LensRun.ListByReviewRun(ctx, id.OrganizationID, rr.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list lens runs: "+err.Error())
			return
		}
		for _, lr := range lensRuns {
			lrr := lensRunResponse{
				ID:           lr.ID,
				LensKey:      lr.LensKey,
				Status:       lr.Status,
				FindingCount: lr.FindingCount,
				ErrorKind:    lr.ErrorKind,
				Findings:     []finding{},
			}
			if lr.Status == "completed" {
				fs, err := d.Matters.Repos.Finding.ListByLensRun(ctx, id.OrganizationID, lr.ID)
				if err == nil {
					for _, f := range fs {
						lrr.Findings = append(lrr.Findings, finding{
							ID:           f.ID,
							Category:     f.Category,
							Excerpt:      f.Excerpt,
							LocationHint: f.LocationHint,
							Details:      f.Details,
						})
					}
				}
			}
			out.Lenses = append(out.Lenses, lrr)
		}
	}

	writeJSON(w, http.StatusOK, out)
}

func (d *Deps) downloadMarkdown(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	matterID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad matter id")
		return
	}
	if _, err := d.Matters.Repos.Matter.Get(r.Context(), id.OrganizationID, matterID); err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	doc, err := d.Matters.Repos.Document.GetByKind(r.Context(), matterID, "markdown")
	if err != nil || doc.ContentMD == nil {
		writeError(w, http.StatusNotFound, "markdown not ready")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+matterID.String()+".md\"")
	_, _ = w.Write([]byte(*doc.ContentMD))
}

func (d *Deps) downloadOriginal(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	matterID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad matter id")
		return
	}
	if _, err := d.Matters.Repos.Matter.Get(r.Context(), id.OrganizationID, matterID); err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	doc, err := d.Matters.Repos.Document.GetByKind(r.Context(), matterID, "original")
	if err != nil || doc.BlobURL == nil {
		writeError(w, http.StatusNotFound, "original not found")
		return
	}
	// SoloMocky stores originals under matters/<id>/<filename>; that key
	// is rebuildable from the matter id and the (one) original filename.
	// Phase Blob will replace this with a 302 to a SAS URL.
	fname := solomocky.SampleDocxFilename
	if doc.Filename != nil {
		fname = *doc.Filename
	}
	blobKey := "matters/" + matterID.String() + "/" + fname
	rc, err := d.Matters.Blob.Get(r.Context(), blobKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "open blob: "+err.Error())
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+fname+"\"")
	_, _ = copyAll(w, rc)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
