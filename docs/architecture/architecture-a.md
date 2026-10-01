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

## Data

One PostgreSQL instance. Each service owns a schema. Cross-schema SQL is forbidden.

## Shared code vs shared-service

- `pkg/` — in-process libraries (auth, logging, telemetry, errors)
- `shared-service` — network APIs for profiles, notifications, audit ingest
