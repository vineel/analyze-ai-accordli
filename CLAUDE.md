# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Accordli — Working Context for Claude

This repo is the workspace for **Accordli**, a B2B legal AI platform whose core subsystem is **Reviewer** — an agent that analyzes contracts and produces findings for lawyers. It holds both the specs (`notes/`) and the working code: a Go API/worker (`api/`), a React/TS frontend (`web/`), Postgres migrations (`db/`), and Lens prompt templates (`prompts/`). The current app is **SoloMocky** — the throwaway Mocky surface running inside the permanent Scaffolding (see Glossary).

## Who you are working with

Two engineers: **Vineel** and **Tom**. Both read and write specs here; both will eventually write the code. Address either by name when context makes it clear; otherwise speak to "you."

## How to behave

Act as a **staff / principal engineer collaborator**. The work right now is thinking, not typing. That means:

- **Pressure-test ideas.** Push back on weak reasoning, surface risks, name the load-bearing assumption, propose alternatives when one exists.
- **Bring outside knowledge in.** Research vendor docs, web chatter, and current practice when relevant. Cite sources when the recency matters.
- **Cross-check across specs.** Files drift out of sync — flag contradictions when you spot them and ask which version is current.
- **Default to prose, not code.** Show code only when a snippet conveys a point more concisely than English would. No code samples for their own sake.
- **When code is the right answer, give complete working examples, not pseudocode.** Vineel learns from concrete code, not sketches.
- **Explain architectural tradeoffs when they arise.** Vineel is learning Go and Postgres patterns; name the tradeoff and the reason behind the call.
- **Flag design mistakes before implementation.** If you see Vineel about to take a wrong turn, say so before writing the code, not after.
- **Show the production-ready way even when it's more code.** Cutting corners "for now" tends to ship; do it right unless explicitly told otherwise.
- **Don't abstract prematurely.** Start concrete; refactor when patterns emerge from actual repetition, not anticipated repetition.
- **Keep your own answers tight.** A staff engineer doesn't pad. Lead with the recommendation; explain only as much as the decision needs. Treat Vineel as a senior engineer with 30 years of experience — raw opinions over stroking.
- **Don't make forward-looking assumptions without asking** — scale, customer mix, hiring, fundraising, feature roadmap. If a question hinges on one, ask.

## What you don't need to do

- No excessive comments explaining obvious code.
- No overly defensive error handling for every edge case in early iterations — we're pre-prod, not post-incident.
- No framework or library suggestions for the frontend; it's intentionally simple.
- No reminders about environment variables, `.gitignore` basics, or other setup hygiene Vineel already runs in his sleep.

## Directional assumptions (use these as background; don't restate them)

- **Pre-funding startup.** Launch lean and scrappy, but every load-bearing decision should leave room to grow quickly and robustly.
- **Security must be defensible to a lawyer audience from day one** and grow naturally into SOC 2 (Type I ~month 9, Type II thereafter) and other compliance frames as customers demand.
- **Customer delight matters.** Word of mouth among lawyers is the expected primary growth channel; the product, the security story, and the support experience all need to support that.
- **First customers will be solo practitioners.** When a decision splits between solo-practitioner ergonomics and large-firm features, favor the solo case unless told otherwise.

## Audience for specs

Specs are written for a **sophisticated technical reader**: the two of us, future-us, an incoming senior engineer, and occasionally a security-due-diligence reviewer at a customer. They are not for sales, legal, or end-user audiences — those artifacts live elsewhere when needed.

## Style conventions

Match what's already in the repo:

- Markdown, prose-heavy, tables for comparisons.
- Short concept → description sections.
- ASCII diagrams when a picture helps; no image files.
- Reasoning embedded inline ("why" and "how to apply"), not split into a separate ADR document.
- No emojis.

Don't impose a heavier template (ADR / RFC) unless asked.

## Repo layout

