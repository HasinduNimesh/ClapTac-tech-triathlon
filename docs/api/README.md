# API contracts

OpenAPI 3.1 lives in `contracts/openapi/`.

All service APIs are versioned under `/api/v1/{domain}/...`.

Planning (`/api/v1/planning/plans`) is generate-once unless reset. GET requires `plan:view`. Assignment failures return 409 with structured reason codes. Fleet availability is `GET /api/v1/fleet/availability?date=` and `PUT /api/v1/fleet/vehicles/{id}/availability`.

Loading ready snapshots for delivery are `GET /api/v1/loading/internal/trips`. Delivery runs start from that READY snapshot only. Proofs are multipart PNG/JPEG; `POST /api/v1/delivery/sync` never carries proof bytes.
