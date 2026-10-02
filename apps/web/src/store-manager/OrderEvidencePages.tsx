import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { apiJSON } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { deferralExplanation } from "./deferralMessage.mjs";
import { colomboTime, formatDay, isDeferred, needsReceipt, statusLabel, timelineSteps } from "./orderStage.mjs";
import { StoreManagerHero } from "./StoreManagerHero";
import { Tracking } from "./useOrderTrackings";

function useTracking(orderId?: string) {
  const { user } = useAuth();
  const token = user?.access_token || "";
  const [tracking, setTracking] = useState<Tracking | null>(null);
  const [error, setError] = useState("");
  const [loadedAt, setLoadedAt] = useState<number>(Date.now());
  const load = useCallback(async () => {
    if (!token || !orderId) return;
    try { setTracking((await apiJSON<{ tracking: Tracking }>(`/orders/${encodeURIComponent(orderId)}/tracking`, token)).tracking); setError(""); setLoadedAt(Date.now()); }
    catch (e) { setError(String(e)); }
  }, [token, orderId]);
  useEffect(() => { void load(); const id = window.setInterval(() => void load(), 60_000); return () => window.clearInterval(id); }, [load]);
  return { tracking, error, loadedAt, reload: load };
}

type Event = { at?: string; actor: string; title: string; detail?: string; kind: "loader" | "dispatcher" | "driver" | "store" | "pending" };

function events(row: Tracking, t: (s: string) => string): Event[] {
  const list: Event[] = [];
  const order = row.order as Tracking["order"] & { createdAt?: string };
  list.push({ at: order.createdAt, actor: t("Store"), title: `${t("Order placed")} · ${order.orderUnits} ${t("units")}`, detail: `${t("Needed")} ${formatDay(order.requestedDeliveryDate)}`, kind: "store" });
  if (row.planning.planRef) list.push({ actor: t("Dispatcher"), title: `${t("Planned on")} ${row.planning.planRef}${row.planning.stopSequence ? ` · ${t("Stop")} ${row.planning.stopSequence}` : ""}`, detail: row.planning.plannedArrivalAt ? `${t("Planned arrival")} ${colomboTime(row.planning.plannedArrivalAt)}` : undefined, kind: "dispatcher" });
  if (row.planning.reasonCode) list.push({ actor: t("Dispatcher"), title: t("Moved to a later run"), detail: `${t(deferralExplanation(row.planning.reasonCode).message)}${row.planning.reasonComment ? ` · ${row.planning.reasonComment}` : ""}`, kind: "dispatcher" });
  for (const sf of row.delivery?.loadingShortfallSummary || []) list.push({ actor: t("Loader"), title: `${sf.affectedUnits ?? 0} ${t("units")} ${t(sf.type || "MISSING").toLowerCase()} ${t("at loading")}`, detail: sf.note || t("Reported before departure"), kind: "loader" });
  for (const c of row.custody || []) list.push({ at: c.recordedAt, actor: t(c.stage), title: `${t("Seal ID")} ${c.sealId} · ${c.condition}`, detail: c.recordedBy, kind: "loader" });
  if (row.delivery?.outcome) {
    const proofs = row.delivery.proofs || [];
    list.push({ at: row.delivery.completedAt || row.delivery.occurredAt, actor: `${t("Driver")}${row.delivery.vehicleId ? ` · ${row.delivery.vehicleId}` : ""}`, title: `${t(row.delivery.outcome)}${row.delivery.reason ? ` · ${t(row.delivery.reason)}` : ""}`, detail: proofs.length ? `${proofs.length} ${t("proof item(s)")}${proofs.some((p) => p.pending) ? ` · ${t("saved on device, then synced")}` : ""}${proofs.find((p) => p.receiverName) ? ` · ${t("Received by")} ${proofs.find((p) => p.receiverName)!.receiverName}` : ""}` : undefined, kind: "driver" });
  }
  if (row.receipt) list.push({ at: row.receipt.confirmedAt, actor: t("Store"), title: `${t("Receipt confirmed")}: ${row.receipt.receivedUnits} ${t("received")}${row.receipt.expectedUnits > row.receipt.receivedUnits ? `, ${row.receipt.expectedUnits - row.receipt.receivedUnits} ${t("short")}` : ""}`, kind: "store" });
  for (const i of row.receiptIssues || []) list.push({ at: i.createdAt, actor: t("Store"), title: `${t(i.issueType)} · ${i.affectedUnits} ${t("units")}`, detail: i.note, kind: "store" });
  if (!row.receipt && needsReceipt(row.stage)) list.push({ actor: t("Store"), title: t("Receipt waiting for confirmation"), kind: "pending" });
  return list;
}

