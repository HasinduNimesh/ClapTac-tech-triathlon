/**
 * Per-person notification state kept in localStorage for items the server has no read flag for.
 * Every function takes the storage so it can be tested and so blocked storage never throws.
 */
export const NOTIFICATION_STATE_EVENT = "waypoint:notification-state";
export const NOTIFICATION_REFRESH_EVENT = "waypoint:notification-refresh";

export function browserStorage() {
  try { return globalThis.localStorage; } catch { return undefined; }
}

export function readStateKey(role, userId) {
  return `waypoint.notifications.read:${role}:${userId || "anonymous"}`;
}
// The store manager's notifications page has always used these two keys; the bell shares them.
export const deferralStateKey = (userId) => `sm-acknowledged-deferrals:${userId}`;
export const etaStateKey = (userId) => `sm-seen-eta:${userId}`;

function emit(name) {
  try {
    if (typeof globalThis.dispatchEvent === "function" && typeof globalThis.Event === "function") globalThis.dispatchEvent(new globalThis.Event(name));
  } catch { /* nothing is listening */ }
}

/** Ask every mounted bell to refresh now, for example right after the person acknowledged something. */
export function requestNotificationRefresh() { emit(NOTIFICATION_REFRESH_EVENT); }

function readJSON(storage, key, fallback) {
  try {
    const raw = storage?.getItem(key);
    return raw ? JSON.parse(raw) : fallback;
  } catch { return fallback; }
}

function writeJSON(storage, key, value) {
  try {
    storage?.setItem(key, JSON.stringify(value));
    return true;
  } catch { return false; }
}

export function readIds(storage, key) {
  const value = readJSON(storage, key, []);
  return new Set(Array.isArray(value) ? value.filter((v) => typeof v === "string") : []);
}

/** Add ids to the stored set, merging with what is stored now so two tabs or components do not overwrite each other. */
export function addIds(storage, key, ids) {
  const next = readIds(storage, key);
  for (const id of ids) next.add(id);
  writeJSON(storage, key, [...next]);
  emit(NOTIFICATION_STATE_EVENT);
  return next;
}

export function replaceIds(storage, key, ids) {
  writeJSON(storage, key, [...ids]);
  emit(NOTIFICATION_STATE_EVENT);
}

/** Forget read marks for items that are gone, so the stored list does not grow forever. Returns true when something was dropped. */
export function pruneIds(storage, key, currentKeys) {
  const stored = readIds(storage, key);
  const present = currentKeys instanceof Set ? currentKeys : new Set(currentKeys);
  const kept = [...stored].filter((id) => present.has(id));
  if (kept.length === stored.size) return false;
  writeJSON(storage, key, kept);
  emit(NOTIFICATION_STATE_EVENT);
  return true;
}

export function readSeenEta(storage, userId) {
  const value = readJSON(storage, etaStateKey(userId), {});
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

export function mergeSeenEta(storage, userId, patch) {
  const next = { ...readSeenEta(storage, userId), ...patch };
  writeJSON(storage, etaStateKey(userId), next);
  emit(NOTIFICATION_STATE_EVENT);
  return next;
}
