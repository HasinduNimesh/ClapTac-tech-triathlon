"""
task2b_solve.py - greedy solver for the Task 2B peak-day allocation scenario.

Reads task2b_peak_day_scenarios.csv + task2b_peak_day_fleet.csv and writes a
submission_task2b.csv (scenario, order_ref, decision, vehicle_id, trip_id)
that satisfies every rule check_allocation.py enforces:

  - one brand and one district per trip
  - vehicle based at the order's depot
  - reefer required for any chilled order on a trip
  - van required for any van_only outlet on a trip
  - vehicle weight/volume capacity
  - at most 2 trips per vehicle per day
  - Fresh trips share a 270-minute pre-dawn budget; all other brands'
    trips share a 480-minute daytime budget (both per vehicle)

It imports trip_time() and the budget constants directly from
check_allocation.py so the solver can never drift from the official
validator's formula.

This is a greedy heuristic (bin-pack each (district, brand) group of orders,
priority-sorted by deferred_yesterday then days_since_last_served, onto
available vehicles respecting capacity and the two time budgets), not a
proven-optimal solver. It is scored by how many orders it serves while
staying 100% feasible per check_allocation.py - run that script on the
output to confirm.

Usage:
    datathon/task2b_solve.py [--data DIR] [--out FILE]

Defaults: --data "Tech-Triathlon 2026 - Datasets/data"
          --out  "Tech-Triathlon 2026 - Datasets/data/Submission Templates/submission_task2b.csv"
"""
import argparse
import csv
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CHECKER_DIR = ROOT / "Tech-Triathlon 2026 - Datasets"
sys.path.insert(0, str(CHECKER_DIR))

import pandas as pd  # noqa: E402
from check_allocation import (  # noqa: E402
    TRIP_BUDGET_DAYTIME,
    TRIP_BUDGET_PREDAWN,
    MAX_TRIPS_PER_VEHICLE,
    trip_time,
)


class VehicleState:
    """Tracks one vehicle's trips across the whole solve."""

    def __init__(self, vehicle_id, vtype, temp, weight_cap, volume_cap, depot):
        self.vehicle_id = vehicle_id
        self.type = vtype
        self.temp = temp
        self.weight_cap = weight_cap
        self.volume_cap = volume_cap
        self.depot = depot
        self.trips_used = 0
        self.fresh_minutes = 0.0
        self.other_minutes = 0.0

    def budget_left(self, brand):
        if brand == "Fresh":
            return TRIP_BUDGET_PREDAWN - self.fresh_minutes
        return TRIP_BUDGET_DAYTIME - self.other_minutes

    def has_trip_slot(self):
        return self.trips_used < MAX_TRIPS_PER_VEHICLE

    def commit_trip(self, brand, minutes):
        self.trips_used += 1
        if brand == "Fresh":
            self.fresh_minutes += minutes
        else:
            self.other_minutes += minutes


def pack_group(orders, vehicles, dtravel, allowance, district):
    """Greedily packs one (district, brand) group's orders onto vehicle trip
    slots. Returns {order_ref: (vehicle_id, trip_id)} for served orders."""
    assigned = {}
    brand = orders[0]["brand"]
    remaining = list(orders)

    for veh in vehicles:
        if not remaining:
            break
        if not veh.has_trip_slot():
            continue

        budget = veh.budget_left(brand)
        if budget <= 0:
            continue

        trip_orders = []
        trip_weight = trip_volume = 0.0
        trip_needs_reefer = False
        # Vans are the scarcest resource: when this is a van, pack its
        # van-only orders first so they are not crowded out by orders that
        # could just as easily ride a truck on a later trip.
        candidates = remaining
        if veh.type == "van":
            candidates = sorted(remaining, key=lambda o: o["parking_constraint"] != "van_only")
        for order in list(candidates):
            if order["parking_constraint"] == "van_only" and veh.type != "van":
                continue
            if order["temp_requirement"] == "chilled" and veh.temp != "reefer":
                continue
            new_weight = trip_weight + order["order_weight_kg"]
            new_volume = trip_volume + order["order_volume_m3"]
            if new_weight > veh.weight_cap + 1e-6 or new_volume > veh.volume_cap + 1e-6:
                continue
            docks = [o["dock_type"] for o in trip_orders] + [order["dock_type"]]
            minutes = trip_time(district, brand, docks, dtravel, allowance)
            if minutes > budget + 1e-6:
                continue
            trip_orders.append(order)
            trip_weight, trip_volume = new_weight, new_volume
            if order["temp_requirement"] == "chilled":
                trip_needs_reefer = True

        if not trip_orders:
            continue
        if trip_needs_reefer and veh.temp != "reefer":
            continue  # shouldn't happen given the per-order check above

        trip_id = veh.trips_used + 1
        minutes = trip_time(district, brand, [o["dock_type"] for o in trip_orders], dtravel, allowance)
        veh.commit_trip(brand, minutes)
        for order in trip_orders:
            assigned[order["order_ref"]] = (veh.vehicle_id, trip_id)
            remaining.remove(order)

    return assigned


