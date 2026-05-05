# Phase 0 Kickoff — SoloMocky Scaffolding

The first thing the next Claude Code session does when it starts building SoloMocky. Captures the pre-decisions made before clearing context, points at the specs, and scopes the session so it produces a clean repo skeleton — not half-built business logic.

To start the session, paste this:

> Build Phase 0 of SoloMocky per `notes/scaffolding/starter-app/phase-0-kickoff.md`. Read that file first, then the docs it points at, then execute the deliverables. Commit incrementally.

## Pre-decisions

1. **Go module path:** `accordli.com/analyze-ai/api`. This is a vanity import path — we control the DNS, and a `?go-get=1` redirect can be added later if we ever need it to be `go install`-able. For now nothing relies on the path resolving over the web.
2. **`docx2md-go` lives in-tree** at `/api/internal/docx2md/`. Find the source on disk — likely `~/accordli-dev/docx2md-go/` — copy it in, update its package imports to `accordli.com/analyze-ai/api/internal/docx2md`, verify it builds. If a sibling directory isn't found, ask before going hunting.
3. **Postgres is local** (already installed on the Mac). Phase 0 creates a database `solomocky_dev` and a role `solomocky_app` with `LOGIN`. No Docker. `DATABASE_URL` in `.env` points at `postgres://solomocky_app@localhost/solomocky_dev?sslmode=disable`.
4. **Anthropic API key env var is `ANTHRO_API_KEY`** — **not** `ANTHROPIC_API_KEY`. The standard name is overused and we want `grep ANTHRO_API_KEY` to be unambiguous. Read from environment first, fall back to `.env` via `godotenv`. `.env.example` lists the variable; `.env` itself is gitignored.

## Reading order (ingest before writing code)

1. `notes/scaffolding/starter-app/starter-app.md` — the canonical Phase plan, full §5 schema, stack
2. `notes/scaffolding/starter-app/mocky-self-contained.md` — what SoloMocky is and isn't
3. `notes/claude-code-artifacts/solomocky-directory-structure.md` — the seam-based isolation pattern; this drives the layout below
4. `notes/product-specs/Reviewer-v2.md` — skim. Don't implement Reviewer yet; the point is to make sure the seam interfaces match its eventual call shape.
5. `CLAUDE.md` — auto-loaded. Glossary and locked decisions live there.

## Phase 0 deliverables (this session only)

Goal: **a buildable skeleton with all seams in place but no business logic.** When the session ends, a fresh clone + `cp .env.example .env` + filling in `ANTHRO_API_KEY` + `make migrate && make dev` should yield the API on `:8080` serving `/health`, the FE on `:5173` showing a hello-world page that fetched `/api/health`, and Postgres with the §5 schema applied.

### Repo skeleton

- `/api` — Go module at `accordli.com/analyze-ai/api`, single `cmd/api` entrypoint.
- `/api/internal/core/` — empty subpackages stubbed (interface + type definitions, no real method bodies) for: `matter`, `reviewrun`, `lens`, `finding`, `docconv`, `llm`. Stub method bodies return `errors.New("not implemented")`.
- `/api/internal/infra/` — seam packages, each with the interface and the SoloMocky-flavored impl (stub methods are fine):
  - `auth/` — `Provider` interface; `HardcodedProvider` returns the Mocky user.
  - `queue/` — `Dispatcher` interface; `GoroutineDispatcher` impl. Handler signature must match what River expects so the later swap is wiring-only.
  - `billing/` — `Reservation` interface (`Reserve / Commit / Rollback`); `NoopReservation` impl.
  - `storage/` — `BlobStore` interface; `LocalFSBlob` impl writing to `./var/blob/`.
  - `observability/` — `Logger` interface, stdout impl.
