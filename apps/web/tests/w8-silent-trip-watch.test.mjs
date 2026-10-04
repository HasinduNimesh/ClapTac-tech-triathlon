/**
 * W8 - SILENT TRIP WATCH - Automated Workflow Acceptance Tests
 *
 * Spec:
 * When: a trip has no update for 30 minutes, or chilled goods have been on board
 *       longer than allowed.
 * 1. Grey the trip out at its last known place, with the time ("No update since 04:41").
 * 2. Turn chilled time on board amber when it runs long.
 * 3. Add an alert to Needs action with Acknowledge and Open.
 *
 * Done when: a silent trip shows as grey with its last update in both the list and the map.
 *
 * The pure logic is tested directly; the Live operations page is checked at source level.
 * No vehicle, time or duration is made up: only real trip data drives the watch.
 *
 * Run:  node --test tests/w8-silent-trip-watch.test.mjs
 */

import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { translate } from "../src/locale.mjs";
import {
  SILENT_TRIP_THRESHOLD_MINUTES,
  CHILLED_ALLOWED_MINUTES_DEFAULT,
  evaluateTripSilentStatus,
  evaluateChilledOnBoardStatus,
  enrichTripWithWatch,
  generateNeedsActionAlerts,
  formatWatchTime,
} from "../src/dispatcher/tripWatch.mjs";

const srcRoot = fileURLToPath(new URL("../src/", import.meta.url));
const read = (rel) => readFileSync(join(srcRoot, rel), "utf8");

const liveOpsSrc   = read("dispatcher/LiveOperationsPage.tsx");
const tripWatchSrc = read("dispatcher/tripWatch.mjs");
const indexCssSrc  = read("index.css");

const minutesAgo = (now, minutes) => new Date(now - minutes * 60 * 1000).toISOString();

// ---------------------------------------------------------------------------
//  W8 Step 1 - Grey out trip at last known place with time
// ---------------------------------------------------------------------------

test("W8-1: 30-minute silent trip logic flags silent trips and reports the last update time and place", () => {
  assert.equal(SILENT_TRIP_THRESHOLD_MINUTES, 30, "Silent trip threshold must be 30 minutes");
  const now = Date.parse("2026-10-04T10:00:00Z");

  const active = evaluateTripSilentStatus({ tripId: "t-active", vehicleId: "VEH001", lastUpdateAt: minutesAgo(now, 10) }, now);
  assert.equal(active.isSilent, false, "Trip with update 10 min ago must NOT be silent");

  const stale = evaluateTripSilentStatus({ tripId: "t-stale", vehicleId: "VEH002", lastUpdateAt: minutesAgo(now, 35), lastKnownPlace: "OUT010" }, now);
  assert.equal(stale.isSilent, true, "Trip with update 35 min ago must be marked silent");
  assert.match(stale.label, /^No update since \d{2}:\d{2}$/, "Silent trip label must state 'No update since HH:MM'");
  assert.equal(stale.timeStr, formatWatchTime(minutesAgo(now, 35)));
  assert.equal(stale.lastKnownPlace, "OUT010");

  const boundary = evaluateTripSilentStatus({ lastUpdateAt: minutesAgo(now, 30) }, now);
  assert.equal(boundary.isSilent, true, "A trip with no update for exactly 30 minutes is silent");
});

test("W8-1: Colombo time is used for the 'since' time", () => {
  assert.equal(formatWatchTime("2026-10-04T04:41:00+05:30"), "04:41");
  assert.equal(formatWatchTime("2026-10-04T04:41:00Z"), "10:11");
});

test("W8-1: nothing is invented when real data is missing", () => {
  const now = Date.now();
  const unknown = evaluateTripSilentStatus({ tripId: "trip-x", vehicleId: "VEH005" }, now);
  assert.equal(unknown.isSilent, false, "A trip with no known update time must not be reported as silent");
  assert.equal(unknown.timeStr, "");
  assert.equal(unknown.lastKnownPlace, "");
  assert.equal(formatWatchTime(undefined), "");
  assert.equal(formatWatchTime("not a date"), "");

  const chilled = evaluateChilledOnBoardStatus({ vehicleId: "VEH005", isChilled: true }, now);
  assert.equal(chilled.isChilledLong, false, "Unknown chilled time on board must not be reported as long");
  assert.equal(chilled.onBoardMinutes, undefined);

  assert.deepEqual(generateNeedsActionAlerts([{ tripId: "trip-x", vehicleId: "VEH005" }], new Set(), now), []);
  assert.doesNotMatch(tripWatchSrc, /VEH005|04:41|OUT027/, "tripWatch.mjs must not hard-code a vehicle, time or place");
  assert.doesNotMatch(liveOpsSrc, /VEH005|trip-veh005|04:41/, "LiveOperationsPage must not inject a made-up trip");
});

