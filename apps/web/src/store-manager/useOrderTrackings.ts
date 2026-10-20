import { useCallback, useEffect, useRef, useState } from "react";
import { apiJSON, Order } from "../api/client";
import { LiveLocation, ArrivalPrediction } from "../api/delivery";

import { useAuth } from "../auth/AuthContext";
import { splitTrackingResults } from "./trackingResults.mjs";

export type Tracking = {
  stage: string;
  order: Order;
  planning: { state?: string; planRef?: string; planId?: string; tripId?: string; depot?: string; stopSequence?: number; reasonCode?: string; reasonComment?: string; plannedArrivalAt?: string; plannedServiceStartAt?: string };
  delivery?: {
    runStatus: string; outcome?: string; reason?: string; vehicleId?: string; depot?: string; location?: LiveLocation; tripId?: string; occurredAt?: string; completedAt?: string;
    arrivalPrediction?: ArrivalPrediction;
    returnedGoods?: { goods: string; units: number; reason: string; resolution: string; occurredAt: string; followupOrderRef?: string; followupDate?: string };

    proofs?: { type: string; mimeType: string; pending: boolean; uploadedAt?: string; receiverName?: string }[];
    loadingShortfallSummary?: { type?: string; affectedUnits?: number; note?: string }[];
    deliveredUnits?: number;
  };
  receiptDue?: { reportBy: string; state: "open" | "due_tomorrow" | "due_today" | "overdue" };
  receipt?: { id?: string; status: string; receivedUnits: number; expectedUnits: number; confirmedAt?: string; confirmedBy?: string; receivedTemperatureC?: number };
  receiptIssues?: { id: string; issueType: string; affectedUnits: number; note?: string; createdAt?: string; createdBy?: string }[];
  custody?: { id: string; stage: string; sealId: string; serialNumbers: string[]; condition: string; recordedBy: string; recordedAt: string; receiverName?: string }[];
};

/** Every order of this outlet with its tracking; orders whose tracking could not be loaded are returned separately. */
type TrackingSet = { rows: Tracking[]; unavailable: Order[] };

// The page and the notification bell both poll this list; they share one request instead of making two.
const SHARE_MS = 5_000;
let shared: { token: string; at: number; promise: Promise<TrackingSet> } | undefined;

async function loadOrderTrackings(token: string): Promise<TrackingSet> {
  const body = await apiJSON<{ items: Order[] }>("/orders", token);
  const settled = await Promise.allSettled(
    (body.items || []).map(async (order) => (await apiJSON<{ tracking: Tracking }>(`/orders/${order.id}/tracking`, token)).tracking),
  );
  return splitTrackingResults(body.items || [], settled);
}

/** Every order of this outlet with its tracking; orders whose tracking could not be loaded are returned separately. */
export function fetchOrderTrackings(token: string): Promise<TrackingSet> {
  if (shared && shared.token === token && Date.now() - shared.at < SHARE_MS) return shared.promise;
  const entry = { token, at: Date.now(), promise: loadOrderTrackings(token) };
  shared = entry;
  // A failed load is never reused.
  entry.promise.catch(() => { if (shared === entry) shared = undefined; });
  return entry.promise;
}

/** Forget the shared result so the next load asks the server again (after the user acted or pressed refresh). */
export function forgetSharedTrackings() { shared = undefined; }

export function useOrderTrackings() {
  const { user } = useAuth();
  const token = user?.access_token || "";
  const [rows, setRows] = useState<Tracking[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [unavailable, setUnavailable] = useState<Order[]>([]);
  const latest = useRef(0);

  const reload = useCallback(async (quiet = false) => {
    if (!token) return;
    const request = ++latest.current;
    if (!quiet) setLoading(true);
    setLoadFailed(false);
    try {
      const split = await fetchOrderTrackings(token);
      if (request !== latest.current) return;
      setRows(split.rows);
      setUnavailable(split.unavailable);
    } catch {
      if (request === latest.current) {
        setLoadFailed(true);
        setRows((current) => current.map((row) => ({ ...row, delivery: row.delivery ? { ...row.delivery, location: undefined } : undefined })));
      }
    } finally {
      if (request === latest.current && !quiet) setLoading(false);
    }
  }, [token]);

  useEffect(() => { void reload(); }, [reload]);
  useEffect(() => {
    if (!rows.some((row) => row.delivery?.runStatus === "in_progress")) return;
    const timer = window.setInterval(() => { void reload(true); }, 15_000);

    return () => window.clearInterval(timer);
  }, [rows, reload]);
  useEffect(() => () => { latest.current += 1; }, []);

  const refresh = useCallback((quiet = false) => { forgetSharedTrackings(); return reload(quiet); }, [reload]);

  // Orders that exist but whose progress could not be loaded: shown by name, never silently dropped.
  return { rows, loading, loadFailed, skipped: unavailable.length, unavailable, reload: refresh };
}
