# Identity (ThunderID contract)

Waypoint authenticates humans with OIDC Authorization Code + PKCE and machines with OAuth2 client credentials.

Local Compose runs `tools/dev-oidc` as service `thunderid` on HTTP `http://localhost:8090`. It implements the ThunderID-facing contract (discovery, JWKS, authorize, token) so `docker compose up` works without pulling the official image. Swap the image for `ghcr.io/thunder-id/thunderid` when that registry is available; keep issuer/JWKS/token URLs.

## Demo users (password: `waypoint`)

| Username | Application role (DB) | Outlet |
|---|---|---|
| store-manager | STORE_MANAGER | OUT034 |
| dispatcher | DISPATCHER | — |
| store-manager-b | STORE_MANAGER | OUT021 |

JWT `sub` values are published at `/admin/users` and copied into `shared.users` by `scripts/thunder-bootstrap.sh`. Do not hand-copy subjects into seed SQL. `shared.users.role` is the RBAC source of truth.

M2M client `waypoint-order-service` uses client credentials with scopes `plans:read-internal deliveries:read-internal audit:write` for narrow order tracking aggregation and receipt audit events.
