import test from "node:test";
import assert from "node:assert/strict";
import { addProduct, setPacks, removeLine, totals, requestLines, linesSource, pickable, MAX_PACKS } from "../src/store-manager/orderLines.mjs";

const milk = { id: "FR-MILK-1L", name: "Fresh milk 1 L", pack: "crate", unitsPerPack: 12, packWeightKg: 12.6, packVolumeM3: 0.021, temperature: "chilled" };
const curd = { id: "FR-CURD-500G", name: "Curd 500 g", pack: "tray", unitsPerPack: 8, packWeightKg: 4.2, packVolumeM3: 0.0114, temperature: "chilled" };
const rice = { id: "FR-RICE-5KG", name: "Samba rice 5 kg", pack: "bag", unitsPerPack: 1, packWeightKg: 5.1, packVolumeM3: 0.009, temperature: "ambient" };

test("adding the same product merges into one line", () => {
  let lines = addProduct([], milk, 2);
  lines = addProduct(lines, milk, 3);
  assert.equal(lines.length, 1);
  assert.equal(lines[0].quantity, 5);
  assert.equal(lines[0].weightKg, 63);
});

test("a bad quantity changes nothing", () => {
  const lines = addProduct([], milk, 2);
  for (const bad of [0, -1, "abc", NaN, undefined]) assert.equal(addProduct(lines, milk, bad), lines);
  assert.equal(addProduct(lines, undefined, 1), lines);
});

test("quantities are capped at the server limit", () => {
  assert.equal(addProduct(addProduct([], rice, 990), rice, 50)[0].quantity, MAX_PACKS);
  assert.equal(setPacks(addProduct([], rice, 1), rice.id, 5000)[0].quantity, MAX_PACKS);
});

test("totals add up and volume rounds up like the server", () => {
  const lines = addProduct(addProduct([], milk, 3), curd, 2);
  assert.deepEqual(totals(lines), { units: 5, weightKg: 46.2, volumeM3: 0.086 });
  assert.deepEqual(totals(addProduct([], curd, 1)), { units: 1, weightKg: 4.2, volumeM3: 0.012 });
  assert.deepEqual(totals([]), { units: 0, weightKg: 0, volumeM3: 0 });
});

test("setPacks rescales one line; zero removes it", () => {
  const lines = addProduct(addProduct([], milk, 3), curd, 2);
  const changed = setPacks(lines, milk.id, 1);
  assert.equal(changed[0].quantity, 1);
  assert.equal(changed[0].weightKg, 12.6);
  assert.equal(changed[1].quantity, 2);
  assert.deepEqual(setPacks(lines, milk.id, 0).map((l) => l.productId), [curd.id]);
  assert.deepEqual(removeLine(lines, curd.id).map((l) => l.productId), [milk.id]);
});

test("the request carries only product and packs", () => {
  assert.deepEqual(requestLines(addProduct([], milk, 3)), [{ productId: "FR-MILK-1L", packQty: 3 }]);
});

test("the source is the helper when any line was read from text", () => {
  assert.equal(linesSource(addProduct([], milk, 1)), "form");
  assert.equal(linesSource(addProduct([], milk, 1, "3 crates of milk")), "text_helper");
});

test("only the chosen goods type can be added, and maxed-out products drop out", () => {
  assert.deepEqual(pickable([milk, curd, rice], "ambient", []).map((p) => p.id), [rice.id]);
  const full = addProduct([], rice, MAX_PACKS);
  assert.deepEqual(pickable([rice], "ambient", full), []);
});
