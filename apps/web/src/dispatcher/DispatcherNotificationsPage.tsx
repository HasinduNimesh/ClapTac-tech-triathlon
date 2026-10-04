import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiFetch, apiJSON } from "../api/client";
import { todayInSriLanka } from "../api/date.mjs";
import { DockAlert, LoadingIssue, LoadingTripDetail, LoadingTripSummary } from "../api/loading";
import { PlanDetail } from "../api/planning";
import { useLocale } from "../i18n";
import { AuditEvent, Availability, Incident, ReceiptIssue } from "./types";
import { DpHero, Drawer, Note, Panel, Tag, Toast } from "./ui";
import { failedSourceCount } from "./sourceFailures.mjs";
import { dateTime, errorText, useApi, useToken } from "./useApi";

type Alert = { key: string; tone: "red" | "amber" | "cool" | "primary"; tag: string; title: string; text: string; action: string; onOpen?: () => void; to?: string };
type Decision = "partial" | "hold" | "move";

export function DispatcherNotificationsPage() {
  const { t } = useLocale();
  const token = useToken();
  const date = todayInSriLanka();
  const [exceptionTrip, setExceptionTrip] = useState<LoadingTripSummary | null>(null);
  const [conflictsOpen, setConflictsOpen] = useState(false);
  const [toast, setToast] = useState("");
  // 404 here only means no plan has been built for the day yet, which is an empty state, not an error.
  const plan = useApi<PlanDetail>(`/planning/plans?date=${date}`);
  const incidents = useApi<{ items: Incident[] }>("/fleet/incidents?openOnly=true");
  const availability = useApi<{ items: Availability[] }>(`/fleet/availability?date=${date}`);
  const loading = useApi<{ items: LoadingTripSummary[] }>(`/loading/trips?date=${date}`);
  const receipts = useApi<{ items: ReceiptIssue[] }>("/orders/receipt-issues");
  const conflicts = useApi<{ items: AuditEvent[] }>("/shared/audit/events?action=DELIVERY_SYNC_CONFLICT&limit=20");
  const temperature = useApi<{ items: AuditEvent[] }>("/shared/audit/events?action=DELIVERY_TEMPERATURE_EXCEPTION&limit=20");
  const dockAlerts = useApi<{ items: DockAlert[] }>(`/loading/alerts?date=${date}`);

  async function resolveAlert(alert: DockAlert) {
    try {
      await apiJSON(`/loading/alerts/${alert.id}/resolve`, token, { method: "POST" });
      setToast(t("Wrong-vehicle alert closed"));
      void dockAlerts.reload();
    } catch (e) { setToast(errorText(e)); }
  }

  const planVehicles = new Set((plan.data?.trips || []).map((trip) => trip.vehicleId));
  const staleVehicles = (availability.data?.items || []).filter((a) => a.status !== "available" && planVehicles.has(a.vehicleId));
  const alerts: Alert[] = [];
  for (const incident of incidents.data?.items || []) alerts.push({ key: incident.id, tone: "red", tag: t("Critical · chilled risk"), title: `${incident.vehicleId}${incident.tripId ? ` · ${incident.tripId}` : ""} · ${t(incident.type)}`, text: `${incident.description} · ${dateTime(incident.reportedAt)}`, action: t("Reassign stops or defer with a reason"), to: "/dispatcher/live" });
  for (const ev of temperature.data?.items || []) alerts.push({ key: ev.event_id, tone: "red", tag: t("Critical · temperature"), title: `${ev.resource_type} ${ev.resource_id}`, text: `${ev.reason || t("Out-of-range temperature recorded")} · ${dateTime(ev.timestamp)}`, action: t("Open live operations"), to: "/dispatcher/live" });
  const isChilled = (x: LoadingTripSummary) => x.vehicleTemperatureCapability === "reefer";
  const shortTrips = (loading.data?.items || []).filter((x) => (x.shortfallCount || 0) > 0).sort((x, y) => Number(isChilled(y)) - Number(isChilled(x)) || (y.shortfallCount || 0) - (x.shortfallCount || 0));
  for (const trip of shortTrips) alerts.push({ key: `short-${trip.tripId}`, tone: isChilled(trip) ? "red" : "amber", tag: isChilled(trip) ? t("Critical · chilled load exception") : t("High · loader shortfall"), title: `${trip.vehicleId} · ${t("Trip")} ${trip.tripNumber ?? 1} · ${trip.shortfallCount} ${t("order(s) short")}`, text: t("Shortfall needs dispatcher acceptance or a new plan before departure"), action: t("Review the load exception"), onOpen: () => setExceptionTrip(trip) });
  for (const alert of (dockAlerts.data?.items || []).filter((a) => !a.resolvedAt)) {
    const at = (loading.data?.items || []).find((x) => x.tripId === alert.tripId);
    alerts.push({ key: `dock-${alert.id}`, tone: "amber", tag: t("High · wrong vehicle"), title: `${alert.orderRef} ${t("found at")} ${at?.vehicleId || alert.tripId}${alert.belongsVehicleId ? ` · ${t("belongs on")} ${alert.belongsVehicleId}` : ""}`, text: `${t("Reported by the loader")} ${alert.reportedBy} · ${dateTime(alert.createdAt)}${alert.note ? ` · ${alert.note}` : ""}`, action: t("Mark handled"), onOpen: () => void resolveAlert(alert) });
  }
  const repeat = (plan.data?.orders || []).filter((o) => (o.outletDeferralCount || 0) >= 2 && (plan.data!.unallocated || []).some((u) => u.orderId === o.id));
  for (const order of repeat) alerts.push({ key: `rep-${order.id}`, tone: "cool", tag: t("Repeat deferral"), title: `${order.orderRef} · ${order.outletId} · ${t("deferred")} ${order.outletDeferralCount} ${t("times")}`, text: `${order.lastServedAt ? `${t("Last served")} ${dateTime(order.lastServedAt)} · ` : ""}${t("fairness review required before another deferral")}`, action: t("Review priority and next-run plan"), to: "/dispatcher/planning" });
  if ((conflicts.data?.items?.length || 0) + (receipts.data?.items?.length || 0) > 0) alerts.push({ key: "loop", tone: "primary", tag: t("After plan change"), title: `${conflicts.data?.items?.length || 0} ${t("sync conflict(s)")} · ${receipts.data?.items?.length || 0} ${t("store receipt issue(s)")}`, text: t("Who has the new plan, what came back from the road, and what the store confirmed."), action: t("Review acknowledgements and receipts"), onOpen: () => setConflictsOpen(true) });

  return (
    <>
      <DpHero title={t("Notifications")} />
      <div className="dp-body dp-body--flush">
        {staleVehicles.length > 0 && plan.data && (
          <div className="dp-banner dp-banner--amber" role="status">
            <Tag tone="amber">{t("Revalidation required")}</Tag>
            <div className="dp-spacer"><p className="dp-banner-title" style={{ color: "var(--dp-ink)", fontSize: "1.0625rem" }}>{t("Fleet availability changed after")} {plan.data.plan.planRef} {t("was built")}</p><p className="dp-banner-text">{staleVehicles.map((v) => `${v.vehicleId} · ${t(v.status)}${v.reason ? ` (${v.reason})` : ""}`).join(" · ")} · {t("current plan is now stale")}</p></div>
            <Link to="/dispatcher/planning" className="dp-btn">{t("Review affected trips")}</Link>
          </div>
        )}
        <Panel title={t("Needs action")} actions={<Tag tone="red">{alerts.length} {t("open")}</Tag>}>
          {alerts.length === 0 ? <p className="muted" style={{ margin: 0 }}>{t("No notifications need action right now.")}</p> : (
            <div className="dp-stack">
              {alerts.map((alert) => (
                <article key={alert.key} className="dp-subcard dp-row" style={{ alignItems: "flex-start" }}>
                  <div style={{ width: 230 }}><Tag tone={alert.tone}>{alert.tag}</Tag></div>
                  <div className="dp-spacer">
                    <h2 className="dp-list-title">{alert.title}</h2>
                    <p className="dp-list-text">{alert.text}</p>
                    {alert.to ? <Link to={alert.to} className="dp-link">{alert.action}</Link> : <button type="button" className="dp-link" onClick={alert.onOpen}>{alert.action}</button>}
                  </div>
                </article>
              ))}
            </div>
          )}
        </Panel>
        {plan.data && <div className="dp-grid-2 dp-grid-2--even">
          <Panel title={t("What changed")} sub={t("Plan health: what changed after the plan was built, and which trips need a second look.")}>
            {staleVehicles.length === 0 && (incidents.data?.items || []).length === 0 ? <p className="muted" style={{ margin: 0 }}>{t("Nothing has changed since the plan was built.")}</p> : (
              <dl className="dp-kv-rows">
                {staleVehicles.map((v) => <div key={v.vehicleId}><dt>{v.vehicleId} · {t("status")}</dt><dd><Tag tone="red">{t(v.status)}</Tag></dd></div>)}
                {(incidents.data?.items || []).map((i) => <div key={i.id}><dt>{i.vehicleId} · {t(i.type)}</dt><dd>{i.date}</dd></div>)}
              </dl>
            )}
            <Note>{t("Plan remains unlocked until every order is assigned or deferred and all hard rules pass.")}</Note>
          </Panel>
          <Panel title={t("Planning assumptions")}>
            <p style={{ margin: 0, fontSize: "0.875rem" }}>{t("Forecast ranges guide planning and never make a hard allocation.")}</p>
            <p className="muted" style={{ fontSize: "0.875rem" }}>{t("If a model is unavailable, historical averages are used and the fallback is marked.")}</p>
            <Link to="/dispatcher/forecast" className="dp-link">{t("Open demand forecast")} →</Link>
          </Panel>
        </div>}
        {failedSourceCount([{ ...plan, missingIsEmpty: true }, loading, incidents]) > 0 && <p className="dp-note dp-note--amber" role="status">{t("Some notification sources could not be loaded. The list may be incomplete.")}</p>}
      </div>
      <LoadExceptionDrawer trip={exceptionTrip} plan={plan.data} token={token} onClose={() => setExceptionTrip(null)} onDone={(m) => { setExceptionTrip(null); setToast(m); void plan.reload(); void loading.reload(); }} />
      <Drawer open={conflictsOpen} onClose={() => setConflictsOpen(false)} title={plan.data ? `${t("After plan")} v${plan.data.publication?.version || plan.data.plan.currentVersion || 1} · ${plan.data.plan.planRef}` : t("After plan change")} sub={t("Who has the new plan, what came back from the road, and what the store confirmed.")}
        footer={<button type="button" className="dp-btn dp-btn--secondary" onClick={() => setConflictsOpen(false)}>{t("Back to alerts")}</button>}>
        <p className="dp-section-label">{t("Plan acknowledgement")} · {plan.data?.publication ? `v${plan.data.publication.version} ${t("published")} ${dateTime(plan.data.publication.publishedAt)}` : t("Not published yet")}</p>
        {(plan.data?.publication?.acknowledgements || []).length === 0 ? <p className="muted" style={{ margin: 0 }}>{t("No field acknowledgements yet.")}</p> : <dl className="dp-kv-rows">{plan.data!.publication!.acknowledgements.map((a) => <div key={`${a.actorId}-${a.acknowledgedAt}`}><dt>{t(a.actorRole)} · {a.actorId}</dt><dd className="dp-cell-sub--green">{t("Acknowledged")} {dateTime(a.acknowledgedAt)}</dd></div>)}</dl>}
        <p className="dp-section-label">{t("Sync conflicts")} ({conflicts.data?.items?.length || 0})</p>
        {(conflicts.data?.items || []).map((ev) => <Note key={ev.event_id} tone="amber" title={`${ev.resource_type} ${ev.resource_id} · ${dateTime(ev.timestamp)}`}>{ev.reason || t("The driver's offline record clashed with a newer plan version. Both records are kept.")}</Note>)}
        {(conflicts.data?.items || []).length === 0 && <p className="muted" style={{ margin: 0 }}>{t("No sync conflicts recorded.")}</p>}
        <p className="dp-section-label">{t("Store receipts")} ({receipts.data?.items?.length || 0})</p>
        {(receipts.data?.items || []).length === 0 ? <p className="muted" style={{ margin: 0 }}>{t("No receipt issues reported.")}</p> : <dl className="dp-kv-rows">{receipts.data!.items.map((item, i) => <div key={`${item.orderRef}-${i}`}><dt>{item.outletId} · {item.orderRef} · {t(item.issue.issueType)}{item.issue.note ? ` · ${item.issue.note}` : ""}</dt><dd className="dp-cell-sub--amber">{item.receipt.receivedUnits} {t("of")} {item.receipt.expectedUnits} · {item.issue.affectedUnits} {t("short")}</dd></div>)}</dl>}
        <Note>{t("Store shortages are compared with the loader's shortfall report and the driver's record. Both parties' records are retained if they disagree.")}</Note>
      </Drawer>
      {toast && <Toast onClose={() => setToast("")}>✓ {toast}</Toast>}
    </>
  );
}

