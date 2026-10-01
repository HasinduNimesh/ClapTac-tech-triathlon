#!/bin/sh
set -eu
URL="${DATABASE_URL:-postgres://waypoint:waypoint@postgres:5432/waypoint?sslmode=disable}"
DIR="$(cd "$(dirname "$0")" && pwd)"

psql "$URL" -v ON_ERROR_STOP=1 <<'SQL'
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    filename TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
SQL

for f in "$DIR"/migrations/*.sql; do
  name="$(basename "$f")"
  applied="$(psql "$URL" -tAc "SELECT 1 FROM public.schema_migrations WHERE filename = '$name'")"
  if [ "$applied" = "1" ]; then
    echo "skip $name"
    continue
  fi
  echo "apply $name"
  psql "$URL" -v ON_ERROR_STOP=1 -f "$f"
  psql "$URL" -v ON_ERROR_STOP=1 -c "INSERT INTO public.schema_migrations (filename) VALUES ('$name')"
done
