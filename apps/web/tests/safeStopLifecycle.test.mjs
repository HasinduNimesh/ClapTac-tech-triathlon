import test from "node:test";
import assert from "node:assert/strict";
import { installSafeStopLock } from "../src/driver/safeStopLifecycle.mjs";

function eventTarget() {
  const listeners = new Map();
  return {
    listeners,
    addEventListener(name, callback) { listeners.set(name, callback); },
    removeEventListener(name, callback) {
      if (listeners.get(name) === callback) listeners.delete(name);
    },
    dispatch(name) { listeners.get(name)?.(); },
  };
}

test("page departure always relocks driver actions, even if visibility still says visible", () => {
  const document = Object.assign(eventTarget(), { visibilityState: "visible" });
  const window = eventTarget();
  const locks = [];
  installSafeStopLock({ document, window, onLock: (locked) => locks.push(locked) });

  window.dispatch("pagehide");

  assert.deepEqual(locks, [false]);
});

test("hiding the document relocks driver actions and cleanup removes listeners", () => {
  const document = Object.assign(eventTarget(), { visibilityState: "hidden" });
  const window = eventTarget();
  const locks = [];
  const cleanup = installSafeStopLock({ document, window, onLock: (locked) => locks.push(locked) });

  document.dispatch("visibilitychange");
  cleanup();
  document.dispatch("visibilitychange");
  window.dispatch("pagehide");

  assert.deepEqual(locks, [false]);
  assert.equal(document.listeners.has("visibilitychange"), false);
  assert.equal(window.listeners.has("pagehide"), false);
});

test("returning to a visible document does not unlock actions", () => {
  const document = Object.assign(eventTarget(), { visibilityState: "visible" });
  const window = eventTarget();
  const locks = [];
  installSafeStopLock({ document, window, onLock: (locked) => locks.push(locked) });

  document.dispatch("visibilitychange");

  assert.deepEqual(locks, []);
});
