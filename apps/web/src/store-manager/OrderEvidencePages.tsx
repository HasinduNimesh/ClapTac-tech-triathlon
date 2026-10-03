import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { apiJSON } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { deferralExplanation } from "./deferralMessage.mjs";
import { colomboTime, formatDay, isDeferred, needsReceipt, statusLabel, timelineSteps } from "./orderStage.mjs";
import { StoreManagerHero } from "./StoreManagerHero";
import { Tracking } from "./useOrderTrackings";
import { DEPOT_LOCATIONS, LatLng, MapLine, MapMarker, WaypointMap, along } from "../components/WaypointMap";

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

function useOutlet(outletId?: string) {
  const { user } = useAuth();
  const token = user?.access_token || "";
  const [outlet, setOutlet] = useState<{ latitude?: number; longitude?: number } | null>(null);
  useEffect(() => {
    if (!token || !outletId) return;
    apiJSON<{ outlet: { latitude?: number; longitude?: number } }>(`/shared/outlets/${encodeURIComponent(outletId)}`, token).then((r) => setOutlet(r.outlet)).catch(() => setOutlet(null));
  }, [token, outletId]);
  return outlet;
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
  const outlet = useOutlet(row?.order.outletId);
  const seq = Math.max(1, row?.planning.stopSequence || 1);
  const earlier = seq - 1;
  const onRoute = row ? /OUT_FOR_DELIVERY|READY_FOR_DEPARTURE/.test(row.stage) : false;
  const delivered = row ? Boolean(row.delivery?.outcome) : false;
  const arrival = row?.planning.plannedArrivalAt ? new Date(row.planning.plannedArrivalAt) : undefined;
  const from = arrival ? new Date(arrival.getTime() - 10 * 60_000) : undefined;
  const to = arrival ? new Date(arrival.getTime() + 10 * 60_000) : undefined;
  const lastUpdate = row?.delivery?.completedAt || row?.delivery?.occurredAt;
  const steps = row ? timelineSteps(row) : [];
  // Only this store and the depot are shown at their (approximate) places.
  // Earlier stops are spread along the way unnamed so other stores stay private.
  const depot = DEPOT_LOCATIONS[row?.planning.depot || ""] || DEPOT_LOCATIONS.DEPOT_NORTH;
  const storeAt: LatLng | undefined = outlet?.latitude != null && outlet.longitude != null ? [outlet.latitude, outlet.longitude] : undefined;
  const store = storeAt || along(depot, [depot[0] - 0.05, depot[1] + 0.05], 1);
  const earlierAt = Array.from({ length: earlier }, (_, i) => { const p = along(depot, store, (i + 1) / (earlier + 1)); return [p[0] + (i % 2 ? 0.01 : -0.01), p[1] + (i % 2 ? -0.012 : 0.012)] as LatLng; });
  const lastEarlier = earlierAt[earlierAt.length - 1] || depot;
  const truck = delivered ? store : onRoute ? along(lastEarlier, store, 0.6) : depot;
  const mapLines: MapLine[] = [{ id: "route", points: [depot, ...earlierAt, store], color: "#9db3ee" }, ...(!delivered ? [{ id: "next", points: [truck, store], color: "#3a57e8", dashed: true }] : [])];
  const mapMarkers: MapMarker[] = [
    { id: "depot", at: depot, kind: "depot", color: "#232d42", label: t("Depot"), title: t("Depot") },
    ...earlierAt.map((p, i) => ({ id: `e${i}`, at: p, kind: "stop" as const, color: "#8a92a6", title: t("Earlier stop (not named)") })),
    { id: "store", at: store, kind: "store", color: "#c03221", label: `${t("Your store")} · ${row?.order.outletId || ""}`, title: t("Your store") },
    { id: "truck", at: truck, kind: "truck", color: "#3a57e8", selected: true, label: `${row?.delivery?.vehicleId || t("Your truck")}${lastUpdate ? ` · ${colomboTime(lastUpdate)}` : ""}`, title: t("Your delivery truck") },
  ];
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
            <WaypointMap label={`${t("Route from the depot to your store")} · ${earlier} ${t("earlier stops")}`} markers={mapMarkers} lines={mapLines} fitKey={row.order.id} />
            <span className="sm-track-live" style={{ zIndex: 500 }}>● {lastUpdate ? `${t("Last driver update")} ${colomboTime(lastUpdate)}` : `${t("Refreshed")} ${colomboTime(loadedAt)}`}</span>
            <div className="sm-track-legend" style={{ zIndex: 500 }}>
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
              <p className="dp-note" style={{ margin: 0, fontSize: "0.8125rem" }}>{t("The truck's place is shown from driver updates, not GPS. If the driver loses signal, the last update time stays visible so an old position is never mistaken for a live one.")} {t("Map positions are approximate.")}</p>
              <Link to={`/store-manager/orders/${encodeURIComponent(row.order.id)}/timeline`} className="dp-btn dp-btn--block">{t("View order details")}</Link>
              <Link to="/store-manager" className="dp-btn dp-btn--secondary dp-btn--block">{t("Back to overview")}</Link>
            </div>
          </aside>
        </div>}
      </div>
    </>
  );
}
