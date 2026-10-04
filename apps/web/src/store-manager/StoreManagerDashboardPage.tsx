import { Link, useSearchParams } from "react-router-dom";
import { DashboardGrid } from "./DashboardCards";
import { DashboardMenu } from "./DashboardMenu";
import { useDashboards } from "./dashboards";
import { useLocale } from "../i18n";
import { ESTIMATES_UNAVAILABLE_MESSAGE, validArrivalAt } from "../api/estimateAvailability.mjs";
import { formatCutoff, withTime } from "./cutoff.mjs";
import { useOrderCutoff } from "./useOrderCutoff";
import iconHistory from "../assets/store-manager/icon-history.svg";
import iconTruck from "../assets/store-manager/icon-truck.svg";
import iconCheck from "../assets/store-manager/icon-check.svg";
import iconChev from "../assets/store-manager/icon-chev.svg";
import iconPlus from "../assets/store-manager/icon-plus.svg";
import { deferralExplanation } from "./deferralMessage.mjs";
import {
  arrivesOn, colomboDate, colomboTime, formatDay, isDeferred, needsReceipt, sortByArrival, statusLabel, statusTone,
} from "./orderStage.mjs";
import { StoreManagerHero } from "./StoreManagerHero";
import { UnavailableOrders } from "./UnavailableOrders";
import { Tracking, useOrderTrackings } from "./useOrderTrackings";

function Chevron() {
  return <img src={iconChev} alt="" aria-hidden="true" width={16} height={16} className="sm-chev-right" />;
}

function nextStep(row: Tracking, t: (key: string) => string) {
  if (needsReceipt(row.stage)) return <Link to={`/store-manager/receipts?order=${encodeURIComponent(row.order.id)}`} className="sm-track-link">{t("Confirm receipt")}</Link>;
  if (isDeferred(row.stage)) return t("Awaiting a new run");
  const eta = row.delivery?.arrivalPrediction?.estimatedArrivalAt || row.planning.plannedArrivalAt;
  if (validArrivalAt(eta) && (row.stage === "PLANNED" || row.stage === "READY_FOR_DEPARTURE" || row.stage === "OUT_FOR_DELIVERY")) {
    return `${t("ETA")} ${colomboTime(eta)}`;

  }
  if (row.stage === "CONFIRMED") return t("Awaiting dispatch planning");
  return t("Complete");
}

