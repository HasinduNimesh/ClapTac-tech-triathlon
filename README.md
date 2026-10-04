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

The platform combines a React PWA, a Flutter loader workspace and driver app, Go domain services, Python assistants, deterministic planning rules, and role- and resource-scoped APIs. Core operations work without an LLM provider.

> **Scope:** this README describes the `main` branch and its local Docker Compose setup. The team’s existing [architecture and AI disclosure](docs/Waypoint%20Architecture%20Data%20Model%20and%20AI%20Tool%20Disclosure%20-%20Team%20ClapTac.pdf) provides the submission reference. Production readiness and live integrations require deployment-specific validation.

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

English, Sinhala, and Tamil are available in the React interface; the Flutter loader interface is English-only. Maps use recorded outlet locations, falling back to approximate district positions until an exact location is recorded. Truck status comes from operational events, not live GPS. See [outlet locations](docs/outlet-locations.md).

## Quick start

### 1. Prerequisites

| Requirement | When needed |
| --- | --- |
| Git | Clone the repository. |
| Docker Engine/Desktop and Docker Compose v2 | Run the complete local application. Docker must be running. |
| Go 1.22+ | Build, test, or develop backend services outside Docker. |
| Node.js 20+ and npm | Build or test the React application outside Docker. |
| Python 3 / Python 3.12 for assistants | Validate and convert seeds; run utility tests. Assistant development uses Python 3.12, FastAPI, and LangGraph. |
| Flutter | Develop the loader web or driver mobile app outside Docker. The loader Docker build supplies its pinned Flutter SDK. |

On Windows, use Docker Desktop with WSL 2 and run the shell commands from WSL. Docker builds supply the application toolchains, including Python for assistants and Flutter for the loader web app.

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

