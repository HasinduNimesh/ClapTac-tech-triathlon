#!/usr/bin/env python3
"""Validate fixture/competition CSV structure before importing it."""

import csv
import datetime as dt
from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[1] / "database" / "seeds"


def read_csv(name, required):
    path = ROOT / name
    if not path.exists():
        if required:
            raise ValueError(f"required seed file is missing: {name}")
        return []
    with path.open(newline="", encoding="utf-8-sig") as stream:
        reader = csv.DictReader(stream)
        if reader.fieldnames is None:
            raise ValueError(f"{name}: header row is missing")
        rows = list(reader)
    if not rows and required:
        raise ValueError(f"{name}: no data rows")
    return rows


def unique(rows, field, name):
    values = [row.get(field, "").strip() for row in rows]
    if any(not value for value in values):
        raise ValueError(f"{name}: {field} contains an empty value")
    if len(values) != len(set(values)):
        raise ValueError(f"{name}: {field} contains duplicate values")


def positive(row, field, name):
    try:
        value = float(row[field])
    except (KeyError, ValueError) as error:
        raise ValueError(f"{name}: {field} must be numeric") from error
    if value <= 0:
        raise ValueError(f"{name}: {field} must be positive")


def main():
    outlets = read_csv("outlets.csv", required=True)
    vehicles = read_csv("vehicles.csv", required=True)
    calendar = read_csv("calendar.csv", required=True)
    travel = read_csv("district_travel.csv", required=False)
    allowances = read_csv("service_allowance.csv", required=False)
    unique(outlets, "outlet_id", "outlets.csv")
    unique(vehicles, "vehicle_id", "vehicles.csv")
    unique(calendar, "date", "calendar.csv")

    outlet_ids = {row["outlet_id"].strip() for row in outlets}
    for required_id in ("OUT034", "OUT021"):
        if required_id not in outlet_ids:
            raise ValueError(f"demo account outlet {required_id} is missing")

    depots = {row["depot"].strip() for row in outlets if row.get("depot", "").strip()}
    for row in vehicles:
        depot = row.get("home_depot", "").strip()
        if not depot or depot not in depots:
            raise ValueError(f"vehicles.csv: {row.get('vehicle_id')} references unknown depot {depot!r}")
        for field in ("weight_cap_kg", "volume_cap_m3", "km_per_l", "weekly_fuel_quota_l"):
            positive(row, field, "vehicles.csv")

    for row in calendar:
        try:
            dt.date.fromisoformat(row["date"])
        except (KeyError, ValueError) as error:
            raise ValueError(f"calendar.csv: invalid date {row.get('date')!r}") from error
        if row.get("is_operating", "").strip().lower() not in {"true", "false", "t", "f", "1", "0", "yes", "no"}:
            raise ValueError(f"calendar.csv: invalid is_operating value {row.get('is_operating')!r}")

    pairs = [(row.get("from_district", ""), row.get("to_district", "")) for row in travel]
    if len(pairs) != len(set(pairs)):
        raise ValueError("district_travel.csv: duplicate district pair")
    for row in travel:
        positive(row, "km", "district_travel.csv")
        positive(row, "minutes", "district_travel.csv")
    for row in allowances:
        positive(row, "minutes", "service_allowance.csv")

    print(f"Seed validation passed: {len(outlets)} outlets, {len(vehicles)} vehicles, "
          f"{len(calendar)} operating-calendar dates, {len(travel)} travel rows, "
          f"{len(allowances)} service-allowance rows.")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError) as error:
        print(f"Seed validation failed: {error}", file=sys.stderr)
        sys.exit(1)
