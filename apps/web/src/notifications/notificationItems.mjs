/**
 * Pure derivation of "what needs attention" for each role. The notification pages and the header bell
 * both call these, so the bell count is always the page's open count.
 * `ctx.t` translates a label and `ctx.dateTime` formats a timestamp; both default to identity.
 */
import { colomboTime, isDeferred, needsReceipt } from "../store-manager/orderStage.mjs";
import { messageTypeLabel } from "../store-manager/outletMessages.mjs";

const identity = (value) => value;
const plainDate = (value) => value ?? "—";
const context = (ctx = {}) => ({ t: ctx.t || identity, dateTime: ctx.dateTime || plainDate });

// ---------------------------------------------------------------- dispatcher

/**
 * The dispatcher's "Needs action" list. `source` holds the loaded lists (any may be missing):
 * plan, incidents, temperature, trips (loading), dockAlerts, conflicts, receipts.
 * `open` tells the page what to do when the item is chosen: a link (`to`), or one of
 * "exception" (trip), "dock" (dockAlert) or "loop".
 */
export function deriveDispatcherAlerts(source = {}, ctx) {
  const { t, dateTime } = context(ctx);
  const plan = source.plan || null;
  const trips = source.trips || [];
  const alerts = [];
  for (const incident of source.incidents || []) {
    alerts.push({ key: incident.id, kind: "incident", tone: "red", tag: t("Critical · chilled risk"), title: `${incident.vehicleId}${incident.tripId ? ` · ${incident.tripId}` : ""} · ${t(incident.type)}`, text: `${incident.description} · ${dateTime(incident.reportedAt)}`, action: t("Reassign stops or defer with a reason"), to: "/dispatcher/live" });
  }
  for (const ev of source.temperature || []) {
    alerts.push({ key: ev.event_id, kind: "temperature", tone: "red", tag: t("Critical · temperature"), title: `${ev.resource_type} ${ev.resource_id}`, text: `${ev.reason || t("Out-of-range temperature recorded")} · ${dateTime(ev.timestamp)}`, action: t("Open live operations"), to: "/dispatcher/live" });
  }
  for (const trip of trips.filter((x) => (x.shortfallCount || 0) > 0)) {
    alerts.push({ key: `short-${trip.tripId}`, kind: "shortfall", tone: "amber", tag: t("High · loader shortfall"), title: `${trip.vehicleId} · ${t("Trip")} ${trip.tripNumber ?? 1} · ${trip.shortfallCount} ${t("order(s) short")}`, text: t("Shortfall needs dispatcher acceptance or a new plan before departure"), action: t("Review the load exception"), open: "exception", trip });
  }
  for (const alert of (source.dockAlerts || []).filter((a) => !a.resolvedAt)) {
    const at = trips.find((x) => x.tripId === alert.tripId);
    alerts.push({ key: `dock-${alert.id}`, kind: "dock", tone: "amber", tag: t("High · wrong vehicle"), title: `${alert.orderRef} ${t("found at")} ${at?.vehicleId || alert.tripId}${alert.belongsVehicleId ? ` · ${t("belongs on")} ${alert.belongsVehicleId}` : ""}`, text: `${t("Reported by the loader")} ${alert.reportedBy} · ${dateTime(alert.createdAt)}${alert.note ? ` · ${alert.note}` : ""}`, action: t("Mark handled"), open: "dock", dockAlert: alert });
  }
  const unallocated = plan?.unallocated || [];
  const repeat = (plan?.orders || []).filter((o) => (o.outletDeferralCount || 0) >= 2 && unallocated.some((u) => u.orderId === o.id));
  for (const order of repeat) {
    alerts.push({ key: `rep-${order.id}`, kind: "repeat-deferral", tone: "cool", tag: t("Repeat deferral"), title: `${order.orderRef} · ${order.outletId} · ${t("deferred")} ${order.outletDeferralCount} ${t("times")}`, text: `${order.lastServedAt ? `${t("Last served")} ${dateTime(order.lastServedAt)} · ` : ""}${t("fairness review required before another deferral")}`, action: t("Review priority and next-run plan"), to: "/dispatcher/planning" });
  }
  const conflictCount = source.conflicts?.length || 0;
  const receiptCount = source.receipts?.length || 0;
  if (conflictCount + receiptCount > 0) {
    alerts.push({ key: "loop", kind: "after-plan-change", tone: "primary", tag: t("After plan change"), title: `${conflictCount} ${t("sync conflict(s)")} · ${receiptCount} ${t("store receipt issue(s)")}`, text: t("Who has the new plan, what came back from the road, and what the store confirmed."), action: t("Review acknowledgements and receipts"), open: "loop" });
  }
  return alerts;
}

// -------------------------------------------------------------- store manager

export const ETA_THRESHOLD_MINUTES = 30;

