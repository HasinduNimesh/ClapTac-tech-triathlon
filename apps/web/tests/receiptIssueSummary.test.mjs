import test from "node:test";
import assert from "node:assert/strict";
import { receiptIssueSummary } from "../src/dispatcher/receiptIssueSummary.mjs";

const t = (k) => k;

test("a missing-goods issue says short", () => {
  assert.equal(receiptIssueSummary("MISSING", 30, 34, 4, t), "30 of 34 · 4 short");
});

test("a count that differs from the driver's record does not say short", () => {
  const text = receiptIssueSummary("QUANTITY_MISMATCH", 33, 34, 1, t);
  assert.equal(text, "33 of 34 · differs from the driver's record by 1");
  assert.ok(!text.includes("short"));
});

test("damaged and other issues use their own words", () => {
  assert.equal(receiptIssueSummary("DAMAGED", 34, 34, 2, t), "34 of 34 · 2 damaged");
  assert.equal(receiptIssueSummary("OTHER", 34, 34, 1, t), "34 of 34 · 1 affected");
  assert.equal(receiptIssueSummary("SOMETHING_NEW", 34, 34, 1, t), "34 of 34 · 1 affected");
});
