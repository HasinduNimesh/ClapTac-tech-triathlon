<div align="center">
  <img src="apps/web/src/assets/login/logo.png" alt="Waypoint" width="200" />
  <h1>Waypoint</h1>
  <p><strong>Delivery planning and operations, from order to confirmed receipt.</strong></p>
  <p>Team ClapTac · Tech-Triathlon 2026</p>
  <p>
    <a href="#quick-start">Quick start</a> ·
    <a href="#configuration">Configuration</a> ·
    <a href="#seeded-credentials">Test accounts</a> ·
    <a href="#screenshots">Screenshots</a> ·
    <a href="#architecture">Architecture</a> ·
    <a href="#development-and-validation">Validation</a>
  </p>
</div>

Waypoint connects **Store Managers, Dispatchers, Loaders, and Drivers** in one delivery workflow. Stores place orders, dispatchers allocate the fleet, loaders resolve loading exceptions, drivers record delivery evidence with offline support, and stores confirm what they received.

The platform combines a React PWA, eight Go services, deterministic planning rules, role- and resource-scoped APIs, and optional AI assistance. Core operations work without an LLM provider.

> **Scope of this README:** setup instructions describe this checkout and its Docker Compose stack. The supplied submission PDF describes a broader system; its original diagrams and the differences from this checkout are preserved in the [architecture reference](docs/architecture/system-diagrams.md#submission-reference-and-checkout-differences).

## Contents

- [Product capabilities](#product-capabilities)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [Seeded credentials](#seeded-credentials)
- [Seed data and demo walkthrough](#seed-data-and-demo-walkthrough)
- [Screenshots](#screenshots)
- [Architecture](#architecture)
- [Data model](#data-model)
- [API reference](#api-reference)
- [Development and validation](#development-and-validation)
- [Operations and troubleshooting](#operations-and-troubleshooting)
- [Security and AI disclosure](#security-and-ai-disclosure)
- [Repository layout](#repository-layout)
- [Limitations and deployment notes](#limitations-and-deployment-notes)
- [Contributing and pull requests](#contributing-and-pull-requests)
- [Documentation](#documentation)

## Product capabilities

| Workspace | Main capabilities |
| --- | --- |
| **Store Manager** | Place orders, track status, review deferrals, confirm receipts, report discrepancies, and view order evidence. |
| **Dispatcher** | Review the order queue, generate and validate plans, allocate vehicles, record deferral reasons, publish plans, monitor operations, and manage fleet availability. |
| **Loader** | Review trip loads, record loaded quantities and missing/damaged items, and mark a resolved load ready for departure. |
| **Driver** | Open assigned trips, record arrival and delivery outcomes, capture proof, queue work locally, and synchronize after reconnection. |
| **Personal automations** | Evidence-based habit suggestions, editable weekly workflows, preview before activation, notifications, and execution history. |
| **Operations** | Audit records, role isolation, service health endpoints, Prometheus metrics, optional Grafana/OTel, and documented backup procedures. |

English, Sinhala, and Tamil are available in the web interface. Maps use approximate outlet positions and reported operational events; they are not live GPS tracking.

## Quick start

### 1. Prerequisites

| Requirement | When needed |
| --- | --- |
| Git | Clone the repository. |
| Docker Engine/Desktop and Docker Compose v2 | Run the complete local application. Docker must be running. |
| Go 1.22+ | Build, test, or develop backend services outside Docker. |
| Node.js 20+ and npm | Build or test the React application outside Docker. |
| Python 3 | Validate and convert seed data; run Python utility tests. |
| Flutter | Only for the optional `apps/driver-mobile` project; not required for the supported web workspaces. |

On Windows, use Docker Desktop with WSL 2 and run the shell commands from WSL. The Docker build supplies the Go and Node toolchains for the application itself.

### 2. Clone and configure

```bash
git clone https://github.com/HasinduNimesh/ClapTac-tech-triathlon.git
cd ClapTac-tech-triathlon
cp .env.example .env
```

If you already have a `.env`, preserve it. Default Compose values are sufficient for the local demo; see [configuration](#configuration) before changing identity URLs or database credentials.

### 3. Start the application

```bash
docker compose up -d --build
docker compose ps -a
docker compose logs --tail=100 migrate seed bootstrap
```

The first build may take several minutes. `migrate`, `seed`, and `bootstrap` are one-shot jobs: **Exited (0) is success**. Wait for those jobs to finish and for the web/backend health checks to become healthy.

Open **[http://localhost](http://localhost)**, choose **Sign in**, and use a [local account](#local-docker-compose-accounts). The account profile selects the workspace automatically; sign out before switching roles.

```bash
curl -fsS http://localhost/health/live
```

The edge health check confirms NGINX is responding. Use `docker compose ps` to inspect individual service health as well.

### Local endpoints

| Component | Address | Availability |
| --- | --- | --- |
| Waypoint web/PWA | [localhost](http://localhost) | Default stack |
| Development OIDC provider | [localhost:8090](http://localhost:8090/.well-known/openid-configuration) | Default stack; sign in through the app |
| PostgreSQL | `127.0.0.1:5432` | Default stack |
| Redis | `127.0.0.1:6379` | Default stack |
| S3-compatible development storage | `http://localhost:9000` | Default stack; S3Mock, not a MinIO console |
| Swagger UI | [localhost:8092](http://localhost:8092) | Separate [API viewer](#api-reference) |
| Grafana / Prometheus | [localhost:3001](http://localhost:3001) / [localhost:9090](http://localhost:9090) | `observability` profile |
| OTLP HTTP collector | `http://localhost:4318` | `observability` profile |

Compose binds published ports to loopback. The optional `backup-test` profile uses port 9001; the `search` profile uses port 9200.

## Configuration

Start with [`.env.example`](.env.example). **[`docker-compose.yml`](docker-compose.yml) determines which values reach each container**; listing a variable in `.env` alone does not override a hardcoded Compose setting.

| Area | Variables / defaults | Configuration notes |
| --- | --- | --- |
| Runtime | `ENVIRONMENT=local`, `AUTH_DISABLED=false` | Compose explicitly keeps authentication enabled. Disabled authentication is rejected outside the local runtime. |
| Database | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` = `waypoint`; `DATABASE_URL` | Keep the credentials in the connection URL consistent with PostgreSQL. Migration, seed, and bootstrap jobs currently hardcode the local connection URL. |
| Identity | `OIDC_ISSUER=http://localhost:8090`, `OIDC_AUDIENCE=waypoint-api` | Services consume these values, but the bundled identity container uses hardcoded local issuer/audience values. Change both sides together. |
| Internal identity endpoints | JWKS: `http://thunderid:8090/oauth2/jwks`; token: `http://thunderid:8090/oauth2/token` | Set directly in Compose. Internal service names resolve inside the Compose network. |
| Service authentication | `ORDER_M2M_CLIENT_SECRET`, `PLANNING_M2M_CLIENT_SECRET`, `FLEET_M2M_CLIENT_SECRET`, `LOADING_M2M_CLIENT_SECRET`, `DELIVERY_M2M_CLIENT_SECRET`, `SHARED_M2M_CLIENT_SECRET`, `INTEGRATION_M2M_CLIENT_SECRET` | Local defaults are provided. `FLEET_M2M_CLIENT_SECRET` is supported by Compose but is not listed in `.env.example`. Use distinct credentials with a real provider. |
| Web build | `VITE_API_BASE_URL`, `VITE_OIDC_ISSUER`, `VITE_OIDC_CLIENT_ID`, `VITE_OIDC_REDIRECT_URI` | Current Compose build arguments are fixed to `/api/v1`, the local issuer, `waypoint-web`, and `http://localhost/auth/callback`. Edit/override build arguments and rebuild `web` when changing them. |
| Cache | `REDIS_URL=redis://redis:6379/0` | Shared cache infrastructure; do not assume every subsystem persists state here. |
| Evidence storage | `MINIO_ENDPOINT=http://minio:9090`, `MINIO_BUCKET=waypoint-proof` | The service is named `minio`, but local Compose runs `adobe/s3mock`. |
| Proof retention | `DELIVERY_PROOF_RETENTION_ENABLED=false`, `DELIVERY_PROOF_RETENTION_DAYS=180` | Retention is disabled by default. Review the [retention runbook](docs/operations/proof-retention.md) before enabling deletion. |
| Optional AI provider | `LLM_BASE_URL`, `LLM_MODEL`, `LLM_API_KEY` | An OpenAI-compatible provider; missing base URL/model produces `agent_unavailable`. Provider keys remain server-side. |
| Optional SMS | `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_FROM` or `TWILIO_MESSAGING_SERVICE_SID`, `TWILIO_STATUS_CALLBACK_URL` | Integration Service supports Twilio; credentials and callback wiring are deployment-specific. |
| Optional tracing | `OTEL_EXPORTER_OTLP_ENDPOINT` | Use `http://otel-collector:4318` for services inside Compose, with the observability profile enabled. |

Changing the public hostname requires coordinated web build arguments, provider registration/redirect URIs, issuer configuration, and edge configuration. A single `.env` edit is not sufficient for this Compose file. Keep `.env` and real provider secrets out of Git.

To enable observability:

```bash
# Optional: set OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318 in .env
docker compose --profile observability up -d --build
```

See [observability](docs/observability.md) for metrics, dashboards, and checks.

## Seeded credentials

### Local Docker Compose accounts

These accounts are defined in [`tools/dev-oidc/main.go`](tools/dev-oidc/main.go) and mapped to application profiles by [`scripts/thunder-bootstrap.sh`](scripts/thunder-bootstrap.sh). **The local password is `waypoint` for every account below.**

| Username | Password | Role | Scope | Workspace |
| --- | --- | --- | --- | --- |
| `store-manager` | `waypoint` | Store Manager | Outlet `OUT034` | `/store-manager` |
| `dispatcher` | `waypoint` | Dispatcher | Planning and dispatch | `/dispatcher` |
| `loader` | `waypoint` | Loader | `DEPOT_NORTH` | `/loader` |
| `driver` | `waypoint` | Driver | Vehicle `VEH001` | `/driver/trips` |
| `store-manager-b` | `waypoint` | Store Manager | Outlet `OUT021`; isolation testing | `/store-manager` |
| `loader-kandy` | `waypoint` | Loader | `DEPOT_SOUTH`; isolation testing | `/loader` |

### Team-provided testing accounts

The following initial credentials were supplied by Team ClapTac for testing and explicitly approved for inclusion in this repository. They belong to the team's separately provisioned test identity environment; **the default local OIDC shim does not seed these email accounts**. Obtain that environment's URL from the team. Team-confirmed roles are listed below. Current password validity has not been verified from this checkout.

| Username | Role | Initial testing password |
| --- | --- | --- |
| `dispatcher@claptac.dev` | Dispatcher | `41a7931d95b729d4e5deb2cfba335547eba72755c944983c1348805e9597b455` |
| `loader@claptac.dev` | Loader | `2321957f8a44bc551650df5349e11ef84357bfb2822695a9882a44f703baf7bac` |
| `damindu@claptac.dev` | Driver | `9cd7571458cd096c297f27664991f68ff604a6ca2cdcb940ba52073939e3c993` |
| `saman@claptac.dev` | Store Manager | `4f9cebf74f3705fb161d22e97c9464ac81fd13e425b3d8ba80c768df305fc29a` |

These are public **test credentials**, not production credentials. Use them only in the intended test environment.

## Seed data and demo walkthrough

### Reference data

The checked-in CSVs are converted from [`Tech-Triathlon 2026 - Datasets/`](Tech-Triathlon%202026%20-%20Datasets/) using [`scripts/convert-official-dataset.py`](scripts/convert-official-dataset.py).

| Dataset | Validated rows |
| --- | ---: |
| Outlets | 120 |
| Vehicles | 60 |
| Operating-calendar dates | 910 |
| Directed travel rows | 34 |
| Service allowances | 2 |

```bash
python3 scripts/validate-seeds.py
```

The travel and service-allowance conversions are **lossy approximations** of the official reference model. The nine brand/dock allowances collapse into two stop categories, and travel data is reshaped into directed pairs. See [seed provenance and approximations](database/seeds/README.md).

After an intentional source-dataset update, regenerate and reload reference data into a disposable local stack:

```bash
python3 scripts/convert-official-dataset.py
python3 scripts/validate-seeds.py
docker compose run --rm seed
```

This reload updates shared reference data and reloads the calendar/travel tables. It does not create a complete delivery scenario. Fresh seeds contain reference data and identity profiles; create orders and trips through the workflow.

### End-to-end review

1. **Store Manager:** sign in as `store-manager`, place an order, and note its delivery date. The 4:00 PM cutoff and operating calendar affect scheduling. Add another outlet's order with `store-manager-b` if needed.
2. **Dispatcher:** open **Order queue**, then **Plan and allocate** for that date. Generate the plan, inspect constraints and vehicle assignments, and resolve unallocated orders with a documented deferral where necessary.
3. **Dispatcher:** confirm/publish the resolved plan. Review the publication and field acknowledgement status.
4. **Loader:** open `/loader`, select the trip, record loading outcomes, and report any missing/damaged quantities. Resolve blocking issues before marking the load ready.
5. **Driver:** open `/driver/trips`. The local driver is scoped to `VEH001`, so use a trip assigned to that vehicle. Start the ready trip, record arrival, evidence, and an outcome.
6. **Offline check:** after loading the trip online, use browser DevTools to go offline. Record queued work, reconnect, and select **Sync Now**. Confirm server acknowledgement before considering it synchronized; do not clear browser storage.
7. **Store Manager:** confirm received quantities or report a discrepancy, then inspect the order evidence timeline.
8. **Dispatcher:** review delivery, loading, receipt, and audit information. Optionally demonstrate **My automations** using the separate [A3/A4 guide](docs/a3-a4-demo.md).

See [the demo script](docs/demo-script.md) and [offline-sync contract](docs/architecture/offline-sync.md) for additional scenarios. The A3/A4 fixture script creates explicitly synthetic history and should be used only in a disposable local environment.

## Screenshots

The gallery covers the main roles, exception handling, delivery evidence, and synchronization. Workflow images were copied unchanged from the existing local demo capture set; the sign-in image was captured from the running local application during this documentation update. They show demonstration data, not a new end-to-end acceptance run. Click an image to inspect it at full size.

### Store ordering and dispatcher planning

| Place an order | Review the order queue |
| --- | --- |
| ![Store Manager order form with quantities, weight, volume, and delivery date](docs/media/readme/order-entry.jpg) | ![Dispatcher order queue with order status and planning inputs](docs/media/readme/order-queue.jpg) |

| Planned trips | Publish and acknowledge a plan |
| --- | --- |
| ![Dispatcher planned trips and vehicle allocation](docs/media/readme/planned-trips.jpg) | ![Plan publication status and field acknowledgements](docs/media/readme/plan-published.jpg) |

### Loading and offline delivery

Full-height mobile captures are shown at a compact width; open the image for readable detail. A separate [driver route overview](docs/media/readme/driver-route.jpg) is also available.

<table>
  <tr><th>Loader workspace</th><th>Driver proof</th><th>Queued work</th><th>After synchronization</th></tr>
  <tr>
    <td><a href="docs/media/readme/loading-mobile.jpg"><img src="docs/media/readme/loading-mobile.jpg" alt="Loader trip and order load checks" width="180" /></a></td>
    <td><a href="docs/media/readme/driver-proof.jpg"><img src="docs/media/readme/driver-proof.jpg" alt="Driver delivery proof capture" width="180" /></a></td>
    <td><a href="docs/media/readme/driver-queued.jpg"><img src="docs/media/readme/driver-queued.jpg" alt="Driver locally queued operations" width="180" /></a></td>
    <td><a href="docs/media/readme/driver-synced.jpg"><img src="docs/media/readme/driver-synced.jpg" alt="Driver state after queued operations synchronize" width="180" /></a></td>
  </tr>
</table>

### Receipt and evidence

| Confirm receipt | Review the evidence timeline |
| --- | --- |
| ![Store receipt confirmation with discrepancy reporting](docs/media/readme/receipt.jpg) | ![Order timeline connecting placement, planning, delivery, and receipt](docs/media/readme/evidence-timeline.jpg) |

<details>
<summary><strong>More screens: sign-in, exceptions, and personal automations</strong></summary>

| Sign in | Order accepted |
| --- | --- |
| <img src="docs/media/readme/sign-in.jpg" alt="Role-based Waypoint sign-in" width="300" /> | ![Order request confirmation](docs/media/readme/order-confirmed.jpg) |

| Capacity constraint | Explain a deferral |
| --- | --- |
| ![Capacity constraint in the planning workspace](docs/media/readme/capacity-block.jpg) | ![Dispatcher deferral reason and next-run decision](docs/media/readme/deferral-reason.jpg) |

| Store deferral notice | Loading shortfall |
| --- | --- |
| ![Store-facing explanation of a deferred order](docs/media/readme/store-deferral.jpg) | <img src="docs/media/readme/loading-shortfall.jpg" alt="Loader missing or damaged quantity report" width="200" /> |

| Habit suggestion | Workflow preview | Execution history |
| --- | --- | --- |
| ![A3 evidence-based habit suggestion](docs/media/a3-a4/a3-habit-suggestion.png) | ![A4 workflow preview before activation](docs/media/a3-a4/a4-preview.png) | ![A4 completed run and notification](docs/media/a3-a4/a4-execution.png) |

The automation captures use the labelled synthetic history described in [the A3/A4 guide](docs/a3-a4-demo.md).

</details>

## Architecture

### Runnable local system

```mermaid
flowchart TB
    People["Store Manager · Dispatcher · Loader · Driver"] --> Web["React + TypeScript PWA"]
    Web <-->|"OIDC authorization code + PKCE"| Identity["Local ThunderID-compatible OIDC shim"]
    Web -->|"HTTP /api/v1 + bearer token"| Edge["NGINX"]
    Edge --> Domain["Order · Planning · Fleet · Loading · Delivery · Shared"]
    Edge --> Integration["Integration Service"]
    Edge --> Agent["Agent Orchestrator"]
    Domain -->|"owned schemas"| DB[("PostgreSQL 16")]
    Domain -->|"delivery proof bytes"| S3["S3-compatible storage / local S3Mock"]
    Domain -.-> Redis[("Redis 7 infrastructure")]
    Web --> Local[("IndexedDB / Dexie offline queue")]
    Agent -->|"allowlisted HTTP tools, caller token"| Domain
    Agent -.-> LLM["Optional LLM provider"]
    Integration -.-> SMS["Optional Twilio SMS"]
    Domain -.-> Telemetry["Optional OTel · Prometheus · Grafana"]
```

Service-to-service calls use HTTP rather than cross-schema SQL. JWT validation, application roles, resource scope, and business rules are enforced at the API boundary. The agent uses service APIs and has no database-driver imports; the current Compose shared environment nevertheless passes it `DATABASE_URL`, so deployment hardening should remove that unnecessary variable.

| Service | API prefix | Responsibility / owned data |
| --- | --- | --- |
| Order | `/api/v1/orders` | Orders, cutoff rules, receipts, discrepancies, custody; `orders` schema |
| Planning | `/api/v1/planning` | Deterministic allocation, trips, deferrals, publications, risks; `planning` schema |
| Fleet | `/api/v1/fleet` | Vehicles, availability, fuel, incidents; `fleet` schema |
| Loading | `/api/v1/loading` | Load sessions, outcomes, issues, readiness; `loading` schema |
| Delivery | `/api/v1/delivery` | Runs, stops, proofs, outcomes, messages, offline sync; `delivery` schema |
| Shared | `/api/v1/shared` | Profiles, outlets, calendar, policies, notifications, automations and audit; `shared` + `audit` schemas |
| Integration | `/api/v1/integrations` | External delivery adapters and callbacks |
| Agent Orchestrator | `/api/v1/agent` | Optional permission-scoped tools and human approvals |

### Operational flow

```mermaid
flowchart LR
    Order["Store places order"] --> Plan["Dispatcher generates plan"]
    Plan --> Check{"Constraints satisfied?"}
    Check -->|"Yes"| Publish["Confirm / publish"]
    Check -->|"No"| Defer["Record deferral and next action"]
    Publish --> Load["Loader records outcomes"]
    Load --> Ready{"Loading resolved?"}
    Ready -->|"No"| Resolve["Resolve loading issue"]
    Resolve --> Load
    Ready -->|"Yes"| Deliver["Driver executes trip"]
    Deliver --> Proof["Outcome + delivery evidence"]
    Proof --> Receipt["Store confirms receipt"]
    Receipt --> Audit["Operational visibility and audit"]
```

**[Complete architecture diagrams](docs/architecture/system-diagrams.md)** include the Kubernetes target, authentication and authorization sequence, service-owned data relationships, driver offline synchronization, optional AI approvals, and **both original diagrams from the submission PDF**.

## Data model

PostgreSQL uses seven owned schemas in this checkout: `shared`, `audit`, `orders`, `planning`, `fleet`, `loading`, and `delivery`. The main chain is:

**Outlet → order → plan allocation → trip → loading session → delivery run/stop → proof → receipt.**

Cross-service references are opaque IDs resolved through APIs. Within a schema, migrations define database relationships and constraints. Published versions, acknowledgements, stable operation IDs, proof metadata, and audit history preserve operational evidence.

- [Data relationships and schema inventory](docs/architecture/system-diagrams.md#service-owned-data-model)
- [Database ownership](database/README.md)
- [Authoritative SQL migrations](database/migrations/)
- [Official-data conversion notes](database/seeds/README.md)

Orders in this checkout use aggregate units, weight, volume, and temperature requirements. Product-line orders and the additional `inventory`/`ai` schemas shown in the submission reference are not present in these migrations.

## API reference

Contracts are in [`contracts/openapi/`](contracts/openapi/). Start the read-only Swagger UI separately:

```bash
docker compose -p waypoint-api-docs -f docker-compose.api-docs.yml up -d
```

Open **[http://localhost:8092](http://localhost:8092)**. Submission buttons are disabled. Business requests require an authorized bearer token and are subject to backend role/resource checks.

```bash
# Stop only the documentation viewer
docker compose -p waypoint-api-docs -f docker-compose.api-docs.yml down
```

See [API conventions](docs/api/README.md), [order import/export](docs/api/order-import-export-v1.md), and the separate [automations contract](contracts/openapi/automations.yaml). The Swagger selector lists eight service contracts; automations is available directly as a YAML file. Service handlers remain the implementation reference where contracts lag behavior.

## Development and validation

Install web dependencies before running the complete suite:

```bash
(cd apps/web && npm ci)
make verify
```

`make verify` runs uncached Go tests (including PostgreSQL/Testcontainers suites), Go build/vet, the agent database-import guard, web tests, the PWA production build, and Python backup/load utility tests. Docker is required for database-backed tests. See [`scripts/test.sh`](scripts/test.sh) for the exact sequence.

| Task | Command from repository root |
| --- | --- |
| Backend tests | `make test` |
| Backend build | `make build` |
| Go static checks | `make lint` |
| Agent import boundary | `make check-agent-imports` |
| Web tests | `cd apps/web && npm test` |
| Web production build | `cd apps/web && npm run build` |
| Seed validation | `python3 scripts/validate-seeds.py` |
| Observability checks | `make validate-observability` |
| Object-storage restore verification | `make verify-object-storage-restore` |

For local frontend iteration, Vite runs on port 3000, but its checked-in proxy targets the Docker hostname `http://nginx`. Host-based Vite development requires adjusting that proxy and the OIDC callback configuration together. The default Compose web build avoids those extra changes.

Historical acceptance evidence is recorded in [submission notes](docs/submission.md) and [A3/A4 verification](docs/a3-a4-demo.md#verification-recorded-on-3-october-2026). Those records are not a claim that every check was rerun for this documentation change.

## Operations and troubleshooting

```bash
# Inspect health and recent logs
docker compose ps -a
docker compose logs --tail=100 nginx shared-service order-service

# Rebuild an updated component
docker compose up -d --build web

# Apply newly added migrations to an existing local stack
docker compose run --rm migrate

# Stop the stack; retain the PostgreSQL named volume
docker compose down
```

**Data persistence:** PostgreSQL has a named volume. The local S3Mock service has no durable volume in this Compose file, so container recreation can lose proof objects. `docker compose down -v` also removes the database volume and is a destructive reset; use it only when intentionally discarding local data.

| Symptom | Check / resolution |
| --- | --- |
| Port already in use | Free or consistently remap port 80, 5432, 6379, 8090, or 9000. Changing the app origin also affects OIDC redirects. |
| Login fails using an email account | Use the short local usernames with `waypoint` for Compose. Team email credentials require their separately provisioned identity environment. |
| Login succeeds but profile/workspace is unavailable | Check `bootstrap` exited successfully and `shared-service` is healthy. |
| Orders or trips are missing | Reference seeds do not create operational scenarios. Match the order's delivery date, role scope, and assigned vehicle. |
| Driver cannot see a planned trip | Check the trip is assigned to `VEH001` for the demo driver and the loading snapshot is ready. |
| Assistant returns `agent_unavailable` | Configure `LLM_BASE_URL` and `LLM_MODEL` for the agent, then recreate it. Core operations remain available. |
| Offline work does not disappear | Check connectivity, sign-in, conflicts, and dependencies, then retry sync. Do not erase IndexedDB to clear the queue. |
| Scripts report `\r` or an illegal shell option | Restore LF line endings for shell/config files; preserve local changes. The repository supplies `.gitattributes`. |
| Schema changes are missing | Apply migrations and rebuild the relevant services. Review migration logs before reseeding. |
| Maps are blank | Map tiles need internet access. Outlet positions are approximations, not GPS fixes. |

Backup and retention procedures: [object storage](docs/operations/object-storage-backup.md), [proof retention](docs/operations/proof-retention.md), [`make backup-postgres`](scripts/create-postgres-backup.sh), and [`make backup-object-storage`](scripts/create-object-storage-backup.sh).

## Security and AI disclosure

Human sign-in uses OIDC Authorization Code with PKCE. APIs validate tokens, resolve the subject to a shared profile, enforce permissions and ownership, then validate the domain operation. Service identities are separate from human identities. Demo identity credentials and HTTP transport are for local development only.

The optional assistant uses fixed, allowlisted tools. Sensitive proposals require approval by the same human actor, expire after five minutes, and are reauthorized by the target service. Planning feasibility remains deterministic. Approval state is currently bounded process memory and does not survive an orchestrator restart.

A3 habit suggestions use deterministic evidence thresholds and saved feedback. A4 drafts a supported weekly workflow, previews it, and activates only after user confirmation. Saved workflows execute in Shared Service with current authorization checks; they do not call the LLM on each scheduled run.

The team-provided disclosure identifies **OpenAI Codex and AI coding assistance** for repository analysis, implementation drafts, testing, review, and documentation. The team retains responsibility for product decisions, accepted changes, testing, and release approval. See [AI disclosure](docs/ai-disclosure.md), [security](docs/security.md), [privacy inventory](docs/privacy-data-inventory.md), and the [original submission document](docs/reference/waypoint-architecture-data-model-ai-disclosure.pdf).

## Repository layout

```text
apps/
  web/                     React + TypeScript PWA and all four web workspaces
  driver-mobile/           Optional Flutter driver project
services/
  order-service/           Orders and receipts
  planning-service/        Allocation, constraints, plans, and deferrals
  fleet-service/           Vehicle operations
  loading-service/         Loading and readiness
  delivery-service/        Routes, outcomes, proof, and offline sync
  shared-service/          Profiles, reference data, audit, and automations
  integration-service/     External notification adapters
  agent-orchestrator/      Guarded optional AI tools
pkg/                       Shared Go libraries
contracts/openapi/         Versioned API specifications
database/                  SQL migrations and converted reference seeds
infrastructure/            NGINX, Kubernetes, storage, and observability
scripts/                   Bootstrap, datasets, validation, backup, and demos
tools/dev-oidc/            Local OIDC-compatible identity service
datathon/                  Separate competition analysis/solver work
docs/                      Runbooks, architecture, screenshots, and references
```

## Limitations and deployment notes

- **Planner:** deterministic greedy allocation with hard constraints; no optimality guarantee. Official travel/service-time inputs are simplified by the current converter.
- **Deployment:** Compose is the documented local path. Kubernetes manifests are target deployment inputs and need real images, identity configuration, secrets, durable storage, DNS, and TLS. A running public deployment was not verified for this README.
- **Clients:** all four supported web roles are in React. The Flutter driver project is optional; there is no `apps/loader-web` directory in this checkout.
- **Reference differences:** Python/FastAPI/LangGraph assistants, an agent trace manager, inventory/AI schemas, and product-line records appear in the supplied PDF but are absent from this checkout. See the [comparison](docs/architecture/system-diagrams.md#submission-reference-and-checkout-differences).
- **Storage:** local S3Mock has no persistent volume. A production storage/backup setup is required for durable delivery evidence.
- **AI and integrations:** provider availability, SMS delivery, and production notification callbacks require external configuration. Do not treat their presence in code as evidence of a tested live integration.
- **Datathon:** separate solver/data work is documented in [`datathon/README.md`](datathon/README.md); it is not automatically a deployed forecasting pipeline.

## Contributing and pull requests

Keep changes focused on the affected domain. Maintain service ownership, update API contracts when changing requests/responses, and add migrations for persistent schema changes. Avoid cross-schema reads and client-side-only authorization.

A reviewable PR should state the problem, resulting behavior, affected roles, validation performed, migration/configuration impact, and known limitations. Include screenshots for visible changes and distinguish actual check results from suggested commands. See the [pull request template](.github/pull_request_template.md).

No repository-level license file is currently included. The team should choose an explicit license before representing the project as licensed open source.

## Documentation

| Topic | Reference |
| --- | --- |
| Architecture, sequences, ER relationships, and source diagrams | [Complete architecture diagrams](docs/architecture/system-diagrams.md) |
| Original team submission | [Architecture, data model, and AI tool disclosure PDF](docs/reference/waypoint-architecture-data-model-ai-disclosure.pdf) |
| Service boundaries | [Architecture A](docs/architecture/architecture-a.md) · [ADR-001](docs/decisions/ADR-001-service-boundaries.md) |
| Audit delivery | [ADR-002](docs/decisions/ADR-002-audit-delivery.md) |
| Offline delivery | [Offline behavior](docs/offline-sync.md) · [Sync contract](docs/architecture/offline-sync.md) |
| Guarded assistant | [Agent plane](docs/architecture/agent-plane.md) |
| Personal automations | [A3/A4 demo and scope](docs/a3-a4-demo.md) |
| Security and privacy | [Security](docs/security.md) · [Data inventory](docs/privacy-data-inventory.md) |
| API contracts | [API reference](docs/api/README.md) |
| Deployment | [Kubernetes guide](infrastructure/kubernetes/README.md) |
| Validation history | [Submission evidence](docs/submission.md) |

Maintained by **Team ClapTac** for **Tech-Triathlon 2026**.
