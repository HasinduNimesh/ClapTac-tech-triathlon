import { FormEvent, useCallback, useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { apiJSON, Order } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { REPORT_WINDOW_HOURS, reportBy } from "./DashboardCards";
import { colomboDate, colomboTime, formatDay } from "./orderStage.mjs";
import { StoreManagerHero } from "./StoreManagerHero";
import { Tracking } from "./useOrderTrackings";

type Pending = { order: Order; tracking: Tracking };
const key = () => (typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `receipt-${Date.now()}-${Math.random().toString(16).slice(2)}`);

export function ReceiptConfirmPage() {
  const { user } = useAuth();
  const { t } = useLocale();
  const token = user?.access_token || "";
  const [params, setParams] = useSearchParams();
  const [items, setItems] = useState<Pending[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [received, setReceived] = useState("");
  const [issueType, setIssueType] = useState("MISSING");
  const [note, setNote] = useState("");
  const [custody, setCustody] = useState({ sealId: "", serials: "", condition: "", receiver: "" });
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try { setItems((await apiJSON<{ items: Pending[] }>("/orders/receipts/pending", token)).items || []); setError(""); }
    catch (e) { setError(String(e)); }
    finally { setLoading(false); }
  }, [token]);
  useEffect(() => { void load(); }, [load]);

  const current = items.find((p) => p.order.id === params.get("order")) || items[0];
  useEffect(() => {
    if (!current) return;
    setReceived(String(current.order.orderUnits));
    setNote("");
    const loaded = current.tracking.custody?.[0];
    setCustody({ sealId: loaded?.sealId || "", serials: (loaded?.serialNumbers || []).join(", "), condition: "", receiver: "" });
  }, [current?.order.id]);

  const order = current?.order;
  const tracking = current?.tracking;
  const units = Number(received || 0);
  const short = order ? Math.max(0, order.orderUnits - units) : 0;
  const tech = order?.brand.toLowerCase() === "tech";
  const due = tracking ? reportBy(tracking) : undefined;
  const driverShort = (tracking?.delivery?.loadingShortfallSummary || []).reduce((s, x) => s + (x.affectedUnits || 0), 0);
  const driverDelivered = order ? order.orderUnits - driverShort : 0;

  async function confirm(e: FormEvent) {
    e.preventDefault();
    if (!order) return;
    setError(""); setMessage("");
    if (tech && (!custody.sealId.trim() || !custody.serials.trim() || !custody.condition.trim() || !custody.receiver.trim())) { setError(t("Tech receipt requires the seal, serials, received condition, and receiver name.")); return; }
    setBusy(true);
    try {
      const payload: { receivedUnits: number; issue?: { issueType: string; affectedUnits: number; note: string; idempotencyKey: string } } = { receivedUnits: units };
      if (short > 0) payload.issue = { issueType, affectedUnits: short, note, idempotencyKey: key() };
      await apiJSON(`/orders/${order.id}/receipt/confirm`, token, { method: "POST", headers: { "Idempotency-Key": payload.issue?.idempotencyKey || key() }, body: JSON.stringify(payload) });
      if (tech) {
        const k = key();
        await apiJSON(`/orders/${order.id}/custody`, token, { method: "POST", headers: { "Idempotency-Key": k }, body: JSON.stringify({ stage: "RECEIVED", sealId: custody.sealId.trim(), serialNumbers: custody.serials.split(/[\n,;]/).map((v) => v.trim()).filter(Boolean), condition: custody.condition.trim(), receiverName: custody.receiver.trim(), idempotencyKey: k }) });
      }
      setMessage(`${t("Receipt saved for")} ${order.orderRef}.`);
      params.delete("order"); setParams(params);
      await load();
    } catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  }

  return (
    <>
      <StoreManagerHero compact crumbs={[{ label: t("Store"), to: "/store-manager" }, { label: t("Receipt confirmation") }]} title={t("Confirm delivery receipt")} subtitle={t("Confirm the quantity received after delivery. Report a discrepancy if anything is missing or damaged.")} />
      <div className="sm-page-body dp-stack">
        {error && <p className="dp-note dp-note--red" role="alert">{error}</p>}
        {message && <p className="dp-note dp-note--green" role="status">{message}</p>}
        {loading && <p className="muted" role="status">{t("Loading your orders…")}</p>}
        {!loading && !current && <section className="dp-panel"><div className="dp-panel-body" style={{ paddingTop: 20 }}><p style={{ margin: 0 }}>{t("No receipts are waiting for confirmation.")}</p><Link to="/store-manager/orders" className="dp-link">{t("Back to your orders")}</Link></div></section>}
        {items.length > 1 && <div className="dp-chips" role="group" aria-label={t("Receipts waiting")}>{items.map((p) => <button key={p.order.id} type="button" className="dp-chip" aria-pressed={p.order.id === order?.id} onClick={() => { params.set("order", p.order.id); setParams(params); }}>{p.order.orderRef}</button>)}</div>}
        {order && tracking && (
          <form onSubmit={confirm} className="dp-stack">
            <div className="dp-row dp-row--between">
              <div>
                <h2 className="dp-panel-title">{order.orderRef} · {t(order.brand)} · {order.outletId}</h2>
                <p className="muted" style={{ margin: 0 }}>{tracking.delivery?.completedAt ? `${t("Delivered")} ${colomboTime(tracking.delivery.completedAt)} · ` : ""}{`${t("Driver recorded")} ${driverDelivered} ${t("of")} ${order.orderUnits}`}</p>
              </div>
              <span className="dp-tag dp-tag--amber">{t("Receipt needs confirmation")}</span>
            </div>
            <div className="dp-grid-2">
              <section className="dp-panel" aria-label={t("Received quantities")}>
                <div className="dp-table-wrap">
                  <table className="dp-table">
                    <thead><tr><th>{t("Item / order")}</th><th>{t("Driver record")}</th><th>{t("Received")}</th><th>{t("Issue")}</th></tr></thead>
                    <tbody>
                      <tr>
                        <td><span className="dp-cell-main">{order.orderRef} · {order.temperatureRequirement === "chilled" ? t("Chilled") : t("Ambient")}</span><span className="dp-cell-sub">{order.orderUnits} {t("ordered")} · {formatDay(order.requestedDeliveryDate)}</span></td>
                        <td><span className="dp-tag dp-tag--cool">{driverDelivered} {t("delivered")}</span></td>
                        <td><label className="visually-hidden" htmlFor="received-units">{t("Received units")}</label><input id="received-units" className="sm-receipt-input" type="number" min={0} max={order.orderUnits} value={received} onChange={(e) => setReceived(e.target.value)} required /></td>
                        <td>{short > 0 ? <span className="dp-tag dp-tag--red">{short} {t("short")}</span> : <span className="dp-tag dp-tag--green">{t("Matched")}</span>}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <div className="dp-panel-body dp-stack" style={{ paddingTop: 16 }}>
                  {short > 0 && <>
                    <div className="dp-note dp-note--red"><strong>{t("Shortage found")}</strong>{t("Record the exact short quantity and attach evidence before confirming.")}</div>
                    <label className="dp-field" style={{ maxWidth: 360 }}>{t("Issue reason")}
                      <select value={issueType} onChange={(e) => setIssueType(e.target.value)}>
                        <option value="MISSING">{t("MISSING")}</option><option value="DAMAGED">{t("DAMAGED")}</option><option value="QUANTITY_MISMATCH">{t("QUANTITY_MISMATCH")}</option><option value="OTHER">{t("OTHER")}</option>
                      </select>
                    </label>
                  </>}
                  {tech && <fieldset className="dp-stack" style={{ border: "1px solid #dee3ed", borderRadius: 8, padding: 12 }}>
                    <legend className="dp-section-label">{t("High-value Tech custody")}</legend>
                    <div className="dp-filters">
                      <label className="dp-field">{t("Seal ID")}<input value={custody.sealId} onChange={(e) => setCustody({ ...custody, sealId: e.target.value })} /></label>
                      <label className="dp-field">{t("Received condition")}<input value={custody.condition} onChange={(e) => setCustody({ ...custody, condition: e.target.value })} /></label>
                      <label className="dp-field">{t("Receiver name")}<input value={custody.receiver} onChange={(e) => setCustody({ ...custody, receiver: e.target.value })} /></label>
                    </div>
                    <label className="dp-field">{t("Serial number(s), comma separated")}<textarea value={custody.serials} onChange={(e) => setCustody({ ...custody, serials: e.target.value })} /></label>
                  </fieldset>}
                  {driverShort > 0 && <p className="dp-note dp-note--amber" style={{ margin: 0 }}>{t("Loading shortfall reported")}: {driverShort} {t("units short before departure.")}</p>}
                </div>
              </section>
              <aside className="dp-panel" aria-label={t("Evidence for this discrepancy")}>
                <div className="dp-panel-body dp-stack" style={{ paddingTop: 20 }}>
                  <div><h3 className="dp-h3">{t("Evidence for this discrepancy")}</h3><p className="dp-panel-sub">{t("Describe what arrived, for example a delivery note number or damage seen.")}</p></div>
                  <label className="dp-field">{t("Note")}<textarea value={note} onChange={(e) => setNote(e.target.value)} maxLength={1000} placeholder={t("e.g. 4 crates of 24 received, seal intact")} /></label>
                  <p className="muted" style={{ margin: 0, fontSize: "0.75rem" }}>{t("Optional for shortages · required for damage")}</p>
                  <div className="dp-note dp-note--amber"><strong>{t("Report-by deadline")}</strong>{due ? `${formatDay(colomboDate(due))}, ${colomboTime(due)} · ${REPORT_WINDOW_HOURS} ${t("hours after delivery")}` : t("Opens after delivery")}</div>
                  <button type="submit" className="dp-btn dp-btn--block" disabled={busy || (short > 0 && issueType === "DAMAGED" && !note.trim())}>{short > 0 ? t("Confirm receipt & report shortage") : t("Confirm Receipt")}</button>
                </div>
              </aside>
            </div>
            <p className="muted" style={{ margin: 0 }}>{`${t("By confirming, you record the quantities received at")} ${order.outletId}.`}</p>
            <div><Link to={`/store-manager/orders/${encodeURIComponent(order.id)}/timeline`} className="dp-btn dp-btn--secondary">{t("Open order evidence timeline")}</Link></div>
          </form>
        )}
      </div>
    </>
  );
}
