# Observability

## Signals

Every Go service exposes `/health/live`, `/health/ready`, and `/metrics`. HTTP counters, errors, and duration use bounded service/method/route-template/status labels. Business metrics cover orders, plans, allocations, bounded constraint reasons, loading, delivery outcomes/proofs/sync conflicts, receipts, and the optional guarded agent. JSON access logs include request/correlation ID, route template, method, status, bytes, and duration; bearer tokens and request bodies are not logged.

HTTP server spans carry service, request/correlation ID, matched route template, and response status. OpenTelemetry trace export is optional through `OTEL_EXPORTER_OTLP_ENDPOINT`; the local collector uses a debug exporter. Compose's `observability` profile runs OTel Collector, Prometheus, and Grafana. OpenSearch is a separate optional `search` profile and is not connected to the trace/log pipeline; it is not required for the small local stack.

## Local use

```bash
make validate-observability
docker compose --profile observability up --build
```

`make validate-observability` renders the Compose profile and runs Prometheus `check config` and `check rules` against the same read-only configuration and alert files mounted by the service. Open Prometheus at `http://localhost:9090` and Grafana at `http://localhost:3001`. The provisioned Waypoint dashboard includes platform request/error/latency and availability panels plus business workflow and agent panels. Check Prometheus targets before interpreting missing series; domain counters appear after matching actions have occurred. OpenSearch is only available when explicitly started with `docker compose --profile search up -d opensearch`; no submission feature depends on it.

Useful queries include `sum by (service) (rate(http_requests_total[5m]))`, `sum by (service) (rate(http_errors_total[5m]))`, `histogram_quantile(0.95, sum by (le, service) (rate(http_request_duration_seconds_bucket[5m])))`, `up`, and the `waypoint_*` workflow counters. SMS instrumentation exposes outbox pending/oldest age, provider-accepted messages awaiting status callbacks, and bounded send/callback outcome counters. Alerts cover a stale queue, missing provider callback, and ambiguous sends requiring manual reconciliation.

Service dependency failures are visible in structured service logs and API error/latency metrics; no central log index is a submission dependency. Do not attach IDs, outlet names, arbitrary error text, or user values as metric labels.

## Driver offline queue monitoring

When the authenticated Driver PWA is online, it best-effort reports the oldest queued-work age and queue size as bounded buckets, at most once every 15 minutes per page session. The report is skipped while offline or when sync is paused; failure never blocks delivery work. `POST /api/v1/delivery/telemetry/offline-queue` accepts only these two fields, rejects unknown fields, caps the body at 1 KiB, requires the assigned-driver sync permission, and returns no content. The service stores no per-report record. Prometheus labels are limited to the documented age and count buckets; the payload contains no user, device, trip, stop, operation, exact timestamp, or delivery content.

`WaypointDriverQueueStale` fires when a report says queued work is at least seven days old, and `WaypointDriverQueueCritical` fires above 30 days. These are aggregate alerts: they signal that dispatch/support should review offline recovery, but intentionally cannot identify a driver or trip. Dispatch must use its normal driver contact and sync-conflict workflow to find and recover the work. Metrics retention follows the configured Prometheus TSDB retention; do not extend it without the privacy owner reviewing the operational need.
