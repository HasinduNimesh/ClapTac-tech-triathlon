/**
 * How many days a week deliveries actually run, from the operating calendar. Undefined when the calendar
 * covers less than a week, so the capacity maths is skipped instead of assuming a number.
 */
export function operatingDaysPerWeek(calendarDays) {
  if (!Array.isArray(calendarDays) || calendarDays.length < 7) return undefined;
  const operating = calendarDays.filter((day) => day && day.isOperating).length;
  return operating / (calendarDays.length / 7);
}
