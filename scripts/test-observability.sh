#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."
docker compose --profile observability config >/dev/null
docker compose --profile observability run --rm --no-deps \
  --entrypoint /bin/promtool prometheus \
  check config /etc/prometheus/prometheus.yml
docker compose --profile observability run --rm --no-deps \
  --entrypoint /bin/promtool prometheus \
  check rules /etc/prometheus/alerts.yml