test("W8-1: Live operations page greys out silent trips with last update and location", () => {
  assert.match(liveOpsSrc, /import \{[^}]*enrichTripWithWatch[^}]*\} from "\.\/tripWatch\.mjs"/, "Page must use the tripWatch logic");
  assert.match(
    liveOpsSrc,
    /row\.state === "silent" \? " trip-greyed-out silent-trip" : ""/,
    "List row must apply 'trip-greyed-out silent-trip' when the trip is silent",
  );
  assert.match(
    liveOpsSrc,
    /<span className="badge-grey">\{t\("No update since"\)\} \{row\.watch\.silentTime\}<\/span>/,
    "List must render a grey 'No update since HH:MM' badge",
  );
  assert.match(liveOpsSrc, /\{t\("Last known"\)\}: \{row\.watch\.lastKnownPlace\}/, "List must render 'Last known: place'");
  assert.match(liveOpsSrc, /lastKnownPlace: lastReported/, "Last known place comes from the last reported stop");
  assert.match(indexCssSrc, /\.trip-greyed-out/);
  assert.match(indexCssSrc, /\.silent-trip/);
  assert.match(indexCssSrc, /\.badge-grey/);
});

// ---------------------------------------------------------------------------
//  W8 Step 2 - Chilled time on board amber when it runs long
// ---------------------------------------------------------------------------

test("W8-2: evaluateChilledOnBoardStatus evaluates the cold chain allowance", () => {
  assert.equal(CHILLED_ALLOWED_MINUTES_DEFAULT, 120, "Default allowed chilled time must be 120 minutes");
  const now = Date.parse("2026-10-04T10:00:00Z");

  const ambient = evaluateChilledOnBoardStatus({ vehicleId: "VEH001", isChilled: false }, now);
  assert.equal(ambient.isChilled, false);
  assert.equal(ambient.isChilledLong, false);

  const ok = evaluateChilledOnBoardStatus({ vehicleId: "VEH002", isChilled: true, chilledOnBoardMinutes: 60, chilledAllowedMinutes: 120 }, now);
  assert.equal(ok.isChilled, true);
  assert.equal(ok.isChilledLong, false);

  const long = evaluateChilledOnBoardStatus({ vehicleId: "VEH003", isChilled: true, chilledOnBoardMinutes: 145, chilledAllowedMinutes: 120 }, now);
  assert.equal(long.isChilledLong, true);
  assert.equal(long.onBoardMinutes, 145);

  const fromStart = evaluateChilledOnBoardStatus({ isChilled: true, chilledStartedAt: minutesAgo(now, 200), chilledAllowedMinutes: 180 }, now);
  assert.equal(fromStart.onBoardMinutes, 200, "Time on board is derived from the real start time");
  assert.equal(fromStart.isChilledLong, true);

  const enriched = enrichTripWithWatch({ tripId: "t", vehicleId: "VEH003", isChilled: true, chilledOnBoardMinutes: 145 }, now);
  assert.equal(enriched.chilledAllowedMinutes, 120);
  assert.equal(enriched.isChilledLong, true);
});

test("W8-2: Live operations page derives chilled time from the real trip start and shows it amber", () => {
  assert.match(liveOpsSrc, /chilledStartedAt: row\.detail\?\.run\?\.startedAt/, "Chilled time on board starts when the run started");
  assert.match(liveOpsSrc, /isChilled\(s\.temperatureRequirement\)\)/, "Only chilled stops still to deliver count");
  assert.match(liveOpsSrc, /row\.watch\?\.isChilledLong &&/, "Amber indicator must be conditional on running long");
  assert.match(liveOpsSrc, /className="status-amber chilled-amber"/);
  assert.match(
    liveOpsSrc,
    /\{t\("Chilled time on board"\)\}: \{row\.watch\.chilledMinutes\}m \(\{t\("exceeds allowed limit"\)\}\)/,
    "Must show 'Chilled time on board: {minutes}m (exceeds allowed limit)'",
  );
  assert.match(indexCssSrc, /\.status-amber/);
  assert.match(indexCssSrc, /\.chilled-amber/);
});

