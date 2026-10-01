import assert from "node:assert/strict";
import test from "node:test";
import { shouldUseCachedDriverData } from "../src/driver/cacheFallback.mjs";

test("cached driver routes cover connectivity, service outages, and expired sessions", () => {
  assert.equal(shouldUseCachedDriverData(new TypeError("fetch failed")), true);
  assert.equal(shouldUseCachedDriverData({ status: 401 }), true);
  assert.equal(shouldUseCachedDriverData({ status: 500 }), true);
  assert.equal(shouldUseCachedDriverData({ status: 503 }), true);
});

test("cached driver routes do not mask authorization, validation, conflict, or missing-trip responses", () => {
  for (const status of [400, 403, 404, 409, 422]) {
    assert.equal(shouldUseCachedDriverData({ status }), false, `status ${status} must remain visible`);
  }
  assert.equal(shouldUseCachedDriverData(new Error("unknown failure")), false);
});
