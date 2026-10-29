import test from "node:test";
import assert from "node:assert/strict";
import { browseVehicles, vehicleNumber } from "../src/dispatcher/vehicleBrowser.mjs";

const fleet = [
  { id: "WP_CAB_10", weightCapacityKg: 1500, volumeCapacityM3: 12 },
  { id: "WP_CAB_2", weightCapacityKg: 6000, volumeCapacityM3: 30 },
  { id: "WP_CAB_1", weightCapacityKg: 1500, volumeCapacityM3: 10 },
];
const ids = (rows) => rows.map((v) => v.id);

test("display replaces underscores with spaces without changing stored IDs", () => {
  assert.equal(vehicleNumber("WP_CAB__1234"), "WP CAB 1234");
  assert.equal(vehicleNumber("VEH001"), "VEH001");
  assert.equal(vehicleNumber(undefined), "");
  assert.equal(fleet[0].id, "WP_CAB_10");
});

test("vehicle search accepts raw, displayed, compact and hyphenated numbers", () => {
  for (const query of ["wp_cab_2", "WP CAB 2", "wpcab2", "wp-cab-2", "  CAB 2 ", "ＷＰＣＡＢ２"]) {
    assert.deepEqual(ids(browseVehicles(fleet, query)), ["WP_CAB_2"]);
  }
  assert.deepEqual(browseVehicles(fleet, "no-such-vehicle"), []);
});

test("vehicle numbers sort naturally in both directions", () => {
  assert.deepEqual(ids(browseVehicles(fleet)), ["WP_CAB_1", "WP_CAB_2", "WP_CAB_10"]);
  assert.deepEqual(ids(browseVehicles(fleet, "", "number-desc")), ["WP_CAB_10", "WP_CAB_2", "WP_CAB_1"]);
});

test("capacity sorts break ties by number and keep missing capacity last", () => {
  const missing = { id: "WP_CAB_0", weightCapacityKg: NaN, volumeCapacityM3: undefined };
  assert.deepEqual(ids(browseVehicles([...fleet, missing], "", "weight-desc")), ["WP_CAB_2", "WP_CAB_1", "WP_CAB_10", "WP_CAB_0"]);
  assert.deepEqual(ids(browseVehicles([...fleet, missing], "", "volume-desc")), ["WP_CAB_2", "WP_CAB_10", "WP_CAB_1", "WP_CAB_0"]);
});

test("search and sorting preserve original objects, IDs and source ordering", () => {
  const before = structuredClone(fleet);
  const result = browseVehicles(fleet, "cab", "weight-desc");
  assert.equal(result[0], fleet[1]);
  assert.deepEqual(fleet, before);
  assert.deepEqual(browseVehicles([], ""), []);
});

test("identical display labels have deterministic ordering without merging records", () => {
  const rows = ["CAB_2", "CAB__2", "CAB 2"].map((id) => ({ id, weightCapacityKg: 1, volumeCapacityM3: 1 }));
  assert.deepEqual(ids(browseVehicles(rows)), ids(browseVehicles([...rows].reverse())));
  assert.equal(browseVehicles(rows, "cab2").length, 3);
});
