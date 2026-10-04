import { useCallback, useEffect, useRef, useState } from "react";
import { apiJSON, Order } from "../api/client";
import { ArrivalPrediction, LiveLocation } from "../api/delivery";
import { useAuth } from "../auth/AuthContext";

export type Tracking = {
  stage: string;
  order: Order;
  planning: { state?: string; planRef?: string; planId?: string; tripId?: string; depot?: string; stopSequence?: number; reasonCode?: string; reasonComment?: string; plannedArrivalAt?: string; plannedServiceStartAt?: string };
  delivery?: {
    runStatus: string; outcome?: string; reason?: string; vehicleId?: string; tripId?: string; occurredAt?: string; completedAt?: string;
    depot?: string; location?: LiveLocation;
    arrivalPrediction?: ArrivalPrediction;
    returnedGoods?: { goods: string; units: number; reason: string; resolution: string; occurredAt: string; followupOrderRef?: string; followupDate?: string };
    proofs?: { type: string; mimeType: string; pending: boolean; uploadedAt?: string; receiverName?: string }[];
    loadingShortfallSummary?: { type?: string; affectedUnits?: number; note?: string }[];
    deliveredUnits?: number;
  };
  receiptDue?: { reportBy: string; state: "open" | "due_tomorrow" | "due_today" | "overdue" };
  receipt?: { id?: string; status: string; receivedUnits: number; expectedUnits: number; confirmedAt?: string; confirmedBy?: string };
  receiptIssues?: { id: string; issueType: string; affectedUnits: number; note?: string; createdAt?: string; createdBy?: string }[];
  custody?: { id: string; stage: string; sealId: string; serialNumbers: string[]; condition: string; recordedBy: string; recordedAt: string; receiverName?: string }[];
};

export function useOrderTrackings() {
  const { user } = useAuth();
  const token = user?.access_token || "";
  const [rows, setRows] = useState<Tracking[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [skipped, setSkipped] = useState(0);
  const latest = useRef(0);

  const reload = useCallback(async (quiet = false) => {
    if (!token) return;
    const request = ++latest.current;
    if (!quiet) setLoading(true);
    setLoadFailed(false);
    try {
      const body = await apiJSON<{ items: Order[] }>("/orders", token);
      const settled = await Promise.allSettled(
        (body.items || []).map(async (order) => (await apiJSON<{ tracking: Tracking }>(`/orders/${order.id}/tracking`, token)).tracking),
      );
      if (request !== latest.current) return;
      const ok = settled.flatMap((result) => (result.status === "fulfilled" ? [result.value] : []));
      ok.sort((a, b) => b.order.requestedDeliveryDate.localeCompare(a.order.requestedDeliveryDate) || b.order.orderRef.localeCompare(a.order.orderRef));
      setRows(ok);
      setSkipped(settled.length - ok.length);
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

  return { rows, loading, loadFailed, skipped, reload };
}
