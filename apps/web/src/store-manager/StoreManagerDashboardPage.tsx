import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiJSON, Order } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import heroBg from "../assets/store-manager/hero-bg.png";
import iconHistory from "../assets/store-manager/icon-history.svg";
import iconTruck from "../assets/store-manager/icon-truck.svg";
import iconCheck from "../assets/store-manager/icon-check.svg";
import iconChev from "../assets/store-manager/icon-chev.svg";
import iconPlus from "../assets/store-manager/icon-plus.svg";

type Tracking = {
  stage: string;
  order: Order;
  planning: { plannedArrivalAt?: string; reasonCode?: string };
  delivery?: { runStatus: string; outcome?: string };
};

function statusLabel(stage: string, outcome?: string) {
  if (outcome === "DELIVERED") return "Delivered";
  if (stage === "IN_TRANSIT" || stage === "ON_ROUTE") return "On route";
  if (stage === "DEFERRED" || stage === "PLANNING_DEFERRED") return "Deferred";
  if (stage === "PLANNED") return "Planned";
  return stage.replace(/_/g, " ");
}

function statusClass(stage: string, outcome?: string) {
  if (outcome === "DELIVERED") return "sm-badge--delivered";
  if (stage === "IN_TRANSIT" || stage === "ON_ROUTE") return "sm-badge--on-route";
  if (stage === "DEFERRED" || stage === "PLANNING_DEFERRED") return "sm-badge--deferred";
  return "sm-badge--default";
}

