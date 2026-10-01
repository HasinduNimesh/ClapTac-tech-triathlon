import assert from "node:assert/strict";
import test from "node:test";
import { timeInputValue } from "../src/dispatcher/outletTime.mjs";

test("normalizes API times with seconds for minute-precision HTML time controls", () => {
  assert.equal(timeInputValue("06:30:00"), "06:30");
  assert.equal(timeInputValue("08:15:32"), "08:15");
  assert.equal(timeInputValue("17:00"), "17:00");
});

test("does not populate malformed or missing time values", () => {
  assert.equal(timeInputValue(""), "");
  assert.equal(timeInputValue(undefined), "");
  assert.equal(timeInputValue("6:30:00"), "");
  assert.equal(timeInputValue("not-time"), "");
});
