import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { isUserEntry, storageWithoutRefreshToken, withoutRefreshToken } from "../src/auth/tokenStorage.mjs";

function memoryStorage() {
  const data = new Map();
  return {
    get length() { return data.size; },
    key: (i) => [...data.keys()][i] ?? null,
    getItem: (k) => (data.has(k) ? data.get(k) : null),
    setItem: (k, v) => { data.set(k, String(v)); },
    removeItem: (k) => { data.delete(k); },
    clear: () => data.clear(),
  };
}

const user = JSON.stringify({ access_token: "a.b.c", id_token: "i.d.t", refresh_token: "r.t.k", token_type: "Bearer", scope: "openid profile", expires_at: 1793354188 });

test("the refresh token is removed and every other field is kept", () => {
  const stored = JSON.parse(withoutRefreshToken(user));
  assert.equal("refresh_token" in stored, false);
  assert.equal(stored.access_token, "a.b.c");
  assert.equal(stored.id_token, "i.d.t");
  assert.equal(stored.expires_at, 1793354188);
  assert.equal(stored.scope, "openid profile");
});

test("text that is not a user object is returned unchanged", () => {
  for (const text of ["not json", "[1,2]", "null", "42", JSON.stringify({ access_token: "x" })]) assert.equal(withoutRefreshToken(text), text);
});

test("only the signed-in user entry is rewritten, and only when the app cannot renew", () => {
  assert.equal(isUserEntry("oidc.user:https://id.waypoint.claptac.dev:waypoint-web"), true);
  assert.equal(isUserEntry("oidc.abc123"), false);

  const backing = memoryStorage();
  const store = storageWithoutRefreshToken(backing, false);
  store.setItem("oidc.user:issuer:client", user);
  store.setItem("oidc.state-id", user);
  assert.equal("refresh_token" in JSON.parse(backing.getItem("oidc.user:issuer:client")), false);
  assert.equal("refresh_token" in JSON.parse(backing.getItem("oidc.state-id")), true, "other entries are left alone");
  assert.equal(JSON.parse(store.getItem("oidc.user:issuer:client")).access_token, "a.b.c", "reads still return the access token");
  assert.equal(store.length, 2);
  assert.equal(store.key(0), "oidc.user:issuer:client");
  store.removeItem("oidc.state-id");
  assert.equal(store.length, 1);
});

test("an app set up to renew keeps the refresh token it needs", () => {
  const backing = memoryStorage();
  storageWithoutRefreshToken(backing, true).setItem("oidc.user:issuer:client", user);
  assert.equal(JSON.parse(backing.getItem("oidc.user:issuer:client")).refresh_token, "r.t.k");
});

test("the user manager saves the signed-in user through this store", () => {
  const manager = readFileSync(new URL("../src/auth/userManager.ts", import.meta.url), "utf8");
  assert.match(manager, /storageWithoutRefreshToken\(window\.sessionStorage, renewable\)/);
  assert.match(manager, /userStore: new WebStorageStateStore\(\{ store \}\)/);
});