def solve(scn_path, fleet_path, vehicles_path, travel_path, allowance_path):
    scn = pd.read_csv(scn_path)
    fleet = pd.read_csv(fleet_path)
    veh_df = pd.read_csv(vehicles_path).set_index("vehicle_id")
    dtravel = pd.read_csv(travel_path).set_index("district").to_dict("index")
    al = pd.read_csv(allowance_path)
    allowance = {(r.brand, r.dock_type): r.service_allowance_min for r in al.itertuples()}

    rows = []
    for scenario, scn_group in scn.groupby("scenario"):
        # Sorted, not just a set: Python's set iteration order for strings is
        # not stable across processes, and the later weight-capacity sort is
        # stable, so an unsorted input order here would let ties among
        # equal-capacity vehicles resolve differently on every run, rewriting
        # the checked-in submission with different (but equally feasible)
        # vehicle assignments.
        avail_ids = sorted(fleet[(fleet.scenario == scenario) & (fleet.status == "available")].vehicle_id)
        vehicles = [
            VehicleState(vid, veh_df.loc[vid, "type"], veh_df.loc[vid, "temp"],
                         veh_df.loc[vid, "weight_cap_kg"], veh_df.loc[vid, "volume_cap_m3"],
                         veh_df.loc[vid, "depot"])
            for vid in avail_ids
        ]
        # Priority: deferred yesterday first, then longest-unserved - mirrors
        # Waypoint's own fairness score (prior deferral, then days unserved).
        scn_group = scn_group.sort_values(["deferred_yesterday", "days_since_last_served"], ascending=False)
        orders = scn_group.to_dict("records")

        groups = {}
        for o in orders:
            groups.setdefault((o["district"], o["brand"]), []).append(o)

        assigned = {}
        # Groups with a van-only requirement go first, while the (scarce)
        # van fleet still has both trip slots open.
        ordered_keys = sorted(groups.keys(), key=lambda k: -sum(
            1 for o in groups[k] if o["parking_constraint"] == "van_only"))
        for (district, brand) in ordered_keys:
            group_orders = groups[(district, brand)]
            group_depot = group_orders[0]["depot"]
            group_vehicles = sorted(
                (v for v in vehicles if v.depot == group_depot),
                key=lambda v: v.weight_cap, reverse=True,
            )
            assigned.update(pack_group(group_orders, group_vehicles, dtravel, allowance, district))

        for o in orders:
            ref = o["order_ref"]
            if ref in assigned:
                vid, trip_id = assigned[ref]
                rows.append({"scenario": scenario, "order_ref": ref, "decision": "served",
                             "vehicle_id": vid, "trip_id": trip_id})
            else:
                rows.append({"scenario": scenario, "order_ref": ref, "decision": "deferred",
                             "vehicle_id": "", "trip_id": ""})

    return pd.DataFrame(rows, columns=["scenario", "order_ref", "decision", "vehicle_id", "trip_id"])


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", default=str(CHECKER_DIR / "data"))
    ap.add_argument("--out", default=str(CHECKER_DIR / "data" / "Submission Templates" / "submission_task2b.csv"))
    args = ap.parse_args()
    data = Path(args.data)

    out = solve(
        data / "Test Data" / "task2b_peak_day_scenarios.csv",
        data / "Test Data" / "task2b_peak_day_fleet.csv",
        data / "General Data" / "vehicles.csv",
        data / "General Data" / "district_travel.csv",
        data / "General Data" / "service_allowance.csv",
    )
    out.to_csv(args.out, index=False)
    served = (out.decision == "served").sum()
    print(f"wrote {len(out)} rows to {args.out}: {served} served, {len(out) - served} deferred")


if __name__ == "__main__":
    main()
