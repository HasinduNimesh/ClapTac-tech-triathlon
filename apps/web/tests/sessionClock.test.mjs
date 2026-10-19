import test from "node:test";
import assert from "node:assert/strict";
import { WARN_MS, canRenew, minutesLeft, msUntilChange, sessionPhase } from "../src/auth/sessionClock.mjs";
import { RETURN_KEY, rememberReturn, takeReturn } from "../src/auth/returnTo.mjs";

const now = 1_000_000_000;

test("a session is active, then warns for its last five minutes, then ends", () => {
  assert.equal(sessionPhase(now + 30 * 60_000, now), "active");
  assert.equal(sessionPhase(now + WARN_MS + 1, now), "active");
  assert.equal(sessionPhase(now + WARN_MS, now), "expiring");
  assert.equal(sessionPhase(now + 1000, now), "expiring");
  assert.equal(sessionPhase(now, now), "expired");
  assert.equal(sessionPhase(now - 5000, now), "expired");
});

test("a refused request ends the session even when the clock says there is time left", () => {
  assert.equal(sessionPhase(now + 30 * 60_000, now, true), "expired");
});

test("an unknown expiry is treated as active rather than guessed", () => {
  assert.equal(sessionPhase(NaN, now), "active");
  assert.equal(sessionPhase(undefined, now), "active");
});

test("minutes left rounds up and never shows zero", () => {
  assert.equal(minutesLeft(now + 4 * 60_000 + 10, now), 5);
  assert.equal(minutesLeft(now + 1000, now), 1);
  assert.equal(minutesLeft(now - 1000, now), 1);
});

test("the timer wakes at the warning, then at the end", () => {
  assert.equal(msUntilChange(now + 30 * 60_000, now), 25 * 60_000);
  assert.equal(msUntilChange(now + 3 * 60_000, now), 3 * 60_000);
  assert.equal(msUntilChange(now - 1, now), null);
  assert.equal(msUntilChange(NaN, now), null);
});

test("renewal is possible only with the offline_access scope", () => {
  assert.equal(canRenew("openid profile"), false);
  assert.equal(canRenew("openid profile offline_access"), true);
  assert.equal(canRenew(undefined), false);
  assert.equal(canRenew("openid offline_access_extra"), false);
});

function store() {
  const data = new Map();
  return { getItem: (k) => data.get(k) ?? null, setItem: (k, v) => void data.set(k, v), removeItem: (k) => void data.delete(k), data };
}

test("signing in again returns to the page the person was on, once", () => {
  const s = store();
  rememberReturn(s, "/dispatcher/planning?date=2026-10-07");
  assert.equal(takeReturn(s, "DISPATCHER"), "/dispatcher/planning?date=2026-10-07");
  assert.equal(takeReturn(s, "DISPATCHER"), null, "it is used once");
});

test("a remembered page is only used inside the role's own area", () => {
  const s = store();
  rememberReturn(s, "/dispatcher/planning");
  assert.equal(takeReturn(s, "STORE_MANAGER"), null);
  assert.equal(s.data.has(RETURN_KEY), false, "and it is discarded");
  rememberReturn(s, "/store-manager/orders/new");
  assert.equal(takeReturn(s, "STORE_MANAGER"), "/store-manager/orders/new");
  rememberReturn(s, "/store-manager");
  assert.equal(takeReturn(s, "STORE_MANAGER"), "/store-manager");
  rememberReturn(s, "/store-manager-evil");
  assert.equal(takeReturn(s, "STORE_MANAGER"), null, "a lookalike prefix is not the area");
  rememberReturn(s, "/dispatcher/x");
  assert.equal(takeReturn(s, "LOADER"), null, "roles without a web area have none");
});

test("anything that could send someone elsewhere is refused", () => {
  for (const bad of ["https://evil.example/x", "//evil.example/x", "/\\evil.example", "javascript:alert(1)", "/login", "/auth/callback?code=1", "dispatcher/x", "/dispatcher/\u0000x", "/dispatcher/" + "a".repeat(400)]) {
    const s = store();
    rememberReturn(s, bad);
    assert.equal(s.data.has(RETURN_KEY), false, `${bad.slice(0, 30)} must not even be stored`);
  }
  const tampered = store();
  tampered.setItem(RETURN_KEY, "https://evil.example/dispatcher");
  assert.equal(takeReturn(tampered, "DISPATCHER"), null, "a value written by something else is not trusted either");
});

test("blocked storage never throws", () => {
  const blocked = { getItem() { throw new Error("blocked"); }, setItem() { throw new Error("blocked"); }, removeItem() { throw new Error("blocked"); } };
  rememberReturn(blocked, "/dispatcher/x");
  assert.equal(takeReturn(blocked, "DISPATCHER"), null);
});
