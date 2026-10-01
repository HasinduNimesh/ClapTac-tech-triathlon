# Data model

Owned schemas: `orders`, `planning`, `fleet`, `loading`, `delivery`, `shared`, `audit`.

Milestone 5 adds:

- `shared.driver_profiles` maps a driver `user_id` to `vehicle_id`
- Loading snapshot columns on `loading.sessions` / `loading.order_loads` (trip number, vehicle type/temp, order ref/outlet/brand/temp)
- `delivery.runs`, `delivery.stops` (dual timestamps), `delivery.proofs`, `delivery.sync_operations` (`APPLIED`/`CONFLICT`/`REJECTED` only)


- `shared.outlets` district, depot, dock, parking, mall window, and open/close times
- `shared.district_travel` and `shared.service_allowance` (optional CSVs; otherwise the planning TravelEstimator heuristic)
- `fleet.vehicles` static attributes only — no status column
- `fleet.vehicle_availability` per date; missing rows mean available
- `planning.plans` unique on `delivery_date`, plus `trips`, `allocations`, and `deferrals`
