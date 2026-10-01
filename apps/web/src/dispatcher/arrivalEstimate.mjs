const MINUTE = 60_000;
export const ARRIVAL_ESTIMATE_VERSION = "event_delay_propagation_v2";

export function previousReportedStop(stops, beforeIndex) {
  for (let index = beforeIndex - 1; index >= 0; index -= 1) {
    const stop = stops[index];
    if (stop?.outcomeAt || stop?.outcomeReceivedAt || stop?.arrivedAt) return stop;
  }
  return undefined;
}

/**
 * Estimate a downstream stop from its published schedule and the last
 * driver-reported completion. This is intentionally event-based, not GPS.
 */
export function estimateArrival({
  plannedArrivalAt,
  plannedDepartureAt,
  previousOutcomeAt,
  previousArrivedAt,
  serviceMinutesPerStop = 20,
  windowCloseAt,
  deliveryDate,
  now = Date.now(),
}) {
  if (!plannedArrivalAt) return { version: ARRIVAL_ESTIMATE_VERSION, kind: "unknown", label: "No planned ETA", confidence: "Unavailable" };

  const planned = Date.parse(plannedArrivalAt);
  if (!Number.isFinite(planned)) return { version: ARRIVAL_ESTIMATE_VERSION, kind: "unknown", label: "No planned ETA", confidence: "Unavailable" };

  let delay = 0;
  const actualOutcome = Date.parse(previousOutcomeAt || "");
  const previousArrival = Date.parse(previousArrivedAt || "");
  const plannedPrevious = Date.parse(plannedDepartureAt || "");
  const allowance = Number.isFinite(serviceMinutesPerStop) && serviceMinutesPerStop >= 0
    ? serviceMinutesPerStop * MINUTE
    : 20 * MINUTE;
  const actualPrevious = Number.isFinite(actualOutcome)
    ? actualOutcome
    : Number.isFinite(previousArrival) ? previousArrival + allowance : Number.NaN;
  if (Number.isFinite(actualPrevious) && Number.isFinite(plannedPrevious)) {
    delay = Math.max(0, actualPrevious - plannedPrevious);
  }
  const estimated = planned + delay;
  const close = /^\d{2}:\d{2}(:\d{2})?$/.test(windowCloseAt || "") && deliveryDate
    ? Date.parse(`${deliveryDate}T${windowCloseAt.length === 5 ? `${windowCloseAt}:00` : windowCloseAt}+05:30`)
    : Date.parse(windowCloseAt || "");
  // An open stop is already a missed-window exception once its window has
  // closed, even when stale schedule math still predicts an earlier arrival.
  // Otherwise distinguish a projected window breach from an ETA that has
  // already elapsed, then use the near-window warning.
  const risk = Number.isFinite(close) && now > close
    ? "Window missed"
    : Number.isFinite(close) && estimated > close
      ? "Window at risk"
      : estimated < now
        ? "ETA passed"
        : Number.isFinite(close) && close - estimated <= 15 * MINUTE
          ? "Watch window"
          : "On track";

  return {
    version: ARRIVAL_ESTIMATE_VERSION,
    kind: risk === "Window missed" || risk === "Window at risk" ? "risk" : risk === "ETA passed" ? "late" : "estimate",
    eta: new Date(estimated).toISOString(),
    label: new Date(estimated).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    risk,
    confidence: "Low · event-based, no GPS",
    delayMinutes: Math.round(delay / MINUTE),
  };
}
