import assert from "node:assert/strict";
import test from "node:test";
import { offlineQueueHealth } from "../src/offline/queueHealth.mjs";

const hour = 60 * 60 * 1000;
const now = Date.parse("2026-09-30T12:00:00Z");
const itemAt = (hoursAgo, privateId = "secret-operation-id") => ({
  operationId: privateId,
  createdAt: new Date(now - hoursAgo * hour).toISOString(),
  tripId: "private-trip-id",
  stopId: "private-stop-id",
});

test("reports only bounded queue-age/count buckets, never record identifiers", () => {
  const report = offlineQueueHealth([itemAt(24), itemAt(1)], now);
  assert.deepEqual(report, { ageBucket: "1d_7d", countBucket: "2_5" });
  assert.equal(JSON.stringify(report).includes("private"), false);
  assert.equal(JSON.stringify(report).includes("secret-operation-id"), false);
});

test("uses stable age and count boundaries, including cleared queues", () => {
  assert.deepEqual(offlineQueueHealth([], now), { ageBucket: "none", countBucket: "none" });
  assert.equal(offlineQueueHealth([itemAt(23.9)], now).ageBucket, "lt24h");
  assert.equal(offlineQueueHealth([itemAt(24)], now).ageBucket, "1d_7d");
  assert.equal(offlineQueueHealth([itemAt(168)], now).ageBucket, "7d_30d");
  assert.equal(offlineQueueHealth([itemAt(720)], now).ageBucket, "30d_plus");
  assert.equal(offlineQueueHealth([itemAt(1), itemAt(2), itemAt(3), itemAt(4), itemAt(5)], now).countBucket, "2_5");
  assert.equal(offlineQueueHealth(Array.from({ length: 6 }, () => itemAt(1)), now).countBucket, "6_plus");
});

test("does not turn invalid or future timestamps into a false age", () => {
  assert.deepEqual(offlineQueueHealth([{ createdAt: "bad" }, itemAt(-3)], now), { ageBucket: "unknown", countBucket: "2_5" });
});
