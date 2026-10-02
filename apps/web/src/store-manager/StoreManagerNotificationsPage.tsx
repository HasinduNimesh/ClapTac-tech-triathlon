import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiJSON, Order } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import heroBg from "../assets/store-manager/hero-bg.png";
import iconPlus from "../assets/store-manager/icon-plus.svg";
import iconChev from "../assets/store-manager/icon-chev.svg";

type Tracking = {
  stage: string;
  order: Order;
  planning: { plannedArrivalAt?: string; reasonCode?: string; reasonComment?: string };
  delivery?: { outcome?: string };
  receipt?: { status: string };
};

export function StoreManagerNotificationsPage() {
  const { user } = useAuth();
  const { t } = useLocale();
  const token = user?.access_token || "";
  const [trackings, setTrackings] = useState<Tracking[]>([]);
  const [error, setError] = useState("");

  const [dismissed, setDismissed] = useState<Set<string>>(new Set());

  const load = useCallback(async () => {
    if (!token) return;
    setError("");
    try {
      const body = await apiJSON<{ items: Order[] }>("/orders", token);
      const rows = await Promise.all(
        (body.items || []).map(async (order) => {
          const res = await apiJSON<{ tracking: Tracking }>(`/orders/${order.id}/tracking`, token);
          return res.tracking;
        })
      );
      setTrackings(rows);
    } catch (e) {
      setError(String(e));
    }
  }, [token]);

  useEffect(() => { void load(); }, [load]);

  const needsReceipt = trackings.filter(t => t.delivery?.outcome === "DELIVERED" && t.receipt?.status !== "CONFIRMED");
  const deferred = trackings.filter(t =>
    (t.stage === "DEFERRED" || t.stage === "PLANNING_DEFERRED") && !dismissed.has(t.order.id)
  );

  const noticeCount = needsReceipt.length + deferred.length;

  return (
    <>
      <div className="sm-hero" style={{ backgroundImage: `url(${heroBg})` }}>
        <div className="sm-hero-content">
          <div>
            <h1 className="sm-hero-title">{t("Notifications")}</h1>
            <p className="sm-hero-sub">{t("Notices grouped for meaningful arrival changes, deferrals and receipts that need review.")}</p>
          </div>
        </div>
        <div className="sm-hero-actions">
          <Link to="/store-manager/orders/new" className="sm-btn-place-order">
            <img src={iconPlus} alt="" aria-hidden="true" width={18} height={18} />
            {t("Place Order")}
          </Link>
        </div>
      </div>

      <div className="sm-notifications-content">
        {error && <p className="status-bad" role="alert">{error}</p>}

        <section className="sm-panel" aria-labelledby="needs-attention-heading">
          <div className="sm-panel-header">
            <h2 id="needs-attention-heading" className="sm-panel-title">{t("Needs attention")}</h2>
            {noticeCount > 0 && (
              <span className="sm-notice-badge">{noticeCount} {t("updates")}</span>
            )}
          </div>

          {needsReceipt.map((row) => (
            <div key={row.order.id} className="sm-notification-item">
              <div className="sm-notification-meta">
                <span className="sm-notif-badge sm-notif-badge--receipt">{t("RECEIPT")}</span>
              </div>
              <div className="sm-notification-body">
                <h3 className="sm-notification-title">{row.order.orderRef} · {t("delivery needs receipt confirmation")}</h3>
                <p className="sm-notification-desc muted">
                  {t("Driver has recorded delivery. Confirm the count and report shortage or damage.")}
                </p>
              </div>
              <Link to="/store-manager/receipts" className="tap primary sm-notif-action">
                {t("Confirm receipt")}
              </Link>
            </div>
          ))}

          {deferred.map((row) => (
            <div key={row.order.id} className="sm-notification-item">
              <div className="sm-notification-meta">
                <span className="sm-notif-badge sm-notif-badge--deferred">{t("DEFERRED")}</span>
              </div>
              <div className="sm-notification-body">
                <h3 className="sm-notification-title">{row.order.orderRef} · {t("moved to a later run")}</h3>
                <p className="sm-notification-desc muted">
                  {row.planning.reasonComment || t("Capacity was fully allocated. Proposed run pending confirmation.")}
                </p>
              </div>
              <button
                type="button"
                className="tap sm-notif-action sm-notif-action--outline"
                onClick={() => setDismissed(prev => new Set([...prev, row.order.id]))}
              >
                {t("Acknowledge update")}
              </button>
            </div>
          ))}

          {noticeCount === 0 && !error && (
            <p className="sm-empty muted">{t("No notices at this time.")}</p>
          )}
        </section>

        <div className="sm-info-banner">
          <p className="sm-info-banner-title">
            {t("Notices are grouped for meaningful arrival changes, deferrals and receipts that need review.")}
          </p>
          <p className="muted">{t("Small ETA movements won't create repeated alerts.")}</p>
        </div>

        <div className="sm-notifications-links">
          <Link to="/store-manager" className="sm-panel-link">
            ← {t("Back to dashboard")}
            <img src={iconChev} alt="" aria-hidden="true" width={14} height={14} className="sm-chev-right" />
          </Link>
        </div>
      </div>
    </>
  );
}
