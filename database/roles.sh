#!/bin/sh
# Optional hardening: one least-privilege database login per service.
#
# Nothing runs this automatically and nothing needs it. Until you set the per-service
# *_DATABASE_URL variables (see .env.example) every service keeps using the single
# `waypoint` login exactly as before, so running or skipping this script cannot change a
# running deployment. It is idempotent: run it again to rotate passwords or re-sync grants
# after a migration adds tables to a schema.
#
# Run it as the role that owns the tables (the one apply.sh uses). Passwords are read from the
# environment, never from arguments, so they do not show up in `ps` or shell history:
#
#   ORDER_DB_PASSWORD=... PLANNING_DB_PASSWORD=... FLEET_DB_PASSWORD=... \
#   LOADING_DB_PASSWORD=... DELIVERY_DB_PASSWORD=... SHARED_DB_PASSWORD=... \
#   sh database/roles.sh
#
# What each login gets (see database/tests/service_roles.sh for the checked matrix):
#   - read/write on the tables of its own schema only, plus usage of its sequences;
#   - order additionally reads the few tables it joins in shared, fleet and delivery;
#   - shared additionally reads and appends to audit.events (no update, no delete);
#   - no DDL, no superuser, no access to any other schema.
# Tables added later by migrations are covered through default privileges.
set -eu
URL="${DATABASE_URL:-postgres://waypoint:waypoint@postgres:5432/waypoint?sslmode=disable}"
PREFIX="${WAYPOINT_DB_ROLE_PREFIX:-waypoint_}"

for need in ORDER PLANNING FLEET LOADING DELIVERY SHARED; do
  eval "value=\${${need}_DB_PASSWORD:-}"
  if [ -z "$value" ]; then
    echo "Set ${need}_DB_PASSWORD before running this script." >&2
    exit 2
  fi
done

# service:schema
for pair in order:orders planning:planning fleet:fleet loading:loading delivery:delivery shared:shared; do
  svc="${pair%%:*}"
  schema="${pair##*:}"
  upper="$(echo "$svc" | tr '[:lower:]' '[:upper:]')"
  echo "role ${PREFIX}${svc} -> schema ${schema}"
  eval "WAYPOINT_ROLE_PW=\${${upper}_DB_PASSWORD}" ; export WAYPOINT_ROLE_PW
  psql "$URL" -v ON_ERROR_STOP=1 -q \
    -v role="${PREFIX}${svc}" -v schema="$schema" -v svc="$svc" <<'SQL'
\getenv pw WAYPOINT_ROLE_PW

SELECT format('CREATE ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS', :'role')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'role') \gexec
SELECT format('ALTER ROLE %I PASSWORD %L', :'role', :'pw') \gexec
SELECT format('ALTER ROLE %I SET search_path = %I', :'role', :'schema') \gexec

-- Start from nothing so a re-run also removes grants that are no longer wanted.
SELECT format('REVOKE ALL ON ALL TABLES IN SCHEMA %I FROM %I', n.nspname, :'role')
FROM pg_namespace n WHERE n.nspname IN ('shared','audit','orders','planning','fleet','loading','delivery','inventory','ai') \gexec
SELECT format('REVOKE ALL ON ALL SEQUENCES IN SCHEMA %I FROM %I', n.nspname, :'role')
FROM pg_namespace n WHERE n.nspname IN ('shared','audit','orders','planning','fleet','loading','delivery','inventory','ai') \gexec
SELECT format('REVOKE ALL ON SCHEMA %I FROM %I', n.nspname, :'role')
FROM pg_namespace n WHERE n.nspname IN ('shared','audit','orders','planning','fleet','loading','delivery','inventory','ai') \gexec

-- Own schema.
GRANT USAGE ON SCHEMA :"schema" TO :"role";
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA :"schema" TO :"role";
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA :"schema" TO :"role";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema" GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO :"role";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema" GRANT USAGE, SELECT ON SEQUENCES TO :"role";

-- Narrow, read-only cross-schema access the code really performs.
SELECT :'svc' = 'order' AS is_order \gset
\if :is_order
GRANT USAGE ON SCHEMA shared, fleet, delivery TO :"role";
GRANT SELECT ON shared.outlets, shared.service_allowance,
                shared.planning_policy_current, shared.planning_policy_versions TO :"role";
GRANT SELECT ON fleet.vehicles TO :"role";
GRANT SELECT ON delivery.runs, delivery.stops TO :"role";
\endif

-- The audit log is written and read by shared-service only, and never changed or deleted.
SELECT :'svc' = 'shared' AS is_shared \gset
\if :is_shared
GRANT USAGE ON SCHEMA audit TO :"role";
GRANT SELECT, INSERT ON audit.events TO :"role";
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA audit TO :"role";
\endif
SQL
done

echo "Done. Point each service at its own login by setting <SERVICE>_DATABASE_URL (see .env.example)."
