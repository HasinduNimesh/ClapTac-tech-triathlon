import test from "node:test";
import assert from "node:assert/strict";
import {
  ackForTrip,
  acknowledgedForTrip,
  isOverdue,
  planLevelAcks,
  reminderForTrip,
  trackerRows,
  unacknowledgedTargets,
} from "../src/dispatcher/ackTracking.mjs";

const trips = [
  { id: "trip-1", vehicleId: "VEH001", tripNumber: 1 },
  { id: "trip-2", vehicleId: "VEH002", tripNumber: 1 },
];
const published = "2026-10-05T04:00:00Z";
const at = (minutesAfter) => Date.parse(published) + minutesAfter * 60000;

test("a driver acknowledgement of one trip does not acknowledge any other trip", () => {
  const acks = [{ actorId: "USR010", actorRole: "DRIVER", tripId: "trip-1", acknowledgedAt: "2026-10-05T04:03:00Z" }];
  assert.equal(ackForTrip(acks, "trip-1", "DRIVER")?.actorId, "USR010");
  assert.equal(ackForTrip(acks, "trip-2", "DRIVER"), undefined);
  const rows = trackerRows({ trips, acks, reminders: [], publishedAt: published, now: at(5), thresholdMinutes: 15 });
  assert.equal(rows[0].driver.state, "acknowledged");
  assert.equal(rows[1].driver.state, "pending");
});

test("loader acknowledgement is tracked per load list, separately from the driver", () => {
  const acks = [{ actorId: "USR012", actorRole: "LOADER", tripId: "trip-2", acknowledgedAt: "2026-10-05T04:04:00Z" }];
  const rows = trackerRows({ trips, acks, reminders: [], publishedAt: published, now: at(5), thresholdMinutes: 15 });
  assert.equal(rows[0].loader.state, "pending");
  assert.equal(rows[1].loader.state, "acknowledged");
  assert.equal(rows[1].driver.state, "pending", "a loader receipt must not mark the driver acknowledged");
});

test("legacy receipts with no trip are plan-level and never mark a trip acknowledged", () => {
  const acks = [{ actorId: "USR099", actorRole: "DRIVER", acknowledgedAt: "2026-10-05T04:01:00Z" }];
  assert.equal(ackForTrip(acks, "trip-1", "DRIVER"), undefined);
  assert.equal(planLevelAcks(acks).length, 1);
  const rows = trackerRows({ trips, acks, reminders: [], publishedAt: published, now: at(30), thresholdMinutes: 15 });
  assert.deepEqual(rows.map((r) => r.driver.state), ["overdue", "overdue"]);
});

test("unacknowledged recipients turn overdue after the threshold and expose a reminder target each", () => {
  assert.equal(isOverdue(published, at(14), 15), false);
  assert.equal(isOverdue(published, at(15), 15), true);
  assert.equal(isOverdue(undefined, at(99), 15), false);
  const acks = [{ actorId: "USR010", actorRole: "DRIVER", tripId: "trip-1", acknowledgedAt: "2026-10-05T04:03:00Z" }];
  const rows = trackerRows({ trips, acks, reminders: [], publishedAt: published, now: at(20), thresholdMinutes: 15 });
  assert.deepEqual(unacknowledgedTargets(rows), [
    { tripId: "trip-1", audience: "LOADER", state: "overdue" },
    { tripId: "trip-2", audience: "DRIVER", state: "overdue" },
    { tripId: "trip-2", audience: "LOADER", state: "overdue" },
  ]);
});

test("the persisted reminder time is shown per trip and audience (latest wins)", () => {
  const reminders = [
    { tripId: "trip-1", audience: "DRIVER", remindedAt: "2026-10-05T04:16:00Z" },
    { tripId: "trip-1", audience: "DRIVER", remindedAt: "2026-10-05T04:25:00Z" },
    { tripId: "trip-1", audience: "LOADER", remindedAt: "2026-10-05T04:20:00Z" },
    { tripId: "trip-2", audience: "DRIVER", remindedAt: "2026-10-05T04:18:00Z" },
  ];
  assert.equal(reminderForTrip(reminders, "trip-1", "DRIVER")?.remindedAt, "2026-10-05T04:25:00Z");
  assert.equal(reminderForTrip(reminders, "trip-2", "LOADER"), undefined);
  const rows = trackerRows({ trips, acks: [], reminders, publishedAt: published, now: at(30), thresholdMinutes: 15 });
  assert.equal(rows[0].driver.remindedAt, "2026-10-05T04:25:00Z");
  assert.equal(rows[0].loader.remindedAt, "2026-10-05T04:20:00Z");
  assert.equal(rows[1].loader.remindedAt, undefined);
});

test("the field apps treat only this user's receipt for this trip (or a legacy plan-level one) as acknowledged", () => {
  const acks = [
    { actorId: "USR010", actorRole: "DRIVER", tripId: "trip-1", acknowledgedAt: "x" },
    { actorId: "USR011", actorRole: "DRIVER", acknowledgedAt: "x" },
  ];
  assert.equal(acknowledgedForTrip(acks, { actorId: "USR010", role: "DRIVER", tripId: "trip-1" }), true);
  assert.equal(acknowledgedForTrip(acks, { actorId: "USR010", role: "DRIVER", tripId: "trip-2" }), false);
  assert.equal(acknowledgedForTrip(acks, { actorId: "USR010", role: "LOADER", tripId: "trip-1" }), false);
  assert.equal(acknowledgedForTrip(acks, { actorId: "USR011", role: "DRIVER", tripId: "trip-9" }), true);
  assert.equal(acknowledgedForTrip(acks, { actorId: undefined, role: "DRIVER", tripId: "trip-1" }), false);
});
