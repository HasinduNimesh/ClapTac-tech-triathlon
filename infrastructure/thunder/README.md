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

## Driver app (native client)

The Flutter driver app signs in with Authorization Code + PKCE through the system browser. It is a public client: there is no client secret, and the app never handles the username or password.

| Setting | Value |
|---|---|
| Client ID | `waypoint-driver` (`MOBILE_OIDC_CLIENT_ID`) |
| Redirect URI | `dev.claptac.waypointdriver:/oauth2redirect` (`MOBILE_OIDC_REDIRECT_URI`; must match `appAuthRedirectScheme` in `apps/driver-mobile/android/app/build.gradle.kts`) |
| Scopes | `openid profile`; add `offline_access` (build option `OIDC_SCOPES`) once the client is allowed refresh tokens, so a driver is not sent back to the browser every time the access token expires |
| ID token | `aud` must be the client ID and the request `nonce` must be echoed (AppAuth validates both). The access token keeps `aud = waypoint-api`. |

After sign-in the app calls `GET /api/v1/shared/profiles/me` with the access token and only continues when the returned roles include `DRIVER`. A 404 means the person exists in ThunderID but is not provisioned in Waypoint.

The local `tools/dev-oidc` server accepts any client and redirect URI and has a `driver` user (password `waypoint`, vehicle `VEH001`). A real ThunderID tenant must register the client and redirect URI explicitly, and must not allow plain HTTP.

The local `tools/dev-oidc` issues a refresh token when `offline_access` is requested and rotates it on every use. `ACCESS_TOKEN_TTL_SECONDS=90` makes people's access tokens short-lived so a refresh can be watched; service-to-service tokens stay at one hour.

To try it against Compose on an Android phone over USB (debug build, plain HTTP to localhost):

```bash
adb reverse tcp:8090 tcp:8090     # identity server, so the phone's localhost:8090 is this machine's
adb reverse tcp:18081 tcp:18081   # shared-service, published by a compose override
flutter run --dart-define=OIDC_ISSUER=http://localhost:8090 --dart-define=API_BASE_URL=http://localhost:18081
```

