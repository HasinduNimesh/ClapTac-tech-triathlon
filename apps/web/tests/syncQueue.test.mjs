import assert from "node:assert/strict";
import test from "node:test";
import { drainFIFOQueue } from "../src/offline/fifoQueue.mjs";
import { translate } from "../src/locale.mjs";
import { createSyncProgress } from "../src/offline/syncProgress.mjs";
import {
  classifySyncResult,
  conflictMessage,
  conflictsForStop,
  deriveStopSyncStatus,
  orderForSync,
  planVersionShown,
  readSyncConflict,
  stampPlanVersion,
  syncOperationBody,
  syncStateLabel,
  waitingCounts,
} from "../src/offline/syncQueue.mjs";

const item = (operationId, type, extra = {}) => ({ operationId, type, tripId: "t1", ...extra });
const ids = (items) => items.map((i) => i.operationId);

test("records are sent before photos, in the order they were made", () => {
  const queue = [
    item("start", "START"),
    item("arrive-1", "ARRIVED", { stopId: "s1" }),
    item("photo-1", "PROOF_UPLOAD", { stopId: "s1" }),
    item("temp-1", "TEMPERATURE_READING", { stopId: "s1" }),
    item("arrive-2", "ARRIVED", { stopId: "s2" }),
    item("photo-2", "PROOF_UPLOAD", { stopId: "s2" }),
    item("incident", "INCIDENT_REPORT"),
  ];
  assert.deepEqual(ids(orderForSync(queue)), ["start", "arrive-1", "temp-1", "arrive-2", "incident", "photo-1", "photo-2"]);
});

test("an outcome or custody record that cites a queued photo waits behind that photo, and completion goes last", () => {
  const queue = [
    item("arrive", "ARRIVED", { stopId: "s1" }),
    item("photo", "PROOF_UPLOAD", { stopId: "s1" }),
    item("custody", "CUSTODY_RECORD", { stopId: "s1", payload: { stage: "DELIVERED", evidenceRef: "photo" } }),
    item("outcome", "STOP_OUTCOME", { stopId: "s1", dependsOnOperationId: "photo" }),
    item("arrive-2", "ARRIVED", { stopId: "s2" }),
    item("failed-2", "STOP_OUTCOME", { stopId: "s2", payload: { code: "FAILED" } }),
    item("complete", "ROUTE_COMPLETED"),
  ];
  const ordered = ids(orderForSync(queue));
  assert.deepEqual(ordered, ["arrive", "arrive-2", "failed-2", "photo", "custody", "outcome", "complete"]);
  assert.ok(ordered.indexOf("photo") < ordered.indexOf("outcome"));
  assert.ok(ordered.indexOf("photo") < ordered.indexOf("custody"));
});

test("a dependency on a photo that was already sent does not hold the record back", () => {
  const queue = [
    item("photo-late", "PROOF_UPLOAD", { stopId: "s2" }),
    item("outcome", "STOP_OUTCOME", { stopId: "s1", dependsOnOperationId: "already-sent-photo" }),
  ];
  assert.deepEqual(ids(orderForSync(queue)), ["outcome", "photo-late"]);
});

test("ordering never mutates the saved queue and tolerates an empty queue", () => {
  const queue = [item("photo", "PROOF_UPLOAD"), item("arrive", "ARRIVED")];
  const before = ids(queue);
  orderForSync(queue);
  assert.deepEqual(ids(queue), before);
  assert.deepEqual(orderForSync([]), []);
  assert.deepEqual(orderForSync(undefined), []);
});

test("draining the ordered queue uploads photos only after every record is accepted", async () => {
  const pending = [
    item("photo", "PROOF_UPLOAD", { stopId: "s1" }),
    item("arrive", "ARRIVED", { stopId: "s1" }),
    item("outcome", "STOP_OUTCOME", { stopId: "s1", dependsOnOperationId: "photo" }),
  ].map((entry, index) => ({ id: index + 1, ...entry }));
  const sent = [];
  const result = await drainFIFOQueue({
    list: async () => orderForSync(pending),
    remove: async (id) => { pending.splice(pending.findIndex((entry) => entry.id === id), 1); },
    apply: async (entry) => { sent.push(entry.operationId); return { applied: true }; },
    onFailure: (error) => ({ kind: "error", text: String(error) }),
  });
  assert.deepEqual(sent, ["arrive", "photo", "outcome"]);
  assert.equal(result.kind, "ok");
});

