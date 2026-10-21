# Architecture A

Waypoint is an independently deployable service monorepo. Clients never talk to PostgreSQL. The agent never talks to PostgreSQL.

## Deployment modes

### Local Compose

```
Browser / Flutter
        ↓
      NGINX
        ↓
     services
```

kGateway is not run locally.

### Azure VM (current production)

```
Internet → Caddy (TLS, waypoint.claptac.dev) → nginx 127.0.0.1:8080 → services
                                                  id.waypoint.claptac.dev → ThunderID (separate container)
```

The VM runs the same Compose file as local development. nginx serves the web app at `/`, the loader app at `/loader-app/` and the driver app download at `/downloads/`, and routes `/api/v1/<domain>` to the services below. It adds the security headers and a per-visitor limit on `/api/` requests; Redis adds per-user limits inside the services. Operational dashboards are not routed publicly; they listen on `127.0.0.1` and are opened through an SSH tunnel.

### Kubernetes / Architecture A

```
Internet
   ↓
NGINX / external edge
   ↓
kGateway
   ↓
services
```

## Edge responsibilities

**NGINX:** TLS termination, static web, edge headers, basic request limits.

**kGateway:** API routing by domain prefix, JWT/API policies, service routing, API rate policies, telemetry.

## Service prefixes

| Prefix | Service |
|---|---|
| `/api/v1/orders` | order-service |
| `/api/v1/planning` | planning-service |
| `/api/v1/fleet` | fleet-service |
| `/api/v1/loading` | loading-service |
| `/api/v1/delivery` | delivery-service |
| `/api/v1/shared` | shared-service |
| `/api/v1/integrations` | integration-service |
| `/api/v1/agent` | agent-orchestrator |
| `/api/v1/agent/order-assistant`, `/api/v1/agent/dashboard-assistant`, `/api/v1/agent/assistants` | agent-assistants (Python) |

Other paths worth knowing: `/api/v1/shared/automations` (personal workflows, habit helper), `/api/v1/shared/products` (the live product list the order helper reads), `/api/v1/integrations/notifications/{status,gateway}` (SMS provider callbacks, signed, no bearer token).

## Data

One PostgreSQL instance. Each service owns a schema and, when `database/roles.sh` has been applied, its own login that can only touch that schema. Cross-schema SQL is forbidden.

Redis is an optional cache (rate limits, the 30-second profile cache, the 20-second live run position). Every call treats Redis trouble as a cache miss, so Redis can be down without failing a request.

Delivery proofs go to an S3-compatible object store configured by `MINIO_ENDPOINT`, `MINIO_BUCKET`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_REGION` and `MINIO_AUTO_CREATE_BUCKET`. The delivery service writes, the loading service reads. The Compose default is a test double; use a durable private bucket (for example Cloudflare R2) in production.

## Shared code vs shared-service

- `pkg/` — in-process libraries (auth, authorization, cache, logging, telemetry, errors, object store, HTTP server and rate limits)
- `shared-service` — network APIs for profiles, notifications, audit ingest
