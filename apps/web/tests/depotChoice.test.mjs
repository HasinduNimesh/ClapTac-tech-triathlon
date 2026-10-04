import assert from "node:assert/strict";
import test from "node:test";
import { effectiveDepot, readDepotChoice, saveDepotChoice } from "../src/dispatcher/depotChoice.mjs";

const memory = () => { const m = new Map(); return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => void m.set(k, v) }; };

test("a dispatcher starts on their own depot, or on all depots when they have none", () => {
  assert.equal(effectiveDepot("DEPOT_SOUTH", undefined), "DEPOT_SOUTH");
  assert.equal(effectiveDepot("", undefined), "");
  assert.equal(effectiveDepot(undefined, undefined), "");
});

test("a choice made in this tab wins, including choosing all depots", () => {
  assert.equal(effectiveDepot("DEPOT_SOUTH", "DEPOT_NORTH"), "DEPOT_NORTH");
  assert.equal(effectiveDepot("DEPOT_SOUTH", ""), "");
});

test("the choice is remembered per person, so the next person starts on their own depot", () => {
  const storage = memory();
  assert.equal(readDepotChoice(storage, "USR002"), undefined);
  saveDepotChoice(storage, "USR002", "DEPOT_SOUTH");
  assert.equal(readDepotChoice(storage, "USR002"), "DEPOT_SOUTH");
  assert.equal(readDepotChoice(storage, "USR009"), undefined);
  saveDepotChoice(storage, "USR002", "");
  assert.equal(readDepotChoice(storage, "USR002"), "", "all depots is a choice, not 'no choice'");
});

test("unusable storage is tolerated", () => {
  const broken = { getItem() { throw new Error("blocked"); }, setItem() { throw new Error("blocked"); } };
  assert.equal(readDepotChoice(broken, "USR002"), undefined);
  assert.doesNotThrow(() => saveDepotChoice(broken, "USR002", "DEPOT_NORTH"));
  assert.equal(readDepotChoice(undefined, "USR002"), undefined);
});
