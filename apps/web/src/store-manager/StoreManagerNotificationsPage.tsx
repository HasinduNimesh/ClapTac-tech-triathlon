import { useCallback, useEffect, useRef, useState } from "react";
import { AutomationInbox } from "../automations/AutomationInbox";

import { Link } from "react-router-dom";
import { apiJSON } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import iconChev from "../assets/store-manager/icon-chev.svg";
import iconPlus from "../assets/store-manager/icon-plus.svg";
import { deferralExplanation } from "./deferralMessage.mjs";
import { colomboTime, isDeferred, needsReceipt } from "./orderStage.mjs";
import { messageStatusLabel, messageTime, messageTypeLabel } from "./outletMessages.mjs";
import { StoreManagerHero } from "./StoreManagerHero";
import { Tracking, useOrderTrackings } from "./useOrderTrackings";

const deferralKey = (row: Tracking) => `${row.order.id}|${row.planning.planRef ?? ""}|${row.planning.reasonCode ?? ""}`;

const storageKey = (userId: string) => `sm-acknowledged-deferrals:${userId}`;

function readAcknowledged(userId: string): Set<string> {
  try {
    const raw = window.localStorage.getItem(storageKey(userId));
    return new Set(raw ? (JSON.parse(raw) as string[]) : []);
  } catch {
    return new Set();
  }
}

function saveAcknowledged(userId: string, ids: Set<string>) {
  try {
    window.localStorage.setItem(storageKey(userId), JSON.stringify([...ids]));
  } catch {
    // Acknowledgement still applies for this visit if storage is unavailable.
  }
}

const etaKey = (userId: string) => `sm-seen-eta:${userId}`;
type EtaChange = { row: Tracking; from: string; to: string; minutes: number };
const ETA_THRESHOLD_MINUTES = 30;

function readSeenEta(userId: string): Record<string, string> {
  try { return JSON.parse(window.localStorage.getItem(etaKey(userId)) || "{}") as Record<string, string>; } catch { return {}; }
}

type OutletMessage = { id: number; eventType: string; body: string; status: string; createdAt: string };

// The messages the system queued for this outlet. Loaded on its own, so a failure here never hides
// the order-based notices on the page.
function useOutletMessages(outletId: string, token: string) {
  const [items, setItems] = useState<OutletMessage[]>([]);
  const [loading, setLoading] = useState(Boolean(outletId && token));
  const [failed, setFailed] = useState(false);
  const latest = useRef(0);
  const reload = useCallback(async () => {
    if (!outletId || !token) { setLoading(false); return; }
    const request = ++latest.current;
    setLoading(true);
    setFailed(false);
    try {
      const body = await apiJSON<{ items?: OutletMessage[] }>(`/shared/outlets/${encodeURIComponent(outletId)}/notifications`, token);
      if (request === latest.current) setItems(body.items || []);
    } catch {
      if (request === latest.current) setFailed(true);
    } finally {
      if (request === latest.current) setLoading(false);
    }
  }, [outletId, token]);
  useEffect(() => { void reload(); }, [reload]);
  useEffect(() => () => { latest.current += 1; }, []);
  return { items, loading, failed, reload };
}

