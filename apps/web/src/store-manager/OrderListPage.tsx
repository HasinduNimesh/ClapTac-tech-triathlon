import { Link, useSearchParams } from "react-router-dom";
import { useLocale } from "../i18n";
import { deferralExplanation } from "./deferralMessage.mjs";
import {
  colomboDate, colomboTime, formatDay, isDeferred, isReceiptConfirmed, needsReceipt, statusLabel, statusTone, timelineSteps,
} from "./orderStage.mjs";
import { CutoffNotice, StoreManagerHero } from "./StoreManagerHero";
import { useOrderTrackings } from "./useOrderTrackings";

export function OrderListPage() {
  const { t } = useLocale();
  const { rows, loading, loadFailed, skipped, reload } = useOrderTrackings();
  const [params, setParams] = useSearchParams();

  const selected = rows.find((row) => row.order.id === params.get("order")) ?? rows[0];
  const temperature = (value: string) => (value === "chilled" ? t("Chilled") : t("Ambient"));
  const crumbs = [{ label: t("Store"), to: "/store-manager" }, { label: t("Your orders") }];

  if (!selected) {
    return (
      <>
        <StoreManagerHero compact crumbs={crumbs} title={t("Your orders")} subtitle={t("Follow each order from confirmation through planning, delivery and store receipt.")}>
          <CutoffNotice />
        </StoreManagerHero>
        <div className="sm-page-body">
          {loadFailed && (
            <div className="sm-load-error" role="alert">
              <span>{t("Orders could not be loaded. Check your connection and try again.")}</span>
              <button type="button" className="tap" onClick={() => void reload()}>{t("Retry")}</button>
            </div>
          )}
          {loading && <p className="muted" role="status">{t("Loading your orders…")}</p>}
          {!loading && !loadFailed && (
            <section className="sm-form-card sm-empty-card">
              <h2 className="sm-form-card-title">{t("No orders yet")}</h2>
              <p className="muted">{t("Orders you place for your outlet will appear here with live status.")}</p>
              <Link to="/store-manager/orders/new" className="tap primary sm-submit">{t("Place Order")}</Link>
            </section>
          )}
        </div>
      </>
    );
  }

  const { order, planning, stage, receipt } = selected;
  const steps = timelineSteps(selected);
  const why = isDeferred(stage) ? deferralExplanation(planning.reasonCode) : null;
  const eta = planning.plannedArrivalAt;

  return (
    <>
      <StoreManagerHero
        compact
        crumbs={[...crumbs.slice(0, 1), { label: t("Your orders"), to: "/store-manager/orders" }, { label: order.orderRef }]}
        title={t("Order details")}
        subtitle={`${order.orderRef} · ${temperature(order.temperatureRequirement)} · ${order.outletId}`}
      >
        <CutoffNotice />
      </StoreManagerHero>

      <div className="sm-page-body">
        {loadFailed && (
          <div className="sm-load-error" role="alert">
            <span>{t("Orders could not be loaded. Check your connection and try again.")}</span>
            <button type="button" className="tap" onClick={() => void reload()}>{t("Retry")}</button>
          </div>
        )}
        {skipped > 0 && <p className="sm-load-note muted" role="status">{t("Some orders could not be loaded and are not shown.")}</p>}

        <div className="sm-order-tabs" role="group" aria-label={t("Choose an order")}>
          {rows.map((row) => (
            <button
              key={row.order.id}
              type="button"
              className={`sm-order-tab${row.order.id === order.id ? " active" : ""}`}
              aria-pressed={row.order.id === order.id}
              onClick={() => setParams({ order: row.order.id }, { replace: true })}
            >
              {row.order.orderRef} · {t(statusLabel(row.stage))}
            </button>
          ))}
        </div>

        <div className="sm-order-detail-grid">
          <section className="sm-form-card" aria-labelledby="order-summary-heading">
            <div className="sm-order-detail-head">
              <div>
                <h2 id="order-summary-heading" className="sm-form-card-title">{temperature(order.temperatureRequirement)} · {order.orderUnits} {t("units")}</h2>
                <p className="muted sm-order-detail-sub">{t("Requested")} {formatDay(order.requestedDeliveryDate)} · {order.orderRef}</p>
              </div>
              <span className={`sm-badge sm-badge--${statusTone(stage)}`}>{t(statusLabel(stage))}</span>
            </div>

            {eta && (
              <p className="sm-eta-box">
                <strong>{t("Expected arrival")}: {formatDay(colomboDate(eta))} · {colomboTime(eta)}</strong>
                <span className="muted">{t("ETA reflects reported events; this is not continuous GPS tracking.")}</span>
              </p>
            )}

            {why && (
              <div className="sm-alert-card sm-deferral-note" role="status">
                <h3 className="sm-alert-title">{t("Order deferred to a later run")}</h3>
                <p className="sm-alert-body">{t(why.message)}{planning.reasonComment ? ` · ${planning.reasonComment}` : ""}</p>
                <p className="sm-alert-body">{t("What happens next")}: {t(why.nextAction)}</p>
              </div>
            )}

            <ol className="sm-timeline" aria-label={t("Order progress")}>
              {steps.map((step) => (
                <li key={step.key} className={`sm-timeline-step sm-timeline-step--${step.state}${step.warn ? " sm-timeline-step--warn" : ""}`} aria-current={step.state === "current" ? "step" : undefined}>
                  <span className="sm-timeline-dot" aria-hidden="true" />
                  <div>
                    <p className="sm-timeline-label">{t(step.label)}</p>
                    <p className="sm-timeline-detail muted">{t(step.detail)}</p>
                  </div>
                </li>
              ))}
            </ol>
          </section>

          <div className="sm-order-detail-side">
            <section className="sm-form-card" aria-labelledby="delivery-info-heading">
              <h2 id="delivery-info-heading" className="sm-form-card-title">{t("Delivery information")}</h2>
              <dl className="sm-summary-list">
                <div><dt>{t("Brand")}</dt><dd>{order.brand || "—"}</dd></div>
                <div><dt>{t("Outlet")}</dt><dd>{order.outletId}</dd></div>
                <div><dt>{t("Goods")}</dt><dd>{temperature(order.temperatureRequirement)}</dd></div>
                <div><dt>{t("Weight (kg)")}</dt><dd>{Number(order.orderWeightKg.toFixed(2))}</dd></div>
                <div><dt>{t("Volume (m³)")}</dt><dd>{Number(order.orderVolumeM3.toFixed(2))}</dd></div>
                <div><dt>{t("Trip")}</dt><dd>{planning.planRef || t("Not yet planned")}</dd></div>
              </dl>
            </section>

            <section className="sm-form-card" aria-labelledby="next-heading">
              <h2 id="next-heading" className="sm-form-card-title">{t("What happens next")}</h2>
              {needsReceipt(stage) ? (
                <>
                  <p className="muted sm-next-text">{t("The driver has recorded a delivery outcome. Confirm the quantity you received.")}</p>
                  <Link to="/store-manager/receipts" className="tap primary sm-submit">{t("Confirm receipt")}</Link>
                </>
              ) : isReceiptConfirmed(stage) ? (
                <>
                  <p className="muted sm-next-text">
                    {t("Receipt recorded")}{receipt ? `: ${receipt.receivedUnits}/${receipt.expectedUnits} ${t("units")}` : ""}
                  </p>
                  <Link to="/store-manager/tracking" className="sm-btn-secondary">{t("Report an issue")}</Link>
                </>
              ) : (
                <>
                  <p className="muted sm-next-text">{t("Receipt and issue actions become available after the driver records a delivery outcome.")}</p>
                  <Link to="/store-manager/tracking" className="sm-btn-secondary">{t("Open detailed tracking")}</Link>
                </>
              )}
            </section>
          </div>
        </div>
      </div>
    </>
  );
}
