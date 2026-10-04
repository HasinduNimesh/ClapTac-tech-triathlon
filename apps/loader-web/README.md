# Waypoint Loader (Flutter web)

The loader's dock workspace, built from the Figma *Loader Workspace* designs, as a Flutter **web** app. It has a tablet layout (900 px and wider, for the shared dock tablet) and a phone layout.

**Always online.** Every action goes straight to the backend and the trip is re-read from the server afterwards. Nothing is queued or stored for later; if the connection drops the app says so, pauses the actions and asks the loader to retry. Only the sign-in session is kept, in this tab's session storage, so a reload stays signed in and closing the tab signs out (it is a shared dock tablet).

## Screens

| Figma frame | In the app |
|---|---|
| Loader / Load List (tablet, phone) | Vehicles and trips for the date with departure time, trip x of y, chilled/ambient, van-only stops, stops and area, kg and m³, "Changed in vN", "Loads after Trip 1 returns"; All / To load / Ready filters; stats (vehicles, ready with last confirmation, loading now with next departure, open shortfalls); stop sequence with outlet, window, dock (rear dock / curb / mall bay) and change notes |
| Plan updated banner | "Plan updated to vN at HH:MM — recheck …", what changed (moved stop, added, taken off), published by, *View changes*, *Acknowledge revised load* (moves the loading onto the new version) |
| Loader / Trip Details (tablet, phone) | Departs HH:MM · in N min, manifest version, reverse-stop guidance (Load N → Stop M, front/middle/rear/doors, chilled zone, dock, lines · kg · m³, window), loaded quantity per line, *Mark Stop N loaded*, *Report missing / damaged* |
| Capacity summary | Weight and volume against the vehicle (closest to full marked), chilled zone, Fresh trip time against the 270-minute budget |
| Report issue (tablet side sheet, phone bottom sheet) | Item, Missing / Damaged / Wrong item, quantity with "on hand", note, *Add photo* (required for damaged), sent to the dispatcher |
| Waiting for dispatcher | Report sent with time and reporter, whether the dispatcher has seen it, time left to departure, the decision when it arrives |
| Wrong vehicle warning | *Check an order number*: "Stop. … belongs on VEHxx", *Put back, not on this truck*, *Tell dispatcher* |
| Ready confirm / Ready | Checklist (plan acknowledged with time, all stops loaded, shortfalls resolved, chilled zone reading 2–4 °C, door seal number), confirmation with manifest, load, shortfalls, temperature and seal, then the ready state logged with the loader and time |
| Notifications | Plan versions, dispatcher decisions, reports seen and closed wrong-vehicle alerts |

## Backend calls

| Action | Endpoint |
|---|---|
| Sign in | Authorization code + PKCE with the identity server (`/oauth2/authorize` in the browser, `POST /oauth2/token` for the code and for renewal), then `GET /api/v1/shared/profiles/me` (LOADER role only). See *Sign-in* below |
| Load list | `GET /api/v1/loading/trips?date=…`, `GET /api/v1/loading/trips/{tripId}`, `GET /api/v1/loading/alerts?date=…` |
| Acknowledge revised plan | `POST /api/v1/planning/plans/{planId}/acknowledgements`, then `POST /api/v1/loading/trips/{tripId}/sync` (`If-Match: version`) if loading had started |
| Start loading | `POST /api/v1/loading/trips/{tripId}/start` (`If-Match: planVersion`) |
| Mark loaded (one order or a whole stop) | `PUT /api/v1/loading/trips/{tripId}/orders/{orderId}/loaded` |
| Tech custody before loading | `POST /api/v1/orders/{orderId}/custody` (`stage: LOADED`, seal, serials, condition) |
| Report missing / damaged / wrong item | `POST /api/v1/loading/trips/{tripId}/orders/{orderId}/issues`, photo: `POST …/issues/{issueId}/photo` (multipart, 4 MB). If the photo fails after the report was filed, the form keeps the report's ID and idempotency key and **retries only the upload** |
| Withdraw a report (undecided or on hold) | `DELETE /api/v1/loading/trips/{tripId}/orders/{orderId}/issues/{issueId}` |
| Tell dispatcher (wrong vehicle) | `POST /api/v1/loading/trips/{tripId}/alerts` |
| Ready to depart | `POST /api/v1/loading/trips/{tripId}/ready` with `chilledTemperatureC` and `sealNumber`. Both are **required by the service for a refrigerated vehicle** (400 without them, 409 if the chilled zone is outside 2–4 °C); 409 `dispatcher_decision_required` while a shortfall is undecided or on hold |

Every write sends an `Idempotency-Key`. The trip's kg/m³ totals, vehicle capacity, departure/return times, outlet windows and dock types come from planning through the loading API.

## Sign-in

The app signs in the standard way for a browser app, the same as the web app: **OIDC authorization code with PKCE for a public client**. The browser is sent to the identity server, the loader types their password there (never in this app), and the browser returns to `<origin>/loader-app/auth/callback?code=…`, where the app exchanges the code (with the PKCE verifier and a checked `state`) for tokens. It then reads `/shared/profiles/me` and only lets a `LOADER` in. The access token is renewed with the refresh token about a minute before it expires (and once if the API answers 401); if the identity server refuses the renewal the loader is signed out with a message. *Sign out* also ends the session at the identity server when it publishes an end-session endpoint. `prompt=login` is sent so the next person on a shared tablet enters their own credentials.

Nothing about sign-in is special for local development: local Compose and production run the same code and differ only in two build settings.

