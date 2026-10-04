#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
go test -count=1 ./...
go build ./...
go vet ./...
./scripts/check-agent-imports.sh
./scripts/test-agent-assistants.sh
(cd apps/web && npm test && NODE_OPTIONS="${NODE_OPTIONS:+$NODE_OPTIONS }--experimental-global-webcrypto" npm run build)
python3 -m unittest scripts.test_object_storage_restore scripts.test_object_storage_backup scripts.test_load_api scripts.test_postgres_backup scripts.test_import_outlet_locations
