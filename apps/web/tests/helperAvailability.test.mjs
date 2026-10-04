import test from "node:test";
import assert from "node:assert/strict";
import { helperStatusFrom, isHelperUnavailable } from "../src/store-manager/helperAvailability.mjs";

test("only an explicit true switches a helper on", () => {
  assert.deepEqual(helperStatusFrom({ orderAssistant: true, dashboardAssistant: true }), { order: true, dashboard: true });
  assert.deepEqual(helperStatusFrom({ orderAssistant: false, dashboardAssistant: false }), { order: false, dashboard: false });
  assert.deepEqual(helperStatusFrom({}), { order: false, dashboard: false });
  assert.deepEqual(helperStatusFrom(null), { order: false, dashboard: false });
  assert.deepEqual(helperStatusFrom({ orderAssistant: "true", dashboardAssistant: 1 }), { order: false, dashboard: false });
});

test("a 503 or agent_unavailable means the helper is off, other failures do not", () => {
  assert.equal(isHelperUnavailable({ status: 503, message: "" }), true);
  assert.equal(isHelperUnavailable({ status: 502, message: '{"detail":"agent_unavailable: fill in the form by hand"}' }), true);
  assert.equal(isHelperUnavailable({ status: 500, message: "boom" }), false);
  assert.equal(isHelperUnavailable(new TypeError("Failed to fetch")), false);
  assert.equal(isHelperUnavailable(null), false);
  assert.equal(isHelperUnavailable("agent_unavailable"), false);
});
