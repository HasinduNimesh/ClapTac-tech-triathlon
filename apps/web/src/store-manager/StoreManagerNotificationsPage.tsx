import { useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import iconChev from "../assets/store-manager/icon-chev.svg";
import iconPlus from "../assets/store-manager/icon-plus.svg";
import { deferralExplanation } from "./deferralMessage.mjs";
import { isDeferred, needsReceipt } from "./orderStage.mjs";
import { StoreManagerHero } from "./StoreManagerHero";
import { useOrderTrackings } from "./useOrderTrackings";

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

export function StoreManagerNotificationsPage() {
  const { profile } = useAuth();
  const { t } = useLocale();
  const userId = profile?.userId || "anonymous";
  const { rows, loading, loadFailed, skipped, reload } = useOrderTrackings();
  const [acknowledged, setAcknowledged] = useState<Set<string>>(() => readAcknowledged(userId));

  const receipts = rows.filter((row) => needsReceipt(row.stage));
  const deferred = rows.filter((row) => isDeferred(row.stage) && !acknowledged.has(row.order.id));
  const noticeCount = receipts.length + deferred.length;

  function acknowledge(orderId: string) {
    const next = new Set(acknowledged).add(orderId);
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
                <p className="sm-notification-desc muted">{t("Driver has recorded delivery. Confirm the count and report shortage or damage.")}</p>
              </div>
              <Link to="/store-manager/receipts" className="sm-notif-action sm-notif-action--solid">{t("Confirm receipt")}</Link>
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
                  <Link to={`/store-manager/orders?order=${encodeURIComponent(row.order.id)}`} className="sm-notif-action sm-notif-action--outline">{t("View order")}</Link>
                  <button type="button" className="sm-notif-action sm-notif-action--outline" onClick={() => acknowledge(row.order.id)}>{t("Acknowledge update")}</button>
                </div>
              </div>
            );
          })}

          {!loading && noticeCount === 0 && !loadFailed && <p className="sm-empty muted">{t("No notices at this time.")}</p>}
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
