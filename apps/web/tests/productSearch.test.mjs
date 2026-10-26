import test from "node:test";
import assert from "node:assert/strict";
import { searchProducts } from "../src/store-manager/productSearch.mjs";
import { addProduct, pickable, MAX_PACKS } from "../src/store-manager/orderLines.mjs";

const rice = { id: "FR-RICE-5KG", name: "Samba rice", family: "grains", size: "5 kg", pack: "bag", temperature: "ambient", unitsPerPack: 1, packWeightKg: 5.1, packVolumeM3: 0.009 };
const milk = { ...rice, id: "FR-MILK-1L", name: "Fresh milk", family: "dairy", size: "1 L", pack: "crate", temperature: "chilled" };
const products = [rice, milk];

test("search matches name, code, family, size and pack without case sensitivity", () => {
  for (const query of ["SAMBA", "fr-rice", "Grains", "5 KG", "bag"]) {
    assert.deepEqual(searchProducts(products, query), [rice]);
  }
});

test("all query words must match, regardless of order or repeated whitespace", () => {
  assert.deepEqual(searchProducts(products, "  BAG   rice 5  "), [rice]);
  assert.deepEqual(searchProducts(products, "rice crate"), []);
  assert.deepEqual(searchProducts(products, "missing"), []);
});

test("empty searches preserve catalog order and input data", () => {
  const snapshot = structuredClone(products);
  assert.deepEqual(searchProducts(products, " \t\n"), products);
  searchProducts(products, "milk");
  assert.deepEqual(products, snapshot);
  assert.deepEqual(searchProducts([], "rice"), []);
});

test("Unicode normalization supports full-width characters and non-Latin names", () => {
  assert.deepEqual(searchProducts(products, "ＲＩＣＥ"), [rice]);
  for (const name of ["සහල්", "அரிசி"]) {
    const translated = { ...rice, name };
    assert.deepEqual(searchProducts([translated], name), [translated]);
  }
});

test("search cannot restore products excluded by temperature or maximum quantity", () => {
  const full = addProduct([], rice, MAX_PACKS);
  assert.deepEqual(searchProducts(pickable(products, "ambient", []), "milk"), []);
  assert.deepEqual(searchProducts(pickable(products, "ambient", full), "rice"), []);
  assert.equal(full[0].quantity, MAX_PACKS);
});
