import test from "node:test";
import assert from "node:assert/strict";
import { pastArrivals } from "../src/dispatcher/planTiming.mjs";

const now = Date.parse("2026-10-20T10:23:00+05:30");

test("counts the planned arrivals that are already behind the clock", () => {
  const allocations = [
    { plannedArrivalAt: "2026-10-20T05:10:00+05:30" },
    { plannedArrivalAt: "2026-10-20T12:00:00+05:30" },
    { plannedArrivalAt: "" },
    {},
  ];
  assert.deepEqual(pastArrivals(allocations, now), { past: 1, total: 2 });
});

test("a plan for a future day has nothing in the past", () => {
  assert.deepEqual(pastArrivals([{ plannedArrivalAt: "2026-10-21T05:10:00+05:30" }], now), { past: 0, total: 1 });
  assert.deepEqual(pastArrivals(undefined, now), { past: 0, total: 0 });
});
