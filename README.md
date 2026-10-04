# Waypoint Intelligent Delivery Platform

Waypoint is an Architecture A group-delivery platform for Store Managers, Dispatchers, Loaders, and Drivers. It covers ordering, deterministic planning, loading, route execution with offline sync and proof, receipt confirmation, and event-based operations visibility. The optional M7 assistant is guarded by role-based tools and human approval.

## Architecture summary

Local flow: React PWA → NGINX → Go services → one PostgreSQL instance with service-owned schemas, Redis, and S3-compatible proof storage. Services communicate over HTTP and do not query one another's schemas. Human identity is ThunderID OIDC; application roles and resource ownership are resolved from `shared-service`. The Agent has no database connection. See [architecture](docs/architecture.md), [data model](docs/data-model.md), and [security](docs/security.md).

Kubernetes manifests describe the intended NGINX/TLS edge → kGateway → services flow. There is no provisioned public URL or production cluster in this workspace; no public deployment is claimed.

## Prerequisites

- Docker Engine and Docker Compose v2 (`docker compose version`)
- Go 1.22+ and Node.js 20+ for local builds/tests outside Docker
- Python 3 for the standard-library seed validator
- Flutter 3.x only to develop the loader app outside Docker (`apps/loader-web`)
- Windows: WSL 2 for Docker Desktop (`wsl --install`)

## Local setup

Step-by-step guides, including demo data and troubleshooting:

- **Web (Dispatcher, Store Manager) and the backend:** [docs/run-locally.md](docs/run-locally.md)
- **Loader workspace (Flutter web, served at `/loader-app/`):** [docs/run-dispatcher-and-loader.md](docs/run-dispatcher-and-loader.md) and [apps/loader-web/README.md](apps/loader-web/README.md)

```bash
cp .env.example .env
docker compose up --build
```

Compose starts PostgreSQL, Redis, MinIO-compatible local proof storage, database migrations, reference seeds, the local ThunderID-compatible development shim and identity bootstrap, all Go services, the web app, and NGINX. Wait for the migration/seed/bootstrap jobs to complete before signing in. This stack is local development only: demo passwords, development M2M values, HTTP, and `adobe/s3mock` must not be exposed as a production setup.

Open:

- App: <http://localhost/>
- Local identity shim: <http://localhost:8090>
- NGINX live check: <http://localhost/health/live>
- Grafana/Prometheus when enabled: <http://localhost:3001> / <http://localhost:9090>

## Environment configuration

`.env.example` lists local placeholders for OIDC, Postgres, Redis, MinIO, optional OTLP, and optional LLM configuration. Keep real credentials out of Git. `AUTH_DISABLED` must remain `false`; the application rejects disabled auth unless the explicit runtime environment is `local`. Configure an OpenAI-compatible provider only when needed with `LLM_BASE_URL`, `LLM_MODEL`, and `LLM_API_KEY`; without it, assistant endpoints return the safe `agent_unavailable` response and the operational app continues.

The checked-in `database/seeds/` files are generated from the official `Tech-Triathlon 2026 - Datasets/` release via `scripts/convert-official-dataset.py` (120 outlets, 60 vehicles, 910 calendar dates — matching the official counts). The travel and service-allowance tables are a documented lossy approximation of the official reference model; see `database/seeds/README.md`. After pulling an updated official dataset, re-run `python3 scripts/convert-official-dataset.py`, then `./scripts/validate-seeds.py`, then `make seed-competition-data`. The importer upserts outlet/vehicle rows and reloads the optional reference tables.

## Seeded judge accounts

Each account has one server-side role; there is no role selector. The local development password is `waypoint` for all four:

| Username | Role | Demo scope |
|---|---|---|
| `store-manager` | Store Manager | Outlet `OUT034` |
| `dispatcher` | Dispatcher | Dispatch/plan operations; works from the Peliyagoda depot (`dispatcher_profiles`), can switch depots |
| `loader` | Loader | North depot / Peliyagoda |
| `driver` | Driver | Vehicle `VEH001` |

`store-manager-b` and `loader-kandy` are isolation/test users for `OUT021` and the South depot. These identities and passwords exist only in the local OIDC-compatible shim. `scripts/thunder-bootstrap.sh` maps stable identity subjects and profiles into `shared` tables; no subject ID needs manual copying.

## Judge walkthrough

Use a current operating date from the seeded calendar and a disposable local environment. Follow the [6–8 minute demo script](docs/demo-script.md):

