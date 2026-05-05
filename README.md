# SoloMocky

Self-contained starter app for Accordli. The Scaffolding grows around this skeleton, piece by piece, until Mocky → Analyze cutover.

See `notes/scaffolding/starter-app/` for the spec.

## Prereqs

- Go 1.22+
- Node 20+
- Local PostgreSQL (`brew install postgresql@16` is fine)

## Setup

```sh
cp .env.example .env
# fill in ANTHRO_API_KEY (nothing reads it yet, but this validates the loader)

createdb solomocky_dev
psql -c "CREATE ROLE solomocky_app LOGIN;" solomocky_dev

make migrate
make dev
```

API listens on `:8080`, FE on `:5173`. Browse to <http://localhost:5173>.

## Make targets

| Target | What it does |
|---|---|
| `make dev` | API + Vite concurrently |
| `make migrate` / `make migrate-down` | goose up/down |
| `make reset` | drop + recreate `solomocky_dev`, re-migrate |
| `make seed` | seed Mocky Org + User (stub today) |
| `make test` | `go test ./...` |
| `make lint` | `go vet`, `tsc --noEmit` |
| `make build` | API binary + FE bundle |
