import assert from "node:assert/strict";
import test from "node:test";
import { drainFIFOQueue } from "../src/offline/fifoQueue.mjs";

function inMemoryQueue(items) {
  const pending = items.map((item, index) => ({ id: index + 1, ...item }));
  return {
    pending,
    list: async () => pending.slice(),
    remove: async (id) => {
      const index = pending.findIndex((item) => item.id === id);
      if (index >= 0) pending.splice(index, 1);
    },
  };
}

test("FIFO drain removes each operation only after acceptance", async () => {
  const queue = inMemoryQueue([
    { operationId: "start", type: "START" },
    { operationId: "arrive", type: "ARRIVED" },
    { operationId: "temperature", type: "TEMPERATURE_READING", payload: { valueC: 4.2 } },
    { operationId: "proof", type: "PROOF_UPLOAD" },
    { operationId: "outcome", type: "STOP_OUTCOME" },
    { operationId: "complete", type: "ROUTE_COMPLETED" },
  ]);
  const applied = [];
  const result = await drainFIFOQueue({
    ...queue,
    apply: async (item) => { applied.push(item.operationId); return { applied: true }; },
    onFailure: (error) => ({ kind: "error", text: String(error) }),
  });

  assert.deepEqual(applied, ["start", "arrive", "temperature", "proof", "outcome", "complete"]);
  assert.deepEqual(queue.pending, []);
  assert.deepEqual(result, { kind: "ok", text: "Synced" });
});

test("a rejected/conflicting operation stays queued and blocks dependent later work", async () => {
  const queue = inMemoryQueue([
    { operationId: "proof", type: "PROOF_UPLOAD" },
    { operationId: "outcome", type: "STOP_OUTCOME" },
    { operationId: "complete", type: "ROUTE_COMPLETED" },
  ]);
  const applied = [];
  const result = await drainFIFOQueue({
    ...queue,
    apply: async (item) => {
      applied.push(item.operationId);
      return { applied: false, detail: "proof rejected: unsupported media type" };
    },
    onFailure: (error) => ({ kind: "error", text: String(error) }),
  });

  assert.deepEqual(applied, ["proof"]);
  assert.deepEqual(queue.pending.map((item) => item.operationId), ["proof", "outcome", "complete"]);
  assert.equal(result.kind, "error");
  assert.match(result.text, /unsupported media type/);
  assert.match(result.text, /3 item\(s\) remain saved/);
});

test("a transient failure leaves the current and later items available for retry", async () => {
  const queue = inMemoryQueue([
    { operationId: "arrive", type: "ARRIVED" },
    { operationId: "outcome", type: "STOP_OUTCOME" },
  ]);
  const result = await drainFIFOQueue({
    ...queue,
    apply: async () => { throw new TypeError("network unavailable"); },
    onFailure: (_error, _item, count) => ({ kind: "offline", text: `Offline · ${count} queued` }),
  });

  assert.deepEqual(queue.pending.map((item) => item.operationId), ["arrive", "outcome"]);
  assert.deepEqual(result, { kind: "offline", text: "Offline · 2 queued" });
});
