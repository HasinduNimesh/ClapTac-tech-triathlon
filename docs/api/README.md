# API reference

Waypoint's checked-in API contracts use OpenAPI 3.1 and live in `contracts/openapi/`. Open them in Swagger UI using the local, opt-in viewer below.

## Services

| Swagger UI entry | Contract |
| --- | --- |
| Agent | [agents.yaml](../../contracts/openapi/agents.yaml) |
| Delivery | [delivery.yaml](../../contracts/openapi/delivery.yaml) |
| Fleet | [fleet.yaml](../../contracts/openapi/fleet.yaml) |
| Integrations | [integration.yaml](../../contracts/openapi/integration.yaml) |
| Loading | [loading.yaml](../../contracts/openapi/loading.yaml) |
| Orders | [orders.yaml](../../contracts/openapi/orders.yaml) |
| Planning | [planning.yaml](../../contracts/openapi/planning.yaml) |
| Shared | [shared.yaml](../../contracts/openapi/shared.yaml) |

The contracts describe the published API surface under `/api/v1`. They are maintained alongside the service handlers; check the relevant handler when an operation or behavior is not yet represented in a contract.

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
