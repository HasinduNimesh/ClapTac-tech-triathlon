import assert from "node:assert/strict";
import test from "node:test";
import { splitTrackingResults } from "../src/store-manager/trackingResults.mjs";

const order = (ref, date) => ({ id: ref, orderRef: ref, requestedDeliveryDate: date });
const ok = (o) => ({ status: "fulfilled", value: { stage: "CONFIRMED", order: o } });
const failed = { status: "rejected", reason: new Error("planning unavailable") };

test("an order whose tracking cannot be loaded is kept, not dropped", () => {
  const a = order("ORD000009", "2026-10-03");
  const b = order("ORD000010", "2026-10-05");
  const { rows, unavailable } = splitTrackingResults([a, b], [ok(a), failed]);
  assert.deepEqual(rows.map((r) => r.order.orderRef), ["ORD000009"]);
  assert.deepEqual(unavailable.map((o) => o.orderRef), ["ORD000010"]);
});

test("when everything loads nothing is unavailable, newest first", () => {
  const a = order("ORD000009", "2026-10-03");
  const b = order("ORD000010", "2026-10-05");
  const { rows, unavailable } = splitTrackingResults([a, b], [ok(a), ok(b)]);
  assert.deepEqual(rows.map((r) => r.order.orderRef), ["ORD000010", "ORD000009"]);
  assert.deepEqual(unavailable, []);
});

test("when tracking is down for every order, every order is still listed", () => {
  const orders = [order("ORD000009", "2026-10-03"), order("ORD000010", "2026-10-05")];
  const { rows, unavailable } = splitTrackingResults(orders, [failed, failed]);
  assert.equal(rows.length, 0);
  assert.deepEqual(unavailable.map((o) => o.orderRef), ["ORD000010", "ORD000009"]);
});
