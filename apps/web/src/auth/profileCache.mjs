const PROFILE_CACHE_KEY = "waypoint.auth.profile.v1";
export const PROFILE_CACHE_MAX_AGE_MS = 24 * 60 * 60 * 1000;

function isProfile(value, subject) {
  return value !== null && typeof value === "object"
    && value.subject === subject
    && typeof value.userId === "string" && value.userId.length > 0
    && Array.isArray(value.roles)
    && value.roles.length > 0
    && value.roles.every((role) => typeof role === "string" && role.length > 0)
    && (value.outletIds === undefined || Array.isArray(value.outletIds))
    && (value.depot === undefined || typeof value.depot === "string")
    && (value.vehicleId === undefined || typeof value.vehicleId === "string");
}

export function clearCachedProfile(storage) {
  try { storage?.removeItem(PROFILE_CACHE_KEY); } catch { /* storage may be unavailable */ }
}

export function readCachedProfile(storage, subject, now = Date.now(), maxAgeMs = PROFILE_CACHE_MAX_AGE_MS) {
  if (!subject || !storage) return null;
  try {
    const raw = storage.getItem(PROFILE_CACHE_KEY);
    if (!raw) return null;
    const entry = JSON.parse(raw);
    if (entry?.version !== 1 || entry.subject !== subject || !Number.isFinite(entry.savedAt)
      || now < entry.savedAt || now - entry.savedAt > maxAgeMs
      || !isProfile(entry.profile, subject)) {
      clearCachedProfile(storage);
      return null;
    }
    return entry.profile;
  } catch {
    clearCachedProfile(storage);
    return null;
  }
}

export function saveCachedProfile(storage, subject, profile, now = Date.now()) {
  if (!storage || !isProfile(profile, subject)) return false;
  try {
    storage.setItem(PROFILE_CACHE_KEY, JSON.stringify({ version: 1, subject, savedAt: now, profile }));
    return true;
  } catch {
    return false;
  }
}

/** Load the server-authorized profile, falling back only on a transport failure. */
export async function loadAuthenticatedProfile({ fetchImpl, storage, subject, accessToken, now = Date.now() }) {
  if (!subject || !accessToken) return null;
  let response;
  try {
    response = await fetchImpl("/api/v1/shared/profiles/me", {
      headers: { Authorization: `Bearer ${accessToken}` },
    });
  } catch {
    return readCachedProfile(storage, subject, now);
  }

  if (!response.ok) {
    if (response.status === 401 || response.status === 403) clearCachedProfile(storage);
    return null;
  }

  try {
    const body = await response.json();
    if (!isProfile(body?.profile, subject)) return null;
    saveCachedProfile(storage, subject, body.profile, now);
    return body.profile;
  } catch {
    return null;
  }
}
