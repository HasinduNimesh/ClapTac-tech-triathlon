# Waypoint Driver (Flutter)

Offline-first route execution. The UI follows the Figma "Driver" phone screens (390 × 844).

## What is built

| Area | Screens |
| --- | --- |
| Sign in | Sign in; no-signal sign in with "Continue offline" |
| Route | Route home; safe stop and outlet access; truck check-out sheet; report a problem sheet |
| Delivery | Stop details (online and offline); record delivery and proof; rejected take-back sheet |
| States | Saved on this device; syncing; synced; upload needs attention; end of day; plan update needs review |
| Tabs | Route, Updates (no Figma frame; reuses the note banners) and Summary |

`lib/app/driver_flow.dart` connects them: sign in → load check → safe stop → stop details → saved on this device → route → end-of-day summary.

What the driver does is queued as delivery sync operations (`lib/sync/operations.dart`), shaped like the entries of `POST /api/v1/delivery/sync` and keyed by the server's trip and stop ids, never by outlet code:

| Driver action | Operation |
| --- | --- |
| Confirm load / depart anyway | not queued: acknowledges the current plan version (`POST /api/v1/planning/plans/{id}/acknowledgements`) then starts the run (`POST /api/v1/delivery/trips/{id}/start`, with an Idempotency-Key). Both need a connection; without one the trip waits and is retried |
| "I've stopped safely" | `ARRIVED` (the server needs an arrival before it accepts an outcome, and the run must be started) |
| Take photo / add signature | `PROOF_UPLOAD`, local-only: the file is kept on the phone and sent by the worker as a multipart upload (`POST /api/v1/delivery/trips/{id}/stops/{id}/proofs`), because `/delivery/sync` rejects proofs. It is queued before the outcome, which depends on it |
| Save delivery | `STOP_OUTCOME` with `code` DELIVERED / PARTIAL / FAILED / REFUSED, `deliveredUnits`, a `reason` code for failed and refused (OUTLET_CLOSED, GOODS_REJECTED, OTHER, ...) and a `note` |
| Finish trip (all stops recorded) | `ROUTE_COMPLETED` |
| Report a problem, missing load item | `INCIDENT_REPORT` with `category` and `description`, `stopId` at the top level when at a stop |

Every operation has a random operation id that is created once and kept, so a repeat is recognised as a duplicate. In a configured build, the queue is stored in SQLite under the signed-in Waypoint user ID. It is replayed in order on app restart and every 15 seconds while the app is open. An `APPLIED` result, or a `DUPLICATE` whose original result was `APPLIED`, marks an entry synced. `CONFLICT` and `REJECTED` stay on the phone and pause later entries; the Updates tab shows the problem. Network failures retain the entry for retry. The queue is not encrypted separately from the phone's application storage; device encryption and screen lock should be enabled on production phones.

The queue and worker have automated tests for SQLite restart persistence, ordering, retries, and `APPLIED`, `DUPLICATE`, `CONFLICT`, and `REJECTED` responses. `test/live_sync_test.dart` is an opt-in integration check that sends one temporary ROAD incident through a local delivery service using a local driver token. It requires `WAYPOINT_LIVE_DRIVER_TOKEN_FILE`, `WAYPOINT_LIVE_TRIP_ID`, `WAYPOINT_LIVE_OPERATION_ID`, and `WAYPOINT_LIVE_API_BASE_URL`; the URL must be loopback HTTP. Clean its incident and sync operation from the disposable database after the check. No credential is kept in the repository.

## Sign-in

Sign-in uses Authorization Code + PKCE through the system browser (AppAuth). The app never sees the username or password. After the browser returns, the app calls `GET /api/v1/shared/profiles/me` with the access token and only continues for accounts whose role is `DRIVER`:

- cancelled in the browser: back to the sign-in screen, no message
- 401: "Your sign-in was not accepted"
- 404: the account exists in the identity provider but is not set up in Waypoint
- another role (dispatcher, loader, store manager): "This app is for drivers"
- network or server error: "Could not reach Waypoint"

The access token and profile are kept in the Android Keystore / iOS Keychain, so a signed-in driver resumes without a network call until the token expires. Finishing the trip clears them.

Configure it per build with `--dart-define` (see `lib/auth/auth_config.dart`):

| Define | Meaning |
| --- | --- |
| `OIDC_ISSUER` | identity provider, e.g. `https://id.waypoint.claptac.dev` |
| `API_BASE_URL` | Waypoint API host, e.g. `https://waypoint.claptac.dev` |
| `OIDC_RESOURCE` | the API the token is for, an absolute URI such as `https://waypoint.claptac.dev/api/v1`; sent as the `resource` parameter (ThunderID rejected the bare `waypoint-api` value) |
| `OIDC_CLIENT_ID` | default `waypoint-driver` |
| `OIDC_REDIRECT_URI` | default `dev.claptac.waypointdriver:/oauth2redirect`; the scheme must match `appAuthRedirectScheme` in `android/app/build.gradle.kts` and may not contain an underscore |

Without `OIDC_ISSUER` and `API_BASE_URL`, sign-in is disabled. Plain HTTP is only accepted in debug builds, for a local identity server; release builds refuse it. The identity provider side is described in `infrastructure/thunder/README.md`.

The demo switches `DEMO_AUTH` (any credentials, no identity provider), `DEMO_ROUTE` (show the sample route after a real sign-in) and `DEMO_UPDATES` (a sample plan update) are forced off in release builds, whatever is passed.

