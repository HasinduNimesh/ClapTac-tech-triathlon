import { useCallback, useEffect, useMemo, useState } from "react";
import { apiJSON } from "../api/client";
import { todayInSriLanka, todayLocal } from "../api/date.mjs";
import type { DockAlert, LoadingTripDetail, LoadingTripSummary } from "../api/loading";
import type { DeliveryTripDetail, DeliveryTripSummary } from "../api/delivery";
import type { PlanDetail } from "../api/planning";
import { useAuth } from "../auth/AuthContext";
import { dateTime } from "../dispatcher/useApi";
import type { AuditEvent, Incident, ReceiptIssue } from "../dispatcher/types";
import { useLocale } from "../i18n";
import { fetchOrderTrackings } from "../store-manager/useOrderTrackings";
import { unreadItems, type BellItem } from "./bellModel.mjs";
import {
  deriveDispatcherAlerts, deriveDriverItems, deriveLoaderItems, deriveStoreMessages, deriveStoreNotices, driverTripsNeedingDetail, loaderTripsNeedingDetail,
  nextSeenEta, prunedAcknowledged, type DispatcherAlert, type DispatcherSource,
} from "./notificationItems.mjs";
import { addIds, browserStorage, deferralStateKey, mergeSeenEta, NOTIFICATION_STATE_EVENT, pruneIds, readIds, readSeenEta, readStateKey, replaceIds } from "./readState.mjs";
import { usePolledResource } from "./usePolledResource";

/** What every role's bell is fed with. */
export type NotificationFeed<T extends BellItem = BellItem> = {
  items: T[];
  /** Items still needing the person, newest derivation order. */
  unread: T[];
  unreadCount: number;
  loading: boolean;
  error: string;
  /** Dismiss items locally (only items flagged `readable`; the others clear when their server state changes). */
  markRead: (keys: string[]) => void;
  markAllRead: () => void;
};

/** Re-render when the stored notification state changes here or in another tab. */
function useStorageRevision(): number {
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const bump = () => setRevision((n) => n + 1);
    window.addEventListener(NOTIFICATION_STATE_EVENT, bump);
    window.addEventListener("storage", bump);
    return () => { window.removeEventListener(NOTIFICATION_STATE_EVENT, bump); window.removeEventListener("storage", bump); };
  }, []);
  return revision;
}

/** Read marks for one role and person, kept in localStorage; items that are gone are forgotten. */
function useLocalReadState(role: string, userId: string, items: BellItem[], settled: boolean) {
  const key = readStateKey(role, userId);
  const revision = useStorageRevision();
  const read = useMemo(() => readIds(browserStorage(), key), [key, revision]);
  // Forget marks for items that no longer exist, but only once a complete load has finished.
  useEffect(() => {
    if (settled) pruneIds(browserStorage(), key, items.map((item) => item.key));
  }, [settled, items, key]);
  const markRead = useCallback((keys: string[]) => { addIds(browserStorage(), key, keys); }, [key]);
  const markAllRead = useCallback(() => {
    addIds(browserStorage(), key, items.filter((item) => item.readable).map((item) => item.key));
  }, [items, key]);
  return { read, markRead, markAllRead };
}

function useSession() {
  const { user, profile } = useAuth();
  const token = user?.access_token || "";
  return { token, userId: profile?.userId || "", enabled: Boolean(token) };
}

// ------------------------------------------------------------------ dispatcher

