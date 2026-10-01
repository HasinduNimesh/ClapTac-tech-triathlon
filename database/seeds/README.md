# Seeds

`seed.sh` loads M2 outlet placeholders, `calendar.csv`, then competition-shaped CSVs via `scripts/import-datasets.sh`.

These five CSVs are now generated from the official `Tech-Triathlon 2026 - Datasets/` folder by
`scripts/convert-official-dataset.py` (120 outlets, 60 vehicles, 910 calendar dates — matching the
official counts exactly), not hand-written placeholders. Re-run it after pulling an updated official
dataset: `python3 scripts/convert-official-dataset.py`.

| File | Destination |
|---|---|
| `outlets.csv` | `shared.outlets` — official data, `name` column synthesized (importer falls back to `"<brand> <outlet_id>"`; the official file has no name field) |
| `vehicles.csv` | `fleet.vehicles` static attributes only — **no status column** |
| `calendar.csv` | `shared.operating_calendar` — `date`/`is_operating` columns only; the official file's richer columns (`dow`, `festival`, `monsoon`, `is_holiday`, ...) are dropped, not used by the current planner |
| `district_travel.csv` | `shared.district_travel` (optional; else TravelEstimator heuristic) — **lossy**, see below |
| `service_allowance.csv` | `shared.service_allowance` (optional) — **lossy**, see below |

Vehicle availability is not imported. Missing `fleet.vehicle_availability` rows mean **available**.

## Known approximations in the converted data

Waypoint's travel/service-time model (`travel.Estimator`: a flat from→to minute lookup, and a binary
mall/non-mall service allowance) is simpler than the official reference model used by
`Tech-Triathlon 2026 - Datasets/check_allocation.py`'s `trip_time()` function (depot→district plus a
separate inter-stop leg; service allowance keyed by `(brand, dock_type)` across 9 combinations). The
converter reshapes the official data to fit the existing model rather than upgrading the model:

- **`district_travel.csv`**: each official row becomes three directed pairs (depot→district,
  district→depot, district→district self-loop for inter-stop legs). The depot↔district time is
  assumed symmetric — the official file gives no return-trip figure. Where a depot's name equals its
  own district's name (Kandy), the three pairs collapse to one key; the inter-stop value wins that
  collision since a route has many inter-stop legs but only one depot leg.
- **`service_allowance.csv`**: the official 9 `(brand, dock_type)` values (15–59 minutes) collapse into
  two unweighted averages (`default` ≈ 36 min, `mall` ≈ 44 min), since the schema only supports two
  stop kinds. This under-serves the slower brands (Tech, Style) and over-serves Fresh.

A proper fix is a schema upgrade (richer `district_travel`/`service_allowance` tables plus a matching
`travel.Estimator`), not a seed-data change — tracked as a follow-up, not done here.
