import assert from "node:assert/strict";
import test from "node:test";
import { formatCoordinates, locationChange, openStreetMapLink, openStreetMapSearchLink, parseCoordinates } from "../src/dispatcher/outletLocation.mjs";

test("coordinates copied from a map app are read", () => {
  for (const text of ["6.93441, 79.84281", "6.93441,79.84281", " 6.93441 79.84281 ", "6.93441;79.84281"]) {
    assert.deepEqual(parseCoordinates(text), { ok: true, latitude: 6.93441, longitude: 79.84281 }, text);
  }
});

test("swapped, foreign, partial and non-numeric positions are refused with a reason", () => {
  assert.match(parseCoordinates("79.84281, 6.93441").message, /swapped/);
  assert.match(parseCoordinates("51.5, -0.12").message, /not in Sri Lanka/);
  for (const text of ["", "6.9", "Colombo", "6.9, 79.8, 5", "6,9 79,8"]) {
    assert.equal(parseCoordinates(text).ok, false, text);
  }
});

test("an empty field keeps the position; removing an exact one is an explicit choice", () => {
  const approximate = { latitude: 6.9, longitude: 79.9, locationApproximate: true };
  const exact = { latitude: 6.93441, longitude: 79.84281, locationApproximate: false };
  assert.deepEqual(locationChange("", approximate), { change: "keep" });
  assert.deepEqual(locationChange("", exact), { change: "keep" }, "an emptied box must not remove a recorded position");
  assert.deepEqual(locationChange("   ", exact), { change: "keep" });
  assert.deepEqual(locationChange("", exact, true), { change: "clear" });
  assert.deepEqual(locationChange(formatCoordinates(6.93441, 79.84281), exact, true), { change: "clear" }, "the pre-filled value plus 'remove' is a removal");
  assert.equal(locationChange("6.9345, 79.8430", exact, true).change, "invalid", "a new value and 'remove' together is refused, not guessed");
  assert.deepEqual(locationChange("", approximate, true), { change: "keep" }, "nothing recorded, nothing to remove");
});

test("a typed position is set, unless it is the one already recorded", () => {
  const approximate = { latitude: 6.9, longitude: 79.9, locationApproximate: true };
  const exact = { latitude: 6.93441, longitude: 79.84281, locationApproximate: false };
  assert.deepEqual(locationChange(formatCoordinates(6.93441, 79.84281), exact), { change: "keep" });
  assert.deepEqual(locationChange("6.9345, 79.8430", exact), { change: "set", latitude: 6.9345, longitude: 79.843 });
  assert.deepEqual(locationChange("6.9345, 79.8430", approximate), { change: "set", latitude: 6.9345, longitude: 79.843 });
  assert.equal(locationChange("nonsense", exact).change, "invalid");
});

test("links open the point, or search the shop by name when there is no point", () => {
  assert.equal(openStreetMapLink(6.93441, 79.84281), "https://www.openstreetmap.org/?mlat=6.93441&mlon=79.84281#map=18/6.93441/79.84281");
  assert.equal(openStreetMapSearchLink("Fresh Pettah", "Colombo"), "https://www.openstreetmap.org/search?query=Fresh%20Pettah%2C%20Colombo%2C%20Sri%20Lanka");
});
