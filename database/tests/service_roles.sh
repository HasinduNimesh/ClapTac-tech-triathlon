#!/bin/sh
# Checks the per-service logins made by database/roles.sh against a database that has the
# migrations applied. Run it with the owner URL; it connects as each service login itself.
#
#   DATABASE_URL=postgres://waypoint:waypoint@host:5432/waypoint \
#   SERVICE_PASSWORDS="order=o1 planning=p1 fleet=f1 loading=l1 delivery=d1 shared=s1" \
#   sh database/tests/service_roles.sh
set -eu
URL="${DATABASE_URL:?Set DATABASE_URL to the owner URL}"
PREFIX="${WAYPOINT_DB_ROLE_PREFIX:-waypoint_}"
: "${SERVICE_PASSWORDS:?Set SERVICE_PASSWORDS like \"order=pw planning=pw ...\"}"

base="${URL%%\?*}"                 # postgres://user:pw@host:port/db
rest="${base#*://}"
hostdb="${rest#*@}"                # host:port/db
query=""; case "$URL" in *\?*) query="?${URL#*\?}";; esac

fail=0
pw_of() { for kv in $SERVICE_PASSWORDS; do [ "${kv%%=*}" = "$1" ] && echo "${kv#*=}"; done; }
as() { # as <svc> <sql>
  psql "postgres://${PREFIX}$1:$(pw_of "$1")@${hostdb}${query}" -v ON_ERROR_STOP=1 -qtAc "$2" >/dev/null 2>&1
}
expect_ok()   { if as "$1" "$2"; then :; else echo "FAIL  $1 should be allowed: $2"; fail=1; fi; }
expect_deny() { if as "$1" "$2"; then echo "FAIL  $1 should be denied:  $2"; fail=1; fi; }

tables() { psql "$URL" -qtAc "SELECT table_name FROM information_schema.tables WHERE table_schema='$1' AND table_type='BASE TABLE' ORDER BY 1 LIMIT 1"; }

for pair in order:orders planning:planning fleet:fleet loading:loading delivery:delivery shared:shared; do
  svc="${pair%%:*}"; own="${pair##*:}"
  t="$(tables "$own")"
  expect_ok   "$svc" "SELECT 1 FROM $own.$t LIMIT 1"
  expect_ok   "$svc" "SELECT 1"
  expect_deny "$svc" "CREATE TABLE $own.zz_probe(i int)"
  expect_deny "$svc" "CREATE TABLE public.zz_probe(i int)"
  expect_deny "$svc" "DROP TABLE $own.$t"
  expect_deny "$svc" "TRUNCATE $own.$t"
  for other in orders planning fleet loading delivery shared audit inventory ai; do
    [ "$other" = "$own" ] && continue
    ot="$(tables "$other")"
    [ -n "$ot" ] || continue
    # The only cross-schema reads allowed are listed below; everything else must fail.
    case "$svc:$other" in
      order:shared|order:fleet|order:delivery|shared:audit) continue;;
    esac
    expect_deny "$svc" "SELECT 1 FROM $other.$ot LIMIT 1"
    expect_deny "$svc" "INSERT INTO $other.$ot DEFAULT VALUES"
  done
done

# The narrow cross-schema reads the order service performs.
for t in shared.outlets shared.service_allowance shared.planning_policy_current shared.planning_policy_versions fleet.vehicles delivery.runs delivery.stops; do
  expect_ok order "SELECT 1 FROM $t LIMIT 1"
done
expect_deny order "UPDATE shared.outlets SET id=id"
expect_deny order "DELETE FROM delivery.stops"
expect_deny order "SELECT 1 FROM shared.profiles LIMIT 1"

# The audit log: shared reads and appends, nobody changes or deletes.
expect_ok   shared "SELECT 1 FROM audit.events LIMIT 1"
expect_deny shared "UPDATE audit.events SET action=action"
expect_deny shared "DELETE FROM audit.events"
expect_deny shared "TRUNCATE audit.events"
for svc in order planning fleet loading delivery; do
  expect_deny "$svc" "SELECT 1 FROM audit.events LIMIT 1"
  expect_deny "$svc" "INSERT INTO audit.events DEFAULT VALUES"
done

# Every login is an ordinary, unprivileged role.
n="$(psql "$URL" -qtAc "SELECT count(*) FROM pg_roles WHERE rolname LIKE '${PREFIX}%' AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls OR rolreplication)")"
[ "$n" = "0" ] || { echo "FAIL  a service login has elevated attributes"; fail=1; }

if [ "$fail" = "0" ]; then echo "service roles: all checks passed"; else exit 1; fi
