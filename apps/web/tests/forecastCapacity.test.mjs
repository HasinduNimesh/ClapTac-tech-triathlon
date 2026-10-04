import assert from "node:assert/strict";
import test from "node:test";
import { operatingDaysPerWeek } from "../src/dispatcher/forecastCapacity.mjs";

const days = (pattern) => pattern.split("").map((c) => ({ isOperating: c === "x" }));

test("operating days a week come from the calendar, not an assumed six", () => {
  assert.equal(operatingDaysPerWeek(days("xxxxxx-".repeat(6))), 6);
  assert.equal(operatingDaysPerWeek(days("xxxxx--".repeat(6))), 5, "a five-day week when the calendar says so");
  assert.equal(operatingDaysPerWeek(days("xxxxxx-xxxxx--")), 5.5, "weeks with a public holiday count for less");
});

test("without a calendar of at least a week nothing is assumed", () => {
  assert.equal(operatingDaysPerWeek(undefined), undefined);
  assert.equal(operatingDaysPerWeek([]), undefined);
  assert.equal(operatingDaysPerWeek(days("xxxxx-")), undefined);
});