/** Identity of a deferral notice: acknowledging one run's deferral does not hide a later one. */
export const deferralKey = (row) => `${row.order.id}|${row.planning.planRef ?? ""}|${row.planning.reasonCode ?? ""}`;

/**
 * The store manager's notices from their order tracking rows. `state.acknowledged` holds acknowledged
 * deferral keys and `state.seenEta` the last arrival time each order's manager has seen.
 */
export function deriveStoreNotices(rows = [], state = {}, ctx) {
  const { t } = context(ctx);
  const acknowledged = state.acknowledged instanceof Set ? state.acknowledged : new Set(state.acknowledged || []);
  const seenEta = state.seenEta || {};
  const receipts = rows.filter((row) => needsReceipt(row.stage));
  const deferred = rows.filter((row) => isDeferred(row.stage) && !acknowledged.has(deferralKey(row)));
  const etaChanges = rows.flatMap((row) => {
    const now = row.planning.plannedArrivalAt;
    const before = seenEta[row.order.id];
    if (!now || !before || before === now || row.delivery?.outcome) return [];
    const minutes = Math.round((new Date(now).getTime() - new Date(before).getTime()) / 60000);
    return Math.abs(minutes) >= ETA_THRESHOLD_MINUTES ? [{ row, from: before, to: now, minutes }] : [];
  });
  const items = [
    ...receipts.map((row) => ({ key: `receipt:${row.order.id}`, kind: "receipt", tone: "primary", tag: t("RECEIPT"), title: `${row.order.orderRef} · ${t("delivery needs receipt confirmation")}`, to: `/store-manager/receipts?order=${encodeURIComponent(row.order.id)}` })),
    ...etaChanges.map((change) => ({ key: `eta:${change.row.order.id}:${change.to}`, kind: "eta-change", tone: "amber", tag: t("ETA CHANGE"), title: `${change.row.order.orderRef} · ${t("arrival moved by")} ${Math.abs(change.minutes)} ${t("minutes")}`, text: `${t("New expected arrival")} ${colomboTime(change.to)} · ${t("was")} ${colomboTime(change.from)}`, to: `/store-manager/orders/${encodeURIComponent(change.row.order.id)}/track` })),
    ...deferred.map((row) => ({ key: `deferral:${deferralKey(row)}`, kind: "deferral", tone: "cool", tag: t("DEFERRED"), title: `${row.order.orderRef} · ${t("moved to a later run")}`, to: `/store-manager/orders/${encodeURIComponent(row.order.id)}/timeline` })),
  ];
  return { receipts, deferred, etaChanges, items, count: items.length };
}

/**
 * The seen-arrival baselines after looking at `rows`: a first sighting becomes the baseline and small
 * moves update it quietly; a move of 30 minutes or more is left in place so it shows as a notice.
 * Returns the new map, or null when nothing changed.
 */
export function nextSeenEta(rows, seenEta = {}) {
  const next = { ...seenEta };
  let changed = false;
  for (const row of rows) {
    const now = row.planning.plannedArrivalAt;
    if (!now) continue;
    const before = next[row.order.id];
    if (!before || Math.abs(new Date(now).getTime() - new Date(before).getTime()) < ETA_THRESHOLD_MINUTES * 60000) {
      if (before !== now) { next[row.order.id] = now; changed = true; }
    }
  }
  return changed ? next : null;
}

/** Acknowledged deferral keys that still match a deferred order; null when nothing needs dropping. */
export function prunedAcknowledged(rows, acknowledged) {
  const current = new Set(rows.filter((row) => isDeferred(row.stage)).map(deferralKey));
  const kept = [...acknowledged].filter((key) => current.has(key));
  return kept.length === acknowledged.size ? null : new Set(kept);
}

/** How long a store message stays in the bell after it was queued. */
export const STORE_MESSAGE_WINDOW_HOURS = 48;
const URGENT_MESSAGES = new Set(["MAJOR_DELAY", "DELIVERY_REJECTED", "LOAD_SHORTFALL"]);

/**
 * Recent messages the system queued for the store (the same notices that go out by text when the store
 * has agreed to texts): a breakdown delay, a short load, a refused delivery, a deferral or an arrival
 * change. They can be marked read in the bell; the full list stays on the notifications page.
 */
export function deriveStoreMessages(messages = [], now = Date.now(), ctx) {
  const { t } = context(ctx);
  const since = now - STORE_MESSAGE_WINDOW_HOURS * 3600_000;
  return messages
    .filter((m) => m && m.body && new Date(m.createdAt).getTime() >= since)
    .map((m) => ({ key: `message:${m.id}`, kind: "store-message", tone: URGENT_MESSAGES.has(m.eventType) ? "red" : "amber", tag: t(messageTypeLabel(m.eventType)), title: m.body, text: colomboTime(m.createdAt), to: "/store-manager/notifications", readable: true }));
}

// --------------------------------------------------------------------- loader

