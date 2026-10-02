# Datathon tasks

This directory holds work for the Tech-Triathlon 2026 Datathon track, which is
judged separately from the Hackathon platform (see `docs/implementation-backlog.md`
and the root `README.md`). The source data lives in `Tech-Triathlon 2026 - Datasets/`.

## Task 2B — peak-day allocation (done)

`task2b_solve.py` is a greedy solver for `task2b_peak_day_scenarios.csv` +
`task2b_peak_day_fleet.csv`. It imports `trip_time()` and the trip-budget
constants directly from the official `check_allocation.py`, so the solver
can never drift from the validator's formula.

Approach: group orders by `(district, brand)` (every rule in the checker is
per-trip-per-brand-per-district, so this grouping is exact, not a heuristic
choice), priority-order orders within a group the same way Waypoint's own
planner does (prior deferral first, then days since last served), then
bin-pack each group onto available vehicles — largest capacity first,
vans prioritized for their van-only orders — respecting weight/volume
capacity and the two shared per-vehicle time budgets (270 min pre-dawn for
Fresh, 480 min daytime for everything else).

Run it, then validate with the official checker. The generated submission is
written to `datathon/output/` and checked in there as the produced artifact,
never into `Tech-Triathlon 2026 - Datasets/` — that directory is the official,
checked-in competition release and must stay byte-identical to what was given:

```bash
python3 -m venv /tmp/datathon-venv && /tmp/datathon-venv/bin/pip install pandas
/tmp/datathon-venv/bin/python3 datathon/task2b_solve.py
/tmp/datathon-venv/bin/python3 "Tech-Triathlon 2026 - Datasets/check_allocation.py" \
  datathon/output/submission_task2b.csv
```

Current result on the one scenario (S1, 85 orders): **73 served, 12 deferred,
0 feasibility errors**. The 12 deferrals were inspected manually — 11 are
large chilled Fresh orders competing for the scenario's 8 reefer-capable
vehicles under the tight 270-minute pre-dawn budget; the 12th
(`S1-078`, 40.66 m³) exceeds the largest vehicle in the entire fleet
(38.0 m³) and cannot be served by any single vehicle regardless of solver
quality.

This is a greedy heuristic, not a proven-optimal solver — a MILP/CP solver
(OR-Tools, PuLP) over the same grouping could very likely serve a few more
of the capacity-bound orders. Not attempted here; flagged as a follow-up if
the served-count score matters more than the time already spent.

## Task 1 — service time + late probability (not started)

Needs a join between `deliveries_train.csv` (`dispatch_status`,
`window_close_time`, ...) and `route_legs_train.csv` (`arrival_time`,
`leave_outlet_time`, ...) on `route_id` + `to_outlet == outlet_id` to derive
labels (`service_min = leave_outlet_time - arrival_time`,
`late = arrival_time > window_close_time`), then a regression + classifier
scored against `task1_test_inputs.csv`.

## Task 2A — weekly demand forecast (not started)

Needs `deliveries_train.csv` aggregated by `(depot, brand, iso_week)` and a
forecast model scored against `task2a_test_inputs.csv`'s
`(depot, brand, iso_year, iso_week)` rows.
