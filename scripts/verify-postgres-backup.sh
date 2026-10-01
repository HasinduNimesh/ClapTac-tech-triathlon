#!/usr/bin/env sh
set -eu

: "${SOURCE_DATABASE_URL:?Set SOURCE_DATABASE_URL to the database to back up}"
: "${RESTORE_DATABASE_URL:?Set RESTORE_DATABASE_URL to a separate, empty disposable database}"
[ "$SOURCE_DATABASE_URL" != "$RESTORE_DATABASE_URL" ] || { echo "Refusing to restore over the source database" >&2; exit 2; }
command -v pg_dump >/dev/null
command -v pg_restore >/dev/null

if [ "$#" -gt 0 ]; then
  backup_file=$1
  cleanup_backup=false
else
  backup_file=$(mktemp "${TMPDIR:-/tmp}/waypoint-backup.XXXXXX")
  cleanup_backup=true
fi
if [ "$cleanup_backup" = true ]; then trap 'rm -f "$backup_file"' EXIT HUP INT TERM; fi
pg_dump --format=custom --no-owner --no-privileges --file="$backup_file" "$SOURCE_DATABASE_URL"
pg_restore --exit-on-error --single-transaction --dbname="$RESTORE_DATABASE_URL" "$backup_file"
pg_restore --list "$backup_file" >/dev/null
echo "Backup archive created and restored successfully into the separate verification database."
