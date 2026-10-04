/**
 * The demand forecast looks ten weeks (70 days) ahead. The service owns the estimate; these helpers hold the
 * horizon the page asks the operating calendar for and how the busy-day grid copes when that calendar has
 * not been set that far ahead.
 */
export const FORECAST_WEEKS = 10;
export const FORECAST_DAYS = FORECAST_WEEKS * 7;
/** Extra calendar days fetched past the horizon so a week that straddles its end is not cut short. */
export const CALENDAR_BUFFER_DAYS = 7;
/** Used only when the calendar has no rows at all: Monday to Saturday run, Sunday does not (0 = Sunday). */
const FALLBACK_WEEKDAY_OPERATING = [false, true, true, true, true, true, true];

export function addDays(date, days) {
  const d = new Date(`${date}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

/** The /shared/calendar window the forecast page requests: today through the ten weeks plus the buffer. */
export function calendarRange(today) {
  return { from: today, to: addDays(today, FORECAST_DAYS - 1 + CALENDAR_BUFFER_DAYS) };
}

/**
 * The usual pattern for each weekday (0 = Sunday), taken from the days the calendar does hold, so public
 * holidays do not turn a whole weekday off. With no rows at all a fixed pattern is the last resort.
 */
export function weekdayDefaults(items) {
  const tally = Array.from({ length: 7 }, () => ({ on: 0, off: 0 }));
  for (const day of items || []) {
    if (!day || typeof day.date !== "string") continue;
    const weekday = new Date(`${day.date}T00:00:00Z`).getUTCDay();
    tally[weekday][day.isOperating ? "on" : "off"] += 1;
  }
  return tally.map((t, weekday) => (t.on + t.off === 0 ? FALLBACK_WEEKDAY_OPERATING[weekday] : t.on >= t.off));
}

/**
 * One entry per day of the horizon starting today, always FORECAST_DAYS long. Days the calendar has no row for
 * take the weekday default and are flagged `known: false`, so the grid never errors or goes blank.
 */
export function buildCalendarGrid(today, items) {
  const byDate = new Map();
  for (const day of items || []) if (day && typeof day.date === "string") byDate.set(day.date, Boolean(day.isOperating));
  const defaults = weekdayDefaults(items);
  return Array.from({ length: FORECAST_DAYS }, (_, i) => {
    const date = addDays(today, i);
    const known = byDate.has(date);
    const weekday = new Date(`${date}T00:00:00Z`).getUTCDay();
    return { date, isOperating: known ? byDate.get(date) : defaults[weekday], known };
  });
}

/**
 * Where the calendar stops being set: the last date of the unbroken run of calendar rows from today.
 * `lastSet` is undefined when today itself has no row; `complete` is true when every day is set.
 */
export function calendarCoverage(grid) {
  let lastSet;
  for (const day of grid) {
    if (!day.known) break;
    lastSet = day.date;
  }
  return { lastSet, complete: grid.length > 0 && grid.every((d) => d.known) };
}

/** Likely range of a week's estimate, from the half-width percent the service reports for that week. */
export function rangeBounds(total, rangePercent) {
  const pct = Number.isFinite(rangePercent) ? Math.max(0, rangePercent) : 0;
  const round = (v) => Math.round(v * 100) / 100;
  return { low: round(total * (1 - pct / 100)), high: round(total * (1 + pct / 100)), pct };
}
