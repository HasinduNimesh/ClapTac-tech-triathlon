/** One rule for "this trip has a loading shortfall the dispatcher still has to act on", used by every dispatcher view. */

/**
 * Short lines still waiting on the dispatcher. The server counts these (openShortfallCount) so a shortfall that has
 * been decided stops asking for action; an older server only sends shortfallCount, which is used as a fallback.
 */
export function openShortfallCount(trip) {
  return Number(trip?.openShortfallCount ?? trip?.shortfallCount ?? 0) || 0;
}

export const hasOpenShortfall = (trip) => openShortfallCount(trip) > 0;

/** Trips with an open shortfall, chilled ones first, then the most short lines. */
export function tripsWithOpenShortfall(trips = []) {
  return trips
    .filter(hasOpenShortfall)
    .sort((a, b) => Number(isChilledShortfall(b)) - Number(isChilledShortfall(a)) || openShortfallCount(b) - openShortfallCount(a));
}

/** Critical only when a chilled line is short. A refrigerated vehicle carrying ambient goods is not a chilled exception. */
export function isChilledShortfall(trip) {
  return trip?.openChilledShortfall === true;
}

/** The same label and severity on every page. */
export function shortfallSeverity(trip) {
  return isChilledShortfall(trip) ? "critical" : "high";
}

/** Link to the page that decides a shortfall, carrying the trip's delivery date so a later day's trip is found. */
export function shortfallLink(trip, fallbackDate = "") {
  const date = trip?.deliveryDate || fallbackDate;
  return date ? `/dispatcher/notifications?date=${encodeURIComponent(date)}` : "/dispatcher/notifications";
}
