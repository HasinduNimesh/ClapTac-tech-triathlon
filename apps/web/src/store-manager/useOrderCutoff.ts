import { useEffect, useState } from "react";
import { apiJSON } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { serverTimeAt } from "./cutoff.mjs";

type CutoffResponse = { cutoff?: { localTime?: string }; serverTime?: string };
type Sample = CutoffResponse & { receivedAt: number; token: string };

/** Fresh service time, advanced using performance.now so changing the laptop clock has no effect. */
export function useOrderCutoffStatus(): { localTime?: string; now?: Date } {
  const { user } = useAuth();
  const token = user?.access_token || "";
  const [sample, setSample] = useState<Sample | null>(null);
  const [elapsedNow, setElapsedNow] = useState(() => performance.now());
  useEffect(() => {
    setSample(null);
    if (!token) return;
    let active = true;
    let request = 0;
    let controller: AbortController | undefined;
    async function refresh() {
      if (document.hidden) return;
      const version = ++request;
      controller?.abort();
      controller = new AbortController();
      try {
        const data = await apiJSON<CutoffResponse>("/orders/cutoff", token, { cache: "no-store", signal: controller.signal });
        if (!active || version !== request || document.hidden) return;
        const receivedAt = performance.now();
        setSample({ ...data, receivedAt, token });
        setElapsedNow(receivedAt);
      } catch {
        if (active && version === request) setSample(null);
      }
    }
    // Sleep/resume can pause monotonic clocks on some platforms. Discard that sample and resync.
    function resume() { setSample(null); void refresh(); }
    function visibility() {
      setSample(null);
      ++request;
      controller?.abort();
      if (!document.hidden) void refresh();
    }
    void refresh();
    const tick = window.setInterval(() => setElapsedNow(performance.now()), 1000);
    const poll = window.setInterval(() => void refresh(), 30_000);
    window.addEventListener("focus", resume);
    window.addEventListener("online", resume);
    document.addEventListener("visibilitychange", visibility);
    return () => {
      active = false;
      controller?.abort();
      window.clearInterval(tick);
      window.clearInterval(poll);
      window.removeEventListener("focus", resume);
      window.removeEventListener("online", resume);
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [token]);
  if (!sample || sample.token !== token) return {};
  return { localTime: sample.cutoff?.localTime, now: serverTimeAt(sample.serverTime, sample.receivedAt, elapsedNow) };
}

/** The daily order cutoff the order service applies, as HH:MM Sri Lanka time; undefined until it is read (or if it cannot be). */
export function useOrderCutoff(): string | undefined {
  return useOrderCutoffStatus().localTime;
}
