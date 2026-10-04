import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import {
  FORECAST_DAYS, FORECAST_WEEKS, addDays, buildCalendarGrid, calendarCoverage, calendarRange, rangeBounds, weekdayDefaults,
} from "../src/dispatcher/forecastHorizon.mjs";

const rows = (from, count, operating = (d) => new Date(`${d}T00:00:00Z`).getUTCDay() !== 0) =>
  Array.from({ length: count }, (_, i) => { const date = addDays(from, i); return { date, isOperating: operating(date) }; });

test("the forecast horizon is ten weeks, seventy days", () => {
  assert.equal(FORECAST_WEEKS, 10);
  assert.equal(FORECAST_DAYS, 70);
});

test("the calendar window covers all ten weeks plus a buffer", () => {
  const { from, to } = calendarRange("2026-10-04");
  assert.equal(from, "2026-10-04");
  assert.equal(to, "2026-12-19", "70 days from today (to 2026-12-12) plus a seven day buffer");
  assert.ok(to >= addDays("2026-10-04", FORECAST_DAYS - 1), "never shorter than the horizon");
});

test("the grid has a cell for every day of the ten weeks even with no calendar rows", () => {
  for (const items of [undefined, [], rows("2026-10-04", 10)]) {
    const grid = buildCalendarGrid("2026-10-04", items);
    assert.equal(grid.length, 70);
    assert.equal(grid[0].date, "2026-10-04");
    assert.equal(grid[69].date, "2026-12-12");
  }
});

test("days past the end of the calendar take the weekday default instead of failing", () => {
  const grid = buildCalendarGrid("2026-10-04", rows("2026-10-04", 21));
  assert.ok(grid.slice(0, 21).every((d) => d.known));
  assert.ok(grid.slice(21).every((d) => !d.known));
  for (const day of grid.slice(21)) {
    const sunday = new Date(`${day.date}T00:00:00Z`).getUTCDay() === 0;
    assert.equal(day.isOperating, !sunday, `${day.date} follows the weekday pattern of the days that are set`);
  }
  assert.deepEqual(calendarCoverage(grid), { lastSet: "2026-10-24", complete: false });
});

test("a public holiday does not switch off its whole weekday", () => {
  const items = rows("2026-10-04", 28, (d) => d !== "2026-10-07" && new Date(`${d}T00:00:00Z`).getUTCDay() !== 0);
  assert.equal(weekdayDefaults(items)[3], true, "Wednesdays still run: one holiday in four");
  assert.equal(weekdayDefaults(items)[0], false);
});

test("with no calendar at all the fallback pattern is used and coverage says nothing is set", () => {
  const grid = buildCalendarGrid("2026-10-04", undefined);
  assert.equal(grid[0].isOperating, false, "2026-10-04 is a Sunday");
  assert.equal(grid[1].isOperating, true);
  assert.deepEqual(calendarCoverage(grid), { lastSet: undefined, complete: false });
});

test("a calendar covering the whole horizon reports complete", () => {
  const grid = buildCalendarGrid("2026-10-04", rows("2026-10-04", 77));
  assert.deepEqual(calendarCoverage(grid), { lastSet: "2026-12-12", complete: true });
});

test("the likely range follows the per-week percentage the service reports", () => {
  assert.deepEqual(rangeBounds(100, 10), { low: 90, high: 110, pct: 10 });
  assert.deepEqual(rangeBounds(100, 28), { low: 72, high: 128, pct: 28 });
  assert.deepEqual(rangeBounds(100, undefined), { low: 100, high: 100, pct: 0 });
});

test("the forecast page asks for the shared horizon, not a hard-coded six weeks", () => {
  const source = readFileSync(new URL("../src/dispatcher/ForecastPage.tsx", import.meta.url), "utf8");
  assert.match(source, /calendarRange\(today\)/);
  assert.doesNotMatch(source, /addDays\(today, 41\)/);
  assert.doesNotMatch(source, /const RANGE = /);
});
