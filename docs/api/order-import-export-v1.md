# Order import/export v1

These adapters let a dispatcher or an ERP/WMS integration exchange confirmed orders using a stable, versioned contract. All routes are under `/api/v1/orders` and require a bearer token with dispatcher order-view-all permission. Imports are limited to 500 rows and a 2 MiB request body.

## CSV

`GET /export.csv` returns `text/csv` with the download name `waypoint-orders-v1.csv`. It exports the orders visible to the dispatcher. When an order has no source-system external ID, its Waypoint order reference is used in the `external_order_id` column.

`POST /import.csv?version=1&sourceSystem={id}` accepts `Content-Type: text/csv`. `sourceSystem` is required, 1–40 ASCII letters/digits/underscore/hyphen. The header must match this exact order and spelling:

```csv
version,external_order_id,outlet_id,brand,requested_delivery_date,order_units,order_weight_kg,order_volume_m3,temperature_requirement
1,ERP-100,OUT034,Fresh,2026-10-05,10,20,1,ambient
```

Rows use version `1`. Dates use `YYYY-MM-DD`; units are positive integers; weight and volume are positive numbers; temperature is `ambient` or `chilled`. The outlet must exist and the optional brand must match that outlet's configured brand. The entire file is parsed and validated before persistence. Duplicate external IDs within one file are rejected.

## JSON API

`POST /import?version=1` accepts one JSON object. The request is limited to 2 MiB and rejects unknown fields and trailing JSON values.

```json
{
  "version": 1,
  "sourceSystem": "erp",
  "orders": [{
    "externalOrderId": "ERP-100",
    "outletId": "OUT034",
    "brand": "Fresh",
    "requestedDeliveryDate": "2026-10-05",
    "orderUnits": 10,
    "orderWeightKg": 20,
    "orderVolumeM3": 1,
    "temperatureRequirement": "ambient"
  }]
}
```

`GET /export?version=1` returns `{ "version": 1, "items": [...] }` using the order-service JSON field names. Unknown API versions are rejected.

## Replay and errors

The idempotency key is the pair `(sourceSystem, externalOrderId)`. An identical replay is reported as a duplicate and does not create another order. Reusing the same key with changed order data returns HTTP 409 and rolls back the import batch. Invalid headers, versions, rows, mappings, duplicate IDs, or quantities return HTTP 400; missing dispatcher authorization returns HTTP 403. Successful import returns HTTP 200 with `version`, `sourceSystem`, `created`, `duplicates`, and per-order `items` including whether each row was created.

The contract is Waypoint's v1 adapter, not an official competition or ERP/WMS schema. Map partner fields into this contract at the integration boundary; obtain and validate the partner's actual schema before claiming compatibility.