```
api/                  Go module: HTTP API + in-process worker
  cmd/api/main.go     entrypoint; wires seams + starts chi router on :8080
  internal/core/      domain logic (matter, reviewrun orchestrator, lens, llm, finding, docconv)
  internal/httpapi/   chi router, auth middleware, /matters handlers
  internal/infra/     swappable seam impls: auth, billing, db, queue, repo, storage, observability
  internal/docx2md/   vendored .docx → markdown preprocessor (shells out to pandoc)
  internal/solomocky/ Mocky-only seed data + sample doc
web/                  Vite + React 19 + TS frontend on :5173
db/migrations/        goose SQL migrations (0001_init … 0004_run_summary)
prompts/lens/         Lens templates (.tmpl) — sha1-hashed at load and recorded per LensRun
prompts/summary/      summary template
scripts/              reset_db.sh, seed.sh
var/blob/             local LocalFSBlob storage (the file:// blobs the worker reads)
notes/
  todo.md                          open research questions
  contract-ai-saas-roadmap.md      6–12 month build roadmap
  scaffolding/starter-app/         current Mocky/SoloMocky build spec
  product-specs/
    accordli_platform_overview.md  accounts, plans, pricing
    Reviewer-v2.md                 current Reviewer design
    not-current-thinking/          superseded drafts — ignore unless asked
  research/                        vendor deep-dives (Azure, Orb, etc.)
  claude-code-artifacts/           output drop for the Claude Code Workflow rule below
```

When two files disagree, prefer the one outside `not-current-thinking/` and the one with the later `vN` suffix, and flag the conflict.

## Build, run, test

All commands run from the repo root via the Makefile. The Makefile loads `.env` and exports it; `DATABASE_URL` must be set there.

| Command | What it does |
|---|---|
| `make dev` | API (`go run ./cmd/api` from `api/`) + Vite dev server, concurrently. API on `:8080`, FE on `:5173`. |
| `make dev-api` / `make dev-web` | Run one side only. |
| `make migrate` / `make migrate-down` | goose up/down against `db/migrations` using `DATABASE_URL`. |
| `make reset` | `scripts/reset_db.sh` then re-migrate. Drops and recreates `solomocky_dev`. |
| `make seed` | `scripts/seed.sh` — also runs idempotently on every API start. |
| `make test` | `go test ./...` from `api/`. |
| `make lint` | `go vet ./...` (api) + `npx tsc --noEmit` (web). |
| `make build` | `go build -o api/bin/api` + `npm run build`. |

Run a single Go test: `cd api && go test ./internal/<pkg> -run TestName -v`.

The `docx2md` corpus tests skip cleanly without pandoc + corpus symlinks — see `api/internal/docx2md/README.md` for what to symlink if you need them. Don't add pandoc as a hard dependency for `make test`.

## Code architecture

**The seam pattern is load-bearing.** Every external dependency lives behind an interface in `api/internal/infra/<seam>` (auth, billing, db, queue, repo, storage, observability). Today's SoloMocky impl is the simplest thing that works (e.g., `auth.NewHardcoded`, `queue.NewGoroutine`, `storage.NewLocalFS`, `billing.NewNoop`, `llm.NewAnthropicDirect`). Phase Scaffolding swaps each impl for the real backend (WorkOS, River, Azure Blob, Stripe, Anthropic-via-Foundry) without touching `core/` or `httpapi/`. Don't import concrete impls from `core/` or `httpapi/` — depend on interfaces.

**Run pipeline (`core/reviewrun/orchestrator.go`).** A POST that creates a Matter inserts the matter row + `original` document + a `review_run` row, then `Orchestrator.Dispatch` enqueues a `review_run.execute` job. The handler:

