#!/bin/sh
set -eu
URL="${DATABASE_URL:-postgres://waypoint:waypoint@postgres:5432/waypoint?sslmode=disable}"
DIR="$(cd "$(dirname "$0")" && pwd)"

psql "$URL" -v ON_ERROR_STOP=1 -f "$DIR/seeds/m2_outlets.sql"

if [ -f "$DIR/seeds/calendar.csv" ]; then
  psql "$URL" -v ON_ERROR_STOP=1 -c "TRUNCATE shared.operating_calendar"
  psql "$URL" -v ON_ERROR_STOP=1 -c "\\copy shared.operating_calendar (date, is_operating) FROM '$DIR/seeds/calendar.csv' WITH (FORMAT csv, HEADER true)"
fi

# Competition-shaped datasets (outlets, vehicles, optional travel tables).
sh "$DIR/import.sh"

# Product catalog and store ranges (regenerate with scripts/generate-catalog-seed.py).
if [ -f "$DIR/seeds/catalog.sql" ]; then
  echo "seed product catalog"
  psql "$URL" -v ON_ERROR_STOP=1 -q -f "$DIR/seeds/catalog.sql"
fi

# Eight weeks of simulated store trading and the agent observations built from it.
# Runs once; call inventory.delete_simulation_run on the seed-baseline run to regenerate.
if [ -f "$DIR/seeds/inventory_history.sql" ]; then
  echo "seed inventory history (first run takes a minute)"
  psql "$URL" -v ON_ERROR_STOP=1 -q -f "$DIR/seeds/inventory_history.sql"
fi
