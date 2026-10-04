# Outlet locations

Where each outlet is drives navigation for drivers and the maps dispatchers and store managers see.

## What is stored

`shared.outlets` has nullable `latitude` and `longitude` (migration `0043_outlet_locations.sql`). The official
dataset has no coordinates, so until a dispatcher records one an outlet is shown at its **district centre plus a
small deterministic offset** and every response says `locationApproximate: true`. Recorded positions come back with
`locationApproximate: false`.

An approximate position is fine for an overview. It is **not** used to navigate: the driver app and the web driver
page search for the shop by name instead, and say the exact location is not recorded.

## Recording a position

Master data → *Delivery location* (dispatchers):

1. Open the shop in a map app and copy its latitude and longitude (the pair a map app copies, `6.93441, 79.84281`).
2. Paste them into *Exact location* and save. *Check this point on OpenStreetMap* opens the point so you can confirm it is the shop.
3. To go back to the approximate position, tick *Remove the exact location* and save. An empty or unchanged field always keeps the current position.

Positions outside Sri Lanka, swapped values and half a pair are refused. Each change is a versioned, audited outlet
update; history records the exact position before and after.

Master data also shows how many outlets have an exact location.

### Many outlets at once

Collect the positions in a spreadsheet and export CSV with the columns `outlet_id,latitude,longitude`:

```
WAYPOINT_TOKEN=<a dispatcher's access token> scripts/import-outlet-locations.py outlets.csv          # check only
WAYPOINT_TOKEN=<...>                         scripts/import-outlet-locations.py outlets.csv --apply  # save
```

It goes through the same API (so it is audited and respects versions), changes nothing without `--apply`, reads the
token from the environment and never prints it. The default API is `https://waypoint.claptac.dev/api/v1`;
`--api` or `WAYPOINT_API_BASE` changes it.

## API

- `PUT /api/v1/shared/outlets/{id}` accepts `"location": {"latitude": .., "longitude": ..}` to set, `"location": null`
  to clear, or no `location` to leave it. `latitude`/`longitude` echoed at the top level are ignored (on a read they
  may be approximate).
- `GET /api/v1/delivery/trips/{id}`: each stop carries `latitude`, `longitude` and `locationApproximate`, read from the
  outlet when the trip is served (not stored, so a corrected position reaches trips already prepared within about 30
  seconds). Stops omit the position if shared-service cannot be reached; apps then search by name.

## Where it is used

| App | What it does with a position |
|---|---|
| Driver phone app | *Open in maps* on the stop screen: directions to an exact position; otherwise a search by shop name with a note. Saved with the route for offline use |
| Web driver page | *Directions* on the stop: the same rule |
| Dispatcher web (Planning, Live operations, Master data) | Maps and the location editor |
| Store manager web (Track order) | The store on the delivery map |
| Loader app | Not used: loaders work at the depot and do not navigate |

## Not built

- **GPS.** Apps do not read the phone's position, so a truck on the map is shown at its last reported stop, and nobody can see where it is between stops.
- **Planning still uses district-to-district travel times** (`district_travel`), not these coordinates, so exact positions do not yet change route sequence or arrival estimates.
- **A shop category** beyond brand (Fresh, Style, Tech). Brand is the only label; add a separate field if more is needed.
- The positions themselves. The live outlets still need to be collected and verified.
