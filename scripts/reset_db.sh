#!/usr/bin/env bash
#
# Drop and recreate the local SoloMocky database, apply db/schema.sql.
# Per CLAUDE.md and solomocky-to-mocky-plan.md, this is the only path for
# schema changes until Phase 8 reintroduces migrations.
#
# Run from the repo root via `make reset` (which also seeds afterwards).

set -euo pipefail

DB="${DB_NAME:-solomocky_dev}"
ROLE="${DB_ROLE:-solomocky_app}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCHEMA_FILE="${REPO_ROOT}/db/schema.sql"

if [[ ! -f "${SCHEMA_FILE}" ]]; then
  echo "schema not found at ${SCHEMA_FILE}" >&2
  exit 1
fi

echo "Dropping and recreating ${DB}..."
dropdb --if-exists "${DB}"
createdb "${DB}"

if ! psql -d postgres -tAc "SELECT 1 FROM pg_roles WHERE rolname='${ROLE}'" | grep -q 1; then
  psql -d postgres -c "CREATE ROLE ${ROLE} LOGIN;"
fi

psql -d postgres -c "GRANT ALL PRIVILEGES ON DATABASE ${DB} TO ${ROLE};"
psql -d "${DB}" -c "GRANT ALL ON SCHEMA public TO ${ROLE};"

echo "Applying ${SCHEMA_FILE}..."
psql -v ON_ERROR_STOP=1 -d "${DB}" -f "${SCHEMA_FILE}"

# Grant the app role read/write on every table created above. Tables are
# owned by the superuser (whoever ran psql), and ${ROLE} won't inherit
# permissions automatically. RLS in Phase 2 will narrow this back down to
# `accordli_app` with row-level scoping.
psql -d "${DB}" -c "GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ${ROLE};"
psql -d "${DB}" -c "GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO ${ROLE};"
psql -d "${DB}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ${ROLE};"
psql -d "${DB}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE ON SEQUENCES TO ${ROLE};"

echo "Done. \`make seed\` will populate the Mocky org (chained by \`make reset\`)."
