export const POLL_INTERVAL_MS: number;
export const MAX_POLL_DELAY_MS: number;
export function nextPollDelay(options?: { baseMs?: number; failures?: number; hidden?: boolean }): number | null;
export function shouldRefreshOnVisible(options?: { lastFetchAt?: number; now?: number; baseMs?: number; failures?: number }): boolean;
