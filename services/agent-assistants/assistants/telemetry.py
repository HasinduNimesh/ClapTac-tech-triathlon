# Metric names and labels match pkg/telemetry so existing Grafana panels include this service.
from prometheus_client import Counter, Histogram

REQUESTS = Counter("http_requests_total", "HTTP requests.", ["service", "method", "path", "status"])
LATENCY = Histogram("http_request_duration_seconds", "HTTP request duration.", ["service", "method", "path"])
ERRORS = Counter("http_errors_total", "HTTP errors.", ["service", "method", "path", "status"])
ASSISTANT_REQUESTS = Counter("waypoint_agent_assistant_requests_total", "Order and dashboard assistant requests.", ["assistant", "result"])
ASSISTANT_LATENCY = Histogram("waypoint_agent_assistant_latency_seconds", "Order and dashboard assistant latency.", ["assistant"])
