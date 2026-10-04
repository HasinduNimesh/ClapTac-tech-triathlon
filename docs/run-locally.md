# Run Waypoint locally (web workspaces)

This guide starts the full stack with Docker and walks through the Dispatcher and Store Manager web workspaces built from the Figma designs. The Loader workspace is a separate Flutter web app served by the same stack at `/loader-app/`: see [docs/run-dispatcher-and-loader.md](run-dispatcher-and-loader.md) and [apps/loader-web/README.md](../apps/loader-web/README.md).

## 1. Prerequisites

- **Docker Desktop** with Compose v2 (`docker compose version`).
- **Windows only:** Docker Desktop needs WSL 2. In an administrator PowerShell run `wsl --install`, restart, then start Docker Desktop and wait until it says *Engine running*.
- **Line endings:** the repo's `.gitattributes` keeps `*.sh`, `Dockerfile` and `*.conf` files as LF. If you cloned before it existed and see `set: illegal option -` or `\r: not found` in the `migrate`, `seed` or `bootstrap` logs, refresh those files once:

  ```bash
  git rm --cached -r -q . && git reset --hard
  ```

  Commit or stash your own changes first — `reset --hard` discards uncommitted edits.

No `.env` file is required: every setting has a local default. Copy `.env.example` to `.env` only to override something.

## 2. Start the stack

From the repo root:

```bash
docker compose up --build
```

The first build takes several minutes. The stack is ready when `migrate`, `seed` and `bootstrap` have each **exited with code 0** and the services report healthy (`docker compose ps`).

Open <http://localhost>. The local identity screen opens at <http://localhost:8090>.

| Username | Password | Workspace |
|---|---|---|
| `dispatcher` | `waypoint` | Dispatcher (web) |
| `store-manager` | `waypoint` | Store Manager, outlet OUT034 (web) |
| `store-manager-b` | `waypoint` | Store Manager, outlet OUT021 (web) |
| `loader` | `waypoint` | Loader, Peliyagoda depot (loader app at <http://localhost/loader-app/>) |
| `driver` | `waypoint` | Driver, vehicle VEH001 (driver app; web pages at `/driver`) |

Each account opens only its own workspace. Sign out to switch.

**Ports used:** 80 (NGINX), 5432 (PostgreSQL), 6379 (Redis), 8090 (identity), 9000/9001 (object storage). Stop anything else using them (a local PostgreSQL, IIS) before starting.

## 3. Create demo data

The seed loads outlets, vehicles and the operating calendar, but **no orders**. Create a few so every screen has something to show:

1. **Store Manager** (`store-manager`): *Place Orders* → add an ambient and a chilled order. Note the delivery date shown (orders after the 4:00 PM cutoff move to the next operating day). Repeat with `store-manager-b` for a second outlet.
2. **Dispatcher** (`dispatcher`): *Plan and allocate* → set the date to that delivery date → *Create or load plan* → *Generate* → *Lock plan and send to dispatch*.
3. **Loader** (<http://localhost/loader-app/>): open the trip, start loading, mark orders loaded, report a missing item. *Ready to depart* stays locked.
4. **Dispatcher**: *Notifications* → *Review the load exception* → choose *Accept a partial load* (or *Hold* / *Move to the next run*). The loader can now confirm *Ready to depart*.
5. **Driver** (driver app): run the trip and record outcomes.
6. **Store Manager**: *Track order* (map), *Confirm delivery receipt*, the order evidence timeline and *Notifications*.

## 4. What to look at

**Dispatcher:** Overview · Order queue (+ order drawer) · Plan and allocate (blocking issues, plan ready, locked plan, manual adjustment, deferral reason, what-if comparison) · Live operations (list and map, breakdown recovery) · Fleet · Deferral history · Demand forecast · Notifications (plan health, load exception, sync conflicts) · Settings (language, text size, contrast).

**Store Manager:** Dashboard with the *Dashboard* menu and *Create new dashboard* (assistant + live preview) · saved dashboards · Track order (map) · Confirm delivery receipt · Order evidence timeline · Notifications (receipts, ETA changes, deferrals) · Settings → Language & display.

**Maps:** Live operations → *Map* and Store Manager → *Track order* use OpenStreetMap tiles (internet access needed). Outlet positions are approximate (district centre plus a small offset; migration `0041_outlet_locations.sql`) and a truck is shown at its last reported stop, not a GPS position.

## 5. Stop or reset

```bash
docker compose down        # stop, keep data
docker compose down -v     # stop and wipe the database (fresh seed next start)
```

After pulling changes to a service or the web app, rebuild just that part, for example `docker compose up -d --build web`.

## 6. Tests

```bash
cd apps/web && npm ci && npm test && npm run build
make verify   # full Go + web suite; needs Docker for the PostgreSQL-backed tests
```

Without a local Go toolchain, run the Go checks in a container from the repo root:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.22 sh -c "go build ./... && go vet ./... && go test ./pkg/... ./services/..."
```
