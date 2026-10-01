import assert from "node:assert/strict";
import test from "node:test";
import { bindSingleOwner } from "../src/offline/singleOwner.mjs";

function memoryStore(initial = "", queue = []) {
  let owner = initial;
  return {
    get: async () => owner,
    set: async (value) => { owner = value; },
    peek: () => owner,
    queue,
  };
}

test("first driver claims the device and can return to it", async () => {
  const store = memoryStore();
  assert.equal(await bindSingleOwner(store, "driver-1"), true);
  assert.equal(await bindSingleOwner(store, "driver-1"), true);
  assert.equal(store.peek(), "driver-1");
});

test("another driver is rejected without changing the owner or stored data", async () => {
  const store = memoryStore("driver-1", ["unsynced proof"]);
  assert.equal(await bindSingleOwner(store, "driver-2"), false);
  assert.equal(store.peek(), "driver-1");
  assert.deepEqual(store.queue, ["unsynced proof"]);
});

test("empty identities cannot claim a device", async () => {
  const store = memoryStore();
  assert.equal(await bindSingleOwner(store, ""), false);
  assert.equal(store.peek(), "");
});
