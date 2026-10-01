const HOUR_MS = 60 * 60 * 1000;
const DAY_MS = 24 * HOUR_MS;
export const COMPLETED_CACHE_RETENTION_MS = 7 * DAY_MS;

export function canPurgeCompletedCache({ serverConfirmedAt, hasPendingQueue }, now = Date.now()) {
  const completedAt = Date.parse(serverConfirmedAt || "");
  return Number.isFinite(completedAt) && completedAt <= now &&
    now - completedAt >= COMPLETED_CACHE_RETENTION_MS && !hasPendingQueue;
}

/** Warn about long-lived unsynced work. Queue items are deliberately never age-deleted. */
export function queueRetentionWarning(items, now = Date.now()) {
  const timestamps = (items || [])
    .map((item) => Date.parse(item.createdAt || ""))
    .filter((createdAt) => Number.isFinite(createdAt) && createdAt <= now);
  if (!timestamps.length) return null;

  const oldestHours = Math.floor((now - Math.min(...timestamps)) / HOUR_MS);
  if (oldestHours >= 30 * 24) {
    return { level: "critical", ageDays: Math.floor(oldestHours / 24), text: "Unsynced work is over 30 days old. Keep this device and contact dispatch/support to recover it; it will not be deleted automatically." };
  }
  if (oldestHours >= 7 * 24) {
    return { level: "urgent", ageDays: Math.floor(oldestHours / 24), text: "Unsynced work is over 7 days old. Keep this device online and contact dispatch to resolve the saved queue." };
  }
  if (oldestHours >= 24) {
    return { level: "warning", ageDays: Math.floor(oldestHours / 24), text: "Unsynced work is over 24 hours old. Reconnect and sync when safe; saved work is retained on this device." };
  }
  return null;
}