test("a failed photo upload keeps every unsent record and blocks only what depends on it", async () => {
  const pending = [
    item("arrive", "ARRIVED", { stopId: "s1" }),
    item("photo", "PROOF_UPLOAD", { stopId: "s1" }),
    item("outcome", "STOP_OUTCOME", { stopId: "s1", dependsOnOperationId: "photo" }),
  ].map((entry, index) => ({ id: index + 1, ...entry }));
  const progress = createSyncProgress();
  const result = await drainFIFOQueue({
    list: async () => orderForSync(pending),
    remove: async (id) => { pending.splice(pending.findIndex((entry) => entry.id === id), 1); },
    apply: async (entry) => {
      progress.markSending(entry.operationId);
      if (entry.type === "PROOF_UPLOAD") throw new Error("upload failed");
      return { applied: true };
    },
    onFailure: (error, entry) => { progress.markFailed(entry.operationId, error.message); return { kind: "error", text: error.message }; },
  });
  assert.equal(result.kind, "error");
  assert.deepEqual(ids(pending), ["photo", "outcome"]);
  const status = deriveStopSyncStatus({ stopId: "s1", stopStatus: "arrived", queue: pending, ...progress.snapshot() });
  assert.equal(status.state, "failed");
  assert.deepEqual(status.failedItems.map((f) => f.operationId), ["photo"]);
  assert.equal(status.waiting, 2);
});

test("planVersion: the version the screen shows is stamped on each queued item", () => {
  assert.equal(planVersionShown({ currentPlanVersion: 3, run: { planVersion: 2 } }), 3);
  assert.equal(planVersionShown({ run: { planVersion: 2 } }), 2);
  assert.equal(planVersionShown({}), undefined);
  assert.equal(planVersionShown(null), undefined);

  const stamped = stampPlanVersion(item("arrive", "ARRIVED"), { currentPlanVersion: 1, run: { planVersion: 1 } });
  assert.equal(stamped.planVersion, 1);
  assert.equal(stamped.operationId, "arrive");
  // The original object is left alone, and an unknown version adds nothing.
  const plain = item("x", "ARRIVED");
  assert.equal(stampPlanVersion(plain, { currentPlanVersion: 4 }).planVersion, 4);
  assert.equal("planVersion" in plain, false);
  assert.equal("planVersion" in stampPlanVersion(plain, undefined), false);
  // A version already on the record is never overwritten by a later screen.
  assert.equal(stampPlanVersion({ ...plain, planVersion: 1 }, { currentPlanVersion: 2 }).planVersion, 1);
});

test("the sync operation carries planVersion only when it is known", () => {
  const stamped = syncOperationBody({ ...item("op", "STOP_OUTCOME", { stopId: "s1", payload: { occurredAt: "2026-10-04T09:00:00Z", code: "DELIVERED" } }), planVersion: 1 });
  assert.equal(stamped.planVersion, 1);
  assert.equal(stamped.occurredAt, "2026-10-04T09:00:00Z");
  assert.equal(stamped.operationId, "op");
  assert.equal("planVersion" in syncOperationBody(item("op2", "ARRIVED")), false);
  assert.equal("planVersion" in syncOperationBody({ ...item("op3", "ARRIVED"), planVersion: 0 }), false);
});

test("a sync result with an older-plan conflict is read; anything malformed is ignored", () => {
  assert.deepEqual(
    readSyncConflict({ status: "APPLIED", conflict: { recordedPlanVersion: 1, currentPlanVersion: 2, detail: "kept" } }),
    { recordedPlanVersion: 1, currentPlanVersion: 2, detail: "kept" },
  );
  assert.equal(readSyncConflict({ status: "APPLIED" }), null);
  assert.equal(readSyncConflict(undefined), null);
  assert.equal(readSyncConflict({ conflict: { recordedPlanVersion: "1", currentPlanVersion: 2 } }), null);
  assert.equal(readSyncConflict({ conflict: { recordedPlanVersion: 2, currentPlanVersion: 2 } }), null);
  assert.equal(readSyncConflict({ conflict: { recordedPlanVersion: 0, currentPlanVersion: 2 } }), null);
});

