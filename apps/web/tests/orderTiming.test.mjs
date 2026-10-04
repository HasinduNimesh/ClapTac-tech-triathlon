import assert from "node:assert/strict";
import test from "node:test";
import { splitByDeliveryDate } from "../src/dispatcher/orderTiming.mjs";

const orders = ["2026-09-30", "2026-09-30", "2026-10-02", "2026-10-03", "2026-10-04", "2026-10-05"].map((requestedDeliveryDate) => ({ requestedDeliveryDate }));

test("waiting orders are split into overdue, due today and later", () => {
  assert.deepEqual(splitByDeliveryDate(orders, "2026-10-04"), { overdue: 4, dueToday: 1, later: 1 });
});

test("nothing waiting, or orders with no date, count as nothing", () => {
  assert.deepEqual(splitByDeliveryDate([], "2026-10-04"), { overdue: 0, dueToday: 0, later: 0 });
  assert.deepEqual(splitByDeliveryDate(undefined, "2026-10-04"), { overdue: 0, dueToday: 0, later: 0 });
  assert.deepEqual(splitByDeliveryDate([{}], "2026-10-04"), { overdue: 0, dueToday: 0, later: 0 });
});
