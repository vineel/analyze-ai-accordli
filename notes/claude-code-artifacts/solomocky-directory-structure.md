# SoloMocky — Directory Structure and Isolation Strategy

## Premise: don't isolate by directory

The tempting move is a parallel top-level directory — `/solomocky/` for the throwaway app, `/api/` and `/web/` for the eventual Scaffolding+Analyze. **Don't.** It produces two copies of the app and two answers to every question (where do migrations live? where do prompts live? which one runs in dev?). The plan in `mocky-self-contained.md` is to *grow Scaffolding into SoloMocky in place*, not to build twice and migrate.

The right isolation is **seam-based**. SoloMocky lives in the target repo layout from Day 1. Every Scaffolding-replaceable concern hides behind a small Go interface. Today, each interface has a SoloMocky-flavored implementation (a hardcoded auth provider, a goroutine dispatcher, a no-op reservation, a local-FS blob store). Tomorrow, the same interface gets the real implementation (WorkOS, River, Stripe, Azure Blob). Call sites don't change. The seams are the isolation.

The few pieces that have **no Scaffolding equivalent** — the sample-doc loader, the seed-data script, the hardcoded-Org constants — go in a single throwaway bucket (`internal/solomocky/`) that gets deleted wholesale at cutover.

## Target structure

This is the layout SoloMocky uses from its first commit. It matches `starter-app.md` §4 with a small addition (the throwaway bucket).

```
/api                              Go module, single binary today
  /cmd
    /api                          HTTP server entrypoint (only entrypoint at SoloMocky)
                                  (Phase: River → adds /cmd/worker as a second entrypoint
                                  on the same module via a -role flag or separate main.go)
  /internal
    /core                         PERMANENT domain code — survives Mocky → Analyze
      /matter                     Matter lifecycle, lock invariant
      /reviewrun                  ReviewRun state machine, Prefix builder, Reserve/Commit
                                  scope (calls into /infra/billing)
      /lens                       LensRun execution, template loading from /prompts/lens
      /finding                    Findings persistence, JSONB shape
      /docconv                    docx2md-go wrapper
      /llm                        llm.Client interface + AnthropicDirect impl
                                  (Phase: Foundry → adds AnthropicFoundry impl;
                                   Phase: failover → adds FailoverClient that wraps two)
    /infra                        SEAM LAYER — each subpkg is one Scaffolding concern
      /auth                       AuthProvider interface
                                    SoloMocky:  HardcodedProvider (single Mocky user)
                                    Phase WorkOS: WorkOSProvider replaces it
      /queue                      Dispatcher interface
                                    SoloMocky:  GoroutineDispatcher (in-process)
                                    Phase River: RiverDispatcher replaces it
                                  Handler signatures match River's job shape so the
                                  swap is a registration change, not a rewrite.
      /billing                    Reservation interface (Reserve/Commit/Rollback)
                                    SoloMocky:  NoopReservation
                                    Phase Stripe: StripeReservation replaces it
      /storage                    BlobStore interface
                                    SoloMocky:  LocalFSBlob (./var/blob/)
                                    Phase Blob: AzureBlob replaces it
      /observability              Logger / Tracer / LLMObserver interfaces
                                    SoloMocky:  stdout impls
                                    Phase obs:  Azure Monitor + Helicone wrappers
    /solomocky                    THROWAWAY BUCKET — deleted at cutover
      sample_doc.go               loads /notes/scaffolding/starter-app/mocky-files/sample-agreement-1.docx
                                  (or a copy promoted into the repo proper at start of build)
      seed.go                     creates Mocky Org + Dept + User on first run if missing
      hardcoded.go                the Mocky Org UUID constant, sample creds, etc.
    /httpapi                      HTTP handlers, request routing, middleware
                                  Handlers depend on /core and /infra interfaces — never
                                  on concrete impls. Auth middleware reads from /infra/auth.
  go.mod                          single module
  go.sum

/web                              Vite + React + TS — served via go embed in prod-shape
  /src
    /pages                        Matters list, Matter detail, hardcoded "login" stub
    /components
    /api                          generated or hand-rolled FE client
  index.html
  vite.config.ts                  dev proxy → http://localhost:8080/api
  package.json
  /dist                           built assets, embedded by Go via go:embed

/db
  /migrations                     goose migrations — FULL §5 schema from Day 1
    0001_init.sql                 organizations, departments, users, memberships
    0002_matters.sql              matters, documents
    0003_runs.sql                 review_runs, lens_runs, findings
                                  (Phase Stripe: 0004_billing.sql adds the billing tables)
                                  (Phase WorkOS: 0005_audit.sql adds audit_events)
                                  (Phase RLS:    0006_rls.sql installs policies)

/prompts                          file-based Go templates from Day 1
  /lens
    entities_v1.tmpl
    open_questions_v1.tmpl
  /summary
    summary_v1.tmpl

/scripts                          dev helpers
  reset_db.sh
  seed.sh                         (calls into /api/internal/solomocky/seed.go via a CLI flag)

/notes                            specs (already exists)
  /scaffolding/starter-app/
    starter-app.md
    mocky-self-contained.md
    mocky-files/sample-agreement-1.docx
  /claude-code-artifacts/

/infra                            EMPTY at SoloMocky stage
  (Phase Azure: /bicep, /workos, /stripe land here per starter-app.md §4)
```

