#!/usr/bin/env sh
set -eu

: "${SOURCE_OBJECT_ENDPOINT:?Set the source object-store endpoint}"
: "${SOURCE_OBJECT_BUCKET:?Set the source proof bucket}"
: "${SOURCE_OBJECT_ACCESS_KEY:?Set source object-store credentials}"
: "${SOURCE_OBJECT_SECRET_KEY:?Set source object-store credentials}"
: "${RESTORE_OBJECT_ENDPOINT:?Set the independent backup endpoint}"
: "${RESTORE_OBJECT_ACCESS_KEY:?Set backup target credentials}"
: "${RESTORE_OBJECT_SECRET_KEY:?Set backup target credentials}"
command -v python3 >/dev/null

prefix=${WAYPOINT_BACKUP_BUCKET_PREFIX:-waypoint-proof-backup}
case "$prefix" in
  [a-z0-9]*) ;;
  *) echo "Backup bucket prefix must start with a lowercase letter or digit" >&2; exit 2 ;;
esac
case "$prefix" in
  *[!a-z0-9-]*|*-) echo "Backup bucket prefix may contain lowercase letters, digits, and internal hyphens only" >&2; exit 2 ;;
esac
[ "${#prefix}" -ge 3 ] || { echo "Backup bucket prefix must be at least 3 characters" >&2; exit 2; }

timestamp=$(date -u +%Y%m%dt%H%M%Sz)
bucket="${prefix}-${timestamp}-$$"
[ "${#bucket}" -le 63 ] || { echo "Generated backup bucket name exceeds the S3 63-character limit" >&2; exit 2; }

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)
cd "$repo_root"
RESTORE_OBJECT_BUCKET="$bucket" python3 scripts/verify-object-storage-restore.py
echo "OBJECT_BACKUP_BUCKET=$bucket"
