#!/usr/bin/env python3
"""Records verified outlet locations from a CSV through the Waypoint API.

The outlets only have approximate positions (their district centre) until a dispatcher records the real
one. Collect them in a spreadsheet, export CSV with the columns

    outlet_id,latitude,longitude

(extra columns are ignored), then:

    WAYPOINT_TOKEN=<a dispatcher's access token> scripts/import-outlet-locations.py outlets.csv           # check only
    WAYPOINT_TOKEN=<...>                         scripts/import-outlet-locations.py outlets.csv --apply   # save

Each save goes through the same versioned, audited PUT /shared/outlets/{id} the Master data page uses,
so history shows who set each position. Nothing is changed without --apply. The token is read from the
environment, never from the command line, and is never printed. Rows outside Sri Lanka, with swapped or
missing numbers, or repeated outlet ids stop the run before anything is sent.
"""
import argparse
import csv
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

LATITUDE_RANGE = (5.5, 10.0)
LONGITUDE_RANGE = (79.3, 82.2)
DEFAULT_API = "https://waypoint.claptac.dev/api/v1"


def in_sri_lanka(latitude, longitude):
    return LATITUDE_RANGE[0] <= latitude <= LATITUDE_RANGE[1] and LONGITUDE_RANGE[0] <= longitude <= LONGITUDE_RANGE[1]


def read_rows(handle):
    """Returns (rows, problems). rows is [(outlet_id, latitude, longitude)]; problems are messages naming the CSV line."""
    reader = csv.DictReader(handle)
    fields = {(name or "").strip().lower() for name in (reader.fieldnames or [])}
    missing = {"outlet_id", "latitude", "longitude"} - fields
    if missing:
        return [], [f"The CSV needs the columns outlet_id, latitude, longitude (missing: {', '.join(sorted(missing))})."]
    rows, problems, seen = [], [], {}
    for line, raw in enumerate(reader, start=2):
        row = {(key or "").strip().lower(): (value or "").strip() for key, value in raw.items()}
        if not any(row.values()):
            continue
        outlet = row.get("outlet_id", "")
        where = f"line {line} ({outlet or 'no outlet_id'})"
        if not outlet:
            problems.append(f"{where}: outlet_id is empty")
            continue
        if outlet in seen:
            problems.append(f"{where}: repeats the outlet on line {seen[outlet]}")
            continue
        seen[outlet] = line
        try:
            latitude, longitude = float(row["latitude"]), float(row["longitude"])
        except ValueError:
            problems.append(f"{where}: latitude and longitude must be numbers")
            continue
        if not in_sri_lanka(latitude, longitude):
            if in_sri_lanka(longitude, latitude):
                problems.append(f"{where}: latitude and longitude look swapped (latitude comes first)")
            else:
                problems.append(f"{where}: {latitude}, {longitude} is not in Sri Lanka")
            continue
        rows.append((outlet, latitude, longitude))
    return rows, problems


def call(method, url, token, body=None):
    """Returns (status, parsed_json_or_None). Never raises on an HTTP error status."""
    data = json.dumps(body).encode() if body is not None else None
    headers = {"Authorization": f"Bearer {token}", "Accept": "application/json"}
    if data is not None:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            status, payload = response.status, response.read()
    except urllib.error.HTTPError as error:
        status, payload = error.code, error.read()
    try:
        return status, json.loads(payload) if payload else None
    except ValueError:
        return status, None


def update_body(outlet, latitude, longitude):
    """The PUT body for an outlet as GET returned it, with the new location. Times lose their seconds, as the API expects."""
    body = {
        "name": outlet.get("name", ""),
        "brand": outlet.get("brand", ""),
        "district": outlet.get("district", ""),
        "depot": outlet.get("depot", ""),
        "dockType": outlet.get("dockType", ""),
        "parkingConstraint": outlet.get("parkingConstraint", ""),
        "mallWindow": bool(outlet.get("mallWindow")),
        "windowOpenTime": (outlet.get("windowOpenTime") or "")[:5],
        "windowCloseTime": (outlet.get("windowCloseTime") or "")[:5],
        "accessInstructions": outlet.get("accessInstructions", ""),
        "chilledTemperatureMinC": outlet.get("chilledTemperatureMinC"),
        "chilledTemperatureMaxC": outlet.get("chilledTemperatureMaxC"),
        "version": outlet.get("version"),
        "location": {"latitude": latitude, "longitude": longitude},
    }
    return body


def run(rows, api, token, apply, out=print):
    """Returns (saved, unchanged, failed)."""
    saved = unchanged = failed = 0
    base = api.rstrip("/")
    for outlet_id, latitude, longitude in rows:
        url = f"{base}/shared/outlets/{urllib.parse.quote(outlet_id, safe='')}"
        status, payload = call("GET", url, token)
        if status != 200 or not isinstance(payload, dict) or "outlet" not in payload:
            out(f"  [FAIL] {outlet_id}: could not read the outlet (HTTP {status})")
            failed += 1
            continue
        outlet = payload["outlet"]
        if outlet.get("locationApproximate") is False and outlet.get("latitude") == latitude and outlet.get("longitude") == longitude:
            out(f"  [same] {outlet_id}: already {latitude}, {longitude}")
            unchanged += 1
            continue
        if not apply:
            out(f"  [would set] {outlet_id}: {latitude}, {longitude}")
            saved += 1
            continue
        status, result = call("PUT", url, token, update_body(outlet, latitude, longitude))
        if status == 200:
            out(f"  [set] {outlet_id}: {latitude}, {longitude}")
            saved += 1
        elif status == 409:
            out(f"  [FAIL] {outlet_id}: changed by someone else while importing; run again")
            failed += 1
        else:
            out(f"  [FAIL] {outlet_id}: HTTP {status}")
            failed += 1
    return saved, unchanged, failed


def main(argv=None):
    parser = argparse.ArgumentParser(description="Record verified outlet locations from a CSV.")
    parser.add_argument("csv_file")
    parser.add_argument("--api", default=os.environ.get("WAYPOINT_API_BASE", DEFAULT_API), help=f"API base (default {DEFAULT_API})")
    parser.add_argument("--apply", action="store_true", help="save the locations (default: only check)")
    args = parser.parse_args(argv)
    token = os.environ.get("WAYPOINT_TOKEN", "").strip()
    if not token:
        sys.exit("Set WAYPOINT_TOKEN to a dispatcher's access token (it is read from the environment so it is never on the command line).")
    with open(args.csv_file, newline="", encoding="utf-8-sig") as handle:
        rows, problems = read_rows(handle)
    if problems:
        print("The CSV has problems; nothing was sent:")
        for problem in problems:
            print(f"  - {problem}")
        return 2
    print(f"{len(rows)} outlet(s) to {'save' if args.apply else 'check'} against {args.api}")
    saved, unchanged, failed = run(rows, args.api, token, args.apply)
    verb = "saved" if args.apply else "would be saved"
    print(f"{saved} {verb}, {unchanged} already correct, {failed} failed.")
    if not args.apply and saved:
        print("Nothing was changed. Run again with --apply to save.")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
