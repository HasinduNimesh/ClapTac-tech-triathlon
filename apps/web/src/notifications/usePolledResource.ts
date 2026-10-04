import { useCallback, useEffect, useRef, useState } from "react";
import { nextPollDelay, POLL_INTERVAL_MS, shouldRefreshOnVisible } from "./polling.mjs";
import { NOTIFICATION_REFRESH_EVENT } from "./readState.mjs";

export type Polled<T> = { data: T | null; error: string; loading: boolean; reload: () => void };

/**
 * Loads something now and then again every minute. Polling pauses while the tab is hidden and resumes
 * (with an immediate refresh when the data is old) when it is shown again; failures wait longer each
 * time. The last good data is kept while a refresh fails. `fetcher` may throw; it is read through a ref
 * so a new function each render does not restart polling. Pass `key` to start over (new person, new day).
 */
export function usePolledResource<T>(fetcher: () => Promise<T>, enabled: boolean, key: string, baseMs = POLL_INTERVAL_MS): Polled<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(enabled);
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;
  const controls = useRef<{ refresh: () => void }>({ refresh: () => undefined });

  useEffect(() => {
    if (!enabled) { setLoading(false); return; }
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let failures = 0;
    let lastFetchAt = 0;
    let inFlight = false;
    const hidden = () => typeof document !== "undefined" && document.visibilityState === "hidden";

    const schedule = () => {
      if (timer) clearTimeout(timer);
      timer = undefined;
      if (cancelled) return;
      const delay = nextPollDelay({ baseMs, failures, hidden: hidden() });
      if (delay !== null) timer = setTimeout(() => void run(), delay);
    };
    const run = async () => {
      if (cancelled || inFlight) return;
      inFlight = true;
      try {
        const next = await fetcherRef.current();
        if (cancelled) return;
        setData(next);
        setError("");
        failures = 0;
      } catch (err) {
        if (cancelled) return;
        failures += 1;
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        inFlight = false;
        lastFetchAt = Date.now();
        if (!cancelled) { setLoading(false); schedule(); }
      }
    };
    const onVisibility = () => {
      if (hidden()) { if (timer) clearTimeout(timer); timer = undefined; return; }
      if (shouldRefreshOnVisible({ lastFetchAt, now: Date.now(), baseMs, failures })) void run();
      else schedule();
    };
    controls.current = { refresh: () => { if (!hidden()) void run(); } };
    const onRefreshRequest = () => controls.current.refresh();

    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener(NOTIFICATION_REFRESH_EVENT, onRefreshRequest);
    setData(null);
    setError("");
    setLoading(true);
    if (hidden()) schedule(); else void run();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener(NOTIFICATION_REFRESH_EVENT, onRefreshRequest);
    };
  }, [enabled, key, baseMs]);

  const reload = useCallback(() => controls.current.refresh(), []);
  return { data, error, loading, reload };
}
