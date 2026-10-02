# Waypoint Field (Flutter)

One app for the two field roles, built from the Figma field designs:

- **Driver (phone):** sign-in with an offline "Continue offline" path, today's route, truck check-out, safe stop and outlet access, record delivery with photo or signature proof, rejected take-back, report a problem, plan-conflict review, saved → syncing → synced / sync-error states, updates from dispatch, end of day.
- **Loader (dock tablet + phone):** versioned load list with the plan-changed banner and acknowledgement, stop sequence, reverse-stop load guidance, report missing/damaged (side sheet on tablet, bottom sheet on phone), waiting-for-dispatcher, wrong-vehicle order check, capacity summary, ready-to-depart confirmation, switch dock user.

The signed-in account decides which workspace opens. Layouts switch to the tablet design at 900 px wide.

## Offline-first

Every write is saved on the device first and queued with a client-generated idempotency key (`lib/sync/sync.dart`). The queue drains in FIFO order and stops at the first failure, so proof → outcome → route completion stay ordered. Queues and cached routes are keyed per signed-in person, so a shared dock tablet never mixes loaders' work.

## Run

```bash
flutter pub get
flutter run --dart-define=API_BASE_URL=http://10.0.2.2/api/v1 --dart-define=OIDC_ISSUER=http://10.0.2.2:8090
```

Defaults point at `http://localhost` (the local Compose stack). Sign in with the seeded `driver` or `loader` account (password `waypoint`). The sign-in form posts to the local identity provider's authorize endpoint and exchanges the code; a production ThunderID tenant would use the browser-based authorization-code flow instead.

Generate platform folders if they are missing:

```bash
flutter create . --project-name waypoint_driver --platforms android,ios
flutter analyze
flutter test
```
