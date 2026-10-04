# Where each screen's data comes from

A check of the apps against the code (not a runtime audit of a deployed database). Nothing in the web apps or the
loader app is mock data: every figure on a page comes from an API call to a Go service over Postgres, and the
services contain no stubs or in-memory stand-ins (no mock, fake or hard-coded responses).

## Real, from the database through the API

| App | Screens |
|---|---|
| Dispatcher web | Overview, Order queue, Planning, Live operations, Fleet, Deferral history, Forecast, Master data, Audit, Notifications |
| Store manager web | Orders, Tracking and order timeline, Receipts, Notifications (derived from the same orders) |
| Driver web page | Trips, stops, proof, messages |
| Loader app | Load list, trip details, reports, ready checks, dock alerts |
| Driver phone app | Route, stops, deliveries, proof, messages, sync, saved route |

Dispatcher, store manager and loader identities come from `shared.users` and the role profile tables
(`store_manager_profiles`, `loader_profiles`, `driver_profiles`, `dispatcher_profiles`).

## Real workflow, but kept on the device, not in the database

These work, but another person or device cannot see them and clearing the browser loses them:

- **Store manager custom dashboards** (the card layout they build) are saved in the browser's local storage, per person and outlet.
- **Store manager "acknowledged" deferral notices and "seen" ETA changes** are saved in local storage. Acknowledging a notice does not tell the dispatcher.
- **Display preferences** (language, text size, contrast) are per browser by design.
- **Driver phone app**: the route, queued deliveries and proofs are on the phone until sent (that is the offline design).

## Fixed values in the code

- **Depot positions on the dispatcher's map** are two constants (`DEPOT_LOCATIONS` in `apps/web/src/components/WaypointMap.tsx`, Peliyagoda and Kandy). There is no depots table; depot names are text on outlets, vehicles and profiles.
- **Depot names** `DEPOT_NORTH` / `DEPOT_SOUTH` ↔ `Peliyagoda` / `Kandy` are a lookup in `apps/web/src/api/loading.ts`; a third depot needs code changes.
- **Outlet positions** are approximate until a dispatcher records them ([outlet-locations.md](outlet-locations.md)).
- A truck on the live map is at its **last reported stop**, not a GPS position.
- The depot selector filters what is already loaded in the browser; it is not an access rule. Dispatchers can see every depot.
- A person's name is not stored: the sidebar shows their internal user id (`shared.users` has no name or email).

## Demo-only, off in real builds

- The driver phone app's sample route, sample plan update and any-credentials sign-in (`DEMO_AUTH`, `DEMO_ROUTE`, `DEMO_UPDATES`) are forced off in release builds. The plan review, synced/upload-failed illustrations, *Forgot password* and *Continue offline* are Figma screens not connected to a real flow (see the driver README).

## Data that is only as real as what was loaded

- The database was filled from the competition-shaped seed files in `database/seeds/` (outlets, vehicles, calendar, district travel). These are fixtures, not a company's real shop list: outlet names may be blank in the seed, and coordinates are not in it.
- **Proof photos and signatures** are stored in the object store configured by `MINIO_ENDPOINT`. The Compose default is `adobe/s3mock`, a test double that does not promise to keep files across a restart. A production deployment needs a real, durable, private bucket (the production deployment branch uses Cloudflare R2). Until then, proof uploaded in a deployment that uses the default can be lost.
