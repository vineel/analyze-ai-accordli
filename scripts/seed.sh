#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
exec go -C api run ./cmd/api -seed
