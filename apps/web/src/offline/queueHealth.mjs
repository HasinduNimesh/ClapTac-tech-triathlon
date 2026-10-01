const HOUR_MS = 60 * 60 * 1000;

/** Reduce local queue state to bounded, identifier-free operational buckets. */
export function offlineQueueHealth(items, now = Date.now()) {
  const queued = Array.isArray(items) ? items : [];
  if (queued.length === 0) return { ageBucket: "none", countBucket: "none" };

  const timestamps = queued
    .map((item) => Date.parse(item?.createdAt || ""))
    .filter((createdAt) => Number.isFinite(createdAt) && createdAt <= now);
  if (!timestamps.length) return { ageBucket: "unknown", countBucket: countBucket(queued.length) };

  const oldestHours = Math.floor((now - Math.min(...timestamps)) / HOUR_MS);
  const ageBucket = oldestHours >= 30 * 24 ? "30d_plus"
    : oldestHours >= 7 * 24 ? "7d_30d"
      : oldestHours >= 24 ? "1d_7d"
        : "lt24h";
  return { ageBucket, countBucket: countBucket(queued.length) };
}

function countBucket(count) {
  return count >= 6 ? "6_plus" : count >= 2 ? "2_5" : "one";
}
