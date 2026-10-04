#!/usr/bin/env bash
# Starts Grafana (and Prometheus, which it reads) from the observability Compose profile on a server.
# Grafana listens on the server's loopback only (127.0.0.1:3001): reach it through an SSH tunnel,
#   ssh -L 3001:127.0.0.1:3001 <user>@<server>   then open http://localhost:3001
# It refuses to start without a real admin password, so a server never runs Grafana's default admin/admin.
set -euo pipefail
cd "$(dirname "$0")/.."

password="$(grep -m1 '^GRAFANA_ADMIN_PASSWORD=' .env 2>/dev/null | cut -d= -f2- || true)"
if [ -z "$password" ] || [ "$password" = "admin" ]; then
  echo "Set GRAFANA_ADMIN_PASSWORD in .env to a real password first, for example:" >&2
  echo "  printf 'GRAFANA_ADMIN_PASSWORD=%s\n' \"\$(openssl rand -base64 24)\" >> .env" >&2
  exit 1
fi

docker compose --profile observability up -d prometheus grafana
docker compose --profile observability ps prometheus grafana
echo "Grafana is on 127.0.0.1:3001 on this server. Tunnel: ssh -L 3001:127.0.0.1:3001 <user>@<this server>"
