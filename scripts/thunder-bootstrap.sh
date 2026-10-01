#!/bin/sh
# Map ThunderID subjects into shared.users. Do not hand-copy subjects into seed SQL.
set -eu
URL="${DATABASE_URL:-postgres://waypoint:waypoint@postgres:5432/waypoint?sslmode=disable}"
THUNDER_URL="${THUNDER_URL:-http://thunderid:8090}"

http_get() {
  url="$1"
  if command -v wget >/dev/null 2>&1; then
    wget -qO- "$url"
  elif command -v curl >/dev/null 2>&1; then
    curl -fsS "$url"
  else
    apk add --no-cache wget >/dev/null
    wget -qO- "$url"
  fi
}

i=0
while [ "$i" -lt 60 ]; do
  if http_get "$THUNDER_URL/health/live" >/dev/null 2>&1; then
    break
  fi
  i=$((i + 1))
  sleep 1
done
if [ "$i" -ge 60 ]; then
  echo "thunder-bootstrap: identity provider not ready" >&2
  exit 1
fi

tmp="$(mktemp)"
http_get "$THUNDER_URL/admin/users.tsv" >"$tmp"

# Skip header. Columns: user_id subject role outlet_id depot
tail -n +2 "$tmp" | while IFS="$(printf '\t')" read -r user_id subject role outlet_id depot vehicle_id; do
  [ -n "$user_id" ] || continue
  [ -n "$subject" ] || continue
  [ -n "$role" ] || continue
  [ "$outlet_id" = "-" ] && outlet_id=""
  [ "$depot" = "-" ] && depot=""
  [ "$vehicle_id" = "-" ] && vehicle_id=""
  psql "$URL" -v ON_ERROR_STOP=1 \
    -c "INSERT INTO shared.users (id, identity_subject, role) VALUES ('$user_id', '$subject', '$role')
        ON CONFLICT (id) DO UPDATE SET identity_subject = EXCLUDED.identity_subject, role = EXCLUDED.role, updated_at = now()"
  if [ "$role" = "STORE_MANAGER" ] && [ -n "$outlet_id" ]; then
    psql "$URL" -v ON_ERROR_STOP=1 \
      -c "INSERT INTO shared.store_manager_profiles (user_id, outlet_id) VALUES ('$user_id', '$outlet_id')
          ON CONFLICT (user_id) DO UPDATE SET outlet_id = EXCLUDED.outlet_id"
  fi
  if [ "$role" = "LOADER" ] && [ -n "$depot" ]; then
    psql "$URL" -v ON_ERROR_STOP=1 \
      -c "INSERT INTO shared.loader_profiles (user_id, depot) VALUES ('$user_id', '$depot')
          ON CONFLICT (user_id) DO UPDATE SET depot = EXCLUDED.depot, updated_at = now()"
  fi
  if [ "$role" = "DRIVER" ] && [ -n "$vehicle_id" ]; then
    psql "$URL" -v ON_ERROR_STOP=1 \
      -c "INSERT INTO shared.driver_profiles (user_id, vehicle_id) VALUES ('$user_id', '$vehicle_id')
          ON CONFLICT (user_id) DO UPDATE SET vehicle_id = EXCLUDED.vehicle_id, updated_at = now()"
  fi
  echo "mapped $user_id <- $subject ($role)"
done

rm -f "$tmp"
echo "thunder-bootstrap complete"
