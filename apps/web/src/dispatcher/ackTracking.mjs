// W2 / LO-9 acknowledgement tracking. A receipt covers one trip and one
// recipient role; matching is by trip identity, never by role alone, so one
// driver's acknowledgement cannot mark every trip as acknowledged.

export const ACK_AUDIENCES = ["DRIVER", "LOADER"];

const text = (value) => (typeof value === "string" ? value : "");

/** The receipt a recipient of `role` recorded for exactly this trip, if any. */
export function ackForTrip(acks, tripId, role) {
  const id = text(tripId);
  if (!id) return undefined;
  return (acks || []).find((a) => a && a.actorRole === role && text(a.tripId) === id);
}

/** Receipts recorded before trip identity existed. They name no trip, so they
 *  are listed as plan-level and never counted against a trip. */
export function planLevelAcks(acks) {
  return (acks || []).filter((a) => a && !text(a.tripId));
}

/** The latest reminder sent to `audience` for this trip, if any. */
export function reminderForTrip(reminders, tripId, audience) {
  const id = text(tripId);
  let latest;
  for (const r of reminders || []) {
    if (!r || text(r.tripId) !== id || r.audience !== audience) continue;
    if (!latest || Date.parse(r.remindedAt) > Date.parse(latest.remindedAt)) latest = r;
  }
  return latest;
}

function cell(acks, reminders, tripId, role, overdue) {
  const ack = ackForTrip(acks, tripId, role);
  const reminder = reminderForTrip(reminders, tripId, role);
  return {
    audience: role,
    state: ack ? "acknowledged" : overdue ? "overdue" : "pending",
    ack,
    acknowledgedAt: ack ? ack.acknowledgedAt : undefined,
    remindedAt: reminder ? reminder.remindedAt : undefined,
  };
}

/** True once `thresholdMinutes` have passed since the plan was published. */
export function isOverdue(publishedAt, now, thresholdMinutes) {
  const published = Date.parse(publishedAt || "");
  if (Number.isNaN(published)) return false;
  return Math.floor((now - published) / 60000) >= thresholdMinutes;
}

/** One row per trip with an independent driver and loader status. */
export function trackerRows({ trips, acks, reminders, publishedAt, now, thresholdMinutes }) {
  const overdue = isOverdue(publishedAt, now, thresholdMinutes);
  return (trips || []).map((trip) => ({
    trip,
    driver: cell(acks, reminders, trip.id, "DRIVER", overdue),
    loader: cell(acks, reminders, trip.id, "LOADER", overdue),
  }));
}

/** Every (trip, audience) still unacknowledged. */
export function unacknowledgedTargets(rows) {
  const out = [];
  for (const row of rows) {
    for (const audience of ACK_AUDIENCES) {
      const c = audience === "DRIVER" ? row.driver : row.loader;
      if (c.state !== "acknowledged") out.push({ tripId: row.trip.id, audience, state: c.state });
    }
  }
  return out;
}

/** Whether the signed-in field user has acknowledged this trip. A legacy
 *  plan-level receipt (no trip recorded) still counts, as it does for the
 *  start/depart gate in the delivery and loading services. */
export function acknowledgedForTrip(acks, { actorId, role, tripId }) {
  if (!actorId) return false;
  return (acks || []).some((a) => a && a.actorId === actorId && a.actorRole === role && (!text(a.tripId) || a.tripId === tripId));
}
