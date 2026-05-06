# SoloMocky Makefile.
#
# DATABASE_URL comes from .env (loaded by the API at runtime). For Make
# targets that talk to Postgres directly (reset_db.sh) we export .env into
# the shell so they pick up DATABASE_URL too.
#
# No goose / migrations targets while we're pre-prod (per CLAUDE.md "Database
# preferences"). `make reset` rebuilds from db/schema.sql; that's the only
# path for schema changes until Phase 8.

SHELL := /bin/bash

ENV_FILE := .env
ifneq ("$(wildcard $(ENV_FILE))","")
include $(ENV_FILE)
export
endif

API_HEALTH_URL := http://localhost:8080/api/health
WEB_URL        := http://localhost:5173

.PHONY: dev dev-api dev-web reset seed test lint build tunnel tidy

dev:
	@command -v npx >/dev/null || { echo "node/npx not found" >&2; exit 1; }
	@echo "API     -> $(API_HEALTH_URL)"
	@echo "Web     -> $(WEB_URL)"
	@if [[ -n "$$TUNNEL_HOSTNAME" ]]; then \
	  echo "Tunnel  -> https://$$TUNNEL_HOSTNAME (run 'make tunnel' separately)"; \
	else \
	  echo "Tunnel  -> set TUNNEL_HOSTNAME in .env to enable 'make tunnel'"; \
	fi
	@( cd api && go run ./cmd/api ) & \
	  ( cd web && npm run dev ) ; \
	  wait

dev-api:
	cd api && go run ./cmd/api

dev-web:
	cd web && npm run dev

reset:
	./scripts/reset_db.sh
	$(MAKE) seed

seed:
	./scripts/seed.sh

test:
	cd api && go test ./...

lint:
	cd api && go vet ./...
	cd web && npx tsc --noEmit

build:
	mkdir -p api/bin
	cd api && go build -o bin/api ./cmd/api
	cd web && npm run build

# Expose this developer's local API to the public internet via Tailscale
# Funnel. One-time setup (sign in to the Accordli tailnet, rename the
# machine, set TUNNEL_HOSTNAME) is documented in
# notes/claude-code-artifacts/phase-0-kickoff.md §A1.
#
# `--bg` makes the funnel persist across reboots and detach from this
# shell. To take it down: `sudo tailscale funnel --bg off`.
tunnel:
	@command -v tailscale >/dev/null || { echo "tailscale not installed (App Store or 'brew install --cask tailscale')" >&2; exit 1; }
	@if [[ -n "$$TUNNEL_HOSTNAME" ]]; then \
	  echo "Tunnel: https://$$TUNNEL_HOSTNAME -> http://localhost:8080"; \
	else \
	  echo "Tunnel: https://<your-machine>.<tailnet>.ts.net -> http://localhost:8080 (set TUNNEL_HOSTNAME in .env to suppress this hint)"; \
	fi
	sudo tailscale funnel --bg 8080

tidy:
	cd api && go mod tidy