test("the conflict notice names both plan versions and says the record was kept", () => {
  assert.equal(
    conflictMessage({ recordedPlanVersion: 1, currentPlanVersion: 2 }),
    "Recorded on plan v1 — the current plan is v2. Your record was kept; dispatch will review it.",
  );
  const si = conflictMessage({ recordedPlanVersion: 1, currentPlanVersion: 2 }, (text) => translate("si", text));
  assert.match(si, /v1/);
  assert.match(si, /v2/);
  assert.doesNotMatch(si, /Your record was kept/);
});

test("notices are matched to their own stop, with trip-wide ones kept separate", () => {
  const notices = [
    { operationId: "a", tripId: "t1", stopId: "s1" },
    { operationId: "b", tripId: "t1", stopId: "s2" },
    { operationId: "c", tripId: "t1" },
    { operationId: "d", tripId: "t2", stopId: "s1" },
  ];
  assert.deepEqual(conflictsForStop(notices, "t1", "s1").map((n) => n.operationId), ["a"]);
  assert.deepEqual(conflictsForStop(notices, "t1", undefined).map((n) => n.operationId), ["c"]);
  assert.deepEqual(conflictsForStop(undefined, "t1", "s1"), []);
});

test("stop status moves from Saved on this phone to Sending to Sent", () => {
  const queue = [item("arrive", "ARRIVED", { stopId: "s1" }), item("photo", "PROOF_UPLOAD", { stopId: "s1" }), item("other", "ARRIVED", { stopId: "s2" })];
  const base = { stopId: "s1", stopStatus: "arrived", queue };

  const saved = deriveStopSyncStatus(base);
  assert.equal(saved.state, "saved");
  assert.equal(saved.waiting, 2);
  assert.equal(syncStateLabel(saved.state), "Saved on this phone");

  const sending = deriveStopSyncStatus({ ...base, sendingIds: ["arrive"] });
  assert.equal(sending.state, "sending");
  assert.equal(syncStateLabel(sending.state), "Sending");

  const sent = deriveStopSyncStatus({ stopId: "s1", stopStatus: "completed", queue: [queue[2]] });
  assert.equal(sent.state, "sent");
  assert.equal(sent.waiting, 0);
  assert.equal(syncStateLabel(sent.state), "Sent");

  assert.equal(deriveStopSyncStatus({ stopId: "s3", stopStatus: "pending", queue }).state, "none");
  assert.equal(syncStateLabel("none"), "");
});

test("a failed record shows Could not send even while another record on the stop is sending", () => {
  const queue = [item("arrive", "ARRIVED", { stopId: "s1" }), item("photo", "PROOF_UPLOAD", { stopId: "s1" })];
  const status = deriveStopSyncStatus({ stopId: "s1", stopStatus: "arrived", queue, sendingIds: ["arrive"], failed: { photo: "HTTP 500" } });
  assert.equal(status.state, "failed");
  assert.equal(status.failedItems[0].reason, "HTTP 500");
  assert.equal(syncStateLabel("failed"), "Could not send");
});

test("waiting counts split records from photos", () => {
  assert.deepEqual(waitingCounts([item("a", "ARRIVED"), item("p", "PROOF_UPLOAD"), item("o", "STOP_OUTCOME")]), { total: 3, photos: 1, records: 2 });
  assert.deepEqual(waitingCounts([]), { total: 0, photos: 0, records: 0 });
  assert.deepEqual(waitingCounts(undefined), { total: 0, photos: 0, records: 0 });
});

