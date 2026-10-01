# OpenSearch

Placeholder for centralized JSON logs.

Local Compose profile `observability` starts a single-node OpenSearch on port 9200.

Suggested index pattern: `waypoint-logs-*`

Services emit structured JSON to stdout. A later collector (filebeat / OTel logs) will ship them here. Milestone 1 does not run a shipper.
