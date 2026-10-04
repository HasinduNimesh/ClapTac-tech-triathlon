#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
forbidden='database/sql|github.com/jackc/pgx|gorm.io/gorm|github.com/lib/pq'
if grep -RInE "^[[:space:]]*\"(${forbidden})" services/agent-orchestrator >/dev/null; then
  echo "agent-orchestrator must not import database drivers" >&2
  grep -RInE "^[[:space:]]*\"(${forbidden})" services/agent-orchestrator >&2 || true
  exit 1
fi
py_forbidden='psycopg|psycopg2|asyncpg|sqlalchemy|sqlite3|pg8000|redis'
if grep -RInE "^[[:space:]]*(import|from)[[:space:]]+(${py_forbidden})" services/agent-assistants >/dev/null; then
  echo "agent-assistants must not import database drivers" >&2
  grep -RInE "^[[:space:]]*(import|from)[[:space:]]+(${py_forbidden})" services/agent-assistants >&2 || true
  exit 1
fi
if grep -RInE "(${py_forbidden})" services/agent-assistants/requirements.txt >/dev/null; then
  echo "agent-assistants must not depend on database drivers" >&2
  exit 1
fi
echo "agent-orchestrator and agent-assistants database import check passed"