async function fetchDispatcherSource(token: string): Promise<{ source: DispatcherSource; failed: number }> {
  const date = todayInSriLanka();
  const get = <T,>(path: string) => apiJSON<T>(path, token);
  const results = await Promise.allSettled([
    get<PlanDetail>(`/planning/plans?date=${date}`),
    get<{ items: Incident[] }>("/fleet/incidents?openOnly=true"),
    get<{ items: LoadingTripSummary[] }>(`/loading/trips?date=${date}`),
    get<{ items: ReceiptIssue[] }>("/orders/receipt-issues"),
    get<{ items: { id: string; settledAt?: string }[] }>(`/delivery/sync-conflicts?date=${date}`),
    get<{ items: AuditEvent[] }>("/shared/audit/events?action=DELIVERY_TEMPERATURE_EXCEPTION&limit=20"),
    get<{ items: DockAlert[] }>(`/loading/alerts?date=${date}`),
  ]);
  const failed = results.filter((r) => r.status === "rejected").length;
  if (failed === results.length) throw new Error("Notifications could not be loaded");
  const value = <T,>(i: number): T | undefined => { const r = results[i]; return r.status === "fulfilled" ? (r.value as T) : undefined; };
  return {
    failed,
    source: {
      plan: value<PlanDetail>(0) ?? null,
      incidents: value<{ items: Incident[] }>(1)?.items,
      trips: value<{ items: LoadingTripSummary[] }>(2)?.items,
      receipts: value<{ items: ReceiptIssue[] }>(3)?.items,
      // Only conflicts nobody has settled yet still need the dispatcher.
      conflicts: value<{ items: { id: string; settledAt?: string }[] }>(4)?.items?.filter((c) => !c.settledAt),
      temperature: value<{ items: AuditEvent[] }>(5)?.items,
      dockAlerts: value<{ items: DockAlert[] }>(6)?.items,
    },
  };
}

/** The dispatcher's open alerts: the same derivation as the notifications page, so the counts always agree. */
export function useDispatcherNotifications(): NotificationFeed<DispatcherAlert> {
  const { t } = useLocale();
  const { token, userId, enabled } = useSession();
  const polled = usePolledResource(() => fetchDispatcherSource(token), enabled, `dispatcher:${userId}:${token}`);
  const items = useMemo(() => deriveDispatcherAlerts(polled.data?.source, { t, dateTime }), [polled.data, t]);
  // Dispatcher alerts have no read flag: each one stays until the underlying problem is resolved.
  return { items, unread: items, unreadCount: items.length, loading: polled.loading, error: polled.error || (polled.data?.failed ? "Some notification sources could not be loaded" : ""), markRead: noop, markAllRead: noop };
}

// --------------------------------------------------------------- store manager

/** Acknowledged deferrals and seen arrival times, shared with the store manager's notifications page. */
export function useStoreNoticeState(userId: string) {
  const revision = useStorageRevision();
  const acknowledged = useMemo(() => readIds(browserStorage(), deferralStateKey(userId)), [userId, revision]);
  const seenEta = useMemo(() => readSeenEta(browserStorage(), userId), [userId, revision]);
  const acknowledge = useCallback((deferralKey: string) => { addIds(browserStorage(), deferralStateKey(userId), [deferralKey]); }, [userId]);
  const acknowledgeEta = useCallback((orderId: string, arrival: string) => { mergeSeenEta(browserStorage(), userId, { [orderId]: arrival }); }, [userId]);
  const recordSeenEta = useCallback((next: Record<string, string>) => { mergeSeenEta(browserStorage(), userId, next); }, [userId]);
  const dropAcknowledged = useCallback((kept: Set<string>) => { replaceIds(browserStorage(), deferralStateKey(userId), kept); }, [userId]);
  return { acknowledged, seenEta, acknowledge, acknowledgeEta, recordSeenEta, dropAcknowledged };
}

/** The store manager's open notices: the same derivation as the notifications page. */
export function useStoreManagerNotifications(): NotificationFeed {
  const { t } = useLocale();
  const { token, enabled } = useSession();
  const { profile } = useAuth();
  const userId = profile?.userId || "anonymous";
  const polled = usePolledResource(() => fetchOrderTrackings(token), enabled, `store:${userId}:${token}`);
  const { acknowledged, seenEta, recordSeenEta, dropAcknowledged } = useStoreNoticeState(userId);
  const rows = polled.data?.rows;
  const skipped = polled.data?.unavailable.length ?? 0;

  // Same housekeeping as the page, so arrival changes are noticed even if the page was never opened.
  useEffect(() => {
    if (!rows) return;
    const baselines = nextSeenEta(rows, seenEta);
    if (baselines) recordSeenEta(baselines);
  }, [rows]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!rows || skipped > 0) return;
    const kept = prunedAcknowledged(rows, acknowledged);
    if (kept) dropAcknowledged(kept);
  }, [rows, skipped, acknowledged, dropAcknowledged]);

  // Messages for the outlet are loaded on their own, so a failure there never hides the order notices.
  const outletId = profile?.outletIds?.[0] || "";
  const messages = usePolledResource(
    () => apiJSON<{ items?: { id: number; eventType: string; body: string; status: string; createdAt: string }[] }>(`/shared/outlets/${encodeURIComponent(outletId)}/notifications`, token),
    enabled && Boolean(outletId), `store-messages:${userId}:${outletId}:${token}`,
  );
  const items = useMemo(() => [
    ...deriveStoreNotices(rows || [], { acknowledged, seenEta }, { t }).items,
    ...deriveStoreMessages(messages.data?.items || [], Date.now(), { t }),
  ], [rows, acknowledged, seenEta, messages.data, t]);
  const { read, markRead, markAllRead } = useLocalReadState("store", userId, items, Boolean(polled.data && messages.data) && !polled.error && !messages.error);
  const unread = unreadItems(items, read);
  return { items, unread, unreadCount: unread.length, loading: polled.loading, error: polled.error, markRead, markAllRead };
}