function LoadExceptionDrawer({ trip, plan, token, onClose, onDone }: { trip: LoadingTripSummary | null; plan: PlanDetail | null; token: string; onClose: () => void; onDone: (message: string) => void }) {
  const { t } = useLocale();
  const detail = useApi<LoadingTripDetail>(trip ? `/loading/trips/${trip.tripId}` : null);
  const [decision, setDecision] = useState<Decision>("partial");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [note, setNote] = useState("");
  const short = (detail.data?.orders || []).filter((o) => (o.issues || []).length > 0);
  const tripId = trip?.tripId;

  // Opening the exception tells the loader their report has been seen.
  useEffect(() => {
    if (!tripId) return;
    for (const order of short) {
      for (const issue of order.issues || []) {
        if (!issue.seenAt && !issue.decision) void apiJSON(`/loading/trips/${tripId}/orders/${order.orderId}/issues/${issue.id}/seen`, token, { method: "POST" }).catch(() => undefined);
      }
    }
    // Only when the trip detail arrives, not on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tripId, detail.data]);

  async function publish() {
    if (!trip) return;
    setBusy(true); setError("");
    const code = decision === "partial" ? "PARTIAL_LOAD" : decision === "hold" ? "HOLD" : "MOVE_TO_NEXT_RUN";
    const recordDecisions = async () => {
      for (const order of short) {
        for (const issue of order.issues || []) {
          await apiJSON(`/loading/trips/${trip.tripId}/orders/${order.orderId}/issues/${issue.id}/decision`, token, { method: "POST", body: JSON.stringify({ decision: code, note }) });
        }
      }
    };
    let planLeftOpen = false;
    try {
      const summary = short.map((o) => `${o.orderRef || o.orderId}: ${(o.issues || []).map((i) => `${i.type} ${i.affectedUnits}`).join(", ")}`).join("; ");
      if (decision === "move") {
        if (!plan) throw new Error(t("No plan is loaded for this date."));
        // Planning goes first and the decision is recorded last, so a failed planning call
        // leaves the shortfall undecided and the trip blocked. The loading service also
        // refuses Ready until the confirmed plan no longer carries the line and the
        // loader has acknowledged that version, whatever order these calls ran in.
        planLeftOpen = true;
        if (plan.plan.status === "confirmed") await apiJSON(`/planning/plans/${plan.plan.id}/revise`, token, { method: "POST" });
        for (const order of short) {
          const allocation = (plan.allocations || []).find((a) => a.tripId === trip.tripId && a.orderId === order.orderId);
          if (allocation) await apiJSON(`/planning/plans/${plan.plan.id}/allocations/${allocation.id}`, token, { method: "DELETE" });
          await apiJSON(`/planning/plans/${plan.plan.id}/deferrals`, token, { method: "POST", body: JSON.stringify({ orderId: order.orderId, reasonCode: "MANUAL_DISPATCHER_DEFERRAL", comment: `Loader shortfall before departure: ${summary}${note ? ` · ${note}` : ""}` }) });
        }
        await apiJSON(`/planning/plans/${plan.plan.id}/confirm`, token, { method: "POST" });
        planLeftOpen = false;
        await recordDecisions();
        onDone(t("New plan version published · the loader must acknowledge it before the trip can depart"));
      } else {
        await recordDecisions();
        const body = decision === "partial" ? `Dispatcher decision: accept partial load and depart on time. ${summary}.` : `Dispatcher decision: hold the trip until replacement stock arrives. ${summary}.`;
        try { await apiJSON(`/delivery/trips/${trip.tripId}/messages`, token, { method: "POST", body: JSON.stringify({ body: note ? `${body} ${note}` : body }) }); } catch { /* the decision is already recorded on the shortfall */ }
        onDone(decision === "partial" ? t("Partial load accepted · driver and loader notified") : t("Trip on hold · driver and loader notified"));
      }
    } catch (e) { setError(`${errorText(e)}${planLeftOpen ? ` ${t("The plan was left open for revision. Finish it in Plan and allocate; the loader's trip stays blocked until a new plan is confirmed.")}` : ""}`); }
    finally { setBusy(false); }
  }

  const options: { value: Decision; title: string; text: string }[] = [
    { value: "partial", title: t("Accept a partial load"), text: t("Trip leaves on time. The store is told the missing units come on the next run.") },
    { value: "hold", title: t("Hold the trip until stock arrives"), text: t("Trip may miss the receiving windows of later stops.") },
    { value: "move", title: t("Move the whole line to the next run"), text: t("Publishes a new plan version and defers the affected orders with a reason.") },
  ];
  return (
    <Drawer open={Boolean(trip)} onClose={onClose} narrow title={trip ? `${t("Load exception")} · ${trip.vehicleId} ${t("Trip")} ${trip.tripNumber ?? 1}` : ""} sub={trip ? `${trip.planRef || ""} · ${trip.shortfallCount} ${t("order(s) short before departure")}` : undefined}
      footer={<><button type="button" className="dp-btn dp-btn--secondary" onClick={onClose}>{t("Cancel")}</button><button type="button" className="dp-btn" disabled={busy || short.length === 0} onClick={() => void publish()}>{decision === "move" ? t("Publish new plan version and notify") : t("Send decision and notify")}</button></>}>
      {error && <p className="dp-note dp-note--red" role="alert">{error}</p>}
      {detail.loading && <p role="status">{t("Loading trip…")}</p>}
      {short.map((o) => (
        <Note key={o.orderId} tone={(o.issues || []).every((i) => i.decision === "PARTIAL_LOAD" || i.decision === "MOVE_TO_NEXT_RUN") ? "green" : "red"}>
          {o.orderRef || o.orderId} · {t("Stop")} {o.stopSequence}: {(o.issues || []).map((i) => `${t(i.type)} ${i.affectedUnits}${o.expectedUnits ? ` ${t("of")} ${o.expectedUnits}` : ""}${i.note ? ` · ${i.note}` : ""}${i.reportedAt ? ` · ${dateTime(i.reportedAt)}` : ""}${i.decision ? ` · ${t("Decided")}: ${t(i.decision)}${i.decidedBy ? ` (${i.decidedBy})` : ""}` : ""}`).join(", ")}
          {trip && (o.issues || []).filter((i) => i.hasPhoto).map((i) => <IssuePhoto key={i.id} path={`/loading/trips/${trip.tripId}/orders/${o.orderId}/issues/${i.id}/photo`} token={token} issue={i} />)}
        </Note>
      ))}
      <fieldset className="dp-stack" style={{ border: 0, padding: 0, margin: 0 }}>
        <legend className="dp-section-label">{t("Choose what happens")}</legend>
        {options.map((o) => (
          <label key={o.value} className="dp-option">
            <input type="radio" name="exception-decision" value={o.value} checked={decision === o.value} onChange={() => setDecision(o.value)} />
            <div><p className="dp-option-title">{o.title}</p><p className="dp-option-text">{o.text}</p></div>
          </label>
        ))}
      </fieldset>
      <label className="dp-field">{t("Note for the loader and driver (optional)")}<textarea value={note} onChange={(e) => setNote(e.target.value)} maxLength={1000} /></label>
      <p className="dp-section-label">{t("What changes")}</p>
      <dl className="dp-kv-rows">
        <div><dt>{t("Plan version")}</dt><dd>{decision === "move" ? `v${plan?.publication?.version || plan?.plan.currentVersion || 1} → v${(plan?.publication?.version || plan?.plan.currentVersion || 1) + 1}` : t("Unchanged")}</dd></div>
        <div><dt>{t("Loader")}</dt><dd>{decision === "hold" ? t("Ready to depart stays locked") : decision === "move" ? t("Acknowledges the new plan version, then Ready to depart unlocks") : t("Ready to depart unlocks")}</dd></div>
        <div><dt>{t("Driver")}</dt><dd>{decision === "move" ? t("Gets the new version on the phone") : t("Gets the decision as a trip message")}</dd></div>
        <div><dt>{t("Logged as")}</dt><dd>{t("Decided by you, with time")}</dd></div>
      </dl>
    </Drawer>
  );
}

/** The loader's photo of missing or damaged goods, fetched with the dispatcher's token. */
function IssuePhoto({ path, token, issue }: { path: string; token: string; issue: LoadingIssue }) {
  const { t } = useLocale();
  const [url, setUrl] = useState("");
  const [failed, setFailed] = useState(false);
  useEffect(() => () => { if (url) URL.revokeObjectURL(url); }, [url]);
  async function show() {
    setFailed(false);
    try {
      const res = await apiFetch(path, token);
      if (!res.ok) throw new Error(String(res.status));
      setUrl(URL.createObjectURL(await res.blob()));
    } catch { setFailed(true); }
  }
  if (url) return <span style={{ display: "block", marginTop: 8 }}><img src={url} alt={`${t("Photo of the reported goods")} · ${t(issue.type)}`} style={{ maxWidth: "100%", maxHeight: 240, borderRadius: 8, display: "block" }} /><button type="button" className="dp-link" onClick={() => setUrl("")}>{t("Hide photo")}</button></span>;
  return <span style={{ display: "block", marginTop: 6 }}><button type="button" className="dp-link" onClick={() => void show()}>{t("View photo")}</button>{failed && <span className="dp-cell-sub--amber"> · {t("Loader photo could not be loaded.")}</span>}</span>;
}
