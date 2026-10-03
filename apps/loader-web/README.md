# Waypoint Loader (Flutter web)

The loader's dock workspace, built from the Figma *Loader Workspace* designs, as a Flutter **web** app. It has a tablet layout (900 px and wider, for the shared dock tablet) and a phone layout.

**Always online.** Every action goes straight to the backend and the trip is re-read from the server afterwards. Nothing is queued or stored for later; if the connection drops the app says so, pauses the actions and asks the loader to retry. Only the sign-in token is kept in the browser so a page reload stays signed in.

## Screens

| Figma frame | In the app |
|---|---|
| Loader / Load List (tablet, phone) | Assigned vehicles and trips for the chosen date, manifest plan version, stop sequence, goods and quantities, plan-changed banner with *Acknowledge revised load*, open-shortfall count |
| Loader / Trip Details (tablet, phone) | Reverse-stop load guidance (last stop in first), per-order *Loaded* / *Report issue*, load summary, ready checklist |
| Report issue (tablet side sheet, phone bottom sheet) | Item, missing/damaged, quantity, note → sent to the dispatcher |
| Waiting for dispatcher | The shortfall shows *Waiting for dispatcher*; *Ready to depart* stays locked until the dispatcher decides |
| Wrong vehicle warning | *Check an order number*: warns when the order belongs on another vehicle, or marks it loaded when it belongs here |
| Ready confirm / Ready | Confirmation dialog (tablet) or sheet (phone), then the ready state logged with the loader's name |

## Backend calls

| Action | Endpoint |
|---|---|
| Sign in | `POST /oauth2/authorize` (`response_mode=json`), `POST /oauth2/token`, `GET /api/v1/shared/profiles/me` (LOADER role only) |
| Load list | `GET /api/v1/loading/trips?date=…`, `GET /api/v1/loading/trips/{tripId}` |
| Acknowledge revised plan | `POST /api/v1/planning/plans/{planId}/acknowledgements` |
| Start loading | `POST /api/v1/loading/trips/{tripId}/start` (`If-Match: planVersion`) |
| Mark loaded | `PUT /api/v1/loading/trips/{tripId}/orders/{orderId}/loaded` |
| Tech custody before loading | `POST /api/v1/orders/{orderId}/custody` (`stage: LOADED`, seal, serials, condition) |
| Report missing / damaged | `POST /api/v1/loading/trips/{tripId}/orders/{orderId}/issues` |
| Withdraw a report (undecided or on hold) | `DELETE /api/v1/loading/trips/{tripId}/orders/{orderId}/issues/{issueId}` |
| Ready to depart | `POST /api/v1/loading/trips/{tripId}/ready` (409 `dispatcher_decision_required` while a shortfall is undecided or on hold) |

Every write sends an `Idempotency-Key`.

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

## Not in this app yet

These are features from the designs or the old React loader page that the loader app does not have, mostly because the backend does not support them yet:

- **Camera barcode scanning.** The React page used the browser's barcode detector; here the loader types the order number (*Check an order number*).
- **Photo on a shortfall report.** Loading issues have no attachment field in the loading service, so the report is quantity + note only.
- **Weight and volume per trip.** The loading API returns units, not kg/m³, so the *Load summary* shows stops and units. The dispatcher already checked weight and volume when the plan was built.
- **What changed in a new plan version.** The API gives the new version number, not a diff, so *View changes* opens the trip rather than a list of changes.
- **Switch dock user.** Removed on purpose: each loader signs out and the next one signs in.
- **Fresh trip time budget (270 min).** Not exposed by the loading API.
- **Temperature check and seal number at ready.** Shown as checklist items, but the ready endpoint does not record them.
- **Editing a report's quantity.** The API supports it (`PUT …/issues/{issueId}`); the app offers withdraw-and-report-again instead.
- **Offline use.** Out of scope by design: the loader app is online-only.
