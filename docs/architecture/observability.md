# Observability

Every backend service exposes:

- `GET /health/live`
- `GET /health/ready`
- `GET /metrics`
- structured JSON logs
- correlation / request IDs
- OpenTelemetry initialization (no-op when OTLP is unset)

Metrics include HTTP request totals/durations/errors and Waypoint domain counters (`waypoint_orders_created_total`, agent tool metrics, and others).

`WaypointRepeatedDeliverySyncConflicts` watches the delivery service's instrumented sync-conflict counter. A conflict can leave an offline device operation queued for dispatcher or support resolution. Network failures that never reach the server are not observable by server-side Prometheus; client-side stale-queue notices remain on the device.

Local stack: `docker compose --profile observability up`

- Prometheus `:9090`
- Grafana `:3001`
- OpenSearch `:9200`
- OTel collector `:4318`
