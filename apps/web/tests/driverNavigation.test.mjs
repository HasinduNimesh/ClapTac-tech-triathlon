import assert from "node:assert/strict";
import test from "node:test";
import { navigationTarget } from "../src/driver/navigation.mjs";

test("an exact position is navigated to", () => {
  assert.deepEqual(navigationTarget({ latitude: 6.93441, longitude: 79.84281, locationApproximate: false, outletName: "Fresh Pettah" }), {
    exact: true,
    href: "https://www.google.com/maps/dir/?api=1&destination=6.93441,79.84281&travelmode=driving",
  });
  // The server omits the flag for exact positions; an absent flag is not "approximate".
  assert.equal(navigationTarget({ latitude: 6.9, longitude: 79.8 }).exact, true);
});

test("an approximate district position is never used as the destination", () => {
  const target = navigationTarget({ latitude: 6.9, longitude: 79.9, locationApproximate: true, outletName: "Fresh Pettah", district: "Colombo" });
  assert.equal(target.exact, false);
  assert.equal(target.href, "https://www.google.com/maps/search/?api=1&query=Fresh%20Pettah%2C%20Colombo%2C%20Sri%20Lanka");
  assert.ok(!target.href.includes("6.9"), "the approximate coordinates must not appear in the link");
});

test("a stop with no position searches by name, falling back to the outlet code", () => {
  assert.equal(navigationTarget({ outletName: "Style Kandy", district: "Kandy" }).href, "https://www.google.com/maps/search/?api=1&query=Style%20Kandy%2C%20Kandy%2C%20Sri%20Lanka");
  assert.equal(navigationTarget({ outletId: "OUT007" }).href, "https://www.google.com/maps/search/?api=1&query=OUT007%2C%20Sri%20Lanka");
  assert.equal(navigationTarget({ latitude: Number.NaN, longitude: 79.8, outletId: "OUT007" }).exact, false);
});
