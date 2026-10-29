import test from "node:test";
import assert from "node:assert/strict";
import { browseOrders } from "../src/store-manager/orderBrowser.mjs";

const row = (id, createdAt, requestedDeliveryDate, stage = "CONFIRMED", goods = "ambient") => ({
  stage, order: { id, orderRef: `ORD${id}`, createdAt, requestedDeliveryDate, temperatureRequirement: goods },
});
const ids = (rows) => rows.map(({ order }) => order.id);

test("newest means creation time, independent of delivery date and API order", () => {
  const rows = [row("2", "2026-10-01T10:00:00Z", "2026-12-01"), row("10", "2026-10-03T10:00:00Z", "2026-11-10"), row("3", "2026-10-02T10:00:00Z", "2026-11-01")];
  assert.deepEqual(ids(browseOrders(rows)), ["10", "3", "2"]);
  assert.deepEqual(ids(browseOrders(rows, { sort: "oldest" })), ["2", "3", "10"]);
  assert.deepEqual(ids(browseOrders(rows, { sort: "delivery" })), ["3", "10", "2"]);
  assert.deepEqual(ids(browseOrders(rows, { sort: "reference" })), ["2", "3", "10"]);
  assert.deepEqual(ids(rows), ["2", "10", "3"]);
});

test("equal dates have deterministic natural reference ordering after a refresh", () => {
  const rows = [row("2", "2026-10-01", "2026-10-02"), row("10", "2026-10-01", "2026-10-02")];
  assert.deepEqual(ids(browseOrders(rows)), ["10", "2"]);
  assert.deepEqual(ids(browseOrders([...rows].reverse())), ["10", "2"]);
});

test("legacy and malformed dates stay last in either chronological direction", () => {
  const rows = [row("2", undefined, ""), row("3", "invalid", "invalid"), row("1", "2026-10-01", "2026-10-02")];
  for (const sort of ["newest", "oldest", "delivery"]) assert.equal(browseOrders(rows, { sort })[0].order.id, "1");
});

test("status, goods and search filters combine without changing source rows", () => {
  const rows = [row("1", "2026-10-01", "2026-10-02", "DEFERRED", "chilled"), row("2", "2026-10-01", "2026-10-02", "DELIVERED", "chilled"), row("3", "2026-10-01", "2026-10-02", "DEFERRED")];
  assert.deepEqual(ids(browseOrders(rows, { stage: "DEFERRED", goods: "chilled", query: "  ord1 " })), ["1"]);
  assert.equal(browseOrders(rows, { stage: "DEFERRED", goods: "chilled", query: "ord2" }).length, 0);
  assert.equal(browseOrders(rows).length, 3);
  assert.deepEqual(browseOrders([], { query: "ord" }), []);
});