- `/api/internal/solomocky/` — throwaway bucket. `hardcoded.go` with Mocky Org UUID + sample creds. `seed.go` and `sample_doc.go` as empty stubs. Each file gets `// THROWAWAY: deleted at Mocky → Analyze cutover` as the top comment.
- `/api/internal/httpapi/` — router setup, middleware, one handler. `GET /health` returns `{"ok":true,"version":"<git sha>"}`. Auth middleware reads from `infra/auth.Provider` (returns Mocky user, no real auth).
- `/api/internal/docx2md/` — vendored `docx2md-go`, imports updated, builds.
- `/web` — Vite + React + TS. One page that fetches `/api/health` and renders the result. `vite.config.ts` proxies `/api/*` to `http://localhost:8080`.
- `/db/migrations/` — goose migrations covering the full `starter-app.md` §5 schema. At minimum: `0001_init.sql` (organizations, departments, users, memberships), `0002_matters.sql` (matters, documents), `0003_runs.sql` (review_runs, lens_runs, findings). Every tenant-scoped table has `organization_id uuid not null`. **No RLS yet.** Comment in the migration noting RLS is a later phase.
- `/prompts/lens/` and `/prompts/summary/` — empty dirs with a `README.md` describing the file naming and template convention.
- `/scripts/` — `reset_db.sh` (drop and recreate `solomocky_dev`), `seed.sh` (calls `cmd/api -seed` or similar).
- `Makefile` at repo root: `dev` (runs api + vite concurrently), `migrate`, `migrate-down`, `reset`, `seed`, `test`, `lint`, `build`.
- `.env.example` — `ANTHRO_API_KEY=`, `DATABASE_URL=postgres://solomocky_app@localhost/solomocky_dev?sslmode=disable`, `PORT=8080`.
- `.gitignore` — `.env`, `var/`, `web/dist/`, `web/node_modules/`, `api/bin/`, `*.test`, `.DS_Store`.
- `README.md` at repo root, ≤30 lines: what SoloMocky is, prereqs (Go, Node, local Postgres), how to run, where the specs live.

### Tech choices to make and proceed (don't ask)

- HTTP router: `chi` is fine. So is stdlib `net/http` + Go 1.22+ pattern matching. Pick one, don't relitigate.
- DB driver: `pgx` v5 (`pgx/v5/pgxpool`).
- Migrations: `goose` (already specified in `starter-app.md`).
- Env loading: `godotenv`.
- FE: React 19 + TypeScript + Vite 7. No state library yet — `useState` and `fetch` are enough.

## Explicit non-goals

- No LLM calls. `llm.Client` is defined; no real method bodies.
- No Matter creation, no ReviewRun dispatch, no real handlers beyond `/health`.
- No tests beyond a single smoke test for `/health`. Plumbing is the goal.
- No WorkOS, Stripe, River, Helicone, PostHog, or any Azure SDK.
- No Bicep, no `/infra/` IaC content. `/infra/` stays empty (or absent).
- No RLS in the migrations (later phase). The app-layer `organization_id` predicates **do** go in from Day 1, in the seam interfaces and any SQL the stubs hint at.
- Don't commit `.env`. Don't commit `var/`, `web/dist/`, or `web/node_modules/`.

## Definition of done

A fresh clone runs end-to-end:

```
git clone …
cp .env.example .env
# fill in ANTHRO_API_KEY (even though nothing uses it yet — this validates the loader)
createdb solomocky_dev
psql -c "CREATE ROLE solomocky_app LOGIN;" solomocky_dev
make migrate
make dev
# browse to http://localhost:5173 — page shows "SoloMocky" and the JSON from /api/health
```

`make test` passes (one smoke test).
`make lint` passes (`go vet ./...`, `golangci-lint run` if configured, `tsc --noEmit` for the FE).

## Commit cadence

Incremental commits, each leaving the tree green:

1. Repo layout, `go.mod`, `Makefile`, `README`, `.env.example`, `.gitignore`.
2. Seam interfaces and stub impls under `/api/internal/`.
3. `docx2md-go` vendored under `/api/internal/docx2md/`.
4. DB migrations for §5 schema.
5. FE hello-world wired through the Vite proxy.
6. `/health` handler + smoke test + a screenshot or curl in the commit body proving it works.

Stop after commit 6 and report back. Phase 1 (real auth, Matter creation, etc.) is a separate session.
