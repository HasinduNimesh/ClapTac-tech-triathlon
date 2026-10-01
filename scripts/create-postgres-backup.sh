#!/usr/bin/env sh
set -eu

: "${SOURCE_DATABASE_URL:?Set SOURCE_DATABASE_URL to the database to back up}"
: "${WAYPOINT_BACKUP_DIR:?Set WAYPOINT_BACKUP_DIR to persistent backup storage}"
command -v pg_dump >/dev/null
command -v pg_restore >/dev/null
command -v sha256sum >/dev/null

umask 077
mkdir -p "$WAYPOINT_BACKUP_DIR"
backup_dir=$(cd "$WAYPOINT_BACKUP_DIR" && pwd -P)
backup_name="waypoint-postgres-$(date -u +%Y%m%dT%H%M%SZ)-$$.dump"
backup_file="$backup_dir/$backup_name"
checksum_file="$backup_file.sha256"
[ ! -e "$backup_file" ] && [ ! -e "$checksum_file" ] || {
  echo "Refusing to overwrite an existing backup artifact" >&2
  exit 2
}

temporary_file=$(mktemp "$backup_dir/.waypoint-postgres.XXXXXX")
checksum_temporary=""
cleanup() {
  rm -f "$temporary_file"
  if [ -n "$checksum_temporary" ]; then rm -f "$checksum_temporary"; fi
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
checksum_temporary=$(mktemp "$backup_dir/.waypoint-checksum.XXXXXX")

pg_dump --format=custom --no-owner --no-privileges --file="$temporary_file" "$SOURCE_DATABASE_URL"
pg_restore --list "$temporary_file" >/dev/null
digest=$(sha256sum "$temporary_file" | awk '{print $1}')
[ -n "$digest" ] || { echo "Could not calculate backup checksum" >&2; exit 1; }
printf '%s  %s\n' "$digest" "$backup_name" > "$checksum_temporary"

mv "$temporary_file" "$backup_file"
if ! mv "$checksum_temporary" "$checksum_file"; then
  rm -f "$backup_file"
  exit 1
fi
if ! (cd "$backup_dir" && sha256sum -c "$backup_name.sha256" >/dev/null); then
  rm -f "$backup_file" "$checksum_file"
  echo "Published backup checksum verification failed" >&2
  exit 1
fi
echo "POSTGRES_BACKUP=PASS archive=$backup_file checksum=$checksum_file"
