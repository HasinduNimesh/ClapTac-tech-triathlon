import { depotCode, depotPosition } from "../api/depots.mjs";

/**
 * Sample routes for the Live operations map when no trips exist for the day (demos, training).
 * Built from the real outlets, depots and vehicles in the database, but the trips themselves are
 * invented: they are never sent anywhere, never counted in the stats or Needs action, and the page
 * labels them as sample data. Deterministic for a given set of outlets, vehicles and date.
 */
export const SAMPLE_TRIP_PREFIX = "sample-";
export const SAMPLE_STOPS_PER_TRIP = 5;
const TRIPS_PER_DEPOT = { DEPOT_NORTH: 3, DEPOT_SOUTH: 2 };
// One of each state, so the map shows every colour in its legend.
const STATES = ["ok", "late", "silent", "waiting", "ok"];
const DONE_BY_STATE = { ok: 2, late: 1, silent: 1, waiting: 0 };

export const isSampleTrip = (tripId) => typeof tripId === "string" && tripId.startsWith(SAMPLE_TRIP_PREFIX);

const hhmm = (value, fallback) => (/^\d{2}:\d{2}/.test(value || "") ? value.slice(0, 5) : fallback);
/** A Sri Lanka time (UTC+05:30) on the given date as an ISO instant. */
const atColombo = (date, time) => new Date(`${date}T${time}:00+05:30`).toISOString();
const addMinutes = (iso, minutes) => new Date(new Date(iso).getTime() + minutes * 60000).toISOString();
const dist = (a, b) => Math.hypot(a[0] - b[0], a[1] - b[1]);

/** Visit order: nearest outlet first, then nearest to the last one. */
function nearestFirst(start, outlets) {
  const left = [...outlets];
  const route = [];
  let here = start;
  while (left.length) {
    let best = 0;
    for (let i = 1; i < left.length; i++) if (dist(here, left[i].at) < dist(here, left[best].at)) best = i;
    const [next] = left.splice(best, 1);
    route.push(next);
    here = next.at;
  }
  return route;
}

/**
 * The time the sample trips are drawn at: now, while it is a working morning or afternoon on the chosen
 * date (06:30 to 15:30 in Sri Lanka); otherwise 10:30 that day, so the map never shows trucks at midnight.
 */
export function sampleReferenceTime(date, now = Date.now()) {
  const from = Date.parse(atColombo(date, "06:30"));
  const to = Date.parse(atColombo(date, "15:30"));
  return now >= from && now <= to ? now : Date.parse(atColombo(date, "10:30"));
}

/**
 * Rows in the shape the Live operations list and map use. `now` places completed stops in the past
 * and the next stop in the near future, so the map reads like a trip under way.
 */
export function buildSampleTrips({ outlets = [], vehicles = [], date, now: wallClock = Date.now() }) {
  const now = sampleReferenceTime(date, wallClock);
  const rows = [];
  let stateIndex = 0;
  for (const [code, count] of Object.entries(TRIPS_PER_DEPOT)) {
    const depotAt = depotPosition(code);
    if (!depotAt) continue;
    // Outlets of this depot with a position, swept around the depot by bearing so each trip covers one area.
    const placed = outlets
      .filter((o) => depotCode(o.depot) === code && Number.isFinite(o.latitude) && Number.isFinite(o.longitude))
      .map((o) => ({ ...o, at: [o.latitude, o.longitude], bearing: Math.atan2(o.latitude - depotAt[0], o.longitude - depotAt[1]) }))
      .sort((a, b) => a.bearing - b.bearing || a.id.localeCompare(b.id));
    const fleet = vehicles.filter((v) => depotCode(v.homeDepot) === code).map((v) => v.id).sort();
    const step = Math.max(1, Math.floor(placed.length / count));
    for (let t = 0; t < count; t++) {
      const chunk = placed.slice(t * step, t * step + SAMPLE_STOPS_PER_TRIP);
      if (chunk.length < 2) continue;
      const state = STATES[stateIndex++ % STATES.length];
      const done = DONE_BY_STATE[state];
      const vehicleId = fleet[t] || `VEH-SAMPLE-${code === "DEPOT_NORTH" ? "N" : "S"}${t + 1}`;
      const tripId = `${SAMPLE_TRIP_PREFIX}${code.toLowerCase()}-${t + 1}`;
      // Stops are spaced 40 minutes apart; the trip started so that `done` stops are behind it.
      const start = new Date(now - (done * 40 + 20) * 60000).toISOString();
      const stops = nearestFirst(depotAt, chunk).map((o, i) => {
        const planned = addMinutes(start, (i + 1) * 40);
        const open = hhmm(o.windowOpenTime, "06:00");
        const close = hhmm(o.windowCloseTime, "18:00");
        const finished = i < done;
        return {
          id: `${tripId}-stop-${i + 1}`, runId: `${tripId}-run`, orderId: `${tripId}-order-${i + 1}`, orderRef: `SAMPLE-${i + 1}`,
          outletId: o.id, outletName: o.name, brand: o.brand, district: o.district,
          temperatureRequirement: /fresh/i.test(o.brand || "") ? "chilled" : "ambient",
          latitude: o.latitude, longitude: o.longitude, locationApproximate: o.locationApproximate ?? true,
          plannedArrivalAt: planned, plannedWindowOpen: atColombo(date, open), plannedWindowClose: atColombo(date, close),
          stopSequence: i + 1, status: finished ? "completed" : "pending",
          ...(finished ? { arrivedAt: addMinutes(planned, -5), outcomeCode: "DELIVERED", outcomeAt: planned } : {}),
        };
      });
      const next = stops[done];
      const lastUpdate = done > 0 ? stops[done - 1].outcomeAt : undefined;
      const silentFor = state === "silent" ? 45 : 0;
      rows.push({
        sample: true,
        summary: { tripId, vehicleId, depot: code, tripNumber: 1, status: state === "waiting" ? "planned" : "in_progress", stopCount: stops.length, completedStops: done, planRef: "SAMPLE" },
        detail: { tripId, run: { id: `${tripId}-run`, tripId, deliveryDate: date, vehicleId, depot: code, status: state === "waiting" ? "planned" : "in_progress", startedAt: state === "waiting" ? undefined : start }, stops },
        state,
        next,
        nextEta: next ? { kind: state === "late" ? "late" : "ok", eta: addMinutes(next.plannedArrivalAt, state === "late" ? 55 : 0), label: state === "late" ? "Expected after the window closes" : "On time" } : undefined,
        lastUpdate: silentFor ? new Date(now - silentFor * 60000).toISOString() : lastUpdate,
        chilled: stops.some((s) => s.temperatureRequirement === "chilled"),
        watch: silentFor ? { isSilent: true, silentTime: new Date(now - silentFor * 60000).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit", timeZone: "Asia/Colombo" }), lastKnownPlace: stops[done - 1]?.outletName || stops[done - 1]?.outletId } : undefined,
      });
    }
  }
  return rows;
}