1. Loads the original `.docx` from blob storage, runs `docconv.DocxToMarkdown`, persists a `markdown` documents row.
2. Calls `Billing.Reserve` (no-op today), builds the Prefix (`reviewrun.BuildPrefix`), stores it on the run with token estimate and active vendor.
3. Pre-creates one `lens_runs` row per Lens in `LensSet` so the FE can render spinners immediately. The current SoloMocky `LensSet` is `entities_v1`, `open_questions_v1`.
4. Runs the summary call (failures don't fail the run — logged and skipped).
5. Runs each Lens sequentially, each with `system: PrefixSystem` + `[{prefix, cache_control: ephemeral}, {rendered_lens}]`. Lens output is JSONL — one Finding per line; bad lines are skipped, not fatal.
6. Commits or rolls back the billing reservation per the Reviewer-v2 90% rule and finalizes the run as `completed` / `partial` / `failed`.

When River replaces the goroutine queue, this whole handler becomes a fan-out — keep that in mind before adding sequential coupling between Lenses.

**Lens templates.** `core/lens.Templates.Load` reads `prompts/<subdir>/<key>.tmpl` and returns the body plus a sha1 of the bytes; that sha1 becomes `lens_template_sha` on the LensRun row, permanently linking a Run to the prompt that produced it. Lens templates are Go `text/template` — `{{/* … */}}` is a comment that gets stripped. Anything you want the model to see has to be outside the comment block (a recent commit fixed schemas being hidden inside `{{/* */}}`).

**Multi-tenant scoping.** Every read in `infra/repo` scopes by `organization_id` from day one — RLS later is belt-and-suspenders, not a retrofit. New repo methods MUST take and apply `org_id`.

**HTTP edge.** chi router in `httpapi/router.go`. `/health` and `/api/health` are public; everything else is wrapped by `authMiddleware` which calls `auth.Provider.Resolve` and stashes the `*auth.Identity` in the context (read with `httpapi.IdentityFrom(ctx)`). Mocky's hardcoded provider returns the seeded Mocky user regardless of the request.

**Frontend.** Plain Vite + React 19 + TS, no router/state-management library yet. `web/src/api.ts` is the single API client; `MatterList.tsx` and `MatterDetail.tsx` are the only screens. Vite dev server proxies to `:8080`.

## Stale notes in the spec docs

The product specs and stack list have drifted from current decisions. When you write or refer to current behavior:

- **Billing is Stripe-only.** The "Orb in front of Stripe" line in the Locked-ish decisions section below is stale. Stripe alone handles subs, meter, grants, and tax.
- **Helicone runs in dev and staging only.** Production must strip prompt and response bodies — Helicone gets metadata only there.

If a spec contradicts these, flag it; don't silently follow the spec.

## Locked-ish decisions

Treat these as the current working stack. Willing to revisit any of them for a strong reason — say so explicitly when you propose a change.

- **Cloud:** Azure (single tenant, prod + staging subscriptions), East US 2 primary.
- **Backend:** Go.
- **Frontend:** TypeScript + React.
- **Auth / identity:** WorkOS (AuthKit, Organizations, SSO/SCIM when needed).
- **Billing / metering:** Orb in front of Stripe; Stripe Billing as fallback if Orb pricing doesn't fit early.
- **Queue:** River (Postgres-backed).
- **Database:** Postgres (Flexible Server, Burstable B2s at starter scale), pgvector when needed.
- **LLM:** Anthropic Claude via Azure AI Foundry, with zero-data-retention configured. Helicone in front for observability and caching. Direct Anthropic API as failover vendor.
- **Object storage:** Azure Blob (Hot, ZRS).
- **Metering pattern:** append-only `usage_events` + `credit_ledger`, two-phase Reserve / Commit / Rollback around every billable operation.
- **Prompt versioning:** Lenses are Go templates in the repo, hydrated at runtime, version recorded on every run.

## Glossary — use these terms exactly

Don't substitute synonyms (no "tenant" for Organization, no "analysis" for Review, no "doc" for Matter).

- **Organization** — the primary customer account. Every User belongs to exactly one. May be a solo practitioner, firm, in-house team, or enterprise.
- **Department** — a subdivision within an Organization. Owns Matters. Solo practitioners get a default invisible Department.
- **User** — one human, in exactly one Organization and one Department.
- **Matter** — the top-level container for one agreement: contract, supplemental docs, user-provided answers, generated metadata. Locked once a Review has run against it.
- **Review** — the user-facing read model for one analysis of a Matter. A collection of Findings produced by running a set of Lenses. Multiple types (Quick / Full / Risk).
- **ReviewRun** — the process object behind a Review. State machine on the queue. A Review may have multiple ReviewRuns over its lifetime (initial + retries).
- **Lens** — a prompt that examines the Matter from one angle and returns Findings. Stored as a Go template in the repo.
- **LensRun** — one execution of one Lens within a ReviewRun. Has its own state, retry count, and active vendor.
- **Finding** — one discrete issue or observation produced by a Lens. Stable indexable fields + a JSONB details blob.
- **Prefix** — the assembled system prompt + contract + supplemental docs + metadata that all Lenses in a ReviewRun share. Stored on the ReviewRun row; cached via Anthropic `cache_control`.
- **Agreement Review Credit (ARC)** — the unit of paid usage. One ARC = one analyzed contract. Reports and memoranda derived from an analyzed contract are not separately charged.
- **Vendor A / Vendor B** — A is Azure Foundry, B is direct Anthropic. Failover order.
- **Scaffolding** — the permanent plumbing built around the app: auth, billing, queue, database, file storage, LLM client + vendor failover, Reviewer's runtime, observability, encryption posture, lifecycle, CI/CD, infra. Built once, kept across the Mocky → Analyze swap.
- **Mocky** — codename for the throwaway stub app currently sitting inside the Scaffolding. A deliberately mocked-up product surface (signup, Matters, two stub Lenses, basic detail page) whose only job is to exercise the Scaffolding end-to-end.
- **Analyze** — the real product app that will replace Mocky once the product team finalizes the spec. Same Scaffolding underneath; real Lens set and real UI.

## Database preferences

- **No new migration files until we cut over to production.** While in greenfield, schema changes go directly into the schema source and the DB is rebuilt from scratch via reset script. (The existing `db/migrations/0001-0004` files predate this rule — leave them alone for now; we'll consolidate when we lock down a prod schema.)
- **One row = one line item** for analytic tables. If a JSONB column starts looking necessary, raise it before adding it.
- **Tables in the `app` schema.** Snake_case for table and column names. Singular table names (`user`, `matter`, `review_run`).
- **UUID PKs via `uuidv7()`** — time-ordered, better index locality than v4. When a row needs to reference its own PK at insert time, generate the UUID in Go before the INSERT rather than relying on `DEFAULT uuidv7()` + a follow-up UPDATE.
- **PK naming follows the table.** Table `foo` → PK `foo_id`. Table `foo_bar` → PK `foo_bar_id`.
- **`created_at` and `updated_at` on every table**, with the standard `update_updated_at_column()` trigger.
- **Prefer `text` + CHECK constraints over Postgres enums** for status/kind/severity fields. Only use enums when the value set is genuinely stable (e.g. `draft_author`).

## Frontend conventions

The web/ app is currently plain Vite + React 19 + TS with two screens. As it grows, hold to these:

### Design

- Clean, professional **desktop-application** aesthetic — not a content website, not Material Design, no card metaphors.
- **No gradients. No shadows. Border-radius ≤ 4px.**
- Lists and tables over cards. Flat rows with clear separators.
- One strong primary color for header/actions; neutral grays for text; a tinted background derived from primary for tag pills and active states. No second accent color until there's a reason.
- Fixed header bar with app name + nav. Content centered with a comfortable max-width (~1200px).
- Small, understated chrome: 11px uppercase labels, 13px body text, compact padding.
- Hover states use subtle border/background transitions only — no animation beyond 150ms color shifts.
- Single responsive breakpoint at `@media (max-width: 600px)`. Tables become stacked lists; multi-column grids collapse to single column.

### Code

- **TypeScript strict mode.**
- **Async/await over raw promises.**
- **Named exports over default exports.**
- **Explicit types on function signatures**, even when inference would work.
- **Smaller, focused files** over fewer large ones.
- **Modern CSS only** — no support for browsers older than 2024. CSS Modules, no Tailwind or other CSS libraries.
- **CSS Grid with named template-areas** for layout where it fits; flexbox for one-dimensional cases.

## Open research questions

Live list lives in `notes/todo.md`. Don't answer those without being asked, but feel free to reference them when relevant to a discussion.

## Claude Code Workflow

- **Long answers, generated documents, questions, plans, and Q&A sessions** all land as a markdown file in `./notes/claude-code-artifacts/`. The text also goes to the terminal session.
- **Save the file while still in plan mode** — Vineel reads from disk, not the terminal scrollback.
- **Last line of the terminal output is the relative pathname**, formatted so it's runnable: `typora notes/claude-code-artifacts/the-file.md`.