#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."
compose_project=waypoint-browser-tests
docker compose -p "$compose_project" --profile backup-test rm -sf objectstore-backup-target >/dev/null 2>&1 || true
trap 'docker compose -p waypoint-browser-tests --profile backup-test stop objectstore-backup-target >/dev/null 2>&1 || true' EXIT HUP INT TERM
docker compose -p "$compose_project" --profile backup-test up -d objectstore-backup-target

export SOURCE_OBJECT_ENDPOINT=http://127.0.0.1:9000
export SOURCE_OBJECT_BUCKET=waypoint-proof
export SOURCE_OBJECT_ACCESS_KEY=s3mock
export SOURCE_OBJECT_SECRET_KEY=s3mock
export RESTORE_OBJECT_ENDPOINT=http://127.0.0.1:9001
export RESTORE_OBJECT_ACCESS_KEY=s3mock
export RESTORE_OBJECT_SECRET_KEY=s3mock
export WAYPOINT_BACKUP_BUCKET_PREFIX=waypoint-proof-restore
./scripts/create-object-storage-backup.sh