export function StoreManagerDashboardPage() {
  const { user } = useAuth();
  const { t } = useLocale();
  const token = user?.access_token || "";

  const [trackings, setTrackings] = useState<Tracking[]>([]);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    if (!token) return;
    setError("");
    try {
      const body = await apiJSON<{ items: Order[] }>("/orders", token);
      const rows = await Promise.all(
        (body.items || []).slice(0, 10).map(async (order) => {
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

  const today = new Date().toLocaleDateString("en-CA", { timeZone: "Asia/Colombo" }); // "YYYY-MM-DD"
  const inTransit = trackings
    .filter(t => {
      if (t.stage !== "IN_TRANSIT" && t.stage !== "ON_ROUTE") return false;
      const arrivalDate = t.planning.plannedArrivalAt?.slice(0, 10);
      return arrivalDate === today || t.order.requestedDeliveryDate === today;
    })
    .sort((a, b) => (a.planning.plannedArrivalAt ?? "").localeCompare(b.planning.plannedArrivalAt ?? ""));
  const deferred = trackings.filter(t => t.stage === "DEFERRED" || t.stage === "PLANNING_DEFERRED");
  const needsReceipt = trackings.filter(t => t.delivery?.outcome === "DELIVERED" && !t.stage.includes("RECEIPT"));
  const attentionCount = deferred.length + needsReceipt.length;

  const nextArrival = inTransit[0]?.planning.plannedArrivalAt
    ? new Date(inTransit[0].planning.plannedArrivalAt).toLocaleTimeString("en-LK", { hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "Asia/Colombo" })
    : null;

  return (
    <>
      <div className="sm-hero" style={{ backgroundImage: `url(${heroBg})` }}>
        <div className="sm-hero-content">
          <div>
            <h1 className="sm-hero-title">{t("Your Deliveries")}</h1>
            <p className="sm-hero-sub">{t("See what is arriving, what needs attention and when to order next.")}</p>
          </div>
        </div>
        <div className="sm-hero-actions">
          <div className="sm-dashboard-picker">
            <div>
              <span className="sm-dashboard-picker-label">{t("DASHBOARD")}</span>
              <span className="sm-dashboard-picker-value">{t("Store overview")}</span>
            </div>
            <img src={iconChev} alt="" aria-hidden="true" width={14} height={14} className="sm-chev-down" />
          </div>
          <Link to="/store-manager/orders/new" className="sm-btn-place-order">
            <img src={iconPlus} alt="" aria-hidden="true" width={18} height={18} />
            {t("Place Order")}
          </Link>
        </div>
      </div>

      <div className="sm-stat-row">
        <div className="sm-stat-card">
          <div className="sm-stat-icon sm-stat-icon--blue">
            <img src={iconHistory} alt="" aria-hidden="true" width={24} height={24} />
          </div>
          <div className="sm-stat-body">
            <p className="sm-stat-label">{t("Next expected arrival")}</p>
            <p className="sm-stat-value">{nextArrival ?? "—"}</p>
            <p className="sm-stat-sub sm-stat-sub--orange">
              {inTransit[0] ? `${t("Today")} · ${inTransit[0].order.temperatureRequirement}` : t("No arrivals today")}
            </p>
          </div>
        </div>
        <div className="sm-stat-card">
          <div className="sm-stat-icon sm-stat-icon--teal">
            <img src={iconTruck} alt="" aria-hidden="true" width={24} height={24} />
          </div>
          <div className="sm-stat-body">
            <p className="sm-stat-label">{t("On today's route")}</p>
            <p className="sm-stat-value">{inTransit.length} {t("orders")}</p>
            <p className="sm-stat-sub muted">{t("Ambient and chilled tracked separately")}</p>
          </div>
        </div>
        <div className="sm-stat-card">
          <div className="sm-stat-icon sm-stat-icon--green">
            <img src={iconCheck} alt="" aria-hidden="true" width={24} height={24} />
          </div>
          <div className="sm-stat-body">
            <p className="sm-stat-label">{t("Needs attention")}</p>
            <p className="sm-stat-value">{attentionCount} {attentionCount === 1 ? t("deferral") : t("items")}</p>
            <p className="sm-stat-sub sm-stat-sub--green">
              {deferred.length > 0 ? t("Next run awaiting confirmation") : t("All clear")}
            </p>
          </div>
        </div>
      </div>

      {error && <p className="status-bad" role="alert">{error}</p>}

      <div className="sm-dashboard-grid">
        <section className="sm-panel" aria-labelledby="arriving-today-heading">
          <div className="sm-panel-header">
            <h2 id="arriving-today-heading" className="sm-panel-title">{t("Arriving Today")}</h2>
            <Link to="/store-manager/orders" className="sm-panel-link">
              {t("View all orders")}
              <img src={iconChev} alt="" aria-hidden="true" width={14} height={14} className="sm-chev-right" />
            </Link>
          </div>
          {inTransit.length === 0 && !error && (
            <p className="sm-empty muted">{t("No orders currently in transit.")}</p>
          )}
          {inTransit.map((row) => (
            <div key={row.order.id} className="sm-order-card">
              <div className="sm-order-card-top">
                <div className="sm-order-icon">
                  <img src={iconHistory} alt="" aria-hidden="true" width={20} height={20} />
                </div>
                <div className="sm-order-info">
                  <p className="sm-order-name">
                    {row.order.temperatureRequirement === "chilled" ? t("Chilled dairy") : t("Ambient groceries")} · {row.order.orderRef}
                  </p>
                  <p className="sm-order-desc muted">{row.order.orderUnits} {t("units")} · {row.order.temperatureRequirement}</p>
                </div>
                <span className={`sm-badge sm-badge--on-route`}>{t("On Route")}</span>
              </div>
              <hr className="sm-order-divider" />
              <div className="sm-order-card-bottom">
                <p className="sm-order-eta">
                  <span className="muted">{t("Expected arrival:")}{" "}</span>
                  <strong>
                    {row.planning.plannedArrivalAt
                      ? new Date(row.planning.plannedArrivalAt).toLocaleTimeString("en-LK", { hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "Asia/Colombo" })
                      : "—"}
                  </strong>
                </p>
                <Link to={`/store-manager/tracking`} className="sm-track-link">
                  {t("Track order")}
                  <img src={iconChev} alt="" aria-hidden="true" width={14} height={14} className="sm-chev-right" />
                </Link>
              </div>
            </div>
          ))}
        </section>

        <section className="sm-panel sm-attention-panel" aria-labelledby="attention-heading">
          <div className="sm-panel-header">
            <h2 id="attention-heading" className="sm-panel-title">{t("Attention needed")}</h2>
            {attentionCount > 0 && (
              <span className="sm-notice-badge">{attentionCount} {t("Notice")}</span>
            )}
          </div>
          {deferred.map((row) => (
            <div key={row.order.id} className="sm-alert-card">
              <h3 className="sm-alert-title">{t("Order deferred to a later run")}</h3>
              <p className="sm-alert-body">
                {row.order.orderRef} {t("could not be allocated to a vehicle for today.")}
                {row.planning.reasonCode && ` ${t("Reason")}: ${row.planning.reasonCode}.`}
              </p>
              <Link to="/store-manager/notifications" className="sm-alert-action">
                {t("See Reason and Updates")}
                <img src={iconChev} alt="" aria-hidden="true" width={14} height={14} className="sm-chev-right" />
              </Link>
            </div>
          ))}
          <div className="sm-ordering-window">
            <h3 className="sm-ordering-title">{t("Ordering window")}</h3>
            <p className="sm-ordering-body muted">
              {t("Place your next order by 4:00 PM today. Orders after the cutoff move to the next eligible run.")}
            </p>
            <Link to="/store-manager/orders/new" className="sm-create-order-link">
              {t("Create Order")}
              <img src={iconChev} alt="" aria-hidden="true" width={14} height={14} className="sm-chev-right" />
            </Link>
          </div>
        </section>
      </div>

      <section className="sm-panel sm-recent-panel" aria-labelledby="recent-orders-heading">
        <div className="sm-panel-header">
          <h2 id="recent-orders-heading" className="sm-panel-title">{t("Recent orders")}</h2>
          <Link to="/store-manager/orders" className="sm-panel-link">
            {t("Order History")}
            <img src={iconChev} alt="" aria-hidden="true" width={14} height={14} className="sm-chev-right" />
          </Link>
        </div>
        <div className="sm-table-wrap">
          <table className="sm-table">
            <thead>
              <tr>
                <th>{t("ORDER")}</th>
                <th>{t("GOODS")}</th>
                <th>{t("NEEDED")}</th>
                <th>{t("STATUS")}</th>
                <th>{t("NEXT STEP")}</th>
              </tr>
            </thead>
            <tbody>
              {trackings.map((row) => (
                <tr key={row.order.id}>
                  <td>{row.order.orderRef}</td>
                  <td>{row.order.temperatureRequirement === "chilled" ? t("Chilled") : t("Ambient")} · {row.order.orderUnits} {t("items")}</td>
                  <td>{row.order.requestedDeliveryDate}</td>
                  <td>
                    <span className={`sm-badge ${statusClass(row.stage, row.delivery?.outcome)}`}>
                      {t(statusLabel(row.stage, row.delivery?.outcome))}
                    </span>
                  </td>
                  <td>
                    {row.planning.plannedArrivalAt
                      ? `ETA ${new Date(row.planning.plannedArrivalAt).toLocaleTimeString("en-LK", { hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "Asia/Colombo" })}`
                      : "—"}
                  </td>
                </tr>
              ))}
              {trackings.length === 0 && !error && (
                <tr><td colSpan={5} className="sm-table-empty muted">{t("No recent orders.")}</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </section>
    </>
  );
}
