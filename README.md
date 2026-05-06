# SoloMocky

Self-contained starter app for Accordli. The Scaffolding grows around this
skeleton, piece by piece, until Mocky → Analyze cutover.

For the build plan: `notes/claude-code-artifacts/solomocky-to-mocky-plan.md`.
For the current Phase 0 setup walkthrough (account creation, tunnel, etc.):
`notes/claude-code-artifacts/phase-0-kickoff.md`.

## Prereqs

- Go 1.25+
- Node 20+
- PostgreSQL 18 (`brew install postgresql@18 && brew services start postgresql@18`)
- Tailscale (Mac App Store or `brew install --cask tailscale`) — only
  required when you want to receive WorkOS / Stripe webhooks at your local
  Tailscale Funnel hostname

## First-time setup

```sh
cp .env.example .env
# Fill in the values per Track A in phase-0-kickoff.md. The minimum to boot
# the app is DATABASE_URL and ANTHRO_API_KEY.

make reset       # drops + recreates solomocky_dev, applies db/schema.sql, seeds
make dev         # API on :8080, FE on :5173
```

To receive real webhooks at a stable hostname (Tailscale Funnel):

```sh
# One-time, per developer (see phase-0-kickoff.md §A1 for full details):
#   - install Tailscale, sign in to the Accordli tailnet (currently named tail9acde7)
#   - rename your machine in the admin console (e.g., vineel-dev-ds9)
#   - set TUNNEL_HOSTNAME in .env to <your-machine>.tail9acde7.ts.net

make tunnel    # sudo tailscale funnel --bg 8080
```

## Make targets

| Target | What it does |
|---|---|
| `make dev` | API + Vite concurrently. Echoes URLs (and tunnel hostname if set). |
| `make dev-api` / `make dev-web` | One side only. |
| `make reset` | Drop + recreate `solomocky_dev`, apply `db/schema.sql`, seed. |
| `make seed` | Re-seed the Mocky org/user (also runs at API startup). |
| `make tunnel` | Bring up the per-developer Cloudflare Tunnel. |
| `make test` | `go test ./...`. |
| `make lint` | `go vet` + `tsc --noEmit`. |
| `make build` | API binary + FE bundle. |
| `make tidy` | `go mod tidy`. |

## Schema management

Pre-prod, there are no migrations. Schema lives in `db/schema.sql`; any
change is `make reset` away. Migrations come back at Phase 8 when there is
prod data to preserve. See CLAUDE.md → "Database preferences".
