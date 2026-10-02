#!/usr/bin/env python3
"""Convert the official Tech-Triathlon 2026 reference CSVs into the
fixture-shaped CSVs that database/seed.sh and database/import.sh expect.

This is the "quick adapter" path (see README departures): it reshapes the
official files to fit Waypoint's current, simpler travel/service-time model
rather than upgrading that model to the official one. Two conversions are
lossy approximations, called out below and in database/seeds/README.md:

- district_travel.csv: the official file gives depot-to-district and
  inter-stop legs separately. We expand each row into three directed pairs
  (depot->district, district->depot, district->district) so the existing
  from/to minute lookup (travel.Estimator) covers the first leg, the return
  leg, and inter-stop legs within the same district. The depot->district and
  district->depot times are assumed symmetric; the official file does not
  give a return-trip figure.
- service_allowance.csv: the official file varies allowance by
  (brand, dock_type) - 9 combinations ranging 15-59 minutes. Waypoint's
  schema only supports two stop kinds (mall / default), so this collapses
  the 9 values into two unweighted averages. This under-serves brands at the
  slow end (Tech, Style) and over-serves Fresh. A later schema upgrade to
  brand x dock_type allowance would remove this approximation.

Usage:
    scripts/convert-official-dataset.py [--src DIR] [--dst DIR]

Defaults: --src "Tech-Triathlon 2026 - Datasets/data/General Data"
          --dst database/seeds
"""
import argparse
import csv
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def read_rows(path):
    with path.open(newline="", encoding="utf-8-sig") as f:
        return list(csv.DictReader(f))


def write_rows(path, fieldnames, rows):
    with path.open("w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=fieldnames)
        w.writeheader()
        for r in rows:
            w.writerow(r)


def convert_outlets(src, dst):
    rows = read_rows(src / "outlets.csv")
    out = []
    for r in rows:
        # The official file's mall_window is a time range ("10:00-12:30") or
        # blank, not a boolean. database/import.sh only recognises
        # '1'/'true'/'t'/'yes' as true, so passing the range through as-is
        # would make every mall outlet false here. The range itself is
        # dropped; window_open_time/window_close_time already carry the
        # actual times and are copied through unchanged below.
        out.append({
            "outlet_id": r["outlet_id"], "brand": r["brand"], "name": "",
            "district": r["district"], "depot": r["depot"], "dock_type": r["dock_type"],
            "parking_constraint": r["parking_constraint"],
            "mall_window": "true" if r.get("mall_window", "").strip() else "false",
            "window_open_time": r["window_open_time"], "window_close_time": r["window_close_time"],
        })
    write_rows(dst / "outlets.csv", ["outlet_id", "brand", "name", "district", "depot",
                                      "dock_type", "parking_constraint", "mall_window",
                                      "window_open_time", "window_close_time"], out)
    return len(out)


def convert_vehicles(src, dst):
    rows = read_rows(src / "vehicles.csv")
    out = []
    for r in rows:
        out.append({**r, "home_depot": r["depot"]})
    fields = ["vehicle_id", "type", "temp", "weight_cap_kg", "volume_cap_m3",
              "fuel_type", "km_per_l", "weekly_fuel_quota_l", "home_depot"]
    write_rows(dst / "vehicles.csv", fields, [{k: r[k] for k in fields} for r in out])
    return len(out)


def convert_calendar(src, dst):
    rows = read_rows(src / "calendar.csv")
    out = [{"date": r["date"], "is_operating": r["is_operating"]} for r in rows]
    write_rows(dst / "calendar.csv", ["date", "is_operating"], out)
    return len(out)


def convert_district_travel(src, dst):
    rows = read_rows(src / "district_travel.csv")
    n_official = len(rows)
    # Keyed by (from, to) so a depot whose name equals its district's name
    # (Kandy depot serves Kandy district) collapses to one row instead of a
    # validator-rejected duplicate pair. Inter-stop legs are written last so
    # they win that collision: a route has many inter-stop legs but only one
    # depot leg, so the more frequent case gets the more representative value.
    pairs = {}
    for r in rows:
        depot, district = r["depot"], r["district"]
        depot_km, depot_min = r["depot_to_district_km"], r["depot_to_district_freeflow_min"]
        stop_km, stop_min = r["inter_stop_km"], r["inter_stop_freeflow_min"]
        pairs[(depot, district)] = {"from_district": depot, "to_district": district, "km": depot_km, "minutes": depot_min}
        pairs[(district, depot)] = {"from_district": district, "to_district": depot, "km": depot_km, "minutes": depot_min}
        pairs[(district, district)] = {"from_district": district, "to_district": district, "km": stop_km, "minutes": stop_min}
    out = list(pairs.values())
    write_rows(dst / "district_travel.csv", ["from_district", "to_district", "km", "minutes"], out)
    return len(out), n_official


def convert_service_allowance(src, dst):
    rows = read_rows(src / "service_allowance.csv")
    mall = [float(r["service_allowance_min"]) for r in rows if r["dock_type"] == "mall_bay"]
    other = [float(r["service_allowance_min"]) for r in rows if r["dock_type"] != "mall_bay"]
    out = [
        {"stop_kind": "default", "minutes": round(sum(other) / len(other))},
        {"stop_kind": "mall", "minutes": round(sum(mall) / len(mall))},
    ]
    write_rows(dst / "service_allowance.csv", ["stop_kind", "minutes"], out)
    return len(out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--src", default=str(ROOT / "Tech-Triathlon 2026 - Datasets" / "data" / "General Data"))
    ap.add_argument("--dst", default=str(ROOT / "database" / "seeds"))
    args = ap.parse_args()
    src, dst = Path(args.src), Path(args.dst)
    dst.mkdir(parents=True, exist_ok=True)

    n_outlets = convert_outlets(src, dst)
    n_vehicles = convert_vehicles(src, dst)
    n_calendar = convert_calendar(src, dst)
    n_travel, n_travel_official = convert_district_travel(src, dst)
    n_allowance = convert_service_allowance(src, dst)

    print(f"converted {n_outlets} outlets, {n_vehicles} vehicles, {n_calendar} calendar dates, "
          f"{n_travel} travel pairs (expanded from {n_travel_official} official rows), "
          f"{n_allowance} service-allowance rows (collapsed from 9 brand x dock_type combinations)")
    print("NOTE: district_travel and service_allowance are lossy approximations of the official")
    print("      model - see the module docstring and database/seeds/README.md.")


if __name__ == "__main__":
    main()
