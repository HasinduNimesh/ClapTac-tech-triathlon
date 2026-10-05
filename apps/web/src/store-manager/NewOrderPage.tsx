import { FormEvent, useEffect, useRef, useState } from "react";
import { Prefill } from "../automations/types";
import { Link, useLocation } from "react-router-dom";
import { ApiError, apiJSON, Order } from "../api/client";
import { todayInSriLanka } from "../api/date.mjs";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { formatDay } from "./orderStage.mjs";
import { CutoffNotice, StoreManagerHero } from "./StoreManagerHero";
import iconCheck from "../assets/store-manager/icon-check.svg";
import { useHelpersAvailable } from "../api/assistants";
import { OrderHelperLauncher } from "./OrderHelperLauncher";
import { OrderItems } from "../components/OrderItems";
import "../components/orderItems.css";
import type { CatalogProduct, DraftLine, FormFill } from "./orderDraft.mjs";
import { addProduct, linesSource, pickable, reasonSentence, removeLine, requestLines, setPacks, suggestionLines, totals } from "./orderLines.mjs";
import type { SuggestedLine, Suggestion } from "./orderLines.mjs";

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
  const location = useLocation();
  const candidate = location.state?.prefillOwner === profile?.userId ? location.state?.habitPrefill as Prefill | undefined : undefined;
  const prefill = candidate && profile?.outletIds?.includes(candidate.outletId) && Number.isInteger(candidate.orderUnits) && candidate.orderUnits > 0 && candidate.orderWeightKg > 0 && candidate.orderVolumeM3 > 0 && ["ambient","chilled"].includes(candidate.temperatureRequirement) ? candidate : undefined;
  const [date, setDate] = useState(minDate);
  const [temp, setTemp] = useState<"ambient" | "chilled">(prefill?.temperatureRequirement || "ambient");
  const [units, setUnits] = useState(prefill ? String(prefill.orderUnits) : "");
  const [weight, setWeight] = useState(prefill ? String(prefill.orderWeightKg) : "");
  const [volume, setVolume] = useState(prefill ? String(prefill.orderVolumeM3) : "");
  // Items are the normal way to order: weight, volume and boxes are worked out from them. Totals-only stays for
  // a regular order prepared from habit and for when the product list cannot be loaded.
  const [lines, setLines] = useState<DraftLine[]>([]);
  const [totalsOnly, setTotalsOnly] = useState(Boolean(prefill));
  const [products, setProducts] = useState<CatalogProduct[] | null>(null);
  const [pickId, setPickId] = useState("");
  const [pickQty, setPickQty] = useState("1");
  // Items that came from "Suggest an order", with why each quantity was chosen.
  const [suggested, setSuggested] = useState<{ reasons: Record<string, SuggestedLine>; coverDays: number; deliveryDate: string; limitedHistory: boolean } | null>(null);
  const [suggesting, setSuggesting] = useState(false);
  const [created, setCreated] = useState<Order | null>(null);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const inFlight = useRef(false);
  useEffect(() => {
    const token = user?.access_token;
    if (!token) return;
    let live = true;
    apiJSON<{ items: CatalogProduct[] }>("/shared/products", token)
      .then((body) => { if (live) setProducts(body.items || []); })
      .catch(() => { if (live) { setProducts([]); } });
    return () => { live = false; };
  }, [user?.access_token]);
  useEffect(() => {
    if (!prefill) return;
    setUnits(String(prefill.orderUnits)); setWeight(String(prefill.orderWeightKg)); setVolume(String(prefill.orderVolumeM3)); setTemp(prefill.temperatureRequirement); setCreated(null);
    setLines([]); setTotalsOnly(true);
  }, [location.key]);

  const outlet = profile?.outletIds?.[0] || "";
  const goods = temp === "chilled" ? t("Chilled") : t("Ambient");

  const helpers = useHelpersAvailable(user?.access_token);
  const [filledFrom, setFilledFrom] = useState("");

  function fillFromText(fill: FormFill, neededBy: string | null) {
    setTemp(fill.temperature);
    setUnits(String(fill.orderUnits));
    setWeight(String(fill.orderWeightKg));
    setVolume(String(fill.orderVolumeM3));
    // Lines read from the text become the order's items; a fill built only from a previous order has totals only.
    setLines(fill.lines);
    setTotalsOnly(fill.lines.length === 0);
    if (neededBy && neededBy >= minDate) setDate(neededBy);
    setFilledFrom(fill.temperature);
    setError("");
    document.getElementById("delivery-details-heading")?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function reset() {
    setCreated(null);
    setUnits("");
    setWeight("");
    setVolume("");
    setLines([]);
    setSuggested(null);
    setTotalsOnly(false);
    setError("");
    setFilledFrom("");
  }

  const preview = totals(lines);
  const usingItems = !totalsOnly;
  const options: CatalogProduct[] = products ? pickable(products, temp, lines) : [];
  const productsDown = products !== null && products.length === 0;
  async function suggestOrder() {
    if (suggesting || !user?.access_token || !products) return;
    setSuggesting(true);
    setError("");
    try {
      const body = await apiJSON<{ suggestion: Suggestion }>(`/orders/suggestion?date=${encodeURIComponent(date)}&temperature=${temp}`, user.access_token);
      const result = suggestionLines(products, body.suggestion);
      if (result.lines.length === 0) {
        setSuggested(null);
        setError(t("There is nothing to suggest yet for this goods type. Add the items yourself."));
      } else {
        setLines(result.lines);
        setSuggested({ reasons: result.reasons, coverDays: body.suggestion.coverDays, deliveryDate: body.suggestion.deliveryDate, limitedHistory: body.suggestion.limitedHistory });
      }
    } catch {
      setError(t("The suggestion is not available right now. Add the items yourself."));
    } finally {
      setSuggesting(false);
    }
  }
  function addItem() {
    const product = options.find((p) => p.id === pickId);
    if (!product) { setError(t("Choose a product to add.")); return; }
    const next = addProduct(lines, product, pickQty);
    if (next === lines) { setError(t("Enter a whole number of boxes above zero.")); return; }
    setError("");
    setLines(next);
    setPickQty("1");
  }

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    if (inFlight.current) return;
    setError("");
    const orderUnits = Number(units);
    const orderWeightKg = Number(weight);
    const orderVolumeM3 = Number(volume);
    if (usingItems && lines.length === 0) {
      setError(t("Add at least one item to the order."));
      return;
    }
    if (!usingItems && (!Number.isInteger(orderUnits) || orderUnits <= 0 || !(orderWeightKg > 0) || !(orderVolumeM3 > 0))) {
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
        // With items the server works out units, weight and volume itself from the catalog.
        body: JSON.stringify(usingItems
          ? { requestedDeliveryDate: date, temperatureRequirement: temp, lines: requestLines(lines), linesSource: suggested ? "habit_helper" : linesSource(lines) }
          : { requestedDeliveryDate: date, orderUnits, orderWeightKg, orderVolumeM3, temperatureRequirement: temp }),
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
            {created.lines && created.lines.length > 0 && <OrderItems lines={created.lines} units={created.orderUnits} />}
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

      {helpers?.order && user?.access_token && <OrderHelperLauncher token={user.access_token} onFill={fillFromText} />}

      <form className="sm-page-body sm-order-form" onSubmit={onSubmit} aria-describedby={error ? "order-error" : undefined}>
        {prefill && <p className="auto-notice">{t("Prepared from your regular order. Check the delivery date and quantities before submitting.")}</p>}
        {error && <p id="order-error" className="status-bad sm-form-error" role="alert">{error}</p>}
        <div className="sm-order-form-grid">
          <div className="sm-order-form-main">
            {filledFrom && <p className="sm-info-banner sm-helper-filled" role="status">{t("The form below was filled in from your text. Check every value before you submit.")}</p>}
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
                <select id="order-type" value={temp} disabled={usingItems && lines.length > 0} onChange={(e) => { setTemp(e.target.value as "ambient" | "chilled"); setPickId(""); }} aria-describedby="order-type-hint">
                  <option value="ambient">{t("Ambient")}</option>
                  <option value="chilled">{t("Chilled")}</option>
                </select>
                <p id="order-type-hint" className="sm-field-hint muted">{t("Ambient and chilled items need separate orders so each can be assigned to a compatible vehicle.")}</p>
              </div>
            </section>

            <section className="sm-form-card" aria-labelledby="items-heading">
              <h2 id="items-heading" className="sm-form-card-title">{t("Items to deliver")}</h2>
              {usingItems ? (
                <>
                  <p className="sm-form-card-sub muted">{t("Pick the products and how many boxes of each. Weight and volume are worked out from the product list, and they are what planning checks against the vehicle.")}</p>
                  {productsDown && <p className="status-bad" role="status">{t("The product list is not available right now. Enter the totals instead.")}</p>}
                  <div className="sm-field-row">
                    <div className="sm-field">
                      <label htmlFor="item-product">{t("Product")}</label>
                      <select id="item-product" value={pickId} onChange={(e) => setPickId(e.target.value)} disabled={products === null || productsDown}>
                        <option value="">{products === null ? t("Loading products…") : t("Choose a product")}</option>
                        {options.map((p) => <option key={p.id} value={p.id}>{p.name} · {p.pack}</option>)}
                      </select>
                    </div>
                    <div className="sm-field">
                      <label htmlFor="item-qty">{t("Number of boxes")}</label>
                      <input id="item-qty" type="number" inputMode="numeric" min="1" max="999" step="1" value={pickQty} onChange={(e) => setPickQty(e.target.value)} />
                    </div>
                  </div>
                  <div className="dp-row" style={{ gap: 8, flexWrap: "wrap" }}>
                    <button type="button" className="sm-btn-secondary" onClick={addItem} disabled={!pickId}>{t("Add item")}</button>
                    <button type="button" className="sm-btn-secondary" onClick={() => void suggestOrder()} disabled={suggesting || products === null || productsDown}>{suggesting ? t("Working out a suggestion…") : t("Suggest an order")}</button>
                  </div>
                  {lines.length === 0 ? <p className="muted" style={{ margin: "12px 0 0" }}>{t("No items added yet.")}</p> : (
                    <div className="order-items-wrap">
                      <table className="order-items">
                        <caption className="visually-hidden">{t("Items on this order")}</caption>
                        <thead><tr><th scope="col">{t("Item")}</th><th scope="col" className="num">{t("Boxes")}</th><th scope="col" className="num">{t("Weight (kg)")}</th><th scope="col" className="num">{t("Volume (m³)")}</th><th scope="col"><span className="visually-hidden">{t("Remove")}</span></th></tr></thead>
                        <tbody>
                          {lines.map((l) => (
                            <tr key={l.productId}>
                              <th scope="row">{l.name}<span className="order-items-sub">{l.pack}</span></th>
                              <td className="num"><input aria-label={`${t("Number of boxes")}: ${l.name}`} type="number" inputMode="numeric" min="1" max="999" step="1" value={l.quantity} onChange={(e) => setLines(setPacks(lines, l.productId, e.target.value))} style={{ width: 72 }} /></td>
                              <td className="num">{l.weightKg}</td>
                              <td className="num">{l.volumeM3}</td>
                              <td><button type="button" className="sm-btn-secondary" aria-label={`${t("Remove")}: ${l.name}`} onClick={() => setLines(removeLine(lines, l.productId))}>✕</button></td>
                            </tr>
                          ))}
                        </tbody>
                        <tfoot><tr><th scope="row">{t("Total")}</th><td className="num">{preview.units}</td><td className="num">{preview.weightKg}</td><td className="num">{preview.volumeM3}</td><td /></tr></tfoot>
                      </table>
                    </div>
                  )}
                  {suggested && lines.length > 0 && (
                    <div className="sm-info-banner" role="status">
                      <strong>{t("Suggested from your past orders and the delivery calendar. Check every quantity before you send.")}</strong>
                      <p style={{ margin: "4px 0" }}>{t("Planned for delivery on")} {formatDay(suggested.deliveryDate)}.{suggested.coverDays > 1 ? ` ${t("This delivery has to last {days} days, because the depot does not deliver on the days after it.").replace("{days}", String(suggested.coverDays))}` : ""}{suggested.limitedHistory ? ` ${t("There are not enough earlier orders with items yet, so quantities come from the store's usual daily sales.")}` : ""}</p>
                      <ul style={{ margin: 0, paddingLeft: 18 }}>
                        {lines.filter((l) => suggested.reasons[l.productId]).map((l) => <li key={l.productId}>{l.name}: {reasonSentence(suggested.reasons[l.productId], suggested.coverDays, t)}</li>)}
                      </ul>
                    </div>
                  )}
                  <p className="sm-field-hint muted">
                    <button type="button" className="linklike" onClick={() => { setTotalsOnly(true); setLines([]); setSuggested(null); }}>{t("I only know the totals")}</button>
                  </p>
                </>
              ) : (
                <>
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
                  <p className="sm-field-hint muted">
                    <button type="button" className="linklike" onClick={() => setTotalsOnly(false)}>{t("Choose items from the product list")}</button>
                  </p>
                </>
              )}
            </section>
          </div>

          <aside className="sm-form-card sm-review-card" aria-labelledby="review-heading">
            <h2 id="review-heading" className="sm-form-card-title">{t("Review before sending")}</h2>
            <dl className="sm-summary-list">
              <div><dt>{t("Outlet")}</dt><dd>{outlet || "—"}</dd></div>
              <div><dt>{t("Date / run")}</dt><dd>{formatDay(date)}</dd></div>
              <div><dt>{t("Goods")}</dt><dd>{goods}</dd></div>
              <div><dt>{t("Items")}</dt><dd>{usingItems ? (lines.length ? `${lines.length} ${lines.length === 1 ? t("item") : t("items")} · ${preview.units} ${t("boxes")}` : "—") : (units ? `${units} ${t("units")}` : "—")}</dd></div>
              <div><dt>{t("Weight (kg)")}</dt><dd>{usingItems ? (lines.length ? preview.weightKg : "—") : (weight || "—")}</dd></div>
              <div><dt>{t("Volume (m³)")}</dt><dd>{usingItems ? (lines.length ? preview.volumeM3 : "—") : (volume || "—")}</dd></div>
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
