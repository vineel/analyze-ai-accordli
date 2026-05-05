#!/usr/bin/env bash
set -euo pipefail

DB="${DB_NAME:-solomocky_dev}"
ROLE="${DB_ROLE:-solomocky_app}"

echo "Dropping and recreating ${DB}..."
dropdb --if-exists "${DB}"
createdb "${DB}"

if ! psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${ROLE}'" | grep -q 1; then
  psql -c "CREATE ROLE ${ROLE} LOGIN;"
fi

psql -c "GRANT ALL PRIVILEGES ON DATABASE ${DB} TO ${ROLE};"
psql "${DB}" -c "GRANT ALL ON SCHEMA public TO ${ROLE};"

echo "Done. Run \`make migrate\` to apply migrations."