Open **[http://localhost](http://localhost)**, choose **Sign in**, and use a [local account](#local-docker-compose-accounts). The React account profile selects its workspace automatically; sign out before switching roles. For the Flutter Loader workspace, open **[http://localhost/loader-app/](http://localhost/loader-app/)** directly and sign in there.

```bash
curl -fsS http://localhost/health/live
```

The edge health check confirms NGINX is responding. Use `docker compose ps` to inspect individual service health as well.

### Local endpoints

| Component | Address | Availability |
| --- | --- | --- |
| Waypoint web/PWA | [localhost](http://localhost) | Default stack |
| Flutter Loader | [localhost/loader-app/](http://localhost/loader-app/) | Default stack |
| Agent trace viewer | [localhost:8085](http://localhost:8085) | Optional `agent-traces` profile |
| Development OIDC provider | [localhost:8090](http://localhost:8090/.well-known/openid-configuration) | Default stack; sign in through the app |
| PostgreSQL | `127.0.0.1:5432` | Default stack |
| Redis | `127.0.0.1:6379` | Default stack |
| S3-compatible development storage | `http://localhost:9000` | Default stack; S3Mock, not a MinIO console |
| Swagger UI | [localhost:8092](http://localhost:8092) | Separate [API viewer](#api-reference) |
| Grafana / Prometheus | [localhost:3001](http://localhost:3001) / [localhost:9090](http://localhost:9090) | `observability` profile |
| OTLP HTTP collector | `http://localhost:4318` | `observability` profile |

Compose binds published ports to loopback by default; `NGINX_BIND` can override the edge binding. The optional `backup-test` profile uses port 9001; the `search` profile uses port 9200.

## Configuration

Start with [`.env.example`](.env.example). **[`docker-compose.yml`](docker-compose.yml) determines which values reach each container**; listing a variable in `.env` alone does not override a hardcoded Compose setting.

| Area | Variables / defaults | Configuration notes |
| --- | --- | --- |
| Runtime | `ENVIRONMENT=local`, `AUTH_DISABLED=false` | Compose explicitly keeps authentication enabled. Disabled authentication is rejected outside the local runtime. |
| Database | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` = `waypoint`; `DATABASE_URL` | Keep the credentials in the connection URL consistent with PostgreSQL. Migration and seed jobs consume `DATABASE_URL`; the bootstrap job still hardcodes the local connection URL and needs a matching override if these defaults change. |
| Identity | `OIDC_ISSUER=http://localhost:8090`, `OIDC_AUDIENCE=waypoint-api` | Services consume these values, but the bundled identity container uses hardcoded local issuer/audience values. Change both sides together. |
| Internal identity endpoints | JWKS: `http://thunderid:8090/oauth2/jwks`; token: `http://thunderid:8090/oauth2/token` | Override with `OIDC_JWKS_URL` and `OIDC_TOKEN_URL`. Internal service names resolve inside the Compose network. |
| Service authentication | `ORDER_M2M_CLIENT_SECRET`, `PLANNING_M2M_CLIENT_SECRET`, `FLEET_M2M_CLIENT_SECRET`, `LOADING_M2M_CLIENT_SECRET`, `DELIVERY_M2M_CLIENT_SECRET`, `SHARED_M2M_CLIENT_SECRET`, `INTEGRATION_M2M_CLIENT_SECRET` | Local defaults are provided. `FLEET_M2M_CLIENT_SECRET` is supported by Compose but is not listed in `.env.example`. Use distinct credentials with a real provider. |
| Web build | `VITE_API_BASE_URL`, `VITE_OIDC_ISSUER`, `VITE_OIDC_CLIENT_ID`, `VITE_OIDC_REDIRECT_URI`, `VITE_OIDC_AUDIENCE` | Compose reads these build arguments from the environment, with local defaults. Set them consistently with the identity provider and rebuild `web`; runtime changes do not rewrite compiled frontend assets. |
| Loader identity | `LOADER_OIDC_CLIENT_ID=waypoint-loader`; shared `VITE_OIDC_ISSUER` and `VITE_OIDC_AUDIENCE` | Rebuild `loader-web` after changes. Register `<origin>/loader-app/auth/callback` for its public PKCE client. |
| Image and edge settings | `WAYPOINT_IMAGE_PREFIX=waypoint`, `WAYPOINT_TAG=local`, `NGINX_BIND=127.0.0.1:80:80` | Image naming and edge binding for Compose. |
| Agent traces | `AGENT_TRACE_URL`, `AGENT_TRACE_INGEST_TOKEN`, `AGENT_TRACE_VIEW_TOKEN`, `AGENT_TRACE_MAX` | Optional `agent-traces` profile; set the URL to `http://agent-manager:8085`. Non-local environments require tokens. |
| Grafana login | `GRAFANA_ADMIN_USER=admin`, `GRAFANA_ADMIN_PASSWORD=admin` | Defaults are local-only; configure credentials for other environments. |
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
| `loader` | `waypoint` | Loader | `DEPOT_NORTH` | `/loader-app/` |
| `driver` | `waypoint` | Driver | Vehicle `VEH001` | `/driver/trips` |
| `store-manager-b` | `waypoint` | Store Manager | Outlet `OUT021`; isolation testing | `/store-manager` |
| `loader-kandy` | `waypoint` | Loader | `DEPOT_SOUTH`; isolation testing | `/loader-app/` |

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

This reload updates shared reference data and reloads the calendar/travel tables. It does not create a complete delivery scenario. Seeds also load the product catalog and eight weeks of labelled simulated store trading, with derived AI observations/insights. This history is not evidence of real operations. Create delivery orders, plans, and trips through the product workflow; see [catalog and history seeds](database/README.md).

### End-to-end review

1. **Store Manager:** sign in as `store-manager`, place an order, and note its delivery date. The 4:00 PM cutoff and operating calendar affect scheduling. Add another outlet's order with `store-manager-b` if needed.
2. **Dispatcher:** open **Order queue**, then **Plan and allocate** for that date. Generate the plan, inspect constraints and vehicle assignments, and resolve unallocated orders with a documented deferral where necessary.
3. **Dispatcher:** confirm/publish the resolved plan. Review the publication and field acknowledgement status.
4. **Loader:** open `/loader-app/`, select the trip, record loading outcomes, and report any missing/damaged quantities. Resolve blocking issues before marking the load ready.
5. **Driver:** open `/driver/trips`. The local driver is scoped to `VEH001`, so use a trip assigned to that vehicle. Start the ready trip, record arrival, evidence, and an outcome.
6. **Offline check:** after loading the trip online, use browser DevTools to go offline. Record queued work, reconnect, and select **Sync Now**. Confirm server acknowledgement before considering it synchronized; do not clear browser storage.
7. **Store Manager:** confirm received quantities or report a discrepancy, then inspect the order evidence timeline.
8. **Dispatcher:** review delivery, loading, receipt, and audit information. Optionally demonstrate **My automations** using the separate [A3/A4 guide](docs/a3-a4-demo.md).

See [the demo script](docs/demo-script.md) and [offline-sync contract](docs/architecture/offline-sync.md) for additional scenarios. The A3/A4 fixture script creates explicitly synthetic history and should be used only in a disposable local environment.

## Screenshots

These screenshots are already tracked in the repository. They demonstrate order assistance, dashboard creation, and personal automations using development data. They are illustrations of the recorded demo flows, not evidence that a fresh acceptance run was performed for this README update.

### Order capture and clarification

| Describe an order | Review resolved product lines |
| --- | --- |
| ![Store Manager order assistant asks for order details](docs/media/a1-a2/a1-order-helper-question.png) | ![Order assistant presents resolved product lines](docs/media/a1-a2/a1-order-helper-lines.png) |

| Clarify a product size | Review the prefilled order |
| --- | --- |
| ![Assistant asks which product size the user intended](docs/media/a1-a2/a1-asks-which-size.png) | ![Order form populated for human review](docs/media/a1-a2/a1-order-form-filled.png) |

### Dashboard creation

| Create a dashboard | Saved dashboard |
| --- | --- |
| ![Store Manager dashboard builder](docs/media/a1-a2/a2-create-dashboard.png) | ![Saved Store Manager dashboard](docs/media/a1-a2/a2-saved-dashboard.png) |

### Habits and personal workflows

| Habit suggestion | Workflow preview | Execution history |
| --- | --- | --- |
| ![A3 evidence-based habit suggestion](docs/media/a3-a4/a3-habit-suggestion.png) | ![A4 workflow preview before activation](docs/media/a3-a4/a4-preview.png) | ![A4 completed run and notification](docs/media/a3-a4/a4-execution.png) |

The automation captures use the labelled synthetic history described in [the A3/A4 guide](docs/a3-a4-demo.md). For the field workflows, see the [Loader screen guide](apps/loader-web/README.md#screens) and [Driver application guide](apps/driver-mobile/README.md).

## Architecture

### Runnable local system

```mermaid
flowchart TB
    People["Store Manager · Dispatcher · Driver"] --> Web["React + TypeScript PWA"]
    Loader["Loader / Flutter web"] --> Edge
    Mobile["Driver / Flutter mobile"] --> Edge
    Web <-->|"OIDC authorization code + PKCE"| Identity["Local ThunderID-compatible OIDC shim"]
    Web -->|"HTTP /api/v1 + bearer token"| Edge["NGINX"]
    Edge --> Domain["Order · Planning · Fleet · Loading · Delivery · Shared"]
    Edge --> Integration["Integration Service"]
    Edge --> Agent["Agent Orchestrator"]
    Edge --> Assistants["Python / FastAPI / LangGraph assistants"]
    Assistants -->|"caller-scoped HTTP tools"| Domain
    Assistants -.-> LLM
    Agent -.-> Traces["Optional Agent Manager / trace viewer"]
    Assistants -.-> Traces
    Domain -->|"owned schemas"| DB[("PostgreSQL 16")]
    Domain -->|"proof and loading-issue media"| S3["S3-compatible storage / local S3Mock"]
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
| Agent Assistants | `/api/v1/agent/order-assistant/`, `/api/v1/agent/dashboard-assistant/` | Python order/dashboard drafting helpers; caller-scoped APIs, no database connection |
| Agent Manager | Optional port `8085` | Bounded agent trace collection/viewing; enabled with `agent-traces` |

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

### Submission architecture diagram

![Waypoint applications, services, identity, and infrastructure](docs/Waypoint%20-%20Architecture%20diagram.png)

### Deployment target

```mermaid
flowchart LR
    Browser["Browser / mobile client"] --> Edge["Public TLS edge / NGINX"]
    Edge --> Gateway["kGateway / Gateway API"]
    Gateway --> API["Versioned services"]
    API --> Database[("PostgreSQL / owned schemas")]
    API --> Storage["Durable object storage"]
    API --> IdP["Production OIDC provider"]
```

kGateway is a Kubernetes target component and does not run in local Compose. The manifests require deployment-specific identity, secrets, TLS, image, and storage configuration. See [Architecture A](docs/architecture/architecture-a.md) and [Kubernetes deployment inputs](infrastructure/kubernetes/README.md).

### Driver offline synchronization

```mermaid
sequenceDiagram
    actor Driver
    participant Client as Driver client
    participant Queue as Local persistent queue
    participant API as Delivery Service
    participant Objects as Object storage
    Driver->>Client: Record arrival, proof, and outcome offline
    Client->>Queue: Persist work with stable operation IDs
    Note over Client,API: Connectivity returns
    Queue-->>Client: Next pending operation
    Client->>API: ARRIVED via JSON sync
    API-->>Client: Operation result
    Client->>API: Proof via multipart upload
    API->>Objects: Store evidence bytes
    API-->>Client: Proof result
    Client->>API: STOP_OUTCOME after proof dependency
    API-->>Client: Applied, duplicate, conflict, or rejected
    Client->>Queue: Update status after acknowledgement
```

The React PWA uses IndexedDB/Dexie; the configured Flutter driver app uses SQLite for queued operations. Trip start and plan acknowledgement require a connection. The Flutter loader is online-only and does not queue offline work. See the [sync contract](docs/architecture/offline-sync.md) and [mobile implementation](apps/driver-mobile/README.md).

### Guarded assistant execution

```mermaid
flowchart LR
    User["Authenticated user"] --> Agent["Assistant / orchestrator"]
    Agent --> Tools["Allowlisted tools and argument validation"]
    Tools --> Draft["Draft / sensitive proposal"]
    Draft --> Approval["Human review and approval"]
    Approval --> API["Business API"]
    API --> Checks["Current role, resource scope, and domain rules"]
    Agent -.-> Provider["Optional LLM provider"]
```

A1/A2 return order/dashboard drafts for review. The Go orchestrator gates its sensitive tools through owner-bound approval. Agents do not determine planning feasibility; the business services enforce deterministic rules.

## Data model

Waypoint uses one PostgreSQL instance with nine schemas. Seven serve the operational platform; `inventory` and `ai` provide schema foundations and simulated history while their dedicated services remain planned.

| Schema | Principal records |
| --- | --- |
| `shared` | Users/profiles, outlets/locations, calendar, product catalog/prices, policies, notifications, dashboards, and A3/A4 automations. |
| `audit` | Operational audit events. |
| `orders` | Orders and product lines, receipts and receipt lines, issues, and custody events. |
| `planning` | Plans, trips, allocations, deferrals, publications, acknowledgements, and disruption risks. |
| `fleet` | Vehicles, availability, fuel, and incidents. |
| `loading` | Sessions, load outcomes, issues, dispatcher decisions, and versioned snapshots. |
| `delivery` | Runs, stops, outcomes, proof, temperatures, messages, incidents, and sync operations. |
| `inventory` | Simulated runs, stock levels/batches, sales, stock counts, and append-only movements. |
| `ai` | Agent definitions/preferences, observations, insights, suggestions/responses, and runs. |

The main operational chain is **Outlet → order → allocation → trip → load → delivery stop → proof → receipt**. Cross-service references are opaque IDs resolved through APIs. Within each owned schema, migrations define relationships and constraints. Product packs move through logistics; stock inventory uses individual sellable units, converted with `units_per_pack`.

![Waypoint data model and core entity relationships](docs/Waypoint%20-%20Data%20model.png)

- [Database ownership, catalog, inventory, and AI records](database/README.md)
- [Authoritative SQL migrations](database/migrations/)
- [Official-data conversion notes](database/seeds/README.md)
- [Architecture, data model, and AI disclosure PDF](docs/Waypoint%20Architecture%20Data%20Model%20and%20AI%20Tool%20Disclosure%20-%20Team%20ClapTac.pdf)

`inventory.stock_movements` and `ai.observations` have append-only protections. The documented simulation cleanup function is restricted to simulated runs. Schema presence alone does not imply that a production inventory or insights service is deployed.

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

See [API conventions](docs/api/README.md), [order import/export](docs/api/order-import-export-v1.md), the [automations contract](contracts/openapi/automations.yaml), and the [assistant contract](contracts/openapi/assistants.yaml). The Swagger selector lists nine service contracts; automations is available directly as a YAML file. Service handlers remain the implementation reference where contracts lag behavior.

## Development and validation

Install web dependencies before running the complete suite:

```bash
(cd apps/web && npm ci)
make verify
```

`make verify` runs uncached Go tests (including PostgreSQL/Testcontainers suites), Go build/vet, the agent database-import guard, Python assistant tests, web tests, the PWA production build, and Python backup/load/location utility tests. Docker is required for database-backed tests. See [`scripts/test.sh`](scripts/test.sh) for the exact sequence.

| Task | Command from repository root |
| --- | --- |
| Backend tests | `make test` |
| Backend build | `make build` |
| Go static checks | `make lint` |
| Agent import boundary | `make check-agent-imports` |
| Python assistant tests | `./scripts/test-agent-assistants.sh` |
| Loader checks | `cd apps/loader-web && flutter pub get && flutter analyze && flutter test` |
| Driver checks | `cd apps/driver-mobile && flutter pub get && flutter analyze && flutter test` |
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
| Maps are blank | Map tiles need internet access. Outlets use recorded locations or a district fallback; truck GPS is not tracked. |

Backup and retention procedures: [object storage](docs/operations/object-storage-backup.md), [proof retention](docs/operations/proof-retention.md), [`make backup-postgres`](scripts/create-postgres-backup.sh), and [`make backup-object-storage`](scripts/create-object-storage-backup.sh).

## Security and AI disclosure

Human sign-in uses OIDC Authorization Code with PKCE. APIs validate tokens, resolve the subject to a shared profile, enforce permissions and ownership, then validate the domain operation. Service identities are separate from human identities. Demo identity credentials and HTTP transport are for local development only.

The optional Go orchestrator uses fixed, allowlisted tools. Sensitive proposals require approval by the same human actor, expire after five minutes, and are reauthorized by the target service. Planning feasibility remains deterministic. Approval state is currently bounded process memory and does not survive an orchestrator restart.

A3 habit suggestions use deterministic evidence thresholds and saved feedback. A4 drafts a supported weekly workflow, previews it, and activates only after user confirmation. Saved workflows execute in Shared Service with current authorization checks; they do not call the LLM on each scheduled run.

The team-provided disclosure identifies **OpenAI Codex and AI coding assistance** for repository analysis, implementation drafts, testing, review, and documentation. The team retains responsibility for product decisions, accepted changes, testing, and release approval. See [AI disclosure](docs/ai-disclosure.md), [security](docs/security.md), [privacy inventory](docs/privacy-data-inventory.md), and the [original submission document](docs/Waypoint%20Architecture%20Data%20Model%20and%20AI%20Tool%20Disclosure%20-%20Team%20ClapTac.pdf).

## Repository layout

```text
apps/
  web/                     React + TypeScript PWA and role-based web workspaces
  loader-web/              Flutter loader web workspace
  driver-mobile/           Flutter driver mobile application
services/
  order-service/           Orders and receipts
  planning-service/        Allocation, constraints, plans, and deferrals
  fleet-service/           Vehicle operations
  loading-service/         Loading and readiness
  delivery-service/        Routes, outcomes, proof, and offline sync
  shared-service/          Profiles, reference data, audit, and automations
  integration-service/     External notification adapters
  agent-orchestrator/      Guarded optional AI tools
  agent-assistants/        Python / FastAPI / LangGraph order and dashboard helpers
pkg/                       Shared Go libraries
contracts/openapi/         Versioned API specifications
database/                  SQL migrations and converted reference seeds
infrastructure/            NGINX, Kubernetes, storage, observability, agent manager
scripts/                   Bootstrap, datasets, validation, backup, and demos
tools/dev-oidc/            Local OIDC-compatible identity service
datathon/                  Separate competition analysis/solver work
docs/                      Runbooks, architecture, screenshots, and references
```

## Limitations and deployment notes

- **Planner:** deterministic greedy allocation with hard constraints; no optimality guarantee. Official travel/service-time inputs are simplified by the current converter.
- **Deployment:** Compose is the documented local path. Kubernetes manifests are target deployment inputs and need real images, identity configuration, secrets, durable storage, DNS, and TLS. A running public deployment was not verified for this README.
- **Clients:** the React PWA, Flutter loader web app, and Flutter driver mobile app have distinct runtime behavior. Loader web requires connectivity; driver queues support offline work. Consult each client guide for its remaining integration and device-testing limitations.
- **Inventory/AI foundations:** database schemas and simulated history exist, while dedicated inventory/insights services remain planned. Do not present seeded analytics as live customer evidence.
- **Storage:** local S3Mock has no persistent volume. A production storage/backup setup is required for durable delivery evidence.
- **AI and integrations:** provider availability, SMS delivery, and production notification callbacks require external configuration. Do not treat their presence in code as evidence of a tested live integration.
- **Datathon:** separate solver/data work is documented in [`datathon/README.md`](datathon/README.md); it is not automatically a deployed forecasting pipeline.

## Contributing and pull requests

Keep changes focused on the affected domain. Maintain service ownership, update API contracts when changing requests/responses, and add migrations for persistent schema changes. Avoid cross-schema reads and client-side-only authorization.

A reviewable PR should state the problem, resulting behavior, affected roles, validation performed, migration/configuration impact, and known limitations. Include screenshots for visible changes and distinguish actual check results from suggested commands.

No repository-level license file is currently included. The team should choose an explicit license before representing the project as licensed open source.

## Documentation

| Topic | Reference |
| --- | --- |
| Architecture and service boundaries | [Architecture overview](docs/architecture.md) · [Architecture A](docs/architecture/architecture-a.md) |
| Original team submission | [Architecture, data model, and AI tool disclosure PDF](docs/Waypoint%20Architecture%20Data%20Model%20and%20AI%20Tool%20Disclosure%20-%20Team%20ClapTac.pdf) |
| Service boundaries | [Architecture A](docs/architecture/architecture-a.md) · [ADR-001](docs/decisions/ADR-001-service-boundaries.md) |
| Audit delivery | [ADR-002](docs/decisions/ADR-002-audit-delivery.md) |
| Offline delivery | [Offline behavior](docs/offline-sync.md) · [Sync contract](docs/architecture/offline-sync.md) |
| Guarded assistants and agent traces | [Agent plane](docs/architecture/agent-plane.md) · [Python assistants](services/agent-assistants/README.md) |
| Loader workspace | [Flutter loader guide](apps/loader-web/README.md) |
| Driver app | [Flutter driver guide](apps/driver-mobile/README.md) |
| Personal automations | [A3/A4 demo and scope](docs/a3-a4-demo.md) |
| Security and privacy | [Security](docs/security.md) · [Data inventory](docs/privacy-data-inventory.md) |
| API contracts | [API reference](docs/api/README.md) |
| Deployment | [Kubernetes guide](infrastructure/kubernetes/README.md) |
| Validation history | [Submission evidence](docs/submission.md) |

Maintained by **Team ClapTac** for **Tech-Triathlon 2026**.
