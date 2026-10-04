import assert from "node:assert/strict";
import test from "node:test";
import { buildSampleTrips, isSampleTrip, SAMPLE_STOPS_PER_TRIP, sampleReferenceTime } from "../src/dispatcher/sampleTrips.mjs";

// Shaped like /shared/outlets: approximate positions around district centres.
const centres = { Colombo: [6.9271, 79.8612], Gampaha: [7.084, 79.9939], Kalutara: [6.5854, 79.9607], Kandy: [7.2906, 80.6337], Matale: [7.4675, 80.6234] };
const outlets = Object.entries(centres).flatMap(([district, [lat, lng]], d) => Array.from({ length: 8 }, (_, i) => ({
  id: `OUT${String(d * 8 + i + 1).padStart(3, "0")}`, name: `${district} ${i + 1}`, brand: i % 3 === 0 ? "Fresh" : "Style", district,
  depot: ["Kandy", "Matale"].includes(district) ? "Kandy" : "Peliyagoda", windowOpenTime: "06:00:00", windowCloseTime: "10:00:00",
  latitude: lat + i * 0.01, longitude: lng + i * 0.01, locationApproximate: true,
})));
const vehicles = [{ id: "VEH001", homeDepot: "DEPOT_NORTH" }, { id: "VEH002", homeDepot: "DEPOT_NORTH" }, { id: "VEH031", homeDepot: "DEPOT_SOUTH" }];
const now = Date.parse("2026-10-04T03:30:00Z");

test("sample trips run from both depots through that depot's real outlets", () => {
  const rows = buildSampleTrips({ outlets, vehicles, date: "2026-10-04", now });
  assert.equal(rows.length, 5, "three from Peliyagoda, two from Kandy");
  for (const row of rows) {
    assert.ok(row.sample && isSampleTrip(row.summary.tripId));
    assert.ok(row.detail.stops.length >= 2 && row.detail.stops.length <= SAMPLE_STOPS_PER_TRIP);
    const depotOutlets = new Set(outlets.filter((o) => (row.summary.depot === "DEPOT_NORTH" ? o.depot === "Peliyagoda" : o.depot === "Kandy")).map((o) => o.id));
    assert.ok(row.detail.stops.every((s) => depotOutlets.has(s.outletId)), "stops stay with their depot");
  }
  assert.deepEqual(rows.map((r) => r.summary.vehicleId).slice(0, 2), ["VEH001", "VEH002"], "real vehicles are used where the depot has them");
  assert.match(rows[2].summary.vehicleId, /^VEH-SAMPLE-/, "a placeholder is used when it runs out");
});

test("each map state appears, with completed stops behind the truck and the next stop ahead", () => {
  const rows = buildSampleTrips({ outlets, vehicles, date: "2026-10-04", now });
  assert.deepEqual([...new Set(rows.map((r) => r.state))].sort(), ["late", "ok", "silent", "waiting"]);
  for (const row of rows) {
    const done = row.detail.stops.filter((s) => s.outcomeCode).length;
    assert.equal(done, row.summary.completedStops);
    assert.equal(row.next, row.detail.stops[done]);
  }
  assert.ok(rows.find((r) => r.state === "silent").watch.isSilent);
});

test("nothing is produced without outlet positions, and no trip id can be mistaken for a real one", () => {
  assert.deepEqual(buildSampleTrips({ outlets: outlets.map(({ latitude, longitude, ...o }) => o), vehicles, date: "2026-10-04", now }), []);
  assert.equal(isSampleTrip("2f6c0d2e-trip"), false);
});

test("sample trips are drawn during working hours, never at night", () => {
  const morning = Date.parse("2026-10-04T03:30:00Z"); // 09:00 in Sri Lanka
  assert.equal(sampleReferenceTime("2026-10-04", morning), morning);
  const night = Date.parse("2026-10-04T16:00:00Z"); // 21:30 in Sri Lanka
  assert.equal(new Date(sampleReferenceTime("2026-10-04", night)).toISOString(), "2026-10-04T05:00:00.000Z", "10:30 in Sri Lanka");
  const rows = buildSampleTrips({ outlets, vehicles, date: "2026-10-04", now: night });
  for (const row of rows) for (const stop of row.detail.stops) {
    const hour = Number(new Date(stop.plannedArrivalAt).toLocaleString("en-GB", { hour: "2-digit", hour12: false, timeZone: "Asia/Colombo" }));
    assert.ok(hour >= 8 && hour <= 14, `${stop.plannedArrivalAt} is during the working day`);
  }
});