1. Store Manager signs in, creates an order, and checks its status/cutoff.
2. Dispatcher opens the order queue, generates a plan, reviews deterministic constraints, records any deferral reason, and confirms a resolved plan.
3. Loader records stop-aware load outcomes and any shortfall, then marks the trip ready once required outcomes are resolved.
4. Driver executes the route, records an outcome and proof, queues a change offline, then reconnects to sync.
5. Store Manager confirms received quantities or reports a discrepancy.
6. Dispatcher checks event-based loading, delivery, and receipt issue visibility; no GPS tracking is implied.
7. Show Grafana metrics and explain optional approval-gated AI.

## Driver offline test

Sign in as `driver / waypoint`, open Driver → Trips, and use browser DevTools to set network to Offline. Record arrival and proof/outcome work, then set network Online and use **Sync Now**. Verify the UI reports queued/syncing/synced state and the end-of-day summary reflects the server-accepted outcome. The app shell is cached; API responses are not. See [offline sync](docs/offline-sync.md) for a local delivery-service outage alternative. Do not use DevTools cache-clearing during this test because that removes browser-local queued state.

## Observability

```bash
docker compose --profile observability up --build
```

Grafana is at port 3001 and Prometheus at 9090. See [dashboard and queries](docs/observability.md). OpenSearch is optional and not required for a small VM. Go services expose `/health/live`, `/health/ready`, and `/metrics`; labels use route templates to keep IDs out of metric cardinality.

## Tests and build commands

Run the complete local acceptance suite with:

```bash
make verify
```

`make verify` runs uncached Go unit and PostgreSQL-backed end-to-end tests, Go build/vet, the agent database-import guard, all web tests and the production PWA build, and backup/load Python checks. Docker must be available for the Testcontainers-backed PostgreSQL suites. On Node 18, the PWA build enables the global Web Crypto compatibility flag. Use `make test` for the Go tests alone. The actual clean-start and browser checks are recorded in [submission evidence](docs/submission.md).

## Significant departures from the Designathon design

The Hackathon build follows the Designathon workflow (order → plan/allocate/defer → load → deliver offline with proof → receipt → dispatcher visibility). Implementation-level departures from the original design are:

- **Identity:** a local ThunderID-compatible OIDC shim (`tools/dev-oidc`) stands in for the production identity provider; the contract (Authorization Code + PKCE, JWKS, `sub` → `shared.users`) is unchanged.
- **Planner:** deterministic greedy allocation with hard constraints enforced server-side. Deferral priority is a transparent score (prior deferrals, days unserved, small chilled/Fresh consequence weights) that never overrides a hard constraint. Each unallocated order reports a primary reason plus the other limiting factors that blocked other vehicles.
- **Store Manager deferral notices:** solver reason codes are translated into plain-language messages with a next action (English, Sinhala, Tamil).
- **Driver client:** the responsive React PWA is the supported driver client; the Flutter shell is an optional, unverified scaffold.
- **Assistant approvals:** pending approvals are held in bounded process memory (5-minute expiry) instead of Redis; the approval interface allows a shared store later.
- **Data:** the checked-in seeds are competition-shaped development fixtures, not the official dataset; replace them with `make seed-competition-data`.
- **Out of scope for now:** maps, live Datathon model integration, live SMS delivery, GPS tracking, and a public production deployment.

## Limitations and deviations

- `database/seeds/` CSVs are converted from the official `Tech-Triathlon 2026 - Datasets/` release via `scripts/convert-official-dataset.py`; the converted district-travel and service-allowance tables are documented lossy approximations (see `database/seeds/README.md`), not the official reference model exactly. `validate-seeds.py` checks structural consistency, not semantic fidelity to the official model.
- The local identity service implements the ThunderID contract for development; replace it with the official identity provider before external deployment.
- The Flutter driver shell is optional; the responsive React PWA is the supported required client. Maps use approximate outlet positions (district centre; no GPS). Datathon forecast imports are outside current implementation scope.
- A public URL, DNS zone, TLS certificate, production cluster, durable DB/object storage, and production secret store have not been supplied. Kubernetes files are deployment inputs and need target-specific release configuration.
- Assistant provider is optional and not configured by default. Planning feasibility remains deterministic; sensitive assistant writes require explicit human approval and service-side reauthorization.
- See [known deployment requirements](docs/architecture.md), [AI disclosure](docs/ai-disclosure.md), and the [submission runbook](docs/submission.md).
