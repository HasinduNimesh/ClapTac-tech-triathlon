# Architecture

Waypoint is a four-role delivery workflow built as a React PWA and independently deployable Go services. The authoritative version of the architecture and the service prefix table live in [Architecture A](architecture/architecture-a.md).

## Request path

- Local development: browser or driver PWA → NGINX → versioned Go APIs.
- Kubernetes target: public TLS edge/NGINX → kGateway → versioned Go APIs. kGateway is not part of Compose.
- Business APIs authenticate the ThunderID JWT, resolve `sub` through `shared.users`, enforce role permission, resolve resource ownership, and then apply domain rules.
- Services own logical PostgreSQL schemas and call peers over HTTP. The agent orchestrator only calls allowlisted service tools; it has no database URL.

## Workflow

Store Manager order → Dispatcher plan/allocation/deferral → Loader load/shortfall/ready → Driver route/proof/offline sync → Store Manager receipt → Dispatcher event-based status. Planning feasibility is deterministic; AI assistance is optional.

## Deployment reality

Compose and the local development identity shim are implemented and exercised. Kubernetes resources are deployment inputs, not a currently running public deployment. This checkout has no provisioned DNS name, TLS certificate, cluster, durable production database/object storage, or public URL. Production release remains blocked on those operator-owned resources and a deployment run; no public URL is claimed.
