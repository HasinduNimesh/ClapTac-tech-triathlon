import test from "node:test";
import assert from "node:assert/strict";
import { hasOpenShortfall, isChilledShortfall, openShortfallCount, shortfallLink, shortfallSeverity, tripsWithOpenShortfall } from "../src/dispatcher/shortfalls.mjs";

test("a decided shortfall no longer needs action", () => {
  assert.equal(hasOpenShortfall({ shortfallCount: 1, openShortfallCount: 0 }), false);
  assert.equal(hasOpenShortfall({ shortfallCount: 1, openShortfallCount: 1 }), true);
});

test("an older server without the open count falls back to the reported count", () => {
  assert.equal(openShortfallCount({ shortfallCount: 2 }), 2);
  assert.equal(hasOpenShortfall({}), false);
  assert.equal(hasOpenShortfall(null), false);
});

test("only a chilled short line is critical, not the vehicle type", () => {
  assert.equal(shortfallSeverity({ openShortfallCount: 1, openChilledShortfall: false, vehicleTemperatureCapability: "reefer" }), "high");
  assert.equal(shortfallSeverity({ openShortfallCount: 1, openChilledShortfall: true }), "critical");
  assert.equal(isChilledShortfall({}), false);
});

test("trips are listed chilled first, then by short lines, and decided ones are left out", () => {
  const trips = [
    { tripId: "a", openShortfallCount: 1 },
    { tripId: "b", openShortfallCount: 3 },
    { tripId: "c", openShortfallCount: 1, openChilledShortfall: true },
    { tripId: "d", shortfallCount: 2, openShortfallCount: 0 },
  ];
  assert.deepEqual(tripsWithOpenShortfall(trips).map((t) => t.tripId), ["c", "b", "a"]);
});

test("the review link carries the trip's date so a later day's trip can be decided", () => {
  assert.equal(shortfallLink({ deliveryDate: "2026-10-21" }), "/dispatcher/notifications?date=2026-10-21");
  assert.equal(shortfallLink({}, "2026-10-20"), "/dispatcher/notifications?date=2026-10-20");
  assert.equal(shortfallLink({}), "/dispatcher/notifications");
});
