# Kubernetes deployment inputs

The Kustomize base intentionally does not create credential Secrets. Provision these
Secrets out of band (for example, with the cluster's external-secrets integration)
before applying an overlay:

- `waypoint-database`: `ORDER_DATABASE_URL`, `PLANNING_DATABASE_URL`,
  `FLEET_DATABASE_URL`, `LOADING_DATABASE_URL`, `DELIVERY_DATABASE_URL`, and
  `SHARED_DATABASE_URL`. Use database credentials scoped to the service's owned
  schema where the database supports it.
- `waypoint-m2m`: `ORDER_M2M_CLIENT_SECRET`, `PLANNING_M2M_CLIENT_SECRET`, `FLEET_M2M_CLIENT_SECRET`,
  `LOADING_M2M_CLIENT_SECRET`, and `DELIVERY_M2M_CLIENT_SECRET`. Each key must
  contain a distinct credential registered for that client in ThunderID.

The agent orchestrator receives neither a database URL nor an M2M client secret.
Do not commit generated Secret manifests or production credentials. The default
Compose stack uses separate development-only M2M values with the local OIDC shim;
those values are not valid production credentials.