const isReady = (trip) => /ready/i.test(trip.loadingStatus || trip.status || "pending");
const tripLabel = (trip, t) => `${trip.vehicleId || trip.tripId} · ${t("Trip")} ${trip.tripNumber ?? 1}`;

/**
 * What the loader still has to deal with, from today's trips (`trips`) and the detail of trips that have
 * shortfalls (`details`, keyed by trip id): a newer plan version to acknowledge, a dispatcher decision on
 * a shortfall, and a shortfall still waiting for that decision. Mirrors the loader app's own notice count.
 * Trips already ready to depart have nothing outstanding.
 */
export function deriveLoaderItems({ trips = [], details = {} } = {}, ctx) {
  const { t } = context(ctx);
  const items = [];
  for (const trip of trips) {
    if (isReady(trip)) continue;
    const version = trip.planVersion || 1;
    if (version > 1 && (trip.acknowledgedVersion || 0) < version) {
      items.push({ key: `plan:${trip.tripId}:v${version}`, kind: "plan-version", tone: "amber", tag: t("New plan version"), title: `${tripLabel(trip, t)} · ${t("plan")} v${version}`, text: t("The dispatcher published a new plan version. Acknowledge the revised load, then continue."), href: "/loader-app/", action: t("Open the loader app") });
    }
    for (const order of details[trip.tripId]?.orders || []) {
      const issues = order.issues || [];
      if (issues.length === 0) continue;
      const decided = issues.filter((issue) => issue.decision);
      const undecided = issues.filter((issue) => !issue.decision);
      const ref = order.orderRef || order.orderId;
      if (decided.length > 0) {
        items.push({ key: `decision:${trip.tripId}:${order.orderId}:${decided.map((i) => `${i.id}=${i.decision}`).join(",")}`, kind: "decision", tone: "primary", tag: t("Dispatcher decision"), title: `${ref} · ${tripLabel(trip, t)}`, text: decided.map((i) => t(i.decision)).join(", "), href: "/loader-app/", action: t("Open the loader app"), readable: true });
      }
      if (undecided.length > 0) {
        items.push({ key: `awaiting:${trip.tripId}:${order.orderId}:${undecided.map((i) => i.id).join(",")}`, kind: "awaiting-decision", tone: "muted", tag: t("Waiting for dispatcher"), title: `${ref} · ${tripLabel(trip, t)}`, text: t("Your shortfall report is waiting for a dispatcher decision."), href: "/loader-app/", action: t("Open the loader app"), readable: true });
      }
    }
  }
  return items;
}

/** Trips of today the loader should fetch the detail of: unfinished ones that reported a shortfall. */
export function loaderTripsNeedingDetail(trips = []) {
  return trips.filter((trip) => !isReady(trip) && (trip.shortfallCount || 0) > 0);
}

// --------------------------------------------------------------------- driver

/**
 * What the driver has to act on, from today's trips: a plan version to acknowledge, a prepared route that
 * is out of date, and dispatcher messages not yet acknowledged. `details` and `messages` are keyed by trip id.
 */
export function deriveDriverItems({ trips = [], details = {}, messages = {}, userId = "" } = {}, ctx) {
  const { t, dateTime } = context(ctx);
  const items = [];
  for (const trip of trips) {
    if (trip.status === "completed") continue;
    const detail = details[trip.tripId];
    const label = `${trip.vehicleId || trip.tripId} · ${t("Trip")} ${trip.tripNumber ?? 1}`;
    if (detail?.run) {
      const current = detail.currentPlanVersion;
      if (current !== detail.run.planVersion) {
        items.push({ key: `stale:${trip.tripId}:v${current}`, kind: "stale-route", tone: "amber", tag: t("Route out of date"), title: label, text: t("This prepared route is stale. Dispatch must refresh its trip instructions."), to: "/driver/trips", action: t("Open my route"), readable: true });
      } else {
        const acknowledged = !!userId && (detail.planAcknowledgements || []).some((a) => a.actorId === userId && a.actorRole === "DRIVER");
        if (!acknowledged) {
          items.push({ key: `plan:${trip.tripId}:v${current}`, kind: "plan-version", tone: "amber", tag: t("Plan to acknowledge"), title: `${label} · ${t("plan")} v${current}`, text: t("Acknowledge before starting this route"), to: "/driver/trips", action: t("Open my route") });
        }
      }
    }
    for (const message of messages[trip.tripId] || []) {
      if (message.acknowledgedAt) continue;
      items.push({ key: `message:${message.id}`, kind: "message", tone: "primary", tag: t("Dispatcher message"), title: message.body, text: `${label} · ${dateTime(message.createdAt)}`, to: "/driver/trips", action: t("Acknowledge receipt") });
    }
  }
  return items;
}

/** Today's trips the driver should fetch details and messages for. */
export function driverTripsNeedingDetail(trips = []) {
  return trips.filter((trip) => trip.status !== "completed");
}
