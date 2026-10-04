import { FormEvent, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ApiError, apiJSON, Order } from "../api/client";
import { todayInSriLanka } from "../api/date.mjs";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { formatDay } from "./orderStage.mjs";
import { CutoffNotice, StoreManagerHero } from "./StoreManagerHero";
import iconCheck from "../assets/store-manager/icon-check.svg";

function failureMessage(error: unknown, fallback: string) {
  if (error instanceof ApiError) {
    try {
      const parsed = JSON.parse(error.message) as { message?: string; error?: string };
      return parsed.message || parsed.error || fallback;
    } catch {
      return error.message || fallback;
    }
  }
  return fallback;
}

export function NewOrderPage() {
  const { user, profile } = useAuth();
  const { t } = useLocale();
  const minDate = todayInSriLanka();
  const [date, setDate] = useState(minDate);
  const [temp, setTemp] = useState<"ambient" | "chilled">("ambient");
  const [units, setUnits] = useState("");
  const [weight, setWeight] = useState("");
  const [volume, setVolume] = useState("");
  const [created, setCreated] = useState<Order | null>(null);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const inFlight = useRef(false);

  const outlet = profile?.outletIds?.[0] || "";
  const goods = temp === "chilled" ? t("Chilled") : t("Ambient");

  function reset() {
    setCreated(null);
    setUnits("");
    setWeight("");
    setVolume("");
    setError("");
  }

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    if (inFlight.current) return;
    setError("");
    const orderUnits = Number(units);
    const orderWeightKg = Number(weight);
    const orderVolumeM3 = Number(volume);
    if (!Number.isInteger(orderUnits) || orderUnits <= 0 || !(orderWeightKg > 0) || !(orderVolumeM3 > 0)) {
      setError(t("Enter a whole number of units and a weight and volume greater than zero."));
      return;
    }
    if (date < minDate) {
      setError(t("Choose today or a later delivery date."));
      return;
    }
    inFlight.current = true;
    setSubmitting(true);
    try {
      const body = await apiJSON<{ order: Order }>("/orders", user!.access_token, {
        method: "POST",
        body: JSON.stringify({ requestedDeliveryDate: date, orderUnits, orderWeightKg, orderVolumeM3, temperatureRequirement: temp }),
      });
      setCreated(body.order);
    } catch (err) {
      setError(failureMessage(err, t("The order could not be submitted. Check your connection and try again.")));
    } finally {
      inFlight.current = false;
      setSubmitting(false);
    }
  }

  if (created) {
    return (
      <>
        <StoreManagerHero
          compact
          crumbs={[{ label: t("Store"), to: "/store-manager" }, { label: t("Order Request Received") }]}
          title={t("Order Request Received")}
          subtitle={t("Your order is confirmed for planning. Dispatch will assign it to a vehicle.")}
        />
        <div className="sm-page-body">
          <section className="sm-form-card sm-confirm-card" aria-labelledby="order-received-heading" role="status">
            <h2 id="order-received-heading" className="sm-confirm-title">
              <img src={iconCheck} alt="" aria-hidden="true" width={24} height={24} />
              {t("Request")} {created.orderRef} {t("received")}
            </h2>
            {created.requestedDeliveryDate !== date && (
              <p className="status-bad sm-cutoff-warning" role="note">
                {t("Scheduled for the next eligible run (placed after 4:00 PM cutoff).")}
              </p>
            )}
            <dl className="sm-summary-list">
              <div><dt>{t("Outlet")}</dt><dd>{created.outletId || outlet || "—"}</dd></div>
              <div><dt>{t("Needed on")}</dt><dd>{formatDay(created.requestedDeliveryDate)}</dd></div>
              <div><dt>{t("Goods")}</dt><dd>{created.temperatureRequirement === "chilled" ? t("Chilled") : t("Ambient")}</dd></div>
              <div><dt>{t("Items")}</dt><dd>{created.orderUnits} {t("units")}</dd></div>
              <div><dt>{t("Status")}</dt><dd>{t("Awaiting dispatch planning")}</dd></div>
            </dl>
            <p className="muted sm-next-steps">
              {t("Next steps: Your order is queued for dispatch planning (Request acknowledged). Dispatch will assign it to a vehicle and notify you of the delivery window.")}
            </p>
            <div className="sm-confirm-actions">
              <Link to={`/store-manager/orders?order=${encodeURIComponent(created.id)}`} className="sm-btn-secondary">{t("View Your Orders")}</Link>
              <button type="button" className="sm-btn-secondary" onClick={reset}>{t("Place Another Order")}</button>
            </div>
          </section>
        </div>
      </>
    );
  }

  return (
    <>
      <StoreManagerHero
        compact
        crumbs={[{ label: t("Store"), to: "/store-manager" }, { label: t("Place an order") }]}
        title={t("Place an Order")}
        subtitle={t("Specify what your outlet needs. A submitted order is confirmed for planning, not yet assigned to a vehicle.")}
      >
        <CutoffNotice />
      </StoreManagerHero>

      <form className="sm-page-body sm-order-form" onSubmit={onSubmit} aria-describedby={error ? "order-error" : undefined}>
        {error && <p id="order-error" className="status-bad sm-form-error" role="alert">{error}</p>}
        <div className="sm-order-form-grid">
          <div className="sm-order-form-main">
            <section className="sm-form-card" aria-labelledby="delivery-details-heading">
              <h2 id="delivery-details-heading" className="sm-form-card-title">{t("Delivery details")}</h2>
              <p className="sm-form-card-sub muted">{t("Your outlet is set from your account. Contact your dispatcher if this is wrong.")}</p>
              <div className="sm-field-row">
                <div className="sm-field">
                  <label htmlFor="order-outlet">{t("Outlet")}</label>
                  <input id="order-outlet" value={outlet || t("No outlet assigned")} readOnly />
                </div>
                <div className="sm-field">
                  <label htmlFor="requested-delivery-date">{t("Delivery date needed")} *</label>
                  <input id="requested-delivery-date" type="date" min={minDate} value={date} onChange={(e) => setDate(e.target.value)} required />
                </div>
              </div>
              <div className="sm-field">
                <label htmlFor="order-type">{t("Order type")} *</label>
                <select id="order-type" value={temp} onChange={(e) => setTemp(e.target.value as "ambient" | "chilled")} aria-describedby="order-type-hint">
                  <option value="ambient">{t("Ambient")}</option>
                  <option value="chilled">{t("Chilled")}</option>
                </select>
                <p id="order-type-hint" className="sm-field-hint muted">{t("Ambient and chilled items need separate orders so each can be assigned to a compatible vehicle.")}</p>
              </div>
            </section>

            <section className="sm-form-card" aria-labelledby="items-heading">
              <h2 id="items-heading" className="sm-form-card-title">{t("Items to deliver")}</h2>
              <p className="sm-form-card-sub muted">{t("Quantity, weight and volume support the capacity checks used for planning.")}</p>
              <div className="sm-field-row sm-field-row--3">
                <div className="sm-field">
                  <label htmlFor="order-units">{t("Units")} *</label>
                  <input id="order-units" type="number" inputMode="numeric" min="1" step="1" placeholder="30" value={units} onChange={(e) => setUnits(e.target.value)} required />
                </div>
                <div className="sm-field">
                  <label htmlFor="order-weight">{t("Weight (kg)")} *</label>
                  <input id="order-weight" type="number" inputMode="decimal" min="0.01" step="0.01" placeholder="150" value={weight} onChange={(e) => setWeight(e.target.value)} required />
                </div>
                <div className="sm-field">
                  <label htmlFor="order-volume">{t("Volume (m³)")} *</label>
                  <input id="order-volume" type="number" inputMode="decimal" min="0.01" step="0.01" placeholder="0.42" value={volume} onChange={(e) => setVolume(e.target.value)} required />
                </div>
              </div>
            </section>
          </div>

          <aside className="sm-form-card sm-review-card" aria-labelledby="review-heading">
            <h2 id="review-heading" className="sm-form-card-title">{t("Review before sending")}</h2>
            <dl className="sm-summary-list">
              <div><dt>{t("Outlet")}</dt><dd>{outlet || "—"}</dd></div>
              <div><dt>{t("Date / run")}</dt><dd>{formatDay(date)}</dd></div>
              <div><dt>{t("Goods")}</dt><dd>{goods}</dd></div>
              <div><dt>{t("Items")}</dt><dd>{units ? `${units} ${t("units")}` : "—"}</dd></div>
              <div><dt>{t("Weight (kg)")}</dt><dd>{weight || "—"}</dd></div>
              <div><dt>{t("Volume (m³)")}</dt><dd>{volume || "—"}</dd></div>
              <div><dt>{t("Planning")}</dt><dd>{t("Awaiting dispatch")}</dd></div>
            </dl>
            <button type="submit" className="tap primary sm-submit" disabled={submitting}>
              {submitting ? t("Submitting…") : t("Submit Order")}
            </button>
            <p className="sm-field-hint muted">{t("A submitted request gets a receipt. Assignment and ETA follow dispatch planning.")}</p>
          </aside>
        </div>
      </form>
    </>
  );
}
