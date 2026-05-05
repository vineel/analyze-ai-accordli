# SoloMocky Makefile.
#
# DATABASE_URL comes from .env (loaded by the API at runtime). For Make
# targets that talk to Postgres directly (goose, reset_db.sh), we export
# .env into the shell.

SHELL := /bin/bash

ENV_FILE := .env
ifneq ("$(wildcard $(ENV_FILE))","")
include $(ENV_FILE)
export
endif

GOOSE := go run github.com/pressly/goose/v3/cmd/goose@v3.22.1
MIGRATIONS_DIR := db/migrations

.PHONY: dev dev-api dev-web migrate migrate-down reset seed test lint build tidy

dev:
	@command -v npx >/dev/null || { echo "node/npx not found" >&2; exit 1; }
	@( cd api && go run ./cmd/api ) & \
	  ( cd web && npm run dev ) ; \
	  wait

dev-api:
	cd api && go run ./cmd/api

dev-web:
	cd web && npm run dev

migrate:
	$(GOOSE) -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" up

migrate-down:
	$(GOOSE) -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" down

reset:
	./scripts/reset_db.sh
	$(MAKE) migrate

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

tidy:
	cd api && go mod tidy
