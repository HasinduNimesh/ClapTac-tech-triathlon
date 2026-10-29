import test from "node:test";
import assert from "node:assert/strict";
import { RAIN_COLOR, SEVERITY_COLORS, WEATHER_SOURCE, disruptionAreas, pendingWeatherSuggestions, suggestionAsRisk } from "../src/dispatcher/disruptionMap.mjs";

const forecast = {
  date: "2026-10-30",
  source: WEATHER_SOURCE,
  sourceUrl: "https://open-meteo.com",
  fetchedAt: "2026-10-29T09:00:00+05:30",
  districts: [
    { district: "Colombo", latitude: 6.9271, longitude: 79.8612, dayRainMm: 22, peakHour: "14:00", maxHourlyRainMm: 12, suggestion: { riskType: "HEAVY_RAIN", severity: "MEDIUM", summary: "Colombo: 22 mm", confidence: 0.8 } },
    { district: "Kandy", latitude: 7.2906, longitude: 80.6337, dayRainMm: 110, suggestion: { riskType: "FLOODING", severity: "HIGH", summary: "Kandy: 110 mm", confidence: 0.9 } },
    { district: "Galle", latitude: 6.0535, longitude: 80.221, dayRainMm: 9 },
    { district: "Matara", latitude: 5.9549, longitude: 80.555, dayRainMm: 2 },
    { district: "Badulla", latitude: 6.9934, longitude: 81.055 },
  ],
};

const risk = (over) => ({ id: "r", deliveryDate: "2026-10-30", scope: "DISTRICT", scopeKey: "Colombo", riskType: "LANDSLIDE", severity: "MEDIUM", summary: "Slip", source: "Manual dispatcher report", confidence: 0.5, createdBy: "u", createdAt: "", ...over });

test("recorded risks are drawn at their district or depot in their strongest current severity", () => {
  const areas = disruptionAreas([
    risk({ id: "a", scopeKey: " colombo ", severity: "LOW" }),
    risk({ id: "b", scopeKey: "Colombo", severity: "MEDIUM", overrideDecision: "OVERRIDE", overrideSeverity: "HIGH" }),
    risk({ id: "c", scope: "DEPOT", scopeKey: "Kandy", riskType: "ROAD_CLOSURE" }),
    risk({ id: "d", scopeKey: "Badulla", overrideDecision: "DISMISSED" }),
    risk({ id: "e", scope: "ROUTE", scopeKey: "TRIP-1" }),
    risk({ id: "f", scopeKey: "Somewhere unknown" }),
  ], forecast, { rain: false });
  assert.deepEqual(areas.map((a) => [a.id, a.color, a.risks.length]), [
    ["risk-district-colombo", SEVERITY_COLORS.HIGH, 2],
    ["risk-depot-DEPOT_SOUTH", SEVERITY_COLORS.MEDIUM, 1],
  ]);
  assert.deepEqual(areas[0].at, [6.9271, 79.8612]);
  assert.equal(areas[0].rain.dayRainMm, 22, "a risk circle carries the district's rain for its tooltip");
});

test("rain circles cover districts with 5 mm or more and no recorded risk", () => {
  const areas = disruptionAreas([risk({ scopeKey: "Colombo" })], forecast);
  assert.deepEqual(areas.filter((a) => a.kind === "rain").map((a) => [a.label, a.color, a.dashed]), [["Kandy", RAIN_COLOR, true], ["Galle", RAIN_COLOR, true]]);
  assert.equal(disruptionAreas([], { ...forecast, districts: [] }).length, 0);
  assert.equal(disruptionAreas(undefined, undefined).length, 0);
});

test("suggestions hide once the dispatcher has added that forecast risk, strongest first", () => {
  assert.deepEqual(pendingWeatherSuggestions(forecast, []).map((d) => d.district), ["Kandy", "Colombo"]);
  const added = [risk({ scopeKey: "Kandy", riskType: "FLOODING", source: WEATHER_SOURCE, overrideDecision: "DISMISSED" })];
  assert.deepEqual(pendingWeatherSuggestions(forecast, added).map((d) => d.district), ["Colombo"]);
  // A hand-recorded risk in the same district does not hide the forecast's own suggestion.
  assert.deepEqual(pendingWeatherSuggestions(forecast, [risk({ scopeKey: "Kandy", riskType: "FLOODING" })]).map((d) => d.district), ["Kandy", "Colombo"]);
});

test("an added suggestion keeps the forecast as its source", () => {
  assert.deepEqual(suggestionAsRisk(forecast.districts[1], forecast), {
    deliveryDate: "2026-10-30", scope: "DISTRICT", scopeKey: "Kandy", riskType: "FLOODING", severity: "HIGH", summary: "Kandy: 110 mm",
    source: WEATHER_SOURCE, sourceReference: "open-meteo.com · 2026-10-30 · read 2026-10-29T09:00:00+05:30", confidence: 0.9,
  });
});
