import test from "node:test";
import assert from "node:assert/strict";
import { calibratedArrivalRange } from "../src/dispatcher/arrivalRange.mjs";

const estimate = { eta: "2026-10-01T08:00:00.000Z", kind: "estimate" };
const history = {
  arrivalRangeStatus: "CALIBRATED", arrivalOffsetP10Minutes: -10, arrivalOffsetP90Minutes: 20,
  arrivalRangeSamples: 40, arrivalRangeHoldouts: 15, arrivalRangeCoverage: 0.8,
};

test("shows calibrated historical residual band around the event-based ETA", () => {
  const range = calibratedArrivalRange(estimate, history);
  assert.equal(range.lower.toISOString(), "2026-10-01T07:50:00.000Z");
  assert.equal(range.upper.toISOString(), "2026-10-01T08:20:00.000Z");
  assert.equal(range.coverage, 0.8);
});

test("withholds range when evidence is not calibrated or bounds are invalid", () => {
  assert.equal(calibratedArrivalRange(estimate, { ...history, arrivalRangeStatus: "POOR_COVERAGE" }), undefined);
  assert.equal(calibratedArrivalRange(estimate, { ...history, arrivalOffsetP10Minutes: 25 }), undefined);
  assert.equal(calibratedArrivalRange({ kind: "unknown" }, history), undefined);
});
