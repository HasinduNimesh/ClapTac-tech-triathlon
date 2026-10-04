// Pure logic for the driver's offline sync queue: send order, plan-version
// stamping, per-stop status, and the plan-conflict notice. No browser APIs.

const RANK_RECORD = 0; // start, arrival, temperature, incidents, custody, failed outcomes
const RANK_PHOTO = 1; // signature / photo uploads
const RANK_NEEDS_PHOTO = 2; // delivered/partial outcomes and custody that cite a queued photo
const RANK_COMPLETION = 3; // route completion needs every stop settled

/** The plan version the driver screen shows for a trip, or undefined when unknown. */
export function planVersionShown(detail) {
  const shown = detail?.currentPlanVersion || detail?.run?.planVersion;
  return Number.isInteger(shown) && shown > 0 ? shown : undefined;
}

/** Stamp a new queue item with the plan version the driver was looking at. */
export function stampPlanVersion(item, detail) {
  if (Number.isInteger(item?.planVersion) && item.planVersion > 0) return item;
  const planVersion = planVersionShown(detail);
  return planVersion === undefined ? item : { ...item, planVersion };
}

/**
 * Send saved records first, then photos. The delivery service refuses a
 * delivered/partial outcome (and a Tech custody record) until the photo it
 * cites has arrived, so those few items wait behind their photo; route
 * completion always goes last. Order inside each group is the order the driver
 * made the records, and an item never moves ahead of work it depends on.
 */
export function orderForSync(items) {
  const queue = Array.isArray(items) ? items : [];
  const photoIds = new Set(queue.filter((item) => item.type === "PROOF_UPLOAD").map((item) => item.operationId));
  const rank = new Map();
  queue.forEach((item, index) => {
    let value = RANK_RECORD;
    if (item.type === "PROOF_UPLOAD") value = RANK_PHOTO;
    else if (item.type === "ROUTE_COMPLETED") value = RANK_COMPLETION;
    else {
      const needs = item.dependsOnOperationId || item.payload?.evidenceRef;
      if (needs && photoIds.has(needs)) value = RANK_NEEDS_PHOTO;
    }
    rank.set(index, value);
  });
  const indexByOperation = new Map(queue.map((item, index) => [item.operationId, index]));
  for (let pass = 0; pass < queue.length; pass += 1) {
    let changed = false;
    queue.forEach((item, index) => {
      const dependency = indexByOperation.get(item.dependsOnOperationId);
      if (dependency !== undefined && rank.get(dependency) > rank.get(index)) {
        rank.set(index, rank.get(dependency));
        changed = true;
      }
    });
    if (!changed) break;
  }
  return queue
    .map((item, index) => ({ item, index }))
    .sort((a, b) => rank.get(a.index) - rank.get(b.index) || a.index - b.index)
    .map(({ item }) => item);
}

/** The /delivery/sync operation for a queued item, carrying its plan version when known. */
export function syncOperationBody(item) {
  const operation = {
    operationId: item.operationId,
    type: item.type,
    tripId: item.tripId,
    stopId: item.stopId,
    occurredAt: item.payload?.occurredAt,
    dependsOnOperationId: item.dependsOnOperationId,
    payload: item.payload,
  };
  if (Number.isInteger(item.planVersion) && item.planVersion > 0) operation.planVersion = item.planVersion;
  return operation;
}

/** Read the conflict the server attaches to a sync result; null when absent or malformed. */
export function readSyncConflict(result) {
  const conflict = result?.conflict;
  const recorded = conflict?.recordedPlanVersion;
  const current = conflict?.currentPlanVersion;
  if (!Number.isInteger(recorded) || !Number.isInteger(current) || recorded < 1 || current <= recorded) return null;
  return { recordedPlanVersion: recorded, currentPlanVersion: current, detail: typeof conflict.detail === "string" ? conflict.detail : "" };
}

/** Plain-language notice shown beside a record that was made on an older plan. */
export function conflictMessage(conflict, t = (text) => text) {
  return `${t("Recorded on plan")} v${conflict.recordedPlanVersion} — ${t("the current plan is")} v${conflict.currentPlanVersion}. ${t("Your record was kept; dispatch will review it.")}`;
}

/** Notices that belong beside one stop (or, with no stopId, the trip-wide ones). */
export function conflictsForStop(notices, tripId, stopId) {
  return (notices || []).filter((notice) => notice.tripId === tripId && (notice.stopId || "") === (stopId || ""));
}

/**
 * Sync status for one stop: "saved" (Saved on this phone), "sending", "failed"
 * (shows Retry), "sent", or "none" for a stop with no records yet.
 * `failed` maps operation id to the reason it could not be sent.
 */
export function deriveStopSyncStatus({ stopId, stopStatus, queue, sendingIds = [], failed = {} }) {
  const mine = (queue || []).filter((item) => item.stopId === stopId);
  const sending = new Set(sendingIds);
  const failedItems = mine.filter((item) => Object.prototype.hasOwnProperty.call(failed, item.operationId))
    .map((item) => ({ operationId: item.operationId, type: item.type, reason: failed[item.operationId] }));
  let state = "none";
  if (failedItems.length) state = "failed";
  else if (mine.some((item) => sending.has(item.operationId))) state = "sending";
  else if (mine.length) state = "saved";
  else if (stopStatus && stopStatus !== "pending") state = "sent";
  return { state, waiting: mine.length, failedItems };
}

/** How many records and photos are still waiting on this phone. */
export function waitingCounts(queue) {
  const items = Array.isArray(queue) ? queue : [];
  const photos = items.filter((item) => item.type === "PROOF_UPLOAD").length;
  return { total: items.length, photos, records: items.length - photos };
}

export const SYNC_STATE_LABELS = {
  saved: "Saved on this phone",
  sending: "Sending",
  sent: "Sent",
  failed: "Could not send",
};

export function syncStateLabel(state) {
  return SYNC_STATE_LABELS[state] || "";
}
