import assert from "node:assert/strict";
import test from "node:test";
import { createSingleFlightAction } from "../src/driver/singleFlightAction.mjs";

test("overlapping driver transitions submit only once and release after completion", async () => {
  const run = createSingleFlightAction();
  let release;
  let calls = 0;
  const pending = run(async () => {
    calls += 1;
    await new Promise((resolve) => { release = resolve; });
  });

  assert.equal(await run(async () => { calls += 1; }), false);
  assert.equal(calls, 1);
  release();
  assert.equal(await pending, true);
  assert.equal(await run(async () => { calls += 1; }), true);
  assert.equal(calls, 2);
});

test("a failed transition releases the lock for an actionable retry", async () => {
  const run = createSingleFlightAction();
  await assert.rejects(run(async () => { throw new Error("offline"); }), /offline/);
  assert.equal(await run(async () => {}), true);
});