| Build setting (`--dart-define`, Docker build arg) | Local Compose | Production |
|---|---|---|
| `OIDC_ISSUER` (Compose passes `VITE_OIDC_ISSUER`) | `http://localhost:8090` (the dev identity server `tools/dev-oidc`) | the real ThunderID issuer, e.g. `https://id.waypoint.claptac.dev` |
| `OIDC_CLIENT_ID` (Compose: `LOADER_OIDC_CLIENT_ID`) | `waypoint-loader` | `waypoint-loader` |
| `OIDC_RESOURCE` (Compose: `VITE_OIDC_AUDIENCE`, the web app's setting) | `waypoint-api` (the dev server's audience) | `https://waypoint.claptac.dev/api/v1`, the value the services validate as `OIDC_AUDIENCE` |
| `OIDC_SCOPES` (optional) | `openid profile offline_access` | `openid profile offline_access` |

A build without `OIDC_ISSUER` does not guess one: its sign-in screen says sign-in is not set up.

**The API resource.** The Waypoint services accept an access token only for their audience (`OIDC_AUDIENCE`), and a production identity server issues one for that API only when the request names it (RFC 8707 `resource`), exactly as the web and driver apps do. The app sends `OIDC_RESOURCE` as `resource` on the authorization request, the code exchange and every renewal, and then reads the access token's `aud`: if it does not contain that resource the loader is told "Signed in, but not for the Waypoint API" with the audience that came back, instead of being signed in and getting a 401 from every call. (The token is read, not verified; the API verifies it. A token that is not a JWT is left to the API.) Set it to the same value as `OIDC_AUDIENCE` on the services and `VITE_OIDC_AUDIENCE` on the web app. An empty value sends no `resource` and skips the check.

**Register the client in ThunderID before deploying** (the dev identity server accepts any client): a **public** client (no secret), grant types *authorization_code* and *refresh_token*, PKCE `S256` required, scopes `openid profile offline_access`, the Waypoint API allowed as a resource/audience for the client, redirect URI `https://<public host>/loader-app/auth/callback` and post-logout redirect URI `https://<public host>/loader-app/` (HTTPS only), and the Waypoint origin allowed for CORS on the token endpoint (the browser calls it directly). Without `offline_access` the loader is simply sent back to sign in whenever the access token expires. The identity server is **not** proxied through the Waypoint NGINX: the app talks to its own origin for the API and to the issuer for sign-in.

## Run

The app is part of the Docker stack. From the repo root:

```bash
docker compose up --build
```

Open <http://localhost/loader-app/>. *Sign in* takes you to the local identity server's sign-in page (`http://localhost:8090`); use `loader` / `waypoint` (Peliyagoda) or `loader-kandy` / `waypoint` (Kandy), and you come back to the app. Resize the browser to see the tablet (≥ 900 px) and phone layouts. The full walkthrough, including how to create a plan so the loader has trips, is in [docs/run-dispatcher-and-loader.md](../../docs/run-dispatcher-and-loader.md).

After changing the app, rebuild just this container:

```bash
docker compose up -d --build loader-web
```

### Develop outside Docker

```bash
flutter pub get
flutter analyze
flutter test
flutter build web --release --base-href /loader-app/ --no-web-resources-cdn
```

`flutter run -d chrome` serves the app from a different port, so the browser blocks its calls to the API (no CORS) and the callback would not match. Use the Docker container, or point a local NGINX at `build/web`, to try it against the backend. Pass `--dart-define=OIDC_ISSUER=…` to any build that should sign in.

## Building and deploying the image

`docker compose build loader-web` builds the Flutter web app from a pinned SDK image (`ghcr.io/cirruslabs/flutter:3.41.2`, the version the app is analysed and tested with) and serves it with nginx under `/loader-app/`. It is the slowest image in the stack (a few minutes the first time), so build it on its own when only the web app changed. Compose adds only the `loader-web` service and an NGINX `depends_on`; no ports, volumes or existing services are changed. To deploy, set the issuer for both web apps in `.env` (`VITE_OIDC_ISSUER`), register the client above, rebuild `loader-web` and `nginx`, and use the usual `docker compose up -d --build` (not `--remove-orphans`).

## Continuous integration

The checks for this app are `flutter pub get`, `flutter analyze` and `flutter test` in `apps/loader-web`, and `docker compose build loader-web` for the image. The CI workflow selects what to run from the changed paths, so add `apps/loader-web/*` as its own component next to `apps/driver-mobile/*` (see the pull request for the exact job).

## Not in this app

- **Switch dock user** (Figma *Loader / Shared device / Switch user*): left out on purpose; each loader signs out and the next one signs in.
- **Offline use**: out of scope by design. The loader app is online-only; only the driver app works offline.
- **SKU item lines, vehicle plates and staff names** shown in the Figma sample data: the system's orders are order-level (units, kg, m³, temperature) with no SKU lines, vehicles have IDs but no registration plates, and users have IDs but no display names. The app shows order references, vehicle IDs and user IDs.
- **Partial-load quantity in a new plan version**: the dispatcher's partial-load decision is recorded on the report and lets the trip leave, but planning does not publish a new version with the reduced quantity (planning has no per-line quantities).
- **Camera barcode scanning**: not in the Figma; the loader types the order number.
- **A registered ThunderID client**: the app is ready for it (see *Sign-in*), but the `waypoint-loader` client, its callback and CORS setting still have to be created in the real tenant. Nothing here has been run against the production ThunderID.
- **Legacy `/loader` pages**: signing in as a loader on the React app still lands on the older loader pages; loaders should open `/loader-app/`. Those pages now also ask for the chilled-zone temperature and door seal, since the loading service requires them.
