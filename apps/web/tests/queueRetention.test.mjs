import assert from "node:assert/strict";
import test from "node:test";
import { canPurgeCompletedCache, queueRetentionWarning } from "../src/offline/queueRetention.mjs";

const day = 24 * 60 * 60 * 1000;
const now = Date.parse("2026-09-30T12:00:00Z");
const itemAt = (daysAgo) => ({ createdAt: new Date(now - daysAgo * day).toISOString() });

test("does not warn for recent work and uses the oldest queued event", () => {
  assert.equal(queueRetentionWarning([itemAt(0.5)], now), null);
  assert.equal(queueRetentionWarning([itemAt(0.2), itemAt(1.2)], now).level, "warning");
});

test("escalates stale queue guidance at seven and thirty days without deleting anything", () => {
  assert.equal(queueRetentionWarning([itemAt(6.9)], now).level, "warning");
  assert.equal(queueRetentionWarning([itemAt(7)], now).level, "urgent");
  const stale = queueRetentionWarning([itemAt(30)], now);
  assert.equal(stale.level, "critical");
  assert.match(stale.text, /will not be deleted automatically/);
});

test("ignores invalid timestamps and future timestamps instead of showing false urgency", () => {
  assert.equal(queueRetentionWarning([{ createdAt: "invalid" }, { createdAt: new Date(now + day).toISOString() }], now), null);
});

test("completed cache reaches the seven-day purge threshold only after server confirmation and with no queued work", () => {
  const completedAt = new Date(now - 7 * day).toISOString();
  assert.equal(canPurgeCompletedCache({ serverConfirmedAt: completedAt, hasPendingQueue: false }, now), true);
  assert.equal(canPurgeCompletedCache({ serverConfirmedAt: completedAt, hasPendingQueue: true }, now), false);
  assert.equal(canPurgeCompletedCache({ serverConfirmedAt: new Date(now - 6 * day).toISOString(), hasPendingQueue: false }, now), false);
  assert.equal(canPurgeCompletedCache({ serverConfirmedAt: "", hasPendingQueue: false }, now), false);
});
