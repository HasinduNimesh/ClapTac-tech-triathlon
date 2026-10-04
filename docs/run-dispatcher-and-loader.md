# Run the Dispatcher and Loader workspaces

Both run from the same Docker stack:

| Workspace | URL | Built with |
|---|---|---|
| Dispatcher (and Store Manager) | <http://localhost/> | React web app (`apps/web`) |
| Loader (dock tablet + phone) | <http://localhost/loader-app/> | Flutter web app (`apps/loader-web`), online-only |

## 1. Start the stack

Prerequisites: Docker Desktop with Compose v2. On Windows, run `wsl --install` in an administrator PowerShell first, restart, then start Docker Desktop.

From the repo root:

```bash
docker compose up --build
```

The first build takes several minutes (Go services, the React app and the Flutter app). It is ready when `migrate`, `seed` and `bootstrap` have **exited with code 0** and `docker compose ps` shows the services, `web` and `loader-web` healthy.

If `migrate` fails with `set: illegal option -`, the shell scripts were checked out with Windows line endings — see *Line endings* in [run-locally.md](run-locally.md).

## 2. Accounts

All passwords are `waypoint`.

| Username | Opens |
|---|---|
| `dispatcher` | <http://localhost/> → Dispatcher workspace |
| `store-manager` | <http://localhost/> → Store Manager workspace (outlet OUT034) |
| `loader` | <http://localhost/loader-app/> → Loader, Peliyagoda depot |
| `loader-kandy` | <http://localhost/loader-app/> → Loader, Kandy depot |

Use a separate browser window (or a private window) for the loader so both stay signed in.

## 3. Walk through a day

The seed has outlets, vehicles and the calendar but **no orders**, so start by creating some.

1. **Store Manager** (`store-manager`, <http://localhost/>): *Place Orders* → add an ambient and a chilled order. Note the delivery date (after the 4:00 PM cutoff it moves to the next operating day).
2. **Dispatcher** (`dispatcher`): *Plan and allocate* → set the date → *Create or load plan* → *Generate* → *Lock plan and send to dispatch*.
3. **Loader** (`loader`, <http://localhost/loader-app/>): set the date in the banner to the plan's delivery date → open a trip → *Start loading* → load stop by stop with *Mark Stop N loaded* (the last stop goes in first) → on one stop choose *Report missing / damaged* (missing, 2 units; for *Damaged* add a photo). *Ready to depart* stays locked and the report shows *Waiting for dispatcher*.
4. **Dispatcher**: *Notifications* → *Review the load exception* (the loader now sees the report as seen) → *View photo* if one was attached → pick *Accept a partial load*, *Hold the trip* or *Move the whole line to the next run* → send.
5. **Loader**: refresh the trip. A partial load unlocks the trip. *Move to the next run* publishes a new plan version: the loader gets the *Plan updated* banner, *View changes* shows the line taken off, and *Acknowledge revised load* moves loading onto the new version. On *Hold*, withdraw the report once stock arrives and load it.
6. **Loader**: on the Ready card record the chilled-zone reading (2–4 °C on a refrigerated truck) and the door seal number → *Confirm ready to depart*.
7. **Dispatcher**: *Live operations* (list and *Map*), *Overview* and *Fleet* show the trip as ready.

Also try in the loader app: *Check an order number* with an order from another vehicle (wrong-vehicle warning → *Tell dispatcher*, which shows up in the dispatcher's *Notifications* as a wrong-vehicle alert), the *Notifications* tab, and a narrow browser window for the phone layout. In the dispatcher: *Plan and allocate* → *Map view* and the *Trip time* column (Fresh trips against the 270-minute budget).

## 4. Rebuild after changes

```bash
docker compose up -d --build web          # React dispatcher/store-manager app
docker compose up -d --build loader-web   # Flutter loader app
docker compose up -d --build loading-service planning-service shared-service
```

Stop with `docker compose down`; add `-v` to wipe the database and start from a fresh seed.

## 5. Tests

```bash
cd apps/web && npm ci && npm test && npm run build
cd apps/loader-web && flutter analyze && flutter test
make verify   # Go unit + PostgreSQL-backed e2e tests (needs Docker)
```
