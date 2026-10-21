# API reference

Waypoint's checked-in API contracts use OpenAPI 3.1 and live in `contracts/openapi/`. Open them in Swagger UI using the local, opt-in viewer below.

## Services

| Swagger UI entry | Contract |
| --- | --- |
| Agent (dispatcher assistant) | [agents.yaml](../../contracts/openapi/agents.yaml) |
| Order and dashboard assistants | [assistants.yaml](../../contracts/openapi/assistants.yaml) |
| Personal automations | [automations.yaml](../../contracts/openapi/automations.yaml) |
| Delivery | [delivery.yaml](../../contracts/openapi/delivery.yaml) |
| Fleet | [fleet.yaml](../../contracts/openapi/fleet.yaml) |
| Integrations | [integration.yaml](../../contracts/openapi/integration.yaml) |
| Loading | [loading.yaml](../../contracts/openapi/loading.yaml) |
| Orders | [orders.yaml](../../contracts/openapi/orders.yaml) |
| Planning | [planning.yaml](../../contracts/openapi/planning.yaml) |
| Shared | [shared.yaml](../../contracts/openapi/shared.yaml) |

The contracts describe the published API surface under `/api/v1`. They are maintained alongside the service handlers; check the relevant handler when an operation or behavior is not yet represented in a contract. Operations added in the 21 October 2026 refresh (order import/export and custody, plan revise, disruption risks, fleet incidents and fuel, delivery trip messages and distance, shared products, policies, audit search, notification preferences, SMS provider callbacks) list their permission and status codes but do not yet describe request and response bodies in detail; the handlers are the reference for the fields. The Swagger viewer lists the contracts in `docker-compose.api-docs.yml`; `automations.yaml` is read as raw YAML.

## Open Swagger UI locally

From the repository root, start the isolated Swagger UI service:

```sh
sudo docker compose -p waypoint-api-docs -f docker-compose.api-docs.yml up -d
```

Open <http://127.0.0.1:8092>. Swagger UI presents the service specs in a selector. The viewer listens on loopback only and does not join or change the application Compose stack. API submission buttons are disabled; use the documented routes with an authorized client when you need to make requests.

Stop the viewer when finished:

```sh
sudo docker compose -p waypoint-api-docs -f docker-compose.api-docs.yml down
```

### Use the viewer on the Azure VM

The container binds to the VM's loopback interface; it does not open a public port. From your computer, create an SSH tunnel:

```sh
ssh -L 8092:127.0.0.1:8092 azureuser@waypoint.claptac.dev
```

Keep that SSH session open and browse to <http://127.0.0.1:8092> on your computer.

## API conventions

All service APIs are versioned under `/api/v1/{domain}/...` and use bearer access tokens unless an operation is explicitly a provider callback or internal service call. Role and resource ownership are enforced by the backend.

Planning (`/api/v1/planning/plans`) is generate-once unless reset. GET requires `plan:view`. Assignment failures return 409 with structured reason codes. Fleet availability is `GET /api/v1/fleet/availability?date=` and `PUT /api/v1/fleet/vehicles/{id}/availability`.

Loading ready snapshots for delivery are `GET /api/v1/loading/internal/trips`. Delivery runs start from that READY snapshot only. Proofs are multipart PNG/JPEG; `POST /api/v1/delivery/sync` never carries proof bytes.

### Errors

Errors are RFC 7807 problem documents (`application/problem+json`): `type`, `title`, `status`, `detail`, and where useful a stable `code` (for example `workshop_pending`). A 500 always carries the generic detail `unexpected error`; the real cause is in the service log. Clients must not parse `detail` as a data field.

| Status | Meaning |
|---|---|
| 400 | Invalid input |
| 401 | Missing, expired or invalid token |
| 403 | Role or ownership check failed |
| 404 | Not found. `GET /planning/plans?date=` answers 404 when no plan exists for that date, which is an expected empty state |
| 409 | Conflict with current state (plan locked, stale version, constraint failure) |
| 429 | Per-user or per-address rate limit; honour `Retry-After` |
| 503 | `agent_unavailable` from the assistants: the model provider is not configured or failed; fill the form by hand |

### Things that often surprise

- A shortfall decision (`POST /delivery/trips/{tripId}/messages`) is stored by trip, so it works before the driver's run exists.
- `POST /planning/plans/{id}/revise` reopens a locked plan with no confirmation step; a later reset deletes its trips and allocations. Ask the user first.
- SMS status: the cellular gateway reports only `SMS_SENT` and `SMS_FAILED` (`POST /integrations/notifications/gateway`, HMAC in `X-CG-Signature`); `SENT` is final for it.
- Orders cannot be cancelled through the API; the dispatcher defers them.