test("progress marks sending, failed, retry and saved, and tells subscribers", () => {
  const progress = createSyncProgress();
  const seen = [];
  const stop = progress.subscribe((snapshot) => seen.push(snapshot));
  progress.markSending("a");
  assert.deepEqual(progress.snapshot(), { sendingIds: ["a"], failed: {} });
  progress.markFailed("a", "HTTP 503");
  assert.deepEqual(progress.snapshot(), { sendingIds: [], failed: { a: "HTTP 503" } });
  progress.clearFailed("a");
  assert.deepEqual(progress.snapshot(), { sendingIds: [], failed: {} });
  progress.markFailed("b", "x");
  progress.markSending("b");
  assert.deepEqual(progress.snapshot(), { sendingIds: ["b"], failed: {} });
  progress.markSaved("b");
  assert.deepEqual(progress.snapshot(), { sendingIds: [], failed: {} });
  stop();
  progress.markSending("c");
  assert.equal(seen.length, 6);
});

test("Sinhala and Tamil provide the offline sync status, retry and plan-notice text", () => {
  const keys = ["Saved on this phone", "Sending", "Sent", "Could not send", "Got it", "Retry", "Saved records are sent first, then photos.", "This could not be sent. Your record is still saved on this phone.", "the current plan is", "Your record was kept; dispatch will review it.", "Recorded on plan"];
  for (const key of keys) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
  assert.equal(translate("en", "Sent"), "Sent");
});

test("the waiting counts translate with the number kept", () => {
  for (const locale of ["si", "ta"]) {
    assert.match(translate(locale, "3 waiting"), /^.*3.*$/);
    assert.notEqual(translate(locale, "3 waiting"), "3 waiting");
    assert.match(translate(locale, "2 photo(s) waiting"), /2/);
    assert.notEqual(translate(locale, "2 photo(s) waiting"), "2 photo(s) waiting");
  }
  assert.equal(translate("en", "3 waiting"), "3 waiting");
});

test("a RETRY result is not success: the queue item stays and shows Could not send", async () => {
  assert.deepEqual(classifySyncResult({ status: "APPLIED" }), { applied: true, retry: false, detail: "" });
  assert.equal(classifySyncResult({ status: "DUPLICATE", originalStatus: "APPLIED" }).applied, true);
  assert.equal(classifySyncResult({ status: "DUPLICATE", originalStatus: "REJECTED" }).applied, false);
  const retry = classifySyncResult({ status: "RETRY", originalStatus: "APPLIED", detail: "Send it again." });
  assert.deepEqual(retry, { applied: false, retry: true, detail: "Send it again." });
  assert.equal(classifySyncResult({ status: "RETRY" }).applied, false);
  assert.equal(classifySyncResult(undefined).applied, false);

  // Drive the real drain loop the way sync.ts does: the RETRY item must not be removed.
  const progress = createSyncProgress();
  let queue = [item("op-old", "STOP_OUTCOME", { id: 1, stopId: "s1" })];
  const removed = [];
  const recovered = { status: "DUPLICATE", originalStatus: "APPLIED", conflict: { recordedPlanVersion: 3, currentPlanVersion: 4, detail: "x" } };
  const responses = [{ status: "RETRY", originalStatus: "APPLIED", detail: "Send it again." }, recovered];
  const run = () => drainFIFOQueue({
    list: async () => queue,
    remove: async (id) => { removed.push(id); queue = queue.filter((q) => q.id !== id); },
    apply: async (queued) => {
      progress.markSending(queued.operationId);
      const outcome = classifySyncResult(responses.shift());
      if (!outcome.applied) {
        progress.markFailed(queued.operationId, outcome.detail);
        return { applied: false, detail: outcome.detail };
      }
      return { applied: true };
    },
    onFailure: async () => ({ kind: "error", text: "failed" }),
  });

  const first = await run();
  assert.equal(first.kind, "error");
  assert.deepEqual(removed, []);
  assert.equal(queue.length, 1);
  const status = deriveStopSyncStatus({ stopId: "s1", queue, failed: progress.snapshot().failed });
  assert.equal(status.state, "failed");
  assert.equal(syncStateLabel(status.state), "Could not send");

  progress.clearFailed("op-old");
  const second = await run();
  assert.equal(second.kind, "ok");
  assert.deepEqual(removed, [1]);
  assert.equal(readSyncConflict(recovered).currentPlanVersion, 4);
});
