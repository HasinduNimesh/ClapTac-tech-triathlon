import assert from "node:assert/strict";
import test from "node:test";
import { failedSourceCount, isSourceFailure } from "../src/dispatcher/sourceFailures.mjs";

test("a source with no error is not a failure", () => {
  assert.equal(isSourceFailure({ error: "", status: 0 }), false);
  assert.equal(isSourceFailure(undefined), false);
});

test("a 404 from a source that may legitimately be missing means empty, not failure", () => {
  assert.equal(isSourceFailure({ error: "404: no plan", status: 404, missingIsEmpty: true }), false);
});

test("a 404 from any other source is still a failure", () => {
  assert.equal(isSourceFailure({ error: "404: not found", status: 404 }), true);
});

test("5xx, auth and network errors stay failures even where a missing plan is tolerated", () => {
  for (const status of [500, 502, 503, 401, 403, 0]) {
    assert.equal(isSourceFailure({ error: "boom", status, missingIsEmpty: true }), true, String(status));
  }
});

test("only real failures are counted", () => {
  assert.equal(failedSourceCount([
    { error: "404: no plan", status: 404, missingIsEmpty: true },
    { error: "", status: 0 },
    { error: "500: down", status: 500 },
  ]), 1);
  assert.equal(failedSourceCount([{ error: "404: no plan", status: 404, missingIsEmpty: true }]), 0);
});
