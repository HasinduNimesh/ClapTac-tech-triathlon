import assert from "node:assert/strict";
import test from "node:test";
import { applyLoadingQueueItem, replayLoadingQueue, shouldQueueLoadingFailure } from "../src/loader/offlineState.mjs";
import { drainFIFOQueue } from "../src/offline/fifoQueue.mjs";

const manifest = () => ({
  tripId: "trip-1",
  status: "pending",
  loadingStatus: "pending",
  planVersion: 4,
  acknowledgedVersion: 4,
  orders: [{ orderId: "order-1", status: "pending", issues: [] }],
});

test("loader offline queue only captures connectivity and server failures", () => {
  assert.equal(shouldQueueLoadingFailure(new TypeError("fetch failed"), true), true);
  assert.equal(shouldQueueLoadingFailure({ status: 503 }, true), true);
  assert.equal(shouldQueueLoadingFailure({ status: 409 }, true), false);
  assert.equal(shouldQueueLoadingFailure({ status: 400 }, true), false);
  assert.equal(shouldQueueLoadingFailure(new Error("offline"), false), true);
});

test("offline loader replay reflects start, shortfall, and ready without mutating the cached manifest", () => {
  const saved = manifest();
  const queued = [
    { operationId: "start-1", type: "START", tripId: "trip-1" },
    { operationId: "issue-1", type: "ISSUE_CREATE", tripId: "trip-1", orderId: "order-1", payload: { type: "MISSING", affectedUnits: 2, note: "Two cartons missing" } },
    { operationId: "ready-1", type: "READY", tripId: "trip-1" },
  ];
  const current = replayLoadingQueue(saved, queued);
  assert.equal(current.status, "ready");
  assert.equal(current.loadingStatus, "ready");
  assert.equal(current.orders[0].status, "shortfall");
  assert.equal(current.orders[0].issues[0].id, "local-issue-1");
  assert.equal(current.orders[0].issues[0].affectedUnits, 2);
  assert.equal(current.shortfallCount, 1);
  assert.equal(current.pendingCount, 0);
  assert.equal(current.loadedCount, 0);
  assert.equal(saved.status, "pending");
  assert.equal(saved.orders[0].issues.length, 0);
});

test("offline loaded confirmation updates cached manifest counts", () => {
  const loaded = applyLoadingQueueItem(manifest(), {
    operationId: "loaded-1", type: "ORDER_LOADED", tripId: "trip-1", orderId: "order-1",
  });
  assert.equal(loaded.orders[0].status, "loaded");
  assert.equal(loaded.loadedCount, 1);
  assert.equal(loaded.shortfallCount, 0);
  assert.equal(loaded.pendingCount, 0);
});

test("loader queue sync stays FIFO and preserves the rejected operation with later work", async () => {
  const pending = [
    { id: 1, operationId: "start-1", type: "START" },
    { id: 2, operationId: "loaded-1", type: "ORDER_LOADED" },
    { id: 3, operationId: "ready-1", type: "READY" },
  ];
  const applied = [];
  const result = await drainFIFOQueue({
    list: async () => pending.slice(),
    remove: async id => { const index = pending.findIndex(item => item.id === id); if (index >= 0) pending.splice(index, 1); },
    apply: async item => {
      applied.push(item.operationId);
      return item.type === "ORDER_LOADED" ? { applied: false, detail: "plan changed before sync" } : { applied: true };
    },
    onFailure: error => ({ kind: "error", text: String(error) }),
  });
  assert.deepEqual(applied, ["start-1", "loaded-1"]);
  assert.deepEqual(pending.map(item => item.operationId), ["loaded-1", "ready-1"]);
  assert.equal(result.kind, "error");
  assert.match(result.text, /plan changed before sync/);
  assert.match(result.text, /2 item\(s\) remain saved/);
});

test("loading an order cannot clear a locally recorded shortfall", () => {
  const issue = { operationId: "issue-1", type: "ISSUE_CREATE", tripId: "trip-1", orderId: "order-1", payload: { type: "DAMAGED", affectedUnits: 1 } };
  const loaded = { operationId: "loaded-1", type: "ORDER_LOADED", tripId: "trip-1", orderId: "order-1" };
  const withIssue = applyLoadingQueueItem(manifest(), issue);
  const afterLoad = applyLoadingQueueItem(withIssue, loaded);
  assert.equal(afterLoad.orders[0].status, "shortfall");
  assert.equal(afterLoad.orders[0].issues.length, 1);
});