## Not built yet

Sign-in, loading the route, starting the trip, recording deliveries with photo and signature proof, and sending them are real and have been run against the local backend. They have **not** been run against the real ThunderID or a production backend, so do not describe this build as production-ready.

- **Trips are loaded, but one at a time.** After sign-in the app reads today's trips (`GET /api/v1/delivery/drivers/me/trips?date=`, using the Waypoint business date), opens the first one that is not completed (`GET /api/v1/delivery/trips/{id}`, which prepares the run for a driver) and shows its stops with the server's trip, run and stop ids. If there is no trip it says "No trip for you today"; if the server cannot be reached it says so and offers Try again; if the token is rejected the driver goes back to sign-in. A local phone check confirmed profile and trip reads through the API with `tools/dev-oidc`. Real ThunderID and outbound operations have not been checked on a phone.
- **Quantities are "units".** The server's `expectedUnits` is shown as "N units" because the order model does not say they are cartons (the delivery service returns it per stop). A stop without it shows "Quantity not recorded" and cannot be delivered partially, since there is nothing to check a quantity against. A delivered or partial outcome sends `deliveredUnits` (the expected units, or the quantity entered, which must be more than none and fewer than all); failed and refused send 0; a stop without `expectedUnits` sends none, because the server rejects it. There is no plate number on the server, so the vehicle id is shown alone.
- **Sending.** A worker sends the SQLite queue in order, on start-up, every 15 seconds and when the connection returns: arrivals, proof uploads, outcomes and incident reports, then the route completion. Nothing is sent for a stop until the run is started on the server (confirming the load does that). A successful outcome is held until its proof has uploaded. Finish trip sends the completion before signing out and asks when something could not be sent. The synced and upload-failed illustrations are not connected to the live flow. The demo build still uses an in-memory queue.
- **Proof.** Photo (the phone's camera app) and signature (a pad, with an optional receiver name) are captured, checked against the server's rules (JPEG or PNG, photo at most 4 MB, signature at most 512 KB) and stored as files in the app's storage until uploaded. A delivered or partial outcome cannot be saved without one. Not handled: if Android stops the app while the camera is open the photo is lost and the driver retakes it, and upload over a weak connection has not been tried.
- **Server gaps.** There is no distinct "re-attempt next run" or "defer" operation (it is written in the `note`), and there is no structured driver load-discrepancy workflow (a missing item is a `GOODS` incident). Contracts and RBAC for these need deciding before they can work as the screens show.
- **Load check.** Confirming only marks the load confirmed on this phone. Reporting a missing item keeps the route locked until the driver confirms again or explicitly departs anyway, which is queued as a second incident.
- **Plan and messages.** Plan acknowledgement (`POST /api/v1/planning/plans/{id}/acknowledgements`) and dispatcher trip messages exist on the server but are not used. The plan review screen ("send both versions for review") has no matching API and is demo-only.
- Connectivity is not detected, so the no-signal sign-in variant is not triggered automatically. Tokens are not refreshed: when the access token expires the driver signs in again.

## Layout

- `lib/theme` design tokens (Figma Foundations), theme and asset paths
- `lib/widgets` shared hero, shell with bottom navigation, buttons, note banners
- `lib/screens/<group>` the screens, each group with its own widgets and asset list
- `lib/app` session state and navigation
- `assets/images`, `assets/icons` exported from Figma; `assets/fonts` Inter (OFL)

## Run

The Android project is checked in (application ID `dev.claptac.waypoint_driver`). iOS is not included; generate it with `flutter create . --platforms=ios --org dev.claptac --project-name waypoint_driver`.

```bash
flutter pub get
# Real sign-in against an identity provider
flutter run --dart-define=OIDC_ISSUER=https://id.example.com --dart-define=API_BASE_URL=https://api.example.com \
  --dart-define=OIDC_RESOURCE=https://api.example.com/api/v1
# Demo without an identity provider (debug builds only)
flutter run --dart-define=DEMO_AUTH=true
```

A build with neither the OIDC settings nor `DEMO_AUTH` refuses to sign in. To try sign-in on a phone against the local Compose stack, see `infrastructure/thunder/README.md`.

### Release builds

A release build is signed with a private keystore and fails without one; it never falls back to the debug key. Create `android/key.properties` (git-ignored) pointing at a keystore that is also kept out of Git:

```properties
storeFile=../../../keys/waypoint-driver.jks   # relative to android/
storePassword=...
keyAlias=waypoint-driver
keyPassword=...
```

```bash
flutter build apk --release --dart-define=OIDC_ISSUER=https://id.waypoint.claptac.dev \
  --dart-define=API_BASE_URL=https://waypoint.claptac.dev \
  --dart-define=OIDC_RESOURCE=https://waypoint.claptac.dev/api/v1
```

For a release build that is only for testing on your own phone, `WAYPOINT_ALLOW_DEBUG_SIGNING=1 flutter build apk --release ...` signs with the debug key. Do not distribute that build. Demo switches are ignored in release builds, so do not pass them.

Install with `adb install -r build/app/outputs/flutter-apk/app-release.apk`. On some Xiaomi phones, enable "Install via USB" in Developer options first.

## Test

```bash
flutter analyze
flutter test
```

To write PNGs of every screen to `test/render/out/` (git-ignored) for visual review against Figma:

```bash
flutter test test/render --update-goldens --dart-define=RENDER_SCREENS=1
```

The agent plane is not on this workflow.
