/** Notification polling stays gentle: one request round a minute, nothing while the tab is hidden, longer waits after failures. */
export const POLL_INTERVAL_MS = 60_000;
export const MAX_POLL_DELAY_MS = 10 * 60_000;
const MAX_BACKOFF_STEPS = 6;

/** How long to wait before the next poll, or null when polling should pause (tab hidden). */
export function nextPollDelay({ baseMs = POLL_INTERVAL_MS, failures = 0, hidden = false } = {}) {
  if (hidden) return null;
  const steps = Math.min(Math.max(0, Math.floor(failures)), MAX_BACKOFF_STEPS);
  return Math.min(baseMs * 2 ** steps, MAX_POLL_DELAY_MS);
}

/** When the tab becomes visible again: fetch now if the data is older than a normal wait, otherwise keep waiting. */
export function shouldRefreshOnVisible({ lastFetchAt = 0, now = Date.now(), baseMs = POLL_INTERVAL_MS, failures = 0 } = {}) {
  if (!lastFetchAt) return true;
  const wait = nextPollDelay({ baseMs, failures, hidden: false });
  return now - lastFetchAt >= wait;
}