export function StoreManagerNotificationsPage() {
  const { profile, user } = useAuth();
  const { t } = useLocale();
  const userId = profile?.userId || "anonymous";
  const messages = useOutletMessages(profile?.outletIds?.[0] || "", user?.access_token || "");
  const { rows, loading, loadFailed, skipped, reload } = useOrderTrackings();
  const [acknowledged, setAcknowledged] = useState<Set<string>>(() => readAcknowledged(userId));

  const receipts = rows.filter((row) => needsReceipt(row.stage));
  const deferred = rows.filter((row) => isDeferred(row.stage) && !acknowledged.has(deferralKey(row)));
  const [seenEta, setSeenEta] = useState<Record<string, string>>(() => readSeenEta(userId));
  const etaChanges: EtaChange[] = rows.flatMap((row) => {
    const now = row.planning.plannedArrivalAt;
    const before = seenEta[row.order.id];
    if (!now || !before || before === now || row.delivery?.outcome) return [];
    const minutes = Math.round((new Date(now).getTime() - new Date(before).getTime()) / 60000);
    return Math.abs(minutes) >= ETA_THRESHOLD_MINUTES ? [{ row, from: before, to: now, minutes }] : [];
  });
  const noticeCount = receipts.length + deferred.length + etaChanges.length;

  useEffect(() => {
    if (loading || loadFailed) return;
    // First sighting of a planned arrival becomes the baseline; later moves under the threshold update it quietly.
    const next = { ...seenEta };
    let changed = false;
    for (const row of rows) {
      const now = row.planning.plannedArrivalAt;
      if (!now) continue;
      const before = next[row.order.id];
      if (!before || Math.abs(new Date(now).getTime() - new Date(before).getTime()) < ETA_THRESHOLD_MINUTES * 60000) {
        if (before !== now) { next[row.order.id] = now; changed = true; }
      }
    }
    if (changed) { setSeenEta(next); try { window.localStorage.setItem(etaKey(userId), JSON.stringify(next)); } catch { /* optional */ } }
  }, [rows, loading, loadFailed]);

  function acknowledgeEta(change: EtaChange) {
    const next = { ...seenEta, [change.row.order.id]: change.to };
    setSeenEta(next);
    try { window.localStorage.setItem(etaKey(userId), JSON.stringify(next)); } catch { /* optional */ }
  }

  useEffect(() => {
    if (loading || loadFailed || skipped > 0) return;
    const current = new Set(rows.filter((row) => isDeferred(row.stage)).map(deferralKey));
    const kept = [...acknowledged].filter((key) => current.has(key));
    if (kept.length === acknowledged.size) return;
    const next = new Set(kept);
    setAcknowledged(next);
    saveAcknowledged(userId, next);
  }, [rows, loading, loadFailed, skipped, acknowledged, userId]);

  function acknowledge(row: Tracking) {
    const next = new Set(acknowledged).add(deferralKey(row));
    setAcknowledged(next);
    saveAcknowledged(userId, next);
  }

  return (
    <>
      <StoreManagerHero
        compact
        crumbs={[{ label: t("Store"), to: "/store-manager" }, { label: t("Notifications") }]}
        title={t("Notifications")}
        subtitle={t("Deferrals and receipts that need your review are grouped here.")}
      >
        <Link to="/store-manager/orders/new" className="sm-btn-place-order">
          <img src={iconPlus} alt="" aria-hidden="true" width={24} height={24} />
          {t("Place Order")}
        </Link>
      </StoreManagerHero>

      <div className="sm-page-body">
        <AutomationInbox />
        {loadFailed && (
          <div className="sm-load-error" role="alert">
            <span>{t("Orders could not be loaded. Check your connection and try again.")}</span>
            <button type="button" className="tap" onClick={() => void reload()}>{t("Retry")}</button>
          </div>
        )}
        {skipped > 0 && !loadFailed && <p className="sm-load-note muted" role="status">{t("Some orders could not be loaded and are not shown.")}</p>}

        <section className="sm-panel" aria-labelledby="needs-attention-heading">
          <div className="sm-panel-header">
            <h2 id="needs-attention-heading" className="sm-panel-title">{t("Needs attention")}</h2>
            {noticeCount > 0 && <span className="sm-notice-badge">{noticeCount} {noticeCount === 1 ? t("update") : t("updates")}</span>}
          </div>

          {loading && <p className="sm-empty muted" role="status">{t("Loading your orders…")}</p>}

          {receipts.map((row) => (
            <div key={row.order.id} className="sm-notification-item">
              <div className="sm-notification-meta"><span className="sm-notif-badge sm-notif-badge--receipt">{t("RECEIPT")}</span></div>
              <div className="sm-notification-body">
                <h3 className="sm-notification-title">{row.order.orderRef} · {t("delivery needs receipt confirmation")}</h3>
                <p className="sm-notification-desc muted">{t("Driver has recorded delivery. Confirm the count and report shortage or damage.")}{row.receiptDue && row.receiptDue.state !== "open" && <> <strong>{row.receiptDue.state === "overdue" ? t("Report-by deadline passed.") : row.receiptDue.state === "due_today" ? t("Report-by deadline is today.") : t("Report-by deadline is tomorrow.")}</strong></>}</p>
              </div>
              <Link to={`/store-manager/receipts?order=${encodeURIComponent(row.order.id)}`} className="sm-notif-action sm-notif-action--solid">{t("Confirm receipt")}</Link>
            </div>
          ))}

          {etaChanges.map((change) => (
            <div key={change.row.order.id} className="sm-notification-item">
              <div className="sm-notification-meta"><span className="sm-notif-badge sm-notif-badge--receipt" style={{ background: "#fff4de", color: "#a45c00" }}>{t("ETA CHANGE")}</span></div>
              <div className="sm-notification-body">
                <h3 className="sm-notification-title">{change.row.order.orderRef} · {t("arrival moved by")} {Math.abs(change.minutes)} {t("minutes")}</h3>
                <p className="sm-notification-desc muted">{`${t("New expected arrival")} ${colomboTime(change.to)} · ${t("was")} ${colomboTime(change.from)}`}</p>
              </div>
              <div className="sm-notif-actions">
                <Link to={`/store-manager/orders/${encodeURIComponent(change.row.order.id)}/track`} className="sm-notif-action sm-notif-action--outline">{t("View order")}</Link>
                <button type="button" className="sm-notif-action sm-notif-action--outline" onClick={() => acknowledgeEta(change)}>{t("Acknowledge update")}</button>
              </div>
            </div>
          ))}

          {deferred.map((row) => {
            const why = deferralExplanation(row.planning.reasonCode);
            return (
              <div key={row.order.id} className="sm-notification-item">
                <div className="sm-notification-meta"><span className="sm-notif-badge sm-notif-badge--deferred">{t("DEFERRED")}</span></div>
                <div className="sm-notification-body">
                  <h3 className="sm-notification-title">{row.order.orderRef} · {t("moved to a later run")}</h3>
                  <p className="sm-notification-desc muted">{t(why.message)}{row.planning.reasonComment ? ` · ${row.planning.reasonComment}` : ""}</p>
                </div>
                <div className="sm-notif-actions">
                  <Link to={`/store-manager/orders/${encodeURIComponent(row.order.id)}/timeline`} className="sm-notif-action sm-notif-action--outline">{t("View order")}</Link>
                  <button type="button" className="sm-notif-action sm-notif-action--outline" onClick={() => acknowledge(row)}>{t("Acknowledge update")}</button>
                </div>
              </div>
            );
          })}

          {!loading && noticeCount === 0 && !loadFailed && <p className="sm-empty muted">{t("No notices at this time.")}</p>}
        </section>

        <section className="sm-panel" aria-labelledby="store-messages-heading">
          <div className="sm-panel-header">
            <h2 id="store-messages-heading" className="sm-panel-title">{t("Messages sent to your store")}</h2>
          </div>
          <p className="sm-messages-note muted">{t("Notices the system recorded for your store appear here, newest first. Some are also sent as text messages.")}</p>

          {messages.loading && <p className="sm-empty muted" role="status">{t("Loading messages…")}</p>}
          {messages.failed && !messages.loading && (
            <div className="sm-load-error" role="alert">
              <span>{t("Messages could not be loaded. The rest of this page is not affected.")}</span>
              <button type="button" className="tap" onClick={() => void messages.reload()}>{t("Retry")}</button>
            </div>
          )}
          {!messages.loading && !messages.failed && messages.items.length === 0 && <p className="sm-empty muted">{t("No notices have been recorded for your store yet.")}</p>}

          {messages.items.map((message) => (
            <div key={message.id} className="sm-notification-item">
              <div className="sm-notification-meta"><span className="sm-notif-badge sm-notif-badge--receipt">{t(messageTypeLabel(message.eventType))}</span></div>
              <div className="sm-notification-body">
                <p className="sm-notification-title sm-message-text">{message.body}</p>
                <p className="sm-notification-desc muted">{messageTime(message.createdAt)} · {t(messageStatusLabel(message.status))}</p>
              </div>
            </div>
          ))}
        </section>

        <div className="sm-info-banner">
          <p className="sm-info-banner-title">{t("Notices are grouped for meaningful arrival changes, deferrals and receipts that need review.")}</p>
          <p className="muted">{t("Small ETA movements won't create repeated alerts.")}</p>
        </div>

        <div className="sm-notifications-links">
          <Link to="/store-manager" className="sm-panel-link">
            <img src={iconChev} alt="" aria-hidden="true" width={16} height={16} className="sm-chev-left" />
            {t("Back to dashboard")}
          </Link>
        </div>
      </div>
    </>
  );
}
