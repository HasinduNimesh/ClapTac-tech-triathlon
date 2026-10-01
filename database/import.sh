#!/bin/sh
# Load competition-shaped CSVs. Vehicle status is NOT in vehicles.csv.
set -eu
URL="${DATABASE_URL:-postgres://waypoint:waypoint@postgres:5432/waypoint?sslmode=disable}"
DIR="$(cd "$(dirname "$0")" && pwd)"

if [ -f "$DIR/seeds/outlets.csv" ]; then
  echo "import outlets.csv"
  psql "$URL" -v ON_ERROR_STOP=1 <<SQL
CREATE TEMP TABLE tmp_outlets (
  outlet_id TEXT, brand TEXT, name TEXT, district TEXT, depot TEXT,
  dock_type TEXT, parking_constraint TEXT, mall_window TEXT,
  window_open_time TEXT, window_close_time TEXT
);
\\copy tmp_outlets FROM '$DIR/seeds/outlets.csv' WITH (FORMAT csv, HEADER true)
INSERT INTO shared.outlets (id, brand, name, district, depot, dock_type, parking_constraint, mall_window, window_open_time, window_close_time)
SELECT outlet_id, brand, COALESCE(NULLIF(name,''), brand || ' ' || outlet_id),
       COALESCE(district,''), COALESCE(depot,''), COALESCE(NULLIF(dock_type,''),'normal'),
       COALESCE(NULLIF(parking_constraint,''),'normal'),
       CASE WHEN lower(mall_window) IN ('1','true','t','yes') THEN true ELSE false END,
       NULLIF(window_open_time,'')::time, NULLIF(window_close_time,'')::time
FROM tmp_outlets
ON CONFLICT (id) DO UPDATE SET
  brand = EXCLUDED.brand, name = EXCLUDED.name, district = EXCLUDED.district,
  depot = EXCLUDED.depot, dock_type = EXCLUDED.dock_type,
  parking_constraint = EXCLUDED.parking_constraint, mall_window = EXCLUDED.mall_window,
  window_open_time = EXCLUDED.window_open_time, window_close_time = EXCLUDED.window_close_time;
SQL
fi

if [ -f "$DIR/seeds/vehicles.csv" ]; then
  echo "import vehicles.csv (static attributes only; status defaults to available)"
  psql "$URL" -v ON_ERROR_STOP=1 <<SQL
CREATE TEMP TABLE tmp_vehicles (
  vehicle_id TEXT, type TEXT, temp TEXT, weight_cap_kg TEXT, volume_cap_m3 TEXT,
  fuel_type TEXT, km_per_l TEXT, weekly_fuel_quota_l TEXT, home_depot TEXT
);
\\copy tmp_vehicles FROM '$DIR/seeds/vehicles.csv' WITH (FORMAT csv, HEADER true)
INSERT INTO fleet.vehicles (vehicle_id, type, temp, weight_cap_kg, volume_cap_m3, fuel_type, km_per_l, weekly_fuel_quota_l, home_depot)
SELECT vehicle_id, type, temp, weight_cap_kg::numeric, volume_cap_m3::numeric,
       COALESCE(NULLIF(fuel_type,''),'diesel'), km_per_l::numeric, weekly_fuel_quota_l::numeric, home_depot
FROM tmp_vehicles
ON CONFLICT (vehicle_id) DO UPDATE SET
  type = EXCLUDED.type, temp = EXCLUDED.temp, weight_cap_kg = EXCLUDED.weight_cap_kg,
  volume_cap_m3 = EXCLUDED.volume_cap_m3, fuel_type = EXCLUDED.fuel_type,
  km_per_l = EXCLUDED.km_per_l, weekly_fuel_quota_l = EXCLUDED.weekly_fuel_quota_l,
  home_depot = EXCLUDED.home_depot, updated_at = now();
SQL
fi

if [ -f "$DIR/seeds/district_travel.csv" ]; then
  echo "import district_travel.csv"
  psql "$URL" -v ON_ERROR_STOP=1 -c "TRUNCATE shared.district_travel"
  psql "$URL" -v ON_ERROR_STOP=1 -c "\\copy shared.district_travel (from_district, to_district, km, minutes) FROM '$DIR/seeds/district_travel.csv' WITH (FORMAT csv, HEADER true)"
else
  echo "district_travel.csv absent: TravelEstimator will use the documented M3 heuristic"
fi

if [ -f "$DIR/seeds/service_allowance.csv" ]; then
  echo "import service_allowance.csv"
  psql "$URL" -v ON_ERROR_STOP=1 -c "TRUNCATE shared.service_allowance"
  psql "$URL" -v ON_ERROR_STOP=1 -c "\\copy shared.service_allowance (stop_kind, minutes) FROM '$DIR/seeds/service_allowance.csv' WITH (FORMAT csv, HEADER true)"
fi

echo "import-datasets complete"
