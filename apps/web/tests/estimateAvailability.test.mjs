import test from "node:test";
import assert from "node:assert/strict";
import {
  ESTIMATES_UNAVAILABLE_MESSAGE, hasUsablePrediction, standardArrivalAt, validArrivalAt, validServiceMinutes,
} from "../src/api/estimateAvailability.mjs";
import { arrivesOn, colomboDate, colomboTime } from "../src/store-manager/orderStage.mjs";

test("published standard arrival snapshot remains usable when the plan ETA is invalid", () => {
  const snapshot = "2026-09-30T09:30:00+05:30";
  assert.equal(standardArrivalAt("invalid", snapshot), snapshot);
  assert.equal(standardArrivalAt("2026-09-30T09:00:00+05:30", snapshot), "2026-09-30T09:00:00+05:30");
  assert.equal(standardArrivalAt("invalid", "invalid"), undefined);
  assert.equal(validArrivalAt("invalid"), false);
  assert.equal(colomboDate("invalid"), "");
  assert.equal(colomboTime("invalid"), "");
  assert.equal(arrivesOn({
    stage: "PLANNED", planning: { plannedArrivalAt: "invalid" },
    order: { requestedDeliveryDate: "2026-09-30" },
  }, "2026-09-30"), true);
});

test("missing or invalid forecast and prediction data selects standard times", () => {
  assert.equal(ESTIMATES_UNAVAILABLE_MESSAGE, "Estimates unavailable, using standard times");
  assert.equal(validServiceMinutes(20), true);
  assert.equal(validServiceMinutes(0), false);
  assert.equal(validServiceMinutes(Number.NaN), false);
  assert.equal(hasUsablePrediction([{ status: "ESTIMATED", probability: 0.4 }]), true);
  assert.equal(hasUsablePrediction([{ status: "ESTIMATED", probability: 2 }]), false);
  assert.equal(hasUsablePrediction([]), false);
  assert.equal(hasUsablePrediction(undefined), false);
  assert.equal(hasUsablePrediction([null, { status: "ESTIMATED", probability: 0.4 }]), false);
  assert.equal(hasUsablePrediction([{ status: "ESTIMATED", probability: 0.4, calibrationStatus: "EVALUATED", brierScore: "bad" }]), false);
});
