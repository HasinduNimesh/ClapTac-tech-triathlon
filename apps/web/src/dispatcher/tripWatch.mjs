/**
 * W8 - Silent trip watch (LO-1, LO-4)
 *
 * Rules:
 * 1. When a trip has no update for 30 minutes, or chilled goods have been on board
 *    longer than allowed:
 *    - Grey the trip out at its last known place, with the time ("No update since HH:MM").
 *    - Turn chilled time on board amber when it runs long.
 *    - Add an alert to Needs action with Acknowledge and Open.
 *
 * This module is pure and only reasons about the data it is given. It never invents
 * a vehicle, a time or a duration: a trip with no known last update is not silent,
 * and chilled time on board is only evaluated when a start time is supplied.
 *
 * Trip input fields (all optional except ids):
 *   tripId, vehicleId
 *   lastUpdateAt            ISO timestamp of the last driver update
 *   lastKnownPlace          label of the last reported place (stop or depot)
 *   isChilled               true when chilled goods are still on board
 *   chilledStartedAt        ISO timestamp from which chilled goods have been on board
 *   chilledOnBoardMinutes   alternative to chilledStartedAt when already known
 *   chilledAllowedMinutes   allowance in minutes (defaults to CHILLED_ALLOWED_MINUTES_DEFAULT)
 */

export const SILENT_TRIP_THRESHOLD_MINUTES = 30;
export const CHILLED_ALLOWED_MINUTES_DEFAULT = 120;

const TIME_ZONE = "Asia/Colombo";

function toMillis(value) {
  if (value === undefined || value === null || value === "") return undefined;
  const ms = value instanceof Date ? value.getTime() : new Date(value).getTime();
  return Number.isNaN(ms) ? undefined : ms;
}

/** Formats a timestamp as HH:MM in Sri Lanka time. Returns "" when the value is missing or invalid. */
export function formatWatchTime(timeVal) {
  if (typeof timeVal === "string" && /^\d{2}:\d{2}$/.test(timeVal)) return timeVal;
  const ms = toMillis(timeVal);
  if (ms === undefined) return "";
  return new Date(ms).toLocaleTimeString("en-GB", { timeZone: TIME_ZONE, hour: "2-digit", minute: "2-digit", hour12: false });
}

/** A trip is silent when its last known update is 30 minutes old or more. No update time means unknown, not silent. */
export function evaluateTripSilentStatus(trip, now = Date.now()) {
  const updateMs = toMillis(trip.lastUpdateAt);
  const lastKnownPlace = trip.lastKnownLocation || trip.lastKnownPlace || "";
  if (updateMs === undefined) {
    return { isSilent: false, elapsedMinutes: 0, timeStr: "", lastKnownPlace, label: "" };
  }
  const elapsedMinutes = Math.max(0, Math.floor((now - updateMs) / 60000));
  const timeStr = formatWatchTime(trip.lastUpdateAt);
  const isSilent = elapsedMinutes >= SILENT_TRIP_THRESHOLD_MINUTES;
  return {
    isSilent,
    elapsedMinutes,
    timeStr,
    lastKnownPlace,
    label: isSilent ? `No update since ${timeStr}` : "",
  };
}

/** Chilled time on board runs long when it exceeds the allowance. Unknown duration is never reported as long. */
export function evaluateChilledOnBoardStatus(trip, now = Date.now()) {
  const allowedMinutes = Number(trip.chilledAllowedMinutes) || CHILLED_ALLOWED_MINUTES_DEFAULT;
  if (trip.isChilled !== true) {
    return { isChilled: false, isChilledLong: false, onBoardMinutes: undefined, allowedMinutes, label: "" };
  }
  let onBoardMinutes;
  if (typeof trip.chilledOnBoardMinutes === "number" && Number.isFinite(trip.chilledOnBoardMinutes)) {
    onBoardMinutes = trip.chilledOnBoardMinutes;
  } else {
    const startMs = toMillis(trip.chilledStartedAt);
    if (startMs !== undefined) onBoardMinutes = Math.max(0, Math.floor((now - startMs) / 60000));
  }
  if (onBoardMinutes === undefined) {
    return { isChilled: true, isChilledLong: false, onBoardMinutes: undefined, allowedMinutes, label: "" };
  }
  return {
    isChilled: true,
    isChilledLong: onBoardMinutes > allowedMinutes,
    onBoardMinutes,
    allowedMinutes,
    label: `${onBoardMinutes}m on board (allowed ${allowedMinutes}m)`,
  };
}

/** Adds the W8 monitoring fields to a trip. */
export function enrichTripWithWatch(trip, now = Date.now()) {
  const silentStatus = evaluateTripSilentStatus(trip, now);
  const chilledStatus = evaluateChilledOnBoardStatus(trip, now);
  return {
    ...trip,
    isSilent: silentStatus.isSilent,
    silentTime: silentStatus.timeStr,
    silentLabel: silentStatus.label,
    silentMinutes: silentStatus.elapsedMinutes,
    lastKnownPlace: silentStatus.lastKnownPlace,
    isChilled: chilledStatus.isChilled,
    isChilledLong: chilledStatus.isChilledLong,
    chilledMinutes: chilledStatus.onBoardMinutes,
    chilledAllowedMinutes: chilledStatus.allowedMinutes,
    chilledLabel: chilledStatus.label,
  };
}

/** Generates alerts for the Needs action panel from silent trips and chilled overages. */
export function generateNeedsActionAlerts(trips, acknowledgedAlertIds = new Set(), now = Date.now()) {
  const alerts = [];
  for (const trip of trips) {
    const enriched = enrichTripWithWatch(trip, now);
    const tripId = enriched.tripId || enriched.id || "";
    const vehicleId = enriched.vehicleId || "Unknown Vehicle";

    if (enriched.isSilent) {
      const alertId = `silent-${tripId || vehicleId}`;
      alerts.push({
        id: alertId,
        tripId,
        vehicleId,
        type: "SILENT_TRIP",
        title: `${vehicleId} · Silent trip`,
        message: enriched.lastKnownPlace ? `${enriched.silentLabel} (${enriched.lastKnownPlace})` : enriched.silentLabel,
        severity: "warning",
        acknowledged: acknowledgedAlertIds.has(alertId),
        lastKnownPlace: enriched.lastKnownPlace,
        timeStr: enriched.silentTime,
        minutes: enriched.silentMinutes,
      });
    }

    if (enriched.isChilledLong) {
      const alertId = `chilled-${tripId || vehicleId}`;
      alerts.push({
        id: alertId,
        tripId,
        vehicleId,
        type: "CHILLED_OVERAGE",
        title: `${vehicleId} · Chilled goods on board running long`,
        message: `${enriched.chilledMinutes}m on board exceeds ${enriched.chilledAllowedMinutes}m allowed limit`,
        severity: "amber",
        acknowledged: acknowledgedAlertIds.has(alertId),
        chilledMinutes: enriched.chilledMinutes,
        allowedMinutes: enriched.chilledAllowedMinutes,
      });
    }
  }
  return alerts;
}