export function OrderTimelinePage() {
  const { orderId } = useParams();
  const { t } = useLocale();
  const { tracking, error } = useTracking(orderId);
  const row = tracking;
  const short = row?.receipt ? Math.max(0, row.receipt.expectedUnits - row.receipt.receivedUnits) : 0;
  const proofs = row?.delivery?.proofs || [];
  return (
    <>
      <StoreManagerHero compact crumbs={[{ label: t("Store"), to: "/store-manager" }, { label: t("Orders"), to: "/store-manager/orders" }, { label: t("Order evidence") }]} title={row ? `${row.order.orderRef} · ${row.order.outletId}${row.planning.planRef ? ` · ${row.planning.planRef}` : ""}` : t("Order evidence")} subtitle={t("One shared record of the order: loader, dispatcher, driver and store.")} />
      <div className="sm-page-body dp-stack">
        {error && <p className="dp-note dp-note--red" role="alert">{error}</p>}
        {!row && !error && <p className="muted" role="status">{t("Loading your orders…")}</p>}
        {row && <>
          <div className="dp-row" style={{ justifyContent: "flex-end" }}>{row.receipt ? <span className="dp-tag dp-tag--green">{t("Receipt confirmed")}{row.receipt.confirmedAt ? ` · ${colomboTime(row.receipt.confirmedAt)}` : ""}</span> : <span className="dp-tag dp-tag--amber">{t(statusLabel(row.stage))}</span>}</div>
          <div className="dp-grid-2">
            <section className="dp-panel" aria-label={t("Order timeline")}>
              <ol className="sm-timeline sm-timeline-wrap">
                {events(row, t).map((e, i) => (
                  <li key={i} className={e.kind === "store" ? "is-store" : e.kind === "pending" ? "is-pending" : undefined}>
                    <span className="sm-timeline-time">{e.at ? colomboTime(e.at) : "—"}</span>
                    <p className="sm-timeline-actor">{e.actor}</p>
                    <p className="sm-timeline-title">{e.title}</p>
                    {e.detail && <p className="muted" style={{ margin: 0, fontSize: "0.8125rem" }}>{e.detail}</p>}
                  </li>
                ))}
              </ol>
            </section>
            <aside className="dp-stack">
              <section className="dp-panel" aria-label={t("Order & stop")}>
                <div className="dp-panel-body" style={{ paddingTop: 20 }}>
                  <h2 className="dp-h3">{t("Order & stop")}</h2>
                  <p className="muted" style={{ margin: "2px 0 8px", fontSize: "0.8125rem" }}>{[row.order.orderRef, row.planning.planRef, row.planning.stopSequence ? `${t("Stop")} ${row.planning.stopSequence}` : ""].filter(Boolean).join(" · ")}</p>
                  <dl className="dp-kv-rows">
                    <div><dt>{t("Loader manifest")}</dt><dd>{row.planning.planRef ? <span className="dp-tag dp-tag--cool">{row.planning.planRef}</span> : "—"}</dd></div>
                    <div><dt>{t("Driver proof")}</dt><dd className="dp-cell-sub--green" style={{ color: "#3a57e8" }}>{proofs.length ? `${proofs.length} · ${proofs[0].uploadedAt ? colomboTime(proofs[0].uploadedAt) : t("pending upload")}` : "—"}</dd></div>
                    <div><dt>{t("Store receipt")}</dt><dd className={row.receipt ? "dp-cell-sub--green" : "dp-cell-sub--amber"}>{row.receipt ? `${t("Confirmed")}${row.receipt.confirmedAt ? ` · ${colomboTime(row.receipt.confirmedAt)}` : ""}` : t("Waiting")}</dd></div>
                  </dl>
                  {short > 0 && <p className="dp-note dp-note--amber" style={{ marginTop: 12 }}>{`${short} ${t("unit shortage retained in the shared record.")}`}</p>}
                </div>
              </section>
              <div className="dp-row">
                <Link to={`/store-manager/receipts?order=${encodeURIComponent(row.order.id)}`} className="dp-btn dp-btn--secondary">{t("Review or amend receipt")}</Link>
                <Link to="/store-manager/notifications" className="dp-btn dp-btn--secondary">{t("Notifications")}</Link>
              </div>
            </aside>
          </div>
        </>}
      </div>
    </>
  );
}

