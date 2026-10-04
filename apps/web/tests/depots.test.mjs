import assert from "node:assert/strict";
import test from "node:test";
import { DEPOT_LOCATIONS, depotCode, depotPosition } from "../src/api/depots.mjs";

test("a depot is found under the dataset name or the profile code", () => {
  assert.deepEqual(depotPosition("Peliyagoda"), DEPOT_LOCATIONS.DEPOT_NORTH);
  assert.deepEqual(depotPosition("DEPOT_NORTH"), DEPOT_LOCATIONS.DEPOT_NORTH);
  assert.deepEqual(depotPosition(" kandy "), DEPOT_LOCATIONS.DEPOT_SOUTH);
  assert.deepEqual(depotPosition("DEPOT_SOUTH"), DEPOT_LOCATIONS.DEPOT_SOUTH);
});

test("Kandy is not drawn at Peliyagoda (the dataset's depot names used to miss the lookup and fall back to the north depot)", () => {
  assert.notDeepEqual(depotPosition("Kandy"), depotPosition("Peliyagoda"));
});

test("an unknown or missing depot has no position, never a stand-in", () => {
  assert.equal(depotPosition("Galle"), undefined);
  assert.equal(depotPosition(""), undefined);
  assert.equal(depotPosition(undefined), undefined);
  assert.equal(depotCode(undefined), "");
});
