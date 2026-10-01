# ADR-001: Service boundaries

## Status

Accepted

## Context

Architecture A deploys independently deployable Go services that share one PostgreSQL instance for Milestone 1.

## Decision

1. Services communicate through versioned HTTP APIs only.
2. Each service owns one or more PostgreSQL schemas and must not query another schema.
3. Shared libraries live in `pkg/`. Shared *product capabilities* live in `shared-service`.
4. The agent orchestrator calls business services through tools and a `CredentialProvider`. It must not import database drivers or connect to PostgreSQL.
5. Authorization is enforced in every backend service, using ThunderID identity plus application profile/assignment lookup.

Path:

```
Agent → Tool → Business Service → Authorization → Validation → Database
```

Forbidden:

```
Agent → Database
delivery-service → SELECT * FROM orders.orders
```

## Consequences

Schema ownership is documented in `database/README.md` and enforced later by DB roles. NetworkPolicies encode the same graph in Kubernetes.
