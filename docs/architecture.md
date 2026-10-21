# Architecture

Waypoint is a four-role delivery workflow built as a React PWA, two Flutter apps and independently deployable Go services, with a small Python service for the order and dashboard assistants. The authoritative service prefix table is in [Architecture A](architecture/architecture-a.md).

## What runs

```
 Store manager / Dispatcher (React PWA)   Loader (Flutter web, /loader-app)   Driver (Flutter Android, offline-first)
                     \                              |                              /
                      +------------- HTTPS: waypoint.claptac.dev -----------------+
                                              Caddy (TLS)
                                                 |
                                       nginx 127.0.0.1:8080  (static web, /api/ routing, edge headers, rate limit)
         +-------------+-------------+-----------+-----------+------------+--------------+------------------+
      order-service planning-svc  fleet-svc  loading-svc  delivery-svc  shared-service  integration-svc   agent tier
                                                                                                          (orchestrator,
                                                                                                           assistants)
         |  one PostgreSQL, one schema per service, one database login per service
         +-- Redis (optional cache; every use fails open) -- object store for proof photos -- ThunderID (OIDC)
```

- **Clients** talk only to HTTPS APIs. They never see PostgreSQL. The driver app keeps an offline queue and syncs when it is back online.
- **Business services** each own a PostgreSQL schema and call each other over HTTP with a machine token. They validate the ThunderID JWT, resolve the person through `shared.users`, check the role, then check resource ownership (outlet, vehicle, depot) and only then apply domain rules.
- **Agent tier**: `agent-orchestrator` (Go; the dispatcher's "Waypoint assistant" with a fixed tool allowlist and approvals) and `agent-assistants` (Python; order helper and dashboard assistant). Neither has a database; both call business services with the caller's own bearer token. See [Agent plane](architecture/agent-plane.md).
- **Identity**: ThunderID is the OpenID Connect provider at `id.waypoint.claptac.dev`. Roles are dispatcher, store manager, loader and driver.

## Workflow

Store Manager order → Dispatcher plan, allocation and deferral → Loader load, shortfall and ready → Driver route, proof and offline sync → Store Manager receipt → Dispatcher live status and audit. Planning feasibility is deterministic code; AI assistance is optional and never writes on its own.

## Data stores

| Store | Used for | Notes |
|---|---|---|
| PostgreSQL 16 | All business data, audit events, notification outbox | Nine schemas; per-service logins (`database/roles.sh`); migrations in `database/migrations` |
| Redis 7 | Per-user rate limits, profile cache (30 s), live run position (20 s) | Optional. With Redis down or unset every call still works, only slower (`pkg/cache`) |
| Object store (S3 API) | Delivery proof photos and signatures | Compose default is `adobe/s3mock`, a test double with no persistent volume. Production should use a private durable bucket such as Cloudflare R2; the service needs `MINIO_AUTO_CREATE_BUCKET=false` for a bucket-scoped key (see `docs/operations/object-storage-r2.md`) |
| Agent manager | Agent traces (what each assistant did and why) | Optional, off unless `AGENT_TRACE_URL` is set; holds no business data |

## Production deployment

One Azure VM runs the same `docker-compose.yml` as local development, with images built by GitHub Actions and pulled from GHCR. Caddy terminates TLS in front of nginx on `127.0.0.1:8080`. The release path is `.github/workflows/ci-cd.yml`: checks (Go, web, driver app, loader app, container builds, security scans), image publish, then a manually started deploy that runs preflight checks (identity configuration, no development credentials, scopes defined) before changing anything. See [CI/CD](ci-cd.md).

Operational tools are bound to the VM's loopback address and reached through an SSH tunnel: Grafana (3001), Prometheus (9090), the agent manager (8085) and the identity console. A cellular SMS gateway runs as a separate stack on the same VM; `integration-service` calls it and receives its webhooks.

## Observability

Every service exposes `/health/live`, `/health/ready` and `/metrics`, writes JSON logs with a correlation id, and logs the real error behind any unexpected 500. Prometheus scrapes the services; Grafana has four provisioned dashboards. See [Observability](architecture/observability.md).

## Testing and performance

[Test and performance report](TEST_AND_PERFORMANCE_REPORT.md): automated suites, end-to-end runs, latency percentiles, known gaps and the tests still needed.

## What is not claimed

Kubernetes manifests are deployment inputs, not a running deployment. High availability is not provided: there is one VM, one database instance and one object store. Backups, restore drills, branch protection and the other items in [SOC 2 readiness](SOC2_READINESS_ASSESSMENT.md) are tracked there.