// ---------------------------------------------------------------------------
//  W8 Step 3 - Needs action alert with Acknowledge and Open
// ---------------------------------------------------------------------------

test("W8-3: generateNeedsActionAlerts creates alerts for silent and chilled trips with acknowledge state", () => {
  const now = Date.parse("2026-10-04T10:00:00Z");
  const trips = [
    { tripId: "trip-a", vehicleId: "VEH010", lastUpdateAt: minutesAgo(now, 45), isChilled: true, chilledOnBoardMinutes: 150 },
    { tripId: "trip-b", vehicleId: "VEH011", lastUpdateAt: minutesAgo(now, 5) },
  ];
  const alerts = generateNeedsActionAlerts(trips, new Set(), now);
  assert.equal(alerts.length, 2, "Only the silent chilled trip raises alerts");

  const silent = alerts.find((a) => a.type === "SILENT_TRIP");
  assert.equal(silent.vehicleId, "VEH010");
  assert.match(silent.title, /VEH010 · Silent trip/);
  assert.equal(silent.acknowledged, false);

  const chilled = alerts.find((a) => a.type === "CHILLED_OVERAGE");
  assert.match(chilled.title, /Chilled goods on board running long/);
  assert.equal(chilled.severity, "amber");

  const acked = generateNeedsActionAlerts(trips, new Set([silent.id]), now).find((a) => a.id === silent.id);
  assert.equal(acked.acknowledged, true, "Alert ID in set must be acknowledged");
});

test("W8-3: Live operations Needs action list offers Acknowledge and Open for watch alerts", () => {
  assert.match(liveOpsSrc, /generateNeedsActionAlerts\(/, "Needs action must be fed by the watch alerts");
  assert.match(liveOpsSrc, /label: t\("Acknowledge"\), onClick: \(\) => setAcknowledged\(\(a\) => \[\.\.\.a, alert\.id\]\)/, "Acknowledge button hides the alert");
  assert.match(liveOpsSrc, /label: t\("Open trip"\), [^}]*onClick: \(\) => setOpenTrip\(alert\.tripId\)/, "Open button opens the trip");
  assert.match(liveOpsSrc, /\{t\("Needs action"\)\}/);
  assert.match(liveOpsSrc, /t\("Chilled goods on board running long"\)/);
});

// ---------------------------------------------------------------------------
//  W8 Done When - silent trip grey in both list and map
// ---------------------------------------------------------------------------

test("W8 done-when: a silent trip is grey with its last update in both the list and the map", () => {
  assert.match(liveOpsSrc, /trip-greyed-out silent-trip/, "List row is greyed");
  assert.match(liveOpsSrc, /s === "silent" \|\| s === "waiting" \? "#8a92a6"/, "Map marker colour is grey for silent trips");
  assert.match(
    liveOpsSrc,
    /row\.state === "silent" && row\.watch\?\.silentTime \? `\$\{t\("No update since"\)\} \$\{row\.watch\.silentTime\}`/,
    "Map marker label must read 'No update since HH:MM'",
  );
  assert.match(liveOpsSrc, /label: `\$\{row\.summary\.vehicleId\} · \$\{silentText\(row\)\}`/, "Map marker uses that label");
  assert.match(liveOpsSrc, /const truck = at\(last\) \|\| depotAt;/, "Marker sits at the last reported stop (last known place)");
});

// ---------------------------------------------------------------------------
//  W8 i18n
// ---------------------------------------------------------------------------

test("W8 i18n: all W8 workflow text keys are translated to Sinhala and Tamil", () => {
  const requiredKeys = [
    "Needs action",
    "No update since",
    "Chilled goods on board running long",
    "Chilled time on board",
    "Acknowledge",
    "Open trip",
    "Last known",
    "exceeds allowed limit",
    "No update from",
  ];
  for (const key of requiredKeys) {
    const si = translate("si", key);
    const ta = translate("ta", key);
    assert.notEqual(si, key, `Missing Sinhala translation for: "${key}"`);
    assert.notEqual(ta, key, `Missing Tamil translation for: "${key}"`);
    assert.ok(si.trim().length > 0 && ta.trim().length > 0);
  }
});