// Store-only route view: this store, its truck and the depot. Earlier stops
// are grey and unnamed so other stores stay private. Positions come from the
// stop sequence and driver updates, not GPS.
export function TrackOrderPage() {
  const { orderId } = useParams();
  const { t } = useLocale();
  const { tracking, error, loadedAt } = useTracking(orderId);
  const row = tracking;
  const seq = Math.max(1, row?.planning.stopSequence || 1);
  const earlier = seq - 1;
  const onRoute = row ? /OUT_FOR_DELIVERY|READY_FOR_DEPARTURE/.test(row.stage) : false;
  const delivered = row ? Boolean(row.delivery?.outcome) : false;
  const arrival = row?.planning.plannedArrivalAt ? new Date(row.planning.plannedArrivalAt) : undefined;
  const from = arrival ? new Date(arrival.getTime() - 10 * 60_000) : undefined;
  const to = arrival ? new Date(arrival.getTime() + 10 * 60_000) : undefined;
  const lastUpdate = row?.delivery?.completedAt || row?.delivery?.occurredAt;
  const steps = row ? timelineSteps(row) : [];
  // Route geometry: depot top, earlier stops along the way, store at the end.
  const pts = Array.from({ length: earlier + 1 }, (_, i) => ({ x: 380 - i * 40 + (i % 2 ? 60 : -40), y: 90 + ((i + 1) * 320) / (earlier + 1) }));
  const store = { x: 330, y: 420 };
  const truckAt = delivered ? store : onRoute ? { x: (pts[Math.max(0, earlier - 1)]?.x ?? 380) * 0.4 + store.x * 0.6, y: (pts[Math.max(0, earlier - 1)]?.y ?? 90) * 0.4 + store.y * 0.6 } : { x: 380, y: 90 };
  return (
    <>
      <StoreManagerHero compact crumbs={[{ label: t("Store"), to: "/store-manager" }, { label: t("Track order") }]} title={row ? `${t("Track order")} · ${row.order.orderRef}` : t("Track order")} subtitle={row ? `${row.order.temperatureRequirement === "chilled" ? t("Chilled") : t("Ambient")} · ${row.order.orderUnits} ${t("units")} · ${t("from driver app updates")}` : undefined}>
        <Link to="/store-manager/orders/new" className="sm-btn-place-order">+ {t("Place Order")}</Link>
      </StoreManagerHero>
      <div className="sm-page-body">
        {error && <p className="dp-note dp-note--red" role="alert">{error}</p>}
        {!row && !error && <p className="muted" role="status">{t("Loading your orders…")}</p>}
        {row && <div className="dp-grid-2">
          <div className="sm-track-map">
            <svg viewBox="0 0 700 520" role="img" aria-label={`${t("Route from the depot to your store")} · ${earlier} ${t("earlier stops")}`}>
              <rect width="700" height="520" fill="#eef0ee" />
              {[60, 160, 260, 360, 460].map((y) => <line key={y} x1="0" y1={y} x2="700" y2={y + 30} stroke="#fff" strokeWidth="6" />)}
              {[120, 280, 440, 600].map((x) => <line key={x} x1={x} y1="0" x2={x - 40} y2="520" stroke="#fff" strokeWidth="6" />)}
              <rect x="0" y="0" width="60" height="520" fill="#d6e4f0" />
              <polyline points={[{ x: 380, y: 90 }, ...pts.slice(1), store].map((p) => `${p.x},${p.y}`).join(" ")} fill="none" stroke="#9db3ee" strokeWidth="4" />
              {pts.slice(1).map((p, i) => <circle key={i} cx={p.x} cy={p.y} r="9" fill="#8a92a6" stroke="#fff" strokeWidth="3"><title>{t("Earlier stop (not named)")}</title></circle>)}
              <circle cx="380" cy="90" r="14" fill="#232d42" /><text x="380" y="95" fontSize="13" fontWeight="700" fill="#fff" textAnchor="middle">D</text>
              <text x="400" y="95" fontSize="12" fontWeight="600" fill="#232d42">{t("Depot")}</text>
              <circle cx={store.x} cy={store.y} r="16" fill="#c03221" /><text x={store.x} y={store.y + 5} fontSize="14" fill="#fff" textAnchor="middle">★</text>
              <rect x={store.x - 150} y={store.y - 44} width="190" height="22" rx="4" fill="#fff" /><text x={store.x - 142} y={store.y - 29} fontSize="12" fontWeight="600" fill="#232d42">{`${t("Your store")} · ${row.order.outletId}`}</text>
              {!delivered && <line x1={truckAt.x} y1={truckAt.y} x2={store.x} y2={store.y} stroke="#3a57e8" strokeWidth="3" strokeDasharray="4 6" />}
              <circle cx={truckAt.x} cy={truckAt.y} r="26" fill="#3a57e8" opacity="0.18" />
              <circle cx={truckAt.x} cy={truckAt.y} r="13" fill="#3a57e8" stroke="#fff" strokeWidth="3" />
              <rect x={truckAt.x + 18} y={truckAt.y - 11} width="120" height="22" rx="4" fill="#3a57e8" /><text x={truckAt.x + 26} y={truckAt.y + 4} fontSize="12" fontWeight="600" fill="#fff">{`${row.delivery?.vehicleId || t("Your truck")}${lastUpdate ? ` · ${colomboTime(lastUpdate)}` : ""}`}</text>
            </svg>
            <span className="sm-track-live">● {lastUpdate ? `${t("Last driver update")} ${colomboTime(lastUpdate)}` : `${t("Refreshed")} ${colomboTime(loadedAt)}`}</span>
            <div className="sm-track-legend">
              <p style={{ margin: 0 }}><span style={{ color: "#3a57e8" }}>●</span> {t("Your delivery truck")}</p>
              <p style={{ margin: 0 }}><span style={{ color: "#c03221" }}>●</span> {t("Your store")}</p>
              <p style={{ margin: 0 }}><span style={{ color: "#8a92a6" }}>●</span> {t("Earlier stops (other stores, not named)")}</p>
              <p style={{ margin: 0 }}><span style={{ color: "#232d42" }}>●</span> {t("Depot")}</p>
            </div>
          </div>
          <aside className="dp-panel">
            <div className="dp-panel-body dp-stack" style={{ paddingTop: 20 }}>
              <p className="dp-section-label">{t("Expected arrival")}</p>
              <p className="dp-stat-value" style={{ fontSize: "2rem" }}>{from && to ? `${colomboTime(from)}–${colomboTime(to)}` : isDeferred(row.stage) ? t("Moved to a later run") : t("Not yet scheduled")}</p>
              <div className="dp-row"><span className={`dp-tag ${delivered ? "dp-tag--green" : isDeferred(row.stage) ? "dp-tag--red" : "dp-tag--primary"}`}>{t(statusLabel(row.stage))}</span></div>
              {seq > 0 && !delivered && <p style={{ margin: 0, fontSize: "0.875rem" }}>{earlier === 0 ? t("Your store is the first stop.") : `${t("Your store is stop")} ${seq}. ${earlier} ${t("earlier stop(s) are not shown by name.")}`}</p>}
              <h3 className="dp-h3">{t("Progress")}</h3>
              <ul className="dp-checks">
                {steps.map((s) => <li key={s.key}><span className={`dp-check${s.state === "done" ? "" : s.state === "current" ? " dp-check--warn" : " dp-check--todo"}`} aria-hidden="true">{s.state === "done" ? "✓" : ""}</span><span><strong>{t(s.label)}</strong><span className="dp-cell-sub">{s.detail}</span></span></li>)}
              </ul>
              <h3 className="dp-h3">{t("Delivery")}</h3>
              <dl className="dp-kv-rows">
                <div><dt>{t("Vehicle")}</dt><dd>{row.delivery?.vehicleId || "—"}</dd></div>
                <div><dt>{t("On this order")}</dt><dd>{row.order.orderUnits} {t("units")}</dd></div>
                <div><dt>{t("Plan")}</dt><dd>{row.planning.planRef || "—"}</dd></div>
              </dl>
              <p className="dp-note" style={{ margin: 0, fontSize: "0.8125rem" }}>{t("The truck's place is shown from driver updates, not GPS. If the driver loses signal, the last update time stays visible so an old position is never mistaken for a live one.")}</p>
              <Link to={`/store-manager/orders/${encodeURIComponent(row.order.id)}/timeline`} className="dp-btn dp-btn--block">{t("View order details")}</Link>
              <Link to="/store-manager" className="dp-btn dp-btn--secondary dp-btn--block">{t("Back to overview")}</Link>
            </div>
          </aside>
        </div>}
      </div>
    </>
  );
}
