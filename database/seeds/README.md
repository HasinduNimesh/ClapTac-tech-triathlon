# Seeds

`seed.sh` loads M2 outlet placeholders, `calendar.csv`, then competition-shaped CSVs via `scripts/import-datasets.sh`.

| File | Destination |
|---|---|
| `outlets.csv` | `shared.outlets` (120 rows in the fixture; replace with official file) |
| `vehicles.csv` | `fleet.vehicles` static attributes only — **no status column** |
| `calendar.csv` | `shared.operating_calendar` |
| `district_travel.csv` | `shared.district_travel` (optional; else TravelEstimator heuristic) |
| `service_allowance.csv` | `shared.service_allowance` (optional) |

Vehicle availability is not imported. Missing `fleet.vehicle_availability` rows mean **available**.
