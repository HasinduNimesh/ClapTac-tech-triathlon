#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
forbidden='database/sql|github.com/jackc/pgx|gorm.io/gorm|github.com/lib/pq'
if grep -RInE "^[[:space:]]*\"(${forbidden})" services/agent-orchestrator >/dev/null; then
  echo "agent-orchestrator must not import database drivers" >&2
  grep -RInE "^[[:space:]]*\"(${forbidden})" services/agent-orchestrator >&2 || true
  exit 1
fi
echo "agent-orchestrator database import check passed"
