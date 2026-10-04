import test from "node:test";
import assert from "node:assert/strict";
import { formFills, lineFromProduct } from "../src/store-manager/orderDraft.mjs";

const yoghurt = { id: "FR-YOGHURT-80G", name: "Yoghurt cup 80 g", pack: "crate", unitsPerPack: 48, packWeightKg: 4.6, packVolumeM3: 0.018, temperature: "chilled" };
const rice = { id: "FR-RICE-5KG", name: "Samba rice 5 kg", pack: "bag", unitsPerPack: 1, packWeightKg: 5.1, packVolumeM3: 0.009, temperature: "ambient" };

test("a manager's answer becomes a line with pack totals", () => {
  const line = lineFromProduct(yoghurt, "3", "yoghurt as usual");
  assert.equal(line.quantity, 3);
  assert.equal(line.weightKg, 13.8);
  assert.equal(line.volumeM3, 0.054);
  assert.equal(lineFromProduct(yoghurt, 0), null);
  assert.equal(lineFromProduct(yoghurt, "abc"), null);
  assert.equal(lineFromProduct(yoghurt, 1000), null);
});

test("ambient and chilled lines fill separate orders", () => {
  const fills = formFills([lineFromProduct(rice, 10), lineFromProduct(yoghurt, 2)], null, false);
  assert.deepEqual(fills.map((f) => [f.temperature, f.orderUnits, f.orderWeightKg, f.orderVolumeM3]), [
    ["ambient", 10, 51, 0.09],
    ["chilled", 2, 9.2, 0.04],
  ]);
});

test("copying a previous order adds its totals only when chosen", () => {
  const previous = { orderRef: "FR-1", temperatureRequirement: "ambient", orderUnits: 14, orderWeightKg: 80.5, orderVolumeM3: 0.4 };
  assert.equal(formFills([lineFromProduct(rice, 2)], previous, false)[0].orderUnits, 2);
  const fill = formFills([lineFromProduct(rice, 2)], previous, true)[0];
  assert.equal(fill.orderUnits, 16);
  assert.equal(fill.fromOrderRef, "FR-1");
  assert.equal(fill.orderVolumeM3, 0.42);
});

test("volume is rounded up to the form's two decimals so it never becomes zero", () => {
  assert.equal(formFills([lineFromProduct({ ...rice, packVolumeM3: 0.001 }, 1)], null, false)[0].orderVolumeM3, 0.01);
});
