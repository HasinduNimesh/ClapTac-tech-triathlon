import { depotCode, depotPosition } from "../api/depots.mjs";
import { effectiveSeverity } from "./liveAlerts.mjs";

export const SEVERITY_COLORS = { HIGH: "#c03221", MEDIUM: "#d9822b", LOW: "#8a92a6" };
export const RAIN_COLOR = "#3a7bd5";
/** Below this much rain in a day a district gets no rain circle: a passing shower is not worth drawing. */
export const RAIN_DRAW_MM = 5;
export const WEATHER_SOURCE = "Open-Meteo forecast";

const SEVERITY_RANK = { HIGH: 3, MEDIUM: 2, LOW: 1 };
const norm = (v) => String(v ?? "").trim().toLowerCase();

/**
 * Circles for the operations and planning maps. Recorded risks are drawn where they apply (a depot, or a
 * district centre from the forecast's district list), one circle per place in its strongest current
 * severity; dismissed risks are left out and ROUTE risks are shown on the trip's line instead. With rain on,
 * districts expecting at least RAIN_DRAW_MM and no recorded risk get a dashed blue rain circle.
 */
export function disruptionAreas(risks, forecast, { rain = true } = {}) {
  const districts = new Map((forecast?.districts || []).map((d) => [norm(d.district), d]));
  const places = new Map();
  for (const risk of risks || []) {
    const severity = effectiveSeverity(risk);
    if (!severity) continue;
    let key, at, label, radiusM;
    if (risk.scope === "DEPOT") {
      key = `depot-${depotCode(risk.scopeKey)}`;
      at = depotPosition(risk.scopeKey);
      label = risk.scopeKey;
      radiusM = 6000;
    } else if (risk.scope === "DISTRICT") {
      const d = districts.get(norm(risk.scopeKey));
      key = `district-${norm(risk.scopeKey)}`;
      at = d ? [d.latitude, d.longitude] : undefined;
      label = d?.district || risk.scopeKey;
      radiusM = 15000;
    }
    if (!at) continue;
    const place = places.get(key) || { id: `risk-${key}`, kind: "risk", at, label, radiusM, severity, risks: [] };
    if (SEVERITY_RANK[severity] > SEVERITY_RANK[place.severity]) place.severity = severity;
    place.risks.push({ ...risk, effectiveSeverity: severity });
    places.set(key, place);
  }
  const areas = [...places.values()].map((p) => ({ ...p, color: SEVERITY_COLORS[p.severity], rain: districts.get(norm(p.label)) }));
  if (rain) {
    for (const d of forecast?.districts || []) {
      if (d.dayRainMm == null || d.dayRainMm < RAIN_DRAW_MM || places.has(`district-${norm(d.district)}`)) continue;
      areas.push({ id: `rain-${norm(d.district)}`, kind: "rain", at: [d.latitude, d.longitude], label: d.district, radiusM: 12000, color: RAIN_COLOR, dashed: true, risks: [], rain: d });
    }
  }
  return areas;
}

/**
 * Forecast suggestions a dispatcher has not added yet: a district's suggestion is hidden once a forecast
 * risk of the same type is recorded for that district on the date, whatever the dispatcher decided on it.
 */
export function pendingWeatherSuggestions(forecast, risks) {
  const added = new Set((risks || []).filter((r) => r.source === WEATHER_SOURCE && r.scope === "DISTRICT").map((r) => `${norm(r.scopeKey)}|${r.riskType}`));
  return (forecast?.districts || [])
    .filter((d) => d.suggestion && !added.has(`${norm(d.district)}|${d.suggestion.riskType}`))
    .sort((a, b) => (SEVERITY_RANK[b.suggestion.severity] - SEVERITY_RANK[a.suggestion.severity]) || (b.dayRainMm ?? 0) - (a.dayRainMm ?? 0));
}

/** The disruption-risk request that records a forecast suggestion, with its source kept for the audit trail. */
export function suggestionAsRisk(district, forecast) {
  const s = district.suggestion;
  return {
    deliveryDate: forecast.date,
    scope: "DISTRICT",
    scopeKey: district.district,
    riskType: s.riskType,
    severity: s.severity,
    summary: s.summary,
    source: WEATHER_SOURCE,
    sourceReference: `open-meteo.com · ${forecast.date}${forecast.fetchedAt ? ` · read ${forecast.fetchedAt}` : ""}`,
    confidence: s.confidence,
  };
}
