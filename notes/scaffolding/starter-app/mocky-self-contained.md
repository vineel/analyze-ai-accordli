# Mocky — Self Contained (SoloMocky)

To start the ball rolling on implementation of the scaffolding of this starter app, we will build a self-contained version of Mocky. Once this is up and running, we will add one piece of scaffolding to it at a time. Call this version **SoloMocky**.

SoloMocky is intentionally stripped to the bare minimum. Much of it will be rewritten as Scaffolding pieces land. We accept that — Claude Code can rewrite small pieces fast. If this approach doesn't work, we'll find out quickly and rethink.

# Plan

1. Build SoloMocky.
2. Add Scaffolding to SoloMocky, piece by logical piece, locally (WorkOS, River, Stripe testmode, vendor failover, RLS, etc.).
3. Add Azure Scaffolding, move to Azure Staging (Flex + CMK, Blob, Container Apps, Bicep, Cloudflare).
4. Build Analyze from within the Scaffolding.
5. Ship.

# SoloMocky Spec

The overall functionality is based on `starter-app.md` in this directory. Below we list specific shortcuts (hacks) that let us build SoloMocky standalone, and the things we do "for real" from Day 1 because getting them right later is much more expensive than getting them right now.

## Shortcuts & Hacks

1. Instead of login, hardcode a user. user: `mocky` / pass: `starter`, team "Mocky Team", Org "Mocky Org".
2. Instead of uploading a doc, ship one doc in the source: `./mocky-files/sample-agreement-1.docx`.
3. Instead of a queue, use goroutines in the API to run the LLM calls.
4. Instead of fanout, run calls sequentially.
5. Instead of Azure-hosted services, use local services.
6. Use local PostgreSQL.
7. Serve the front-end from the same Go service. Dev uses a Vite proxy to hit the API; prod-shape uses `go embed` of the Vite build. (The eventual Azure path will use Azure's static-file serving with CDN; not our concern here.)
8. Use Anthropic API directly as the vendor for Claude Sonnet. No second vendor at this stage.
9. Almost no observability — console logging on stdout only.

## NOT shortcuts (do these for real on Day 1)

1. **Document conversion uses `docx2md-go`** (our own package). The `starter-app.md` §6.5 reference to pandoc is stale and is being reconciled there.
2. **Use the data model from `starter-app.md` §5 verbatim:** `matters / documents / review_runs / lens_runs / findings`. Narrow indexable fields + JSONB details. No shortcut schema. Migrating from a stub schema to the real one later is real DB surgery; the right CREATE TABLEs cost nothing extra now.
3. **`organization_id` on every tenant-scoped row, with the constant Mocky Org UUID.** Every read includes `WHERE organization_id = $org` even though there's only one Org. The predicate works with or without RLS; when RLS lands it becomes belt-and-suspenders, and we don't have to retrofit the predicate into every query.
4. **Lock the Matter once a ReviewRun has been initiated against it.** Same invariant as Reviewer.
5. **Per-Lens spinners on the Matter detail page**, each resolving into a count ("12 facts", "6 questions") as its LensRun lands. No single global spinner; that UX would be thrown away.
6. **Lenses are file-based Go templates in `/prompts/lens/`**, hydrated at runtime. Record the template git SHA on `lens_runs.lens_template_sha`. Same model as `Reviewer-v2`.
7. **Goroutine handlers are shaped like River jobs:** a function that takes a job ID and writes its own row to completion. Swapping in River later is a dispatcher change, not a rewrite.
8. **Wrap each run in a Reserve / Commit / Rollback scope, no-op at SoloMocky stage.** The seam is what matters; Phase 6 of `starter-app.md` slots real billing into the same shape.
9. **All LLM calls go through a single `llm.Client` Go interface** with one Anthropic-direct implementation. Vendor B (Anthropic via Azure Foundry) lands later as an additional implementation behind the same interface.

## Seed Data

1. The hardcoded user (and its Mocky Org + Mocky Team).
2. The sample `.docx` file.
3. The Lens prompts (file-based; loaded from `/prompts/lens/`).

# SoloMocky Flow

1. User hits the Main page. All Matters in the Mocky Org are listed.
2. User clicks "Create New Matter". A dialog says "Placeholder for upload agreement. Click Continue to use sample agreement." [Continue]
3. User clicks Continue. A new Matter is created. The sample agreement is converted via `docx2md-go` and stored as a `documents` row of kind `markdown`. A `review_runs` row is inserted (Reserve no-op), the Matter is locked, and the Lenses are dispatched as goroutines (sequentially for SoloMocky). Each LensRun writes its own findings to completion. When the last LensRun terminates, Commit no-op.
4. User lands on the Matter page. Per-Lens spinners resolve into counts as each LensRun completes; expanding a panel shows the Findings rows. Buttons to download the markdown version and to download the original `.docx`.
