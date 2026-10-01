import assert from "node:assert/strict";
import test from "node:test";
import { resolveFuelAttempt } from "../src/dispatcher/fuelSubmission.mjs";

test("a retry of the same fuel entry reuses its idempotency key", () => {
  const entry = { vehicleId: "VEH-1", date: "2026-09-30", liters: "12.500", receiptRef: " pump 004 " };
  const first = resolveFuelAttempt(null, entry, () => "attempt-1");
  const retry = resolveFuelAttempt(first, { ...entry, liters: 12.5, receiptRef: "pump 004" }, () => "attempt-2");
  assert.equal(retry, first);
  assert.equal(retry.operationId, "attempt-1");
});

test("a materially changed fuel entry receives a new idempotency key", () => {
  const entry = { vehicleId: "VEH-1", date: "2026-09-30", liters: 12.5, receiptRef: "pump 004" };
  const first = resolveFuelAttempt(null, entry, () => "attempt-1");
  const changed = resolveFuelAttempt(first, { ...entry, liters: 13 }, () => "attempt-2");
  assert.equal(changed.operationId, "attempt-2");
});
