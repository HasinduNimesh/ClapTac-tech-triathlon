import assert from "node:assert/strict";
import test from "node:test";
import { singleFlight } from "../src/offline/singleFlight.mjs";

test("overlapping sync triggers share one FIFO drain", async () => {
  let calls = 0;
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  const run = singleFlight(async (token) => {
    calls += 1;
    await gate;
    return token;
  });

  const first = run("driver-token");
  const second = run("driver-token");
  await Promise.resolve();
  assert.equal(calls, 1);
  release();
  assert.deepEqual(await Promise.all([first, second]), ["driver-token", "driver-token"]);

  assert.equal(await run("driver-token"), "driver-token");
  assert.equal(calls, 2, "a completed drain must release the single-flight lock");
});

test("a rejected drain releases the lock for an actionable retry", async () => {
  let calls = 0;
  const run = singleFlight(async () => {
    calls += 1;
    if (calls === 1) throw new Error("temporary service failure");
    return "synced";
  });

  await assert.rejects(run(), /temporary service failure/);
  assert.equal(await run(), "synced");
  assert.equal(calls, 2);
});

test("distinct owner/token pairs never share another driver's drain", async () => {
  const calls = [];
  const run = singleFlight(async (token, owner) => {
    calls.push(`${owner}:${token}`);
    return owner;
  }, (token, owner) => `${owner}:${token}`);
  assert.deepEqual(await Promise.all([run("token-a", "driver-a"), run("token-b", "driver-b")]), ["driver-a", "driver-b"]);
  assert.deepEqual(calls.sort(), ["driver-a:token-a", "driver-b:token-b"]);
});

test("separate tabs serialize a shared driver's queue with a Web Lock", async () => {
  const tails = new Map();
  const lockNames = [];
  const locks = {
    request(name, _options, callback) {
      lockNames.push(name);
      const previous = tails.get(name) || Promise.resolve();
      let release;
      const held = new Promise((resolve) => { release = resolve; });
      const tail = previous.then(() => held);
      tails.set(name, tail);
      return previous.then(callback).finally(() => {
        release();
        if (tails.get(name) === tail) tails.delete(name);
      });
    },
  };
  let active = 0;
  let maximumActive = 0;
  let calls = 0;
  const createTabDrain = () => singleFlight(async () => {
    calls += 1;
    active += 1;
    maximumActive = Math.max(maximumActive, active);
    await new Promise((resolve) => setTimeout(resolve, 5));
    active -= 1;
  }, (_token, owner) => owner, locks);
  const drainInTabA = createTabDrain();
  const drainInTabB = createTabDrain();

  await Promise.all([drainInTabA("token-a", "driver-1"), drainInTabB("token-b", "driver-1")]);
  assert.equal(calls, 2, "the waiting tab checks for work after acquiring the lock");
  assert.equal(maximumActive, 1, "the shared queue must never be drained concurrently across tabs");
  assert.ok(lockNames.every((name) => name === "waypoint-delivery-sync:driver-1"));
  assert.ok(lockNames.every((name) => !name.includes("token")), "bearer tokens must not appear in lock names");
});

test("unsupported Web Locks fall back to the in-tab single-flight guard", async () => {
  let calls = 0;
  const run = singleFlight(async () => { calls += 1; return calls; }, () => "owner", null);
  await Promise.all([run(), run()]);
  assert.equal(calls, 1);
});
