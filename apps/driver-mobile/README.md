# Waypoint Field (Flutter)

One app for the two field roles, built from the Figma field designs:

- **Driver (phone):** sign-in with an offline "Continue offline" path, today's route, truck check-out, safe stop and outlet access, record delivery with photo or signature proof, rejected take-back, report a problem, plan-conflict review, saved → syncing → synced / sync-error states, updates from dispatch, end of day.
- **Loader (dock tablet + phone):** versioned load list with the plan-changed banner and acknowledgement, stop sequence, reverse-stop load guidance, report missing/damaged (side sheet on tablet, bottom sheet on phone), waiting for the dispatcher's decision, wrong-vehicle order check, capacity summary, ready-to-depart confirmation, switch dock user.

The signed-in account decides which workspace opens. Layouts switch to the tablet design at 900 px wide.

## Offline-first

Every write is saved on the device first and queued with a client-generated idempotency key (`lib/sync/sync.dart`). The queue drains in FIFO order and stops at the first failure, so proof → outcome → route completion stay ordered. Queues and cached routes are keyed per signed-in person, so a shared dock tablet never mixes loaders' work.

A loader shortfall keeps *Ready to depart* locked until the dispatcher records a decision (`PARTIAL_LOAD`, `HOLD`, `MOVE_TO_NEXT_RUN`); the loading service enforces the same rule.

## Run it

The app needs the Waypoint backend. Start it first with Docker from the repo root — see [docs/run-locally.md](../../docs/run-locally.md):

```bash
docker compose up --build
```

Then create demo data (a store order and a locked plan for its delivery date) as described there, otherwise the loader and driver see no trips.

### Windows desktop (simplest on a Windows PC)

1. Install Flutter 3.x and check it with `flutter doctor`.
2. Install **Visual Studio 2022** with the **Desktop development with C++** workload.
3. Turn on **Developer Mode** (Flutter needs symlinks for plugins): run `start ms-settings:developers` and switch it on.
4. From this folder:

   ```bash
   flutter pub get
   flutter run -d windows
   ```

The desktop build talks to `http://localhost` (API) and `http://localhost:8090` (identity) by default.

### Android emulator

```bash
flutter run --dart-define=API_BASE_URL=http://10.0.2.2/api/v1 --dart-define=OIDC_ISSUER=http://10.0.2.2:8090
```

`10.0.2.2` is how the emulator reaches the host's `localhost`. On a physical phone, use the computer's LAN IP instead.

If a platform folder is missing (for example `android/`), generate it once:

```bash
flutter create . --project-name waypoint_driver --platforms android
```

### Sign in

| Username | Password | Opens |
|---|---|---|
| `driver` | `waypoint` | Driver route (resize the window to phone width) |
| `loader` | `waypoint` | Loader workspace (900 px or wider = tablet design, narrower = phone design) |
| `loader-kandy` | `waypoint` | Loader for the Kandy depot |

The sign-in form posts to the local identity server and asks for the authorization code as JSON (`response_mode=json`, supported by `tools/dev-oidc`). A production ThunderID tenant would use the browser-based authorization-code flow instead.

## Checks

```bash
flutter analyze
flutter test
```

## Troubleshooting

| Message | Fix |
|---|---|
| `Building with plugins requires symlink support` | Turn on Developer Mode (`start ms-settings:developers`). |
| `Unable to find suitable Visual Studio toolchain` | Install the *Desktop development with C++* workload in Visual Studio 2022. |
| Signs in but shows no trip | Create an order and lock a plan for its delivery date as the dispatcher. |
| "No connection" banner while the stack is up | Check `docker compose ps`; on the emulator use the `10.0.2.2` defines above. |
