import test from "node:test";
import assert from "node:assert/strict";
import {
  PROFILE_CACHE_MAX_AGE_MS,
  clearCachedProfile,
  loadAuthenticatedProfile,
  readCachedProfile,
  saveCachedProfile,
} from "../src/auth/profileCache.mjs";

const subject = "oidc-driver-17";
const profile = {
  userId: "DRIVER000017",
  subject,
  roles: ["DRIVER"],
  depot: "Peliyagoda",
  vehicleId: "VEH001",
};

function memoryStorage() {
  const values = new Map();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
    removeItem: (key) => values.delete(key),
  };
}

test("an authenticated server profile is cached and returned", async () => {
  const storage = memoryStorage();
  const result = await loadAuthenticatedProfile({
    fetchImpl: async (url, init) => {
      assert.equal(url, "/api/v1/shared/profiles/me");
      assert.equal(init.headers.Authorization, "Bearer token-1");
      return { ok: true, json: async () => ({ profile }) };
    },
    storage,
    subject,
    accessToken: "token-1",
    now: 10_000,
  });

  assert.deepEqual(result, profile);
  assert.deepEqual(readCachedProfile(storage, subject, 10_001), profile);
});

test("a transport failure restores only a fresh profile for the signed-in subject", async () => {
  const storage = memoryStorage();
  saveCachedProfile(storage, subject, profile, 10_000);
  const result = await loadAuthenticatedProfile({
    fetchImpl: async () => { throw new TypeError("Failed to fetch"); },
    storage,
    subject,
    accessToken: "token-1",
    now: 10_001,
  });

  assert.deepEqual(result, profile);
  assert.equal(await loadAuthenticatedProfile({
    fetchImpl: async () => { throw new TypeError("Failed to fetch"); },
    storage,
    subject: "different-driver",
    accessToken: "other-token",
    now: 10_001,
  }), null);
});

test("offline profile cache expires after 24 hours and rejects future timestamps", () => {
  const storage = memoryStorage();
  saveCachedProfile(storage, subject, profile, 10_000);
  assert.equal(readCachedProfile(storage, subject, 10_000 + PROFILE_CACHE_MAX_AGE_MS + 1), null);
  assert.equal(storage.getItem("waypoint.auth.profile.v1"), null);
  saveCachedProfile(storage, subject, profile, 20_000);
  assert.equal(readCachedProfile(storage, subject, 19_999), null);
});

test("server authorization failures clear the role cache and never fall back", async () => {
  const storage = memoryStorage();
  saveCachedProfile(storage, subject, profile, 10_000);
  for (const status of [401, 403]) {
    const result = await loadAuthenticatedProfile({
      fetchImpl: async () => ({ ok: false, status }),
      storage,
      subject,
      accessToken: "token-1",
      now: 10_001,
    });
    assert.equal(result, null);
    assert.equal(readCachedProfile(storage, subject, 10_001), null);
    saveCachedProfile(storage, subject, profile, 10_000);
  }
});

test("server errors, invalid profiles, and missing identity never use stale roles", async () => {
  const storage = memoryStorage();
  saveCachedProfile(storage, subject, profile, 10_000);
  const args = { storage, subject, accessToken: "token-1", now: 10_001 };
  assert.equal(await loadAuthenticatedProfile({
    ...args,
    fetchImpl: async () => ({ ok: false, status: 503 }),
  }), null);
  assert.equal(await loadAuthenticatedProfile({
    ...args,
    fetchImpl: async () => ({ ok: true, json: async () => ({ profile: { ...profile, subject: "other" } }) }),
  }), null);
  assert.equal(await loadAuthenticatedProfile({
    ...args,
    subject: undefined,
    fetchImpl: async () => { throw new TypeError("offline"); },
  }), null);
});

test("logout removes the persisted profile", () => {
  const storage = memoryStorage();
  saveCachedProfile(storage, subject, profile, 10_000);
  clearCachedProfile(storage);
  assert.equal(readCachedProfile(storage, subject, 10_001), null);
});
