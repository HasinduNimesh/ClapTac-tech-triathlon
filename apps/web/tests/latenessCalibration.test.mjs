import test from "node:test";
import assert from "node:assert/strict";
import { evaluatedLatenessCalibrations } from "../src/dispatcher/latenessCalibration.mjs";

test("keeps every evaluated brand segment and withholds weak or missing scores", () => {
  const history = [
    { brand: "Fresh", calibrationStatus: "EVALUATED", brierScore: 0.2381, calibrationSampleCount: 15, calibrationVersion: "rolling-v1" },
    { brand: "Style", calibrationStatus: "EVALUATED", brierScore: 0.19, calibrationSampleCount: 12, calibrationVersion: "rolling-v1" },
    { brand: "Home", calibrationStatus: "INSUFFICIENT_HOLDOUT", calibrationSampleCount: 4, calibrationVersion: "rolling-v1" },
    { brand: "Bulk", calibrationStatus: "EVALUATED", calibrationSampleCount: 15, calibrationVersion: "rolling-v1" },
  ];

  assert.deepEqual(evaluatedLatenessCalibrations(history).map(item => item.brand), ["Fresh", "Style"]);
});
