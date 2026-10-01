import assert from "node:assert/strict";
import test from "node:test";
import { ARRIVAL_ESTIMATE_VERSION, estimateArrival, previousReportedStop } from "../src/dispatcher/arrivalEstimate.mjs";

test("downstream ETA uses the latest arrived stop before an earlier completed stop", () => {
  const completed = { outcomeAt: "2026-09-30T08:30:00+05:30" };
  const active = { arrivedAt: "2026-09-30T09:00:00+05:30" };
  assert.equal(previousReportedStop([completed, active, {}], 2), active);
  assert.equal(previousReportedStop([completed, {}], 1), completed);
  assert.equal(previousReportedStop([completed], 0), undefined);
});

test("projects a downstream ETA by the observed delay of the last stop", () => {
  const result = estimateArrival({
    plannedArrivalAt: "2026-09-30T08:30:00+05:30",
    plannedDepartureAt: "2026-09-30T08:15:00+05:30",
    previousOutcomeAt: "2026-09-30T08:25:00+05:30",
    windowCloseAt: "2026-09-30T09:00:00Z",
    deliveryDate: "2026-09-30",
    now: Date.parse("2026-09-30T08:26:00+05:30"),
  });
  assert.equal(result.eta, "2026-09-30T03:10:00.000Z");
  assert.equal(result.delayMinutes, 10);
  assert.equal(result.risk, "On track");
  assert.equal(result.version, ARRIVAL_ESTIMATE_VERSION);
  assert.match(result.confidence, /Low/);
});

test("projects an unfinished arrived stop using its configured service-time allowance", () => {
  const result = estimateArrival({
    plannedArrivalAt: "2026-09-30T09:30:00+05:30",
    plannedDepartureAt: "2026-09-30T09:15:00+05:30",
    previousArrivedAt: "2026-09-30T09:00:00+05:30",
    serviceMinutesPerStop: 30,
    windowCloseAt: "10:00",
    deliveryDate: "2026-09-30",
    now: Date.parse("2026-09-30T09:20:00+05:30"),
  });
  assert.equal(result.eta, "2026-09-30T04:15:00.000Z");
  assert.equal(result.delayMinutes, 15);
  assert.equal(result.risk, "Watch window");
  assert.equal(result.version, "event_delay_propagation_v2");
});

test("a completed outcome time takes precedence over an arrival-plus-service projection", () => {
  const result = estimateArrival({
    plannedArrivalAt: "2026-09-30T09:30:00+05:30",
    plannedDepartureAt: "2026-09-30T09:15:00+05:30",
    previousArrivedAt: "2026-09-30T09:00:00+05:30",
    previousOutcomeAt: "2026-09-30T09:12:00+05:30",
    serviceMinutesPerStop: 40,
    now: Date.parse("2026-09-30T09:20:00+05:30"),
  });
  assert.equal(result.eta, "2026-09-30T04:00:00.000Z");
  assert.equal(result.delayMinutes, 0);
});

test("flags an ETA beyond the delivery window and handles missing schedules", () => {
  const atRisk = estimateArrival({
    plannedArrivalAt: "2026-09-30T08:30:00+05:30",
    plannedDepartureAt: "2026-09-30T08:15:00+05:30",
    previousOutcomeAt: "2026-09-30T08:55:00+05:30",
    windowCloseAt: "09:00",
    deliveryDate: "2026-09-30",
    now: Date.parse("2026-09-30T08:56:00+05:30"),
  });
  assert.equal(atRisk.risk, "Window at risk");
  assert.equal(estimateArrival({}).kind, "unknown");
});

test("marks a closed window missed when the current time has passed it", () => {
  const result = estimateArrival({
    plannedArrivalAt: "2026-09-30T08:30:00+05:30",
    windowCloseAt: "08:00",
    deliveryDate: "2026-09-30",
    now: Date.parse("2026-09-30T08:10:00+05:30"),
  });
  assert.equal(result.risk, "Window missed");
});

test("closed delivery window takes precedence over a stale earlier ETA", () => {
  const result = estimateArrival({
    plannedArrivalAt: "2026-09-30T08:30:00+05:30",
    windowCloseAt: "09:00",
    deliveryDate: "2026-09-30",
    now: Date.parse("2026-09-30T09:20:00+05:30"),
  });
  assert.equal(result.risk, "Window missed");
});

test("an elapsed ETA is reported as late even when the delivery window is near", () => {
  const result = estimateArrival({
    plannedArrivalAt: "2026-09-30T09:50:00+05:30",
    windowCloseAt: "10:02",
    deliveryDate: "2026-09-30",
    now: Date.parse("2026-09-30T10:00:00+05:30"),
  });
  assert.equal(result.risk, "ETA passed");
});

test("an open stop’s window risk advances as the estimate clock crosses the close time", () => {
  const input = {
    plannedArrivalAt: "2026-09-30T09:50:00+05:30",
    windowCloseAt: "10:00",
    deliveryDate: "2026-09-30",
  };
  assert.equal(estimateArrival({ ...input, now: Date.parse("2026-09-30T09:49:00+05:30") }).risk, "Watch window");
  assert.equal(estimateArrival({ ...input, now: Date.parse("2026-09-30T10:01:00+05:30") }).risk, "Window missed");
});
