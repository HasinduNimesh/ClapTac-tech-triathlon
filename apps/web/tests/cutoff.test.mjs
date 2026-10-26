import assert from "node:assert/strict";
import test from "node:test";
import { cutoffHasPassed, formatCutoff, minutesOfDay, serverTimeAt, withTime } from "../src/store-manager/cutoff.mjs";

test("a cutoff from the policy is read as a time of day", () => {
  assert.equal(minutesOfDay("16:00"), 960);
  assert.equal(minutesOfDay("15:30:00"), 930);
  for (const bad of ["", "4pm", "24:00", "12:75", undefined, null]) assert.equal(minutesOfDay(bad), undefined, String(bad));
});

test("the cutoff shown is the one the policy holds, not 4:00 PM", () => {
  assert.match(formatCutoff("16:00"), /^4:00\s?PM$/i);
  assert.match(formatCutoff("15:30:00"), /^3:30\s?PM$/i);
  assert.match(formatCutoff("09:05"), /^9:05\s?AM$/i);
  assert.equal(formatCutoff("nonsense"), undefined, "an unusable cutoff is not guessed");
});

test("the cutoff is formatted for each language the app offers", () => {
  // The words depend on the browser's language data; here only that each gives a time.
  for (const locale of ["en", "si", "ta"]) assert.match(formatCutoff("16:00", locale), /\d/, locale);
});

test("whether the cutoff has passed is judged in Sri Lanka time against the configured cutoff", () => {
  // 10:29 UTC is 15:59 in Colombo (UTC+5:30); 10:30 UTC is 16:00.
  assert.equal(cutoffHasPassed("16:00", new Date("2026-10-05T10:29:00Z")), false);
  assert.equal(cutoffHasPassed("16:00", new Date("2026-10-05T10:30:00Z")), true);
  // A cutoff moved earlier is honoured: 15:30 had passed at 15:59.
  assert.equal(cutoffHasPassed("15:30", new Date("2026-10-05T10:29:00Z")), true);
  assert.equal(cutoffHasPassed(undefined, new Date("2026-10-05T10:29:00Z")), undefined);
});

test("the time is filled into a translated sentence", () => {
  assert.equal(withTime("Order by {time} today.", "3:30 PM"), "Order by 3:30 PM today.");
});

test("cutoff status never falls back to the device clock", () => {
  assert.equal(cutoffHasPassed("16:00"), undefined);
  assert.equal(cutoffHasPassed("16:00", new Date("invalid")), undefined);
});

test("server time advances across cutoff without a page reload", () => {
  const serverTime = "2026-10-26T15:59:59+05:30";
  assert.equal(cutoffHasPassed("16:00", serverTimeAt(serverTime, 100, 100)), false);
  assert.equal(cutoffHasPassed("16:00", serverTimeAt(serverTime, 100, 1100)), true);
  assert.equal(cutoffHasPassed("16:00", serverTimeAt("2026-10-26T23:59:59+05:30", 100, 1100)), false, "resets on the next Colombo day");
});

test("device wall-clock changes cannot change a server-clock sample", (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: new Date("2026-10-26T03:00:00Z") });
  const serverTime = "2026-10-26T10:30:00Z";
  const before = serverTimeAt(serverTime, 2000, 3000);
  t.mock.timers.setTime(new Date("2036-01-01T22:00:00Z").getTime());
  assert.equal(serverTimeAt(serverTime, 2000, 3000).getTime(), before.getTime());
  assert.equal(cutoffHasPassed("16:00", before), true);
});

test("invalid, timezone-less, stale and backwards clock samples are unknown", () => {
  for (const value of [undefined, "", "invalid", "2026-10-26T15:00:00", "2026-10-26"]) {
    assert.equal(serverTimeAt(value, 0, 1000), undefined);
  }
  const time = "2026-10-26T09:30:00Z";
  assert.equal(serverTimeAt(time, 100, 99), undefined);
  assert.equal(serverTimeAt(time, 0, 90_001), undefined);
  assert.equal(serverTimeAt(time, NaN, 1), undefined);
  assert.equal(serverTimeAt(time, 0, Infinity), undefined);
  assert.equal(serverTimeAt(time, 0, 90_000).toISOString(), "2026-10-26T09:31:30.000Z");
});