// ------------------------------------------------------------------- loader

async function fetchLoaderSource(token: string) {
  const date = todayInSriLanka();
  const body = await apiJSON<{ items: LoadingTripSummary[] }>(`/loading/trips?date=${date}`, token);
  const trips = body.items || [];
  const need = loaderTripsNeedingDetail(trips);
  const settled = await Promise.allSettled(need.map((trip) => apiJSON<LoadingTripDetail>(`/loading/trips/${encodeURIComponent(trip.tripId)}`, token)));
  const details: Record<string, LoadingTripDetail> = {};
  settled.forEach((r, i) => { if (r.status === "fulfilled") details[need[i].tripId] = r.value; });
  return { trips, details };
}

/** The loader's open items: plan versions to acknowledge and shortfalls awaiting or carrying a dispatcher decision. */
export function useLoaderNotifications(): NotificationFeed {
  const { t } = useLocale();
  const { token, userId, enabled } = useSession();
  const polled = usePolledResource(() => fetchLoaderSource(token), enabled, `loader:${userId}:${token}`);
  const items = useMemo(() => deriveLoaderItems(polled.data || undefined, { t, dateTime }), [polled.data, t]);
  const { read, markRead, markAllRead } = useLocalReadState("loader", userId, items, Boolean(polled.data) && !polled.error);
  const unread = unreadItems(items, read);
  return { items, unread, unreadCount: unread.length, loading: polled.loading, error: polled.error, markRead, markAllRead };
}

// ------------------------------------------------------------------- driver

type DriverMessage = { id: string; body: string; createdAt: string; acknowledgedAt?: string };

async function fetchDriverSource(token: string) {
  const date = todayLocal();
  const body = await apiJSON<{ items: DeliveryTripSummary[] }>(`/delivery/trips?date=${date}`, token);
  const trips = body.items || [];
  const need = driverTripsNeedingDetail(trips);
  const detailResults = await Promise.allSettled(need.map((trip) => apiJSON<DeliveryTripDetail>(`/delivery/trips/${trip.tripId}`, token)));
  const messageResults = await Promise.allSettled(need.map((trip) => apiJSON<{ items: DriverMessage[] }>(`/delivery/trips/${trip.tripId}/messages`, token)));
  const details: Record<string, DeliveryTripDetail> = {};
  const messages: Record<string, DriverMessage[]> = {};
  detailResults.forEach((r, i) => { if (r.status === "fulfilled") details[need[i].tripId] = r.value; });
  messageResults.forEach((r, i) => { if (r.status === "fulfilled") messages[need[i].tripId] = r.value.items || []; });
  return { trips, details, messages };
}

/** The driver's open items: a plan to acknowledge, a stale prepared route and dispatcher messages not yet acknowledged. */
export function useDriverNotifications(): NotificationFeed {
  const { t } = useLocale();
  const { token, userId, enabled } = useSession();
  const polled = usePolledResource(() => fetchDriverSource(token), enabled, `driver:${userId}:${token}`);
  const items = useMemo(() => deriveDriverItems({ ...(polled.data || {}), userId }, { t, dateTime }), [polled.data, userId, t]);
  const { read, markRead, markAllRead } = useLocalReadState("driver", userId, items, Boolean(polled.data) && !polled.error);
  const unread = unreadItems(items, read);
  return { items, unread, unreadCount: unread.length, loading: polled.loading, error: polled.error, markRead, markAllRead };
}

function noop() { /* nothing to dismiss: these items clear when the underlying state changes */ }