## The seam pattern (concrete)

Every Scaffolding-replaceable concern follows the same three-part shape:

```go
// /api/internal/infra/queue/queue.go
package queue

type Job struct { ID string; Kind string; Args []byte }

type Dispatcher interface {
    Enqueue(ctx context.Context, j Job) error
    Register(kind string, h func(ctx context.Context, j Job) error)
}
```

```go
// /api/internal/infra/queue/goroutine.go      (SoloMocky impl)
package queue

type GoroutineDispatcher struct { handlers map[string]func(...) error }

func (d *GoroutineDispatcher) Enqueue(ctx context.Context, j Job) error {
    h := d.handlers[j.Kind]
    go func() { _ = h(context.Background(), j) }() // shaped like a River job
    return nil
}
```

```go
// /api/internal/infra/queue/river.go          (lands at Phase: River)
package queue

type RiverDispatcher struct { client *river.Client }
// Same Enqueue/Register signatures. Same Job shape. Same handler signature.
// Swapping is one wiring change at startup.
```

Call sites depend on `queue.Dispatcher`, never on `GoroutineDispatcher` or `RiverDispatcher` directly. This is the pattern that makes "add Scaffolding piece by piece" mean *swap one impl* rather than *rewrite handlers*.

The same shape applies to:

| Seam package | SoloMocky impl | Replaced by |
|---|---|---|
| `infra/auth` | `HardcodedProvider` | `WorkOSProvider` |
| `infra/queue` | `GoroutineDispatcher` | `RiverDispatcher` |
| `infra/billing` | `NoopReservation` | `StripeReservation` |
| `infra/storage` | `LocalFSBlob` | `AzureBlob` |
| `infra/observability` | stdout logger | Azure Monitor + Helicone |
| `core/llm` | `AnthropicDirect` | `AnthropicDirect` + `AnthropicFoundry` + `Failover` |

Note that `core/llm` lives in `/core/` rather than `/infra/`, because the LLM client *is* part of Reviewer's permanent runtime — adding Foundry and failover is augmenting `core`, not replacing it. The other seams are pure swap-outs and live in `/infra/`.

## What goes in `/internal/solomocky/`

Only code that:

1. Has no Scaffolding-equivalent (sample doc, seed data, hardcoded constants), AND
2. Will be deleted wholesale at the Mocky → Analyze cutover.

Specifically: `sample_doc.go` (reads the bundled `.docx`), `seed.go` (idempotent `INSERT … ON CONFLICT DO NOTHING` for the Mocky Org / User / Dept), `hardcoded.go` (the Mocky Org UUID, the placeholder dialog text). No business logic. If logic is creeping in here, it belongs in `/core/` instead — and the SoloMocky-flavored part probably belongs as a `Hardcoded*` impl behind an `infra/` interface.

This bucket gets a `// THROWAWAY: deleted at Mocky → Analyze cutover` header on each file so future-us has no doubt.

## Cutover mechanics (what "delete SoloMocky" actually does)

When Analyze replaces Mocky:

1. Delete `/api/internal/solomocky/` entirely.
2. Delete `/web/src/pages/login-stub.tsx` (or whatever the hardcoded-auth stub is named) and the matching FE routes.
3. Delete the two stub Lens templates from `/prompts/lens/` and replace with the real Lens set.
4. Replace Mocky's `/web` UI with Analyze's UI.

Everything else stays. `/core/`, `/infra/`, `/db/migrations/`, the seam interfaces, the wiring at startup — all permanent. By that point, every `/infra/` package has its real implementation; deleting `/internal/solomocky/` removes the only code that had no real-world counterpart.

## Why not a `/solomocky/` top-level directory

To name the alternative explicitly so we can rule it out:

> `/solomocky/api/`, `/solomocky/web/` — a complete sub-app. `/api/` and `/web/` start empty and grow as Scaffolding lands. Cut over by deleting `/solomocky/` and pointing deploy at `/api/`.

Three problems:

1. **Two implementations of the schema, prompts, and domain model** until cutover, kept in sync by hand.
2. **No way to "add Scaffolding piece by piece" to SoloMocky.** Each Scaffolding piece either goes into `/api/` (where SoloMocky doesn't run, so it's untestable end-to-end) or goes into `/solomocky/` (which is the directory we said was throwaway). Both options break the plan.
3. **The cutover becomes a Big Bang** — exactly the thing the seam-based approach is designed to avoid. Mocky → Analyze should be a deletion of leaf code, not a fork merge.

Seam-based isolation gives the same conceptual separation ("this is throwaway, this is permanent") with an order of magnitude less coordination overhead.

## TL;DR

- Use the layout in `starter-app.md` §4 from Day 1, plus an `/api/internal/solomocky/` throwaway bucket.
- Each Scaffolding-replaceable concern is a Go interface in `/api/internal/infra/<concern>/` with one SoloMocky-flavored impl today and a real impl when its phase lands.
- `/api/internal/core/` and `/api/internal/infra/` are permanent. `/api/internal/solomocky/` is deletable.
- Cutover deletes the throwaway bucket plus Mocky's UI and stub Lens prompts. Everything else stays.
