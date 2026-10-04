import assert from "node:assert/strict";
import test from "node:test";
import { disruptionsForTrip, effectiveSeverity, groupActionsByTrip } from "../src/dispatcher/liveAlerts.mjs";

const risk = (over) => ({ id: "r1", deliveryDate: "2026-10-04", scope: "DISTRICT", scopeKey: "Gampaha", riskType: "FLOODING", severity: "MEDIUM", summary: "", source: "DMC", confidence: 0.7, createdBy: "u", createdAt: "", ...over });
const trip = { tripId: "trip-1", vehicleId: "VEH014", depot: "Peliyagoda", planRef: "PLAN000003", remainingDistricts: ["Gampaha", "Colombo"] };

test("a district risk reaches a trip only through a stop it still has to make", () => {
  assert.equal(disruptionsForTrip([risk()], trip).length, 1);
  assert.equal(disruptionsForTrip([risk()], { ...trip, remainingDistricts: ["Colombo"] }).length, 0);
  assert.equal(disruptionsForTrip([risk({ scopeKey: " gampaha " })], trip).length, 1, "case and spaces do not matter");
});

test("depot risks match whichever depot name the trip carries", () => {
  assert.equal(disruptionsForTrip([risk({ scope: "DEPOT", scopeKey: "DEPOT_NORTH" })], trip).length, 1);
  assert.equal(disruptionsForTrip([risk({ scope: "DEPOT", scopeKey: "DEPOT_SOUTH" })], trip).length, 0);
});

test("route risks match the trip, vehicle or plan reference", () => {
  for (const key of ["trip-1", "veh014", "PLAN000003"]) assert.equal(disruptionsForTrip([risk({ scope: "ROUTE", scopeKey: key })], trip).length, 1, key);
  assert.equal(disruptionsForTrip([risk({ scope: "ROUTE", scopeKey: "VEH001" })], trip).length, 0);
});

test("dispatcher decisions change what the chip shows", () => {
  assert.equal(effectiveSeverity(risk({ overrideDecision: "DISMISSED" })), undefined);
  assert.equal(effectiveSeverity(risk({ overrideDecision: "OVERRIDE", overrideSeverity: "HIGH" })), "HIGH");
  assert.equal(effectiveSeverity(risk({ overrideDecision: "ACKNOWLEDGED" })), "MEDIUM");
  const found = disruptionsForTrip([risk({ id: "a", severity: "LOW" }), risk({ id: "b", overrideDecision: "DISMISSED" }), risk({ id: "c", severity: "HIGH" })], trip);
  assert.deepEqual(found.map((r) => r.id), ["c", "a"], "strongest first, dismissed left out");
});

test("alerts for one trip are grouped under its worst severity", () => {
  const groups = groupActionsByTrip([
    { key: "late-t1", severity: "high", tripId: "t1" },
    { key: "inc-x", severity: "high" },
    { key: "silent-t2", severity: "medium", tripId: "t2" },
    { key: "short-t1", severity: "medium", tripId: "t1" },
    { key: "chill-t2", severity: "critical", tripId: "t2" },
  ]);
  assert.deepEqual(groups.map((g) => g.key), ["trip-t2", "trip-t1", "inc-x"]);
  assert.equal(groups[0].severity, "critical");
  assert.deepEqual(groups[1].items.map((i) => i.key), ["late-t1", "short-t1"]);
  assert.equal(groups[2].items.length, 1);
});
