import { useCallback, useEffect, useState } from "react";
import { ApiError, apiJSON } from "../api/client";
import { useAuth } from "../auth/AuthContext";

export type ApiState<T> = { data: T | null; error: string; loading: boolean; reload: () => Promise<void> };

export function errorText(err: unknown) {
  return err instanceof ApiError ? `${err.status}: ${err.message}` : err instanceof Error ? err.message : String(err);
}

// Small read hook for dispatcher screens. A null path skips the request, so a
// screen can wait for an earlier value (for example the selected plan) first.
export function useApi<T>(path: string | null): ApiState<T> {
  const { user } = useAuth();
  const token = user?.access_token || "";
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(Boolean(path));
  const reload = useCallback(async () => {
    if (!token || !path) { setLoading(false); return; }
    setLoading(true);
    try {
      setData(await apiJSON<T>(path, token));
      setError("");
    } catch (err) {
      setError(errorText(err));
    } finally {
      setLoading(false);
    }
  }, [token, path]);
  useEffect(() => { void reload(); }, [reload]);
  return { data, error, loading, reload };
}

export function useToken() {
  const { user } = useAuth();
  return user?.access_token || "";
}

const colombo = "Asia/Colombo";
export function clock(value?: string) {
  if (!value) return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleTimeString("en-LK", { timeZone: colombo, hour: "2-digit", minute: "2-digit", hour12: false });
}
export function dayLabel(value?: string) {
  if (!value) return "—";
  const d = new Date(value.length === 10 ? `${value}T00:00:00+05:30` : value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleDateString("en-GB", { timeZone: colombo, weekday: "short", day: "numeric", month: "short" });
}
export function dateTime(value?: string) {
  if (!value) return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString("en-GB", { timeZone: colombo, day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", hour12: false });
}
export function minutesAgo(value?: string, now = Date.now()) {
  if (!value) return undefined;
  const d = new Date(value).getTime();
  return Number.isNaN(d) ? undefined : Math.max(0, Math.round((now - d) / 60000));
}
export function kg(value: number) { return `${Math.round(value).toLocaleString("en-LK")} kg`; }
export function m3(value: number) { return `${value.toFixed(1)} m³`; }
export function pct(part: number, whole: number) { return whole > 0 ? Math.round((part / whole) * 100) : 0; }
export function hhmm(value?: string) { return value ? value.slice(0, 5) : "—"; }
export function isChilled(temp?: string) { const v = (temp || "").toLowerCase(); return v === "chilled" || v === "frozen" || v === "refrigerated"; }
