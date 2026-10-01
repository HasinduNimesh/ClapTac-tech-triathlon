# Security

## Identity (ThunderID)

OIDC Authorization Code + PKCE for humans. Client credentials for selected machine clients.

JWT access tokens contain:

- `sub`
- `roles`
- `scopes`

Outlet membership and driver vehicle assignment are **application data**, resolved after authentication.

```
JWT { sub, roles, scopes }
        ↓
Profile / assignment lookup
        ↓
Resource authorization
```

Frontend role checks are UX only. JWT proves `sub` only for RBAC; `shared.users.role` is authoritative.

Machine clients use client credentials. Planning is scoped to `orders:read-internal fleet:read-internal outlets:read-internal deliveries:read-internal audit:write policy:read-internal`. Fleet uses `audit:write` to drain its transactional master-data audit outbox. Loading is scoped to `plans:read-internal orders:read-internal audit:write`. Delivery is scoped to `loading:read-internal outlets:read-internal audit:write`. Secrets live on Kubernetes Secret `waypoint-m2m` with per-service keys (`ORDER_M2M_CLIENT_SECRET`, `PLANNING_M2M_CLIENT_SECRET`, `FLEET_M2M_CLIENT_SECRET`, `LOADING_M2M_CLIENT_SECRET`, `DELIVERY_M2M_CLIENT_SECRET`), not a ConfigMap.

## Authorization layers

1. Authentication
2. Permission / RBAC
3. Resource-level authorization (outlet, assigned trip)
4. Business-rule validation

## AUTH_DISABLED

Allowed only when `ENVIRONMENT=local`. Staging and production fail process start if `AUTH_DISABLED=true`.
