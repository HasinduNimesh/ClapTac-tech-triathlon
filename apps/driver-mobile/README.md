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
Deliveries go through `DeliveryRepository` into the local database and `SyncQueue`, each event carrying a client-generated `idempotencyKey`.

## Not built yet

- Sign-in is disabled in a normal build because staff accounts (ThunderID/OIDC) are not connected. `flutter run --dart-define=DEMO_AUTH=true` accepts any credentials for demos only.
- Nothing is uploaded or sent to dispatch (deliveries, load discrepancies and problem reports are only queued locally, and the screens say so): `InMemoryLocalDatabase` and `InMemorySyncQueue` are used, so data is lost when the app closes, and the "synced" and "syncing" screens are never reached from the flow.
- Photo and signature capture only toggle a "captured" state.
- Connectivity is not detected, so the no-signal sign-in variant is not triggered automatically.
- The signed-in driver and today's route are sample data: there is no profile lookup (`GET /api/v1/shared/profiles/me`) or assigned-trip lookup yet, so the app cannot be used for real deliveries.
- Load check: confirming sets the load as confirmed on this phone only. Reporting a missing item queues a `GOODS` `INCIDENT_REPORT` (there is no driver-facing load-shortfall operation yet), keeps the route locked, and lets the driver either go back to the check or explicitly depart anyway, which is queued as a second incident.
- Problem reports are queued as `INCIDENT_REPORT` operations in the shape of the delivery sync contract; they are never sent, and the screens say so.
- Plan update review is demo data; "send for review" only changes local navigation.
- `flutter run --dart-define=DEMO_UPDATES=true` adds a sample plan update so the plan review screen can be opened.

## Layout

- `lib/theme` design tokens (Figma Foundations), theme and asset paths
- `lib/widgets` shared hero, shell with bottom navigation, buttons, note banners
- `lib/screens/<group>` the screens, each group with its own widgets and asset list
- `lib/app` session state and navigation
- `assets/images`, `assets/icons` exported from Figma; `assets/fonts` Inter (OFL)

## Run

Platform projects are not checked in. Generate them once:

```bash
flutter create . --platforms=android,ios --project-name waypoint_driver
flutter pub get
flutter run
```

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
