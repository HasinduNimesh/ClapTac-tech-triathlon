# Observability

Every backend service exposes:

- `GET /health/live`
- `GET /health/ready`
- `GET /metrics`
- structured JSON logs (an unexpected 500 also logs its real cause as `internal_error`; the caller sees only "unexpected error")
- correlation / request IDs
- OpenTelemetry initialization (no-op when OTLP is unset)

Metrics include HTTP request totals/durations/errors and Waypoint domain counters (`waypoint_orders_created_total`, agent tool metrics, and others).

`WaypointRepeatedDeliverySyncConflicts` watches the delivery service's instrumented sync-conflict counter. A conflict can leave an offline device operation queued for dispatcher or support resolution. Network failures that never reach the server are not observable by server-side Prometheus; client-side stale-queue notices remain on the device.

Local stack: `docker compose --profile observability up`

- Prometheus `:9090`
- Grafana `:3001`
- OpenSearch `:9200`
- OTel collector `:4318`

## Dashboards (Grafana, provisioned from `infrastructure/observability/grafana/dashboards`)

| Dashboard | Answers |
|---|---|
| Waypoint Operations (`waypoint-overview`) | The original overview: request rate, errors, p95, planning, loading, delivery and receipt counters |
| Waypoint · Service health (`waypoint-service-health`) | Is it up and fast: services up, requests per second, 5xx rate, p50/p95/p99 latency, slowest and busiest routes, firing alerts; filter by service |
| Waypoint · Business flow (`waypoint-business-flow`) | The delivery chain in numbers, how long each step takes, SMS and audit queues |
| Waypoint · AI assistants (`waypoint-ai-assistants`) | Dispatcher assistant questions, tools called, failures, refusals, approvals, answer time |

Business counters live in each service's memory, so a deploy resets them; read them as "since the last restart". Latency here is server time; add the client's network time (about 100 ms from Colombo) for what a user feels.

## Alerts (`infrastructure/observability/prometheus/alerts.yml`)

Service down, high server-error rate, high latency, audit and SMS outbox stale or large, SMS status missing, repeated delivery sync conflicts, driver offline queue stale or critical. A text sent through the cellular gateway is final at `SENT` because the gateway reports only sent or failed, so only `QUEUED` and provider-reported `SENT` (Twilio) count as "awaiting a provider status".

## Not covered

The assistants service (Python) is not scraped by Prometheus, so the order helper and dashboard assistant have no metrics. Agent traces are available through the agent manager instead.

## Reaching the tools on the VM

Prometheus, Grafana, the agent manager and the identity console bind to `127.0.0.1` on the VM. Open an SSH tunnel, for example `ssh -N -L 3001:127.0.0.1:3001 -L 9090:127.0.0.1:9090 -L 8085:127.0.0.1:8085 <user>@<vm>`, then browse to `http://localhost:3001` (Grafana), `:9090` (Prometheus) and `:8085` (agent manager, needs the view token).
