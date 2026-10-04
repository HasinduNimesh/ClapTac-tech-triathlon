# Waypoint Loader (Flutter web)

The loader's dock workspace, built from the Figma *Loader Workspace* designs, as a Flutter **web** app. It has a tablet layout (900 px and wider, for the shared dock tablet) and a phone layout.

**Always online.** Every action goes straight to the backend and the trip is re-read from the server afterwards. Nothing is queued or stored for later; if the connection drops the app says so, pauses the actions and asks the loader to retry. Only the sign-in token is kept in the browser so a page reload stays signed in.

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
| Sign in | `POST /oauth2/authorize` (`response_mode=json`), `POST /oauth2/token`, `GET /api/v1/shared/profiles/me` (LOADER role only) |
| Load list | `GET /api/v1/loading/trips?date=…`, `GET /api/v1/loading/trips/{tripId}`, `GET /api/v1/loading/alerts?date=…` |
| Acknowledge revised plan | `POST /api/v1/planning/plans/{planId}/acknowledgements`, then `POST /api/v1/loading/trips/{tripId}/sync` (`If-Match: version`) if loading had started |
| Start loading | `POST /api/v1/loading/trips/{tripId}/start` (`If-Match: planVersion`) |
| Mark loaded (one order or a whole stop) | `PUT /api/v1/loading/trips/{tripId}/orders/{orderId}/loaded` |
| Tech custody before loading | `POST /api/v1/orders/{orderId}/custody` (`stage: LOADED`, seal, serials, condition) |
| Report missing / damaged / wrong item | `POST /api/v1/loading/trips/{tripId}/orders/{orderId}/issues`, photo: `POST …/issues/{issueId}/photo` (multipart) |
| Withdraw a report (undecided or on hold) | `DELETE /api/v1/loading/trips/{tripId}/orders/{orderId}/issues/{issueId}` |
| Tell dispatcher (wrong vehicle) | `POST /api/v1/loading/trips/{tripId}/alerts` |
| Ready to depart | `POST /api/v1/loading/trips/{tripId}/ready` with `chilledTemperatureC` and `sealNumber` (409 `dispatcher_decision_required` while a shortfall is undecided or on hold; 409 if the chilled zone is outside 2–4 °C) |

Every write sends an `Idempotency-Key`. The trip's kg/m³ totals, vehicle capacity, departure/return times, outlet windows and dock types come from planning through the loading API.

## Run

The app is part of the Docker stack. From the repo root:

```bash
docker compose up --build
```

Open <http://localhost/loader-app/> and sign in as `loader` / `waypoint` (Peliyagoda) or `loader-kandy` / `waypoint` (Kandy). Resize the browser to see the tablet (≥ 900 px) and phone layouts. The full walkthrough, including how to create a plan so the loader has trips, is in [docs/run-dispatcher-and-loader.md](../../docs/run-dispatcher-and-loader.md).

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

`flutter run -d chrome` serves the app from a different port, so the browser blocks its calls to the API (no CORS). Use the Docker container, or point a local NGINX at `build/web`, to try it against the backend.

## Not in this app

- **Switch dock user** (Figma *Loader / Shared device / Switch user*): left out on purpose; each loader signs out and the next one signs in.
- **Offline use**: out of scope by design. The loader app is online-only; only the driver app works offline.
- **SKU item lines, vehicle plates and staff names** shown in the Figma sample data: the system's orders are order-level (units, kg, m³, temperature) with no SKU lines, vehicles have IDs but no registration plates, and users have IDs but no display names. The app shows order references, vehicle IDs and user IDs.
- **Partial-load quantity in a new plan version**: the dispatcher's partial-load decision is recorded on the report and lets the trip leave, but planning does not publish a new version with the reduced quantity (planning has no per-line quantities).
- **Camera barcode scanning**: not in the Figma; the loader types the order number.
- **Production sign-in**: the app uses the local identity server's JSON authorize mode. Production ThunderID needs the browser redirect (PKCE) flow, and there is no server deployment config yet.
