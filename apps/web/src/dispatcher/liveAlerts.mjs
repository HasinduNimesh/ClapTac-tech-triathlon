import { depotCode } from "../api/depots.mjs";

/**
 * LO-6 and the LO-2 amendment: which recorded road/weather risks may delay a trip, and the
 * Needs-action list grouped per trip. Pure functions so the live board and the tests share them.
 */
const SEVERITY_RANK = { HIGH: 3, MEDIUM: 2, LOW: 1 };
const norm = (v) => String(v ?? "").trim().toLowerCase();

/** The severity the dispatcher works with: an override replaces the recorded one; a dismissed risk has none. */
export function effectiveSeverity(risk) {
  if (!risk || risk.overrideDecision === "DISMISSED") return undefined;
  if (risk.overrideDecision === "OVERRIDE" && risk.overrideSeverity) return risk.overrideSeverity;
  return risk.severity;
}

/**
 * Risks that touch a trip: DEPOT by its depot, DISTRICT by any stop still to do, ROUTE by the
 * trip, vehicle or plan reference the dispatcher typed. Strongest first; dismissed risks are left out.
 * trip = { tripId, vehicleId, depot, planRef, remainingDistricts: string[] }
 */
export function disruptionsForTrip(risks, trip) {
  const districts = new Set((trip.remainingDistricts || []).map(norm).filter(Boolean));
  const routeKeys = new Set([trip.tripId, trip.vehicleId, trip.planRef].map(norm).filter(Boolean));
  const out = [];
  for (const risk of risks || []) {
    const severity = effectiveSeverity(risk);
    if (!severity) continue;
    const key = norm(risk.scopeKey);
    const hit = risk.scope === "DEPOT" ? Boolean(trip.depot) && depotCode(trip.depot) === depotCode(risk.scopeKey)
      : risk.scope === "DISTRICT" ? districts.has(key)
      : risk.scope === "ROUTE" ? routeKeys.has(key)
      : false;
    if (hit) out.push({ ...risk, effectiveSeverity: severity });
  }
  return out.sort((a, b) => (SEVERITY_RANK[b.effectiveSeverity] || 0) - (SEVERITY_RANK[a.effectiveSeverity] || 0));
}

const ACTION_RANK = { critical: 4, high: 3, medium: 2, low: 1 };

/**
 * One group per trip: every alert carrying the same tripId sits together under the trip's worst
 * severity. Alerts with no trip stay on their own. Groups keep the ranked order of their first alert,
 * then move up if a later alert in the same trip is more severe.
 */
export function groupActionsByTrip(actions) {
  const groups = [];
  const byTrip = new Map();
  for (const action of actions || []) {
    if (!action.tripId) { groups.push({ key: action.key, items: [action] }); continue; }
    let group = byTrip.get(action.tripId);
    if (!group) {
      group = { key: `trip-${action.tripId}`, tripId: action.tripId, label: action.tripLabel, items: [] };
      byTrip.set(action.tripId, group);
      groups.push(group);
    }
    group.items.push(action);
  }
  const rank = (severity) => ACTION_RANK[severity] || 0;
  return groups
    .map((g, index) => ({ ...g, index, severity: g.items.reduce((s, i) => (rank(i.severity) > rank(s) ? i.severity : s), g.items[0].severity) }))
    .sort((a, b) => rank(b.severity) - rank(a.severity) || a.index - b.index)
    .map(({ index, ...g }) => g);
}
