import test from "node:test";
import assert from "node:assert/strict";
import { createLoaderInactivityTimer, LOADER_INACTIVITY_MS } from "../src/loader/inactivityTimer.mjs";

test("loader activity resets the five-minute sign-out and switching user shares it", async () => {
  let scheduled;
  let signOuts = 0;
  const clock = {
    setTimeout(callback, delay) { scheduled = { callback, delay }; return scheduled; },
    clearTimeout(handle) { if (scheduled === handle) scheduled = undefined; },
  };
  const timer = createLoaderInactivityTimer(async () => { signOuts += 1; }, clock);
  const first = scheduled;
  assert.equal(first.delay, LOADER_INACTIVITY_MS);
  timer.reset();
  assert.notEqual(scheduled, first);
  assert.equal(signOuts, 0);
  const current = scheduled;
  current.callback();
  await timer.switchUser();
  assert.equal(signOuts, 1);
  assert.equal(scheduled, undefined);
  timer.reset();
  assert.equal(scheduled, undefined);
  timer.stop();
});