export function StoreManagerDashboardPage() {
  const { t, locale } = useLocale();
  const cutoffTime = formatCutoff(useOrderCutoff(), locale);
  const [params, setParams] = useSearchParams();
  const { rows, loading, loadFailed, unavailable, reload } = useOrderTrackings();
  const { dashboards } = useDashboards();
  const selected = dashboards.find((d) => d.id === params.get("dashboard"));
  const heroActions = (
    <>
      <DashboardMenu dashboards={dashboards} current={selected?.id || ""} />
      <Link to="/store-manager/dashboards/new" className="sm-btn-secondary sm-btn-create-dash">+ {t("Create new dashboard")}</Link>
      <Link to="/store-manager/orders/new" className="sm-btn-place-order">
        <img src={iconPlus} alt="" aria-hidden="true" width={24} height={24} />
        {t("Place Order")}
      </Link>
    </>
  );

  if (selected) {
    return (
      <>
        <StoreManagerHero title={selected.name} subtitle={`${t("Made with the dashboard assistant")} · ${t("updated")} ${colomboTime(selected.updatedAt)}`} compact>{heroActions}</StoreManagerHero>
        <div className="sm-page-body dp-stack">
          {params.get("saved") && (
            <div className="dp-note dp-note--green dp-row dp-row--between" role="status">
              <span>✓ {t("Dashboard saved. Switch between your dashboards from the Dashboard menu.")}</span>
              <span className="dp-row">
                <Link to={`/store-manager/dashboards/new?edit=${encodeURIComponent(selected.id)}`} className="dp-btn dp-btn--secondary dp-btn--sm">{t("Edit with chat")}</Link>
                <button type="button" className="dp-link" onClick={() => { params.delete("saved"); setParams(params); }}>{t("Dismiss")}</button>
              </span>
            </div>
          )}
          {!params.get("saved") && <div className="dp-row" style={{ justifyContent: "flex-end" }}><Link to={`/store-manager/dashboards/new?edit=${encodeURIComponent(selected.id)}`} className="dp-btn dp-btn--secondary dp-btn--sm">{t("Edit with chat")}</Link></div>}
          {loadFailed && <div className="sm-load-error" role="alert"><span>{t("Orders could not be loaded. Check your connection and try again.")}</span><button type="button" className="tap" onClick={() => void reload()}>{t("Retry")}</button></div>}
          {loading ? <p className="muted" role="status">{t("Loading your orders…")}</p> : <DashboardGrid dashboard={selected} rows={rows} />}
          <p className="muted" style={{ fontSize: "0.8125rem", margin: 0 }}>{t("This dashboard reads only your outlet's orders, deliveries and receipts. It never changes them.")}</p>
        </div>
      </>
    );
  }

  const today = colomboDate(Date.now());
  const arriving = rows.filter((row) => arrivesOn(row, today)).sort(sortByArrival);
  const deferred = rows.filter((row) => isDeferred(row.stage));
  const awaitingReceipt = rows.filter((row) => needsReceipt(row.stage));
  const attentionCount = deferred.length + awaitingReceipt.length;
  const nextArrival = arriving.find((row) => validArrivalAt(row.delivery?.arrivalPrediction?.estimatedArrivalAt || row.planning.plannedArrivalAt));
  const estimatesUnavailable = arriving.some((row) => !validArrivalAt(row.delivery?.arrivalPrediction?.estimatedArrivalAt || row.planning.plannedArrivalAt));

  const temperature = (value: string) => (value === "chilled" ? t("Chilled") : t("Ambient"));

  return (
    <>
      <StoreManagerHero title={t("Your Deliveries")} subtitle={t("See what is arriving, what needs attention and when to order next.")}>
        {heroActions}
      </StoreManagerHero>

      <div className="sm-stat-row">
        <div className="sm-stat-card">
          <div className="sm-stat-icon sm-stat-icon--blue"><img src={iconHistory} alt="" aria-hidden="true" width={24} height={24} /></div>
          <div className="sm-stat-body">
            <p className="sm-stat-label">{t("Next expected arrival")}</p>
            <p className="sm-stat-value">{validArrivalAt(nextArrival?.delivery?.arrivalPrediction?.estimatedArrivalAt || nextArrival?.planning.plannedArrivalAt) ? colomboTime((nextArrival?.delivery?.arrivalPrediction?.estimatedArrivalAt || nextArrival!.planning.plannedArrivalAt)!) : "—"}</p>

            <p className="sm-stat-sub sm-stat-sub--orange">
              {nextArrival ? `${t("Today")} · ${temperature(nextArrival.order.temperatureRequirement)}` : t("No arrivals today")}
            </p>
          </div>
        </div>
        <div className="sm-stat-card">
          <div className="sm-stat-icon sm-stat-icon--teal"><img src={iconTruck} alt="" aria-hidden="true" width={24} height={24} /></div>
          <div className="sm-stat-body">
            <p className="sm-stat-label">{t("On today's route")}</p>
            <p className="sm-stat-value">{arriving.length} {arriving.length === 1 ? t("order") : t("orders")}</p>
            <p className="sm-stat-sub muted">{t("Ambient and chilled tracked separately")}</p>
          </div>
        </div>
        <div className="sm-stat-card">
          <div className="sm-stat-icon sm-stat-icon--green"><img src={iconCheck} alt="" aria-hidden="true" width={24} height={24} /></div>
          <div className="sm-stat-body">
            <p className="sm-stat-label">{t("Needs attention")}</p>
            <p className="sm-stat-value">{attentionCount} {attentionCount === 1 ? t("item") : t("items")}</p>
            <p className="sm-stat-sub sm-stat-sub--green">
              {deferred.length > 0 ? t("Next run awaiting confirmation") : awaitingReceipt.length > 0 ? t("Receipts waiting for confirmation") : t("All clear")}
            </p>
          </div>
        </div>
      </div>

      {estimatesUnavailable && <p role="status">{ESTIMATES_UNAVAILABLE_MESSAGE}</p>}
      {loadFailed && (
        <div className="sm-load-error" role="alert">
          <span>{t("Orders could not be loaded. Check your connection and try again.")}</span>
          <button type="button" className="tap" onClick={() => void reload()}>{t("Retry")}</button>
        </div>
      )}
      {!loadFailed && <UnavailableOrders orders={unavailable} onRetry={() => void reload()} />}

      <div className="sm-dashboard-grid">
        <section className="sm-panel" aria-labelledby="arriving-today-heading">
          <div className="sm-panel-header">
            <h2 id="arriving-today-heading" className="sm-panel-title">{t("Arriving Today")}</h2>
            <Link to="/store-manager/orders" className="sm-panel-link">{t("View all orders")}<Chevron /></Link>
          </div>
          {loading && <p className="sm-empty muted" role="status">{t("Loading your orders…")}</p>}
          {!loading && arriving.length === 0 && !loadFailed && <p className="sm-empty muted">{t("No deliveries are scheduled for today.")}</p>}
          {arriving.map((row) => (
            <div key={row.order.id} className="sm-order-card">
              <div className="sm-order-card-top">
                <div className="sm-order-icon"><img src={iconHistory} alt="" aria-hidden="true" width={24} height={24} /></div>
                <div className="sm-order-info">
                  <p className="sm-order-name">{temperature(row.order.temperatureRequirement)} · {row.order.orderRef}</p>
                  <p className="sm-order-desc muted">{row.order.orderUnits} {t("units")} · {Number(row.order.orderWeightKg.toFixed(2))} <span>kg</span></p>
                </div>
                <span className={`sm-badge sm-badge--${statusTone(row.stage)}`}>{t(statusLabel(row.stage))}</span>
              </div>
              <hr className="sm-order-divider" />
              <div className="sm-order-card-bottom">
                <p className="sm-order-eta">
                  <span className="muted">{t("Expected arrival:")} </span>
                  <strong>{validArrivalAt(row.delivery?.arrivalPrediction?.estimatedArrivalAt || row.planning.plannedArrivalAt) ? colomboTime((row.delivery?.arrivalPrediction?.estimatedArrivalAt || row.planning.plannedArrivalAt)!) : "—"}</strong>

                </p>
                <Link to={`/store-manager/orders/${encodeURIComponent(row.order.id)}/track`} className="sm-track-link">{t("Track order")}<Chevron /></Link>
              </div>
            </div>
          ))}
        </section>

        <section className="sm-panel sm-attention-panel" aria-labelledby="attention-heading">
          <div className="sm-panel-header">
            <h2 id="attention-heading" className="sm-panel-title">{t("Attention needed")}</h2>
            {attentionCount > 0 && <span className="sm-notice-badge">{attentionCount} {t("Notice")}</span>}
          </div>
          {deferred.map((row) => {
            const why = deferralExplanation(row.planning.reasonCode);
            return (
              <div key={row.order.id} className="sm-alert-card">
                <h3 className="sm-alert-title">{t("Order deferred to a later run")}</h3>
                <p className="sm-alert-body">{row.order.orderRef} · {t(why.message)}</p>
                <Link to={`/store-manager/orders?order=${encodeURIComponent(row.order.id)}`} className="sm-alert-action">
                  {t("See Reason and Updates")}<Chevron />
                </Link>
              </div>
            );
          })}
          {awaitingReceipt.map((row) => (
            <div key={row.order.id} className="sm-alert-card">
              <h3 className="sm-alert-title">{t("Delivery needs receipt confirmation")}</h3>
              <p className="sm-alert-body">{row.order.orderRef} · {t("Confirm the count and report any shortage or damage.")}</p>
              <Link to="/store-manager/receipts" className="sm-alert-action">{t("Confirm receipt")}<Chevron /></Link>
            </div>
          ))}
          <div className="sm-ordering-window">
            <h3 className="sm-ordering-title">{t("Ordering window")}</h3>
            <p className="sm-ordering-body muted">{cutoffTime ? withTime(t("Place your next order by {time} today. Orders after the cutoff move to the next eligible run."), cutoffTime) : t("Orders after the daily cutoff move to the next eligible run.")}</p>
            <Link to="/store-manager/orders/new" className="sm-create-order-link">{t("Create Order")}<Chevron /></Link>
          </div>
        </section>
      </div>

      <section className="sm-panel sm-recent-panel" aria-labelledby="recent-orders-heading">
        <div className="sm-panel-header">
          <h2 id="recent-orders-heading" className="sm-panel-title">{t("Recent orders")}</h2>
          <Link to="/store-manager/orders" className="sm-panel-link">{t("Order History")}<Chevron /></Link>
        </div>
        <div className="sm-table-wrap">
          <table className="sm-table">
            <thead>
              <tr>
                <th scope="col">{t("ORDER")}</th>
                <th scope="col">{t("GOODS")}</th>
                <th scope="col">{t("NEEDED")}</th>
                <th scope="col">{t("STATUS")}</th>
                <th scope="col">{t("NEXT STEP")}</th>
              </tr>
            </thead>
            <tbody>
              {rows.slice(0, 10).map((row) => (
                <tr key={row.order.id}>
                  <td><Link to={`/store-manager/orders?order=${encodeURIComponent(row.order.id)}`} className="sm-track-link">{row.order.orderRef}</Link></td>
                  <td>{temperature(row.order.temperatureRequirement)} · {row.order.orderUnits} {t("units")}</td>
                  <td>{formatDay(row.order.requestedDeliveryDate)}</td>
                  <td><span className={`sm-badge sm-badge--${statusTone(row.stage)}`}>{t(statusLabel(row.stage))}</span></td>
                  <td>{nextStep(row, t)}</td>
                </tr>
              ))}
              {!loading && rows.length === 0 && unavailable.length === 0 && !loadFailed && (
                <tr><td colSpan={5} className="sm-table-empty muted">{t("No orders yet. Place your first order to get started.")}</td></tr>
              )}
              {loading && <tr><td colSpan={5} className="sm-table-empty muted" role="status">{t("Loading your orders…")}</td></tr>}
            </tbody>
          </table>
        </div>
      </section>
    </>
  );
}
