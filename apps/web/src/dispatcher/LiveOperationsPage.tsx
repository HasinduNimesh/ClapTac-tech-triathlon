import { FormEvent, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { apiJSON } from "../api/client";
import { todayInSriLanka } from "../api/date.mjs";
import { DeliveryStop, DeliveryTripDetail, DeliveryTripSummary, LatenessProbability, LiveLocation } from "../api/delivery";
import { DEPOT_LABELS, LoadingTripSummary, sameDepot } from "../api/loading";
import { PlanDetail } from "../api/planning";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { ARRIVAL_ESTIMATE_VERSION, estimateArrival, previousReportedStop } from "./arrivalEstimate.mjs";
import { calibratedArrivalRange } from "./arrivalRange.mjs";
import { evaluatedLatenessCalibrations } from "./latenessCalibration.mjs";
import { singleFlightMessagePost } from "./singleFlightMessagePost.mjs";
import { enrichTripWithWatch, generateNeedsActionAlerts } from "./tripWatch.mjs";
import { FRESH_TRIP_BUDGET_MINUTES } from "./planModel";
import { useDepot } from "./DispatcherLayout";
import { Incident, Outlet, outletMap } from "./types";
import { DEPOT_LOCATIONS, LatLng, MapLine, MapMarker, WaypointMap } from "../components/WaypointMap";
import { LiveLocationMap } from "../components/LiveLocationMap";
import { ESTIMATES_UNAVAILABLE_MESSAGE, validArrivalAt } from "../api/estimateAvailability.mjs";
import { Check, ChipGroup, DpHero, Drawer, Note, Panel, Stat, StatRow, Tag, Toast } from "./ui";
import { clock, dateTime, errorText, isChilled, minutesAgo, useApi, useToken } from "./useApi";

type TripMessage = { id: string; tripId: string; stopId?: string; body: string; sentBy: string; createdAt: string; acknowledgedBy?: string; acknowledgedAt?: string };
type BreakdownProposal = { planId: string; vehicleId: string; items: { tripNumber: number; stops: { allocationId: string; orderId: string; orderRef: string; outletId: string; stopSequence: number; urgencyRank?: number; chilled?: boolean; windowClose?: string }[]; options: { vehicleId: string; valid: boolean; projectedStops: number; failures: { reasonCode: string }[] }[] }[]; confirmed: boolean; critical?: boolean; severity?: string };
type RowState = "broken" | "late" | "silent" | "done" | "ok" | "waiting";
type Row = { summary: DeliveryTripSummary; detail?: DeliveryTripDetail; state: RowState; next?: DeliveryStop; nextEta?: ReturnType<typeof estimateArrival>; lastUpdate?: string; chilled: boolean; incident?: Incident; watch?: ReturnType<typeof enrichTripWithWatch> };
type Severity = "critical" | "high" | "medium" | "low";
type ActionItem = { key: string; severity: Severity; title: string; text: string; actions: { label: string; onClick?: () => void; to?: string; primary?: boolean }[] };

const fallbackServiceTime = { minutes: 20, version: "fixed_20m_v1" };
const postTripMessageOnce = singleFlightMessagePost(async ({ token, tripId, body, stopId }) =>
  apiJSON(`/delivery/trips/${tripId}/messages`, token, { method: "POST", body: JSON.stringify({ body, stopId: stopId || undefined }) }),
);

function lastEvent(stops: DeliveryStop[]) {
  return stops.flatMap((s) => [s.outcomeReceivedAt, s.outcomeAt, s.arrivedReceivedAt, s.arrivedAt]).filter(Boolean).sort().slice(-1)[0] as string | undefined;
}

export function LiveOperationsPage() {
  const { t } = useLocale();
  const token = useToken();
  const { depot } = useDepot();
  const [date, setDate] = useState(todayInSriLanka);
  const [view, setView] = useState<"list" | "map">("list");
  const [filter, setFilter] = useState<"all" | "risk" | "silent" | "chilled">("all");
  const [rows, setRows] = useState<Row[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [now, setNow] = useState(Date.now);
  const [openTrip, setOpenTrip] = useState("");
  const [recovery, setRecovery] = useState<Incident | null>(null);
  const [acknowledged, setAcknowledged] = useState<string[]>([]);
  const [toast, setToast] = useState("");
  const plan = useApi<PlanDetail>(`/planning/plans?date=${date}`);
  const incidents = useApi<{ items: Incident[] }>("/fleet/incidents?openOnly=true");
  const outlets = useApi<{ items: Outlet[] }>("/shared/outlets");
  const loading = useApi<{ items: LoadingTripSummary[] }>(`/loading/trips?date=${date}`);
  const forecast = useApi<{ forecast?: { serviceMinutesPerStop?: number; serviceEstimateVersion?: string } }>("/orders/forecast");
  const serviceMinutes = forecast.data?.forecast?.serviceMinutesPerStop ?? fallbackServiceTime.minutes;

  useEffect(() => { const id = window.setInterval(() => setNow(Date.now()), 60_000); return () => window.clearInterval(id); }, []);

  async function load() {
    if (!token) return;
    setError("");
    try {
      const body = await apiJSON<{ items: DeliveryTripSummary[] }>(`/delivery/trips?date=${date}`, token);
      const items = body.items || [];
      const details = await Promise.all(items.map((trip) => apiJSON<DeliveryTripDetail>(`/delivery/trips/${trip.tripId}`, token).catch(() => undefined)));
      setRows(items.map((summary, index) => ({ summary, detail: details[index], state: "ok" as RowState, chilled: false })));
    } catch (e) { setError(errorText(e)); }
    finally { setLoaded(true); }
  }
  useEffect(() => { void load(); }, [token, date]);

  const openIncidents = (incidents.data?.items || []).filter((i) => i.date === date || !i.date);
  const evaluated: Row[] = rows.filter((r) => !depot || sameDepot(r.summary.depot, depot)).map((row) => {
    const stops = row.detail?.stops || [];
    const nextIndex = stops.findIndex((s) => !s.outcomeCode);
    const next = nextIndex >= 0 ? stops[nextIndex] : undefined;
    let nextEta: Row["nextEta"];
    if (next) {
      const allocation = plan.data?.allocations?.find((a) => a.tripId === row.summary.tripId && a.orderId === next.orderId);
      const previous = previousReportedStop(stops, nextIndex);
      const previousAllocation = previous && plan.data?.allocations?.find((a) => a.tripId === row.summary.tripId && a.orderId === previous.orderId);
      nextEta = estimateArrival({ plannedArrivalAt: allocation?.plannedArrivalAt, plannedDepartureAt: previousAllocation?.plannedDepartureAt, previousOutcomeAt: previous?.outcomeAt || previous?.outcomeReceivedAt, previousArrivedAt: previous?.arrivedAt, serviceMinutesPerStop: serviceMinutes, windowCloseAt: next.plannedWindowClose, deliveryDate: row.detail?.run.deliveryDate, now });
    }
    const lastUpdate = lastEvent(stops);
    const incident = openIncidents.find((i) => i.vehicleId === row.summary.vehicleId);
    const started = /progress|started|en_route/i.test(row.summary.status || "") || Boolean(lastUpdate);
    const done = /complete/i.test(row.summary.status || "") || (stops.length > 0 && !next);
    // W8: silent-trip and chilled-on-board watch, from real trip data only.
    const active = started && !done;
    const lastReported = [...stops].reverse().find((s) => s.outcomeCode || s.arrivedAt);
    const watch = enrichTripWithWatch({
      tripId: row.summary.tripId,
      vehicleId: row.summary.vehicleId,
      lastUpdateAt: active ? lastUpdate : undefined,
      lastKnownPlace: lastReported ? [lastReported.outletId, lastReported.outletName].filter(Boolean).join(" · ") : (DEPOT_LABELS[row.summary.depot || ""] || row.summary.depot || ""),
      isChilled: active && stops.some((s) => !s.outcomeCode && isChilled(s.temperatureRequirement)),
      chilledStartedAt: row.detail?.run?.startedAt,
      chilledAllowedMinutes: FRESH_TRIP_BUDGET_MINUTES,
    }, now);
    const silent = watch.isSilent;
    const state: RowState = incident ? "broken" : done ? "done" : nextEta && (nextEta.kind === "late" || nextEta.kind === "risk") ? "late" : silent ? "silent" : started ? "ok" : "waiting";
    return { ...row, next, nextEta, lastUpdate, state, incident, watch, chilled: stops.some((s) => isChilled(s.temperatureRequirement)) };
  });
  const order: RowState[] = ["broken", "late", "silent", "ok", "waiting", "done"];
  const sorted = [...evaluated].sort((a, b) => order.indexOf(a.state) - order.indexOf(b.state));
  const visible = sorted.filter((r) => filter === "all" || (filter === "risk" && r.state === "late") || (filter === "silent" && r.state === "silent") || (filter === "chilled" && r.chilled));
  const totalStops = evaluated.reduce((s, r) => s + (r.summary.stopCount ?? r.detail?.stops.length ?? 0), 0);
  const doneStops = evaluated.reduce((s, r) => s + (r.summary.completedStops ?? r.detail?.stops.filter((x) => x.outcomeCode).length ?? 0), 0);
  const onRoad = evaluated.filter((r) => r.state !== "done" && r.state !== "waiting").length;
  const lateRows = evaluated.filter((r) => r.state === "late");
  const strandedStops = evaluated.filter((r) => r.state === "broken").reduce((s, r) => s + (r.detail?.stops.filter((x) => !x.outcomeCode).length || 0), 0);

  const actions: ActionItem[] = [];
  for (const row of evaluated.filter((r) => r.state === "broken")) {
    const remaining = row.detail?.stops.filter((s) => !s.outcomeCode) || [];
    actions.push({ key: `inc-${row.incident!.id}`, severity: "critical", title: `${row.summary.vehicleId} ${t("broke down")}: ${remaining.length} ${t("stops stranded")}, ${remaining.filter((s) => isChilled(s.temperatureRequirement)).length} ${t("of them chilled")}`, text: `${t(row.incident!.type)} · ${row.incident!.description} · ${t("reported")} ${dateTime(row.incident!.reportedAt)}`, actions: [{ label: t("Acknowledge"), onClick: () => setAcknowledged((a) => [...a, `inc-${row.incident!.id}`]) }, { label: `→ ${t("Open recovery")}`, primary: true, onClick: () => setRecovery(row.incident!) }] });
  }
  for (const incident of openIncidents.filter((i) => !evaluated.some((r) => r.incident?.id === i.id))) {
    actions.push({ key: `inc-${incident.id}`, severity: "high", title: `${incident.vehicleId} · ${t(incident.type)}`, text: `${incident.description} · ${t("blocked for planning on")} ${incident.date}`, actions: [{ label: t("Open recovery"), primary: true, onClick: () => setRecovery(incident) }] });
  }
  for (const row of lateRows) actions.push({ key: `late-${row.summary.tripId}`, severity: "high", title: `${row.summary.vehicleId} ${t("will reach")} ${row.next?.outletName || row.next?.outletId || ""} ${t("after its window closes")}`, text: `${t("Window")} ${clock(row.next?.plannedWindowOpen)}–${clock(row.next?.plannedWindowClose)} · ${t(row.nextEta?.label || "")}`, actions: [{ label: t("Acknowledge"), onClick: () => setAcknowledged((a) => [...a, `late-${row.summary.tripId}`]) }, { label: t("Open trip"), onClick: () => setOpenTrip(row.summary.tripId) }] });
  const watchAlerts = generateNeedsActionAlerts(evaluated.flatMap((r) => (r.watch ? [r.watch] : [])), new Set<string>(), now)
    .filter((a) => a.type === "CHILLED_OVERAGE" || evaluated.find((r) => r.summary.tripId === a.tripId)?.state === "silent");
  for (const alert of watchAlerts) {
    const ackAction = { label: t("Acknowledge"), onClick: () => setAcknowledged((a) => [...a, alert.id]) };
    const openAction = { label: t("Open trip"), primary: alert.type === "CHILLED_OVERAGE", onClick: () => setOpenTrip(alert.tripId) };
    if (alert.type === "SILENT_TRIP") actions.push({ key: alert.id, severity: "medium", title: `${t("No update from")} ${alert.vehicleId} ${t("for")} ${alert.minutes} ${t("minutes")}`, text: `${t("No update since")} ${alert.timeStr}${alert.lastKnownPlace ? ` · ${t("Last known")}: ${alert.lastKnownPlace}` : ""} · ${t("Records made offline will sync when signal returns")}`, actions: [ackAction, openAction] });
    else actions.push({ key: alert.id, severity: "high", title: `${alert.vehicleId} · ${t("Chilled goods on board running long")}`, text: `${t("Chilled time on board")}: ${alert.chilledMinutes}m (${t("exceeds allowed limit")})`, actions: [ackAction, openAction] });
  }
  for (const trip of (loading.data?.items || []).filter((x) => (x.shortfallCount || 0) > 0)) actions.push({ key: `short-${trip.tripId}`, severity: "medium", title: `${t("Loading shortfall on")} ${trip.vehicleId} ${t("trip")} ${trip.tripNumber ?? 1}: ${trip.shortfallCount} ${t("order(s) short before departure")}`, text: `${DEPOT_LABELS[trip.depot || ""] || trip.depot || ""} · ${trip.planRef || ""}`, actions: [{ label: t("Review shortfall"), primary: true, to: "/dispatcher/notifications" }] });
  const openActions = actions.filter((a) => !acknowledged.includes(a.key));
  const rail = (s: Severity) => (s === "critical" ? "dp-list-item--critical" : s === "high" ? "dp-list-item--high" : s === "medium" ? "dp-list-item--medium" : "");
  const sevTone = (s: Severity) => (s === "critical" ? "red" : s === "high" ? "amber" : s === "medium" ? "primary" : "muted") as "red" | "amber" | "primary" | "muted";

  const statusTag = (row: Row) => row.state === "broken" ? <Tag tone="red">⚠ {t("Broken down")}</Tag> : row.state === "late" ? <Tag tone="amber">{t("Will miss window")}</Tag> : row.state === "silent" ? <Tag>{t("No update")}</Tag> : row.state === "done" ? <Tag tone="green">{t("Completed")}</Tag> : row.state === "waiting" ? <Tag>{t("Not started")}</Tag> : <Tag tone="green">{t("On track")}</Tag>;

  return (
    <>
      <DpHero title={t("Live operations")} subtitle={t("Every trip on the road right now, from driver updates. Problems that need a decision come first.")}>
        <div className="dp-row">
          <label className="visually-hidden" htmlFor="live-date">{t("Delivery date")}</label>
          <input id="live-date" className="dp-input" style={{ width: 170 }} type="date" value={date} onChange={(e) => setDate(e.target.value)} />
          <button type="button" className="dp-btn dp-btn--ghost-light" onClick={() => void load()}>{t("Refresh")}</button>
        </div>
      </DpHero>
      <StatRow cols={4}>
        <Stat label={t("Trips on the road")} value={onRoad} sub={`${evaluated.length} ${t("trips today")}`} />
        <Stat label={t("Stops completed")} value={`${doneStops} ${t("of")} ${totalStops}`} sub={`${totalStops - doneStops} ${t("stops left")}`} />
        <Stat label={t("Stops delayed or at risk")} value={lateRows.length + strandedStops} sub={`${strandedStops} ${t("stranded")} · ${lateRows.length} ${t("late")}`} subTone="amber" />
        <Stat label={t("Issues needing action")} value={openActions.length} sub={`${openActions.filter((a) => a.severity === "critical").length} ${t("critical")}, ${openActions.filter((a) => a.severity === "high").length} ${t("high")}`} subTone="red" />
      </StatRow>
      <div className="dp-body">
        {error && <p className="dp-note dp-note--red" role="alert">{error}</p>}
        <Panel title={<>{t("Needs action")} <Tag tone="red">{openActions.length}</Tag></>} sub={t("Ranked by impact: breakdowns and chilled goods, closing windows, silent trips, then loading and delivery issues")} flush>
          {openActions.length === 0 ? <p className="dp-empty">{t("Nothing needs a decision right now.")}</p> : (
            <div className="dp-list">
              {openActions.map((item) => (
                <div key={item.key} className={`dp-list-item dp-list-item--rail ${rail(item.severity)}`}>
                  <div className="dp-list-main">
                    <p className="dp-list-title"><Tag tone={sevTone(item.severity)}>{t(item.severity === "critical" ? "Critical" : item.severity === "high" ? "High" : item.severity === "medium" ? "Medium" : "Low")}</Tag> {item.title}</p>
                    <p className="dp-list-text">{item.text}</p>
                  </div>
                  <div className="dp-list-actions">
                    {item.actions.map((action) => action.to
                      ? <Link key={action.label} to={action.to} className={`dp-btn dp-btn--sm${action.primary ? "" : " dp-btn--secondary"}`}>{action.label}</Link>
                      : <button key={action.label} type="button" className={`dp-btn dp-btn--sm${action.primary ? "" : " dp-btn--secondary"}`} onClick={action.onClick}>{action.label}</button>)}
                  </div>
                </div>
              ))}
            </div>
          )}
        </Panel>
        <Panel title={t("Trips on the road")} flush actions={<>
          <ChipGroup label={t("View")} value={view} onChange={setView} options={[{ value: "list", label: t("List") }, { value: "map", label: t("Map") }]} />
          <ChipGroup label={t("Trip filters")} value={filter} onChange={setFilter} options={[
            { value: "all", label: `${t("All")} ${evaluated.length}` },
            { value: "risk", label: `${t("At risk")} · ${lateRows.length}` },
            { value: "silent", label: `${t("No update")} · ${evaluated.filter((r) => r.state === "silent").length}` },
            { value: "chilled", label: `${t("Chilled")} · ${evaluated.filter((r) => r.chilled).length}` },
          ]} />
          <span className="muted" style={{ fontSize: "0.8125rem" }}>◷ {t("Progress comes from driver updates, not GPS")}</span>
        </>}>
          {view === "map" ? <TripMap rows={visible} outlets={outletMap(outlets.data?.items)} onOpen={setOpenTrip} /> : (
            <div className="dp-table-wrap">
              <table className="dp-table">
                <thead><tr><th>{t("Vehicle / depot")}</th><th>{t("Brand · district")}</th><th>{t("Trip")}</th><th>{t("Progress")}</th><th>{t("Next stop")}</th><th>{t("Planned / estimated")}</th><th>{t("Cooling")}</th><th>{t("Last update")}</th><th>{t("Status")}</th></tr></thead>
                <tbody>
                  {loaded && visible.length === 0 && <tr><td colSpan={9}>{t("No delivery trips for this date.")}</td></tr>}
                  {!loaded && <tr><td colSpan={9}>{t("Loading trips…")}</td></tr>}
                  {visible.map((row) => {
                    const stops = row.detail?.stops || [];
                    const first = stops[0];
                    const ago = minutesAgo(row.lastUpdate, now);
                    return (
                      <tr key={row.summary.tripId} className={`is-clickable${row.state === "broken" ? " is-alert" : ""}${row.state === "silent" ? " trip-greyed-out silent-trip" : ""}`} onClick={() => setOpenTrip(row.summary.tripId)}>
                        <td><button type="button" className="dp-link" onClick={(e) => { e.stopPropagation(); setOpenTrip(row.summary.tripId); }}>{row.summary.vehicleId}</button><span className="dp-cell-sub">{DEPOT_LABELS[row.summary.depot || ""] || row.summary.depot}</span></td>
                        <td><span className="dp-cell-main">{t(first?.brand || "—")}</span><span className="dp-cell-sub">{first?.district || ""}</span></td>
                        <td>{row.summary.tripNumber ?? 1} {t("of")} 2</td>
                        <td><span className="dp-dots" aria-label={`${row.summary.completedStops ?? 0} ${t("of")} ${row.summary.stopCount ?? stops.length} ${t("stops")}`}>{stops.map((s) => <span key={s.id} className={`dp-dot${s.outcomeCode ? (/fail|refus/i.test(s.outcomeCode) ? " dp-dot--failed" : " dp-dot--done") : ""}`} />)}</span> <span className="dp-cell-sub" style={{ display: "inline" }}>{row.summary.completedStops ?? 0} {t("of")} {row.summary.stopCount ?? stops.length}</span></td>
                        <td><span className={`dp-cell-main${row.state === "broken" ? " dp-cell-sub--red" : ""}`}>{row.next ? `${row.next.outletId}${row.next.outletName ? ` ${row.next.outletName}` : ""}` : "—"}</span><span className="dp-cell-sub">{row.state === "broken" ? t("Stranded with the truck") : row.next ? `${t("Window")} ${clock(row.next.plannedWindowOpen)} ${t("to")} ${clock(row.next.plannedWindowClose)}` : ""}</span></td>
                        <td><span className="dp-cell-main">{clock(plan.data?.allocations?.find((a) => a.tripId === row.summary.tripId && a.orderId === row.next?.orderId)?.plannedArrivalAt)}</span><span className={`dp-cell-sub${row.nextEta?.kind === "late" || row.nextEta?.kind === "risk" ? " dp-cell-sub--amber" : " dp-cell-sub--green"}`}>{row.state === "broken" ? t("No ETA") : row.nextEta?.eta ? clock(row.nextEta.eta) : t("Unknown")}</span></td>
                        <td>{row.chilled ? <Tag tone="cool">❄ {t("Chilled")}</Tag> : <Tag tone="primary">{t("Ambient")}</Tag>}{row.watch?.isChilledLong && <span className="status-amber chilled-amber">{t("Chilled time on board")}: {row.watch.chilledMinutes}m ({t("exceeds allowed limit")})</span>}</td>
                        <td><span className={`dp-cell-main${row.state === "silent" ? " dp-cell-sub--amber" : ""}`}>{clock(row.lastUpdate)}</span><span className={`dp-cell-sub${row.state === "silent" ? " dp-cell-sub--amber" : ""}`}>{ago === undefined ? t("No driver update yet") : `${ago} ${t("min ago")}`}</span>{row.state === "silent" && row.watch && <><span className="badge-grey">{t("No update since")} {row.watch.silentTime}</span>{row.watch.lastKnownPlace && <span className="dp-cell-sub">{t("Last known")}: {row.watch.lastKnownPlace}</span>}</>}</td>
                        <td>{statusTag(row)}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
          <div className="dp-table-foot"><span>{`${t("Showing")} ${visible.length} ${t("trips")} · ${depot ? DEPOT_LABELS[depot] : t("All depots")} · ${date}`}</span><span>{t("Completed trips move to the bottom")}</span></div>
        </Panel>
      </div>
      <TripDrawer tripId={openTrip} plan={plan.data} serviceMinutes={serviceMinutes} serviceVersion={forecast.data?.forecast?.serviceEstimateVersion || fallbackServiceTime.version} onClose={() => setOpenTrip("")} />
      <RecoveryDrawer incident={recovery} plan={plan.data} rows={evaluated} onClose={() => setRecovery(null)} onDone={(message) => { setRecovery(null); setToast(message); void plan.reload(); void incidents.reload(); void load(); }} />
      {toast && <Toast onClose={() => setToast("")}>✓ {toast}</Toast>}
    </>
  );
}

// Trips on an OpenStreetMap map. Outlet positions are approximate (district
// centre + offset). A truck sits at its last reported stop, or the depot before
// the first update; trips with no recent update stay grey so silence is visible.
function TripMap({ rows, outlets, onOpen }: { rows: Row[]; outlets: Map<string, Outlet>; onOpen: (tripId: string) => void }) {
  const { t } = useLocale();
  const [selected, setSelected] = useState("");
  const color = (s: RowState) => (s === "broken" ? "#c03221" : s === "late" ? "#d9822b" : s === "silent" || s === "waiting" ? "#8a92a6" : "#008b52");
  const tone = (s: RowState) => (s === "broken" ? "red" : s === "late" ? "amber" : s === "silent" || s === "waiting" ? "muted" : "green") as "red" | "amber" | "muted" | "green";
  const stateLabel = (s: RowState) => t(s === "broken" ? "Broken down" : s === "late" ? "Will miss window" : s === "silent" ? "No update" : s === "done" ? "Completed" : s === "waiting" ? "Not started" : "On track");
  const silentText = (row: Row) => (row.state === "silent" && row.watch?.silentTime ? `${t("No update since")} ${row.watch.silentTime}` : stateLabel(row.state));
  const at = (stop?: DeliveryStop): LatLng | undefined => {
    const o = stop ? outlets.get(stop.outletId || "") : undefined;
    return o?.latitude != null && o.longitude != null ? [o.latitude, o.longitude] : undefined;
  };
  const markers: MapMarker[] = [];
  const lines: MapLine[] = [];
  const depots = new Set<string>();
  for (const row of rows) {
    const stops = row.detail?.stops || [];
    const depotCode = row.summary.depot || "DEPOT_NORTH";
    const depot = DEPOT_LOCATIONS[depotCode] || DEPOT_LOCATIONS.DEPOT_NORTH;
    depots.add(depotCode);
    const path: LatLng[] = [depot, ...stops.map(at).filter((p): p is LatLng => Boolean(p))];
    const focus = selected === row.summary.tripId;
    lines.push({ id: row.summary.tripId, points: path, color: focus ? "#3a57e8" : "#9db3ee", weight: focus ? 5 : 3, dashed: row.state === "waiting" });
    const last = [...stops].reverse().find((s) => s.outcomeCode || s.arrivedAt);
    const truck = at(last) || depot;
    const next = at(row.next);
    if (next && row.state !== "done") lines.push({ id: `${row.summary.tripId}-next`, points: [truck, next], color: color(row.state), dashed: true, weight: 3 });
    if (focus) {
      for (const s of stops) {
        const p = at(s);
        if (p) markers.push({ id: `${row.summary.tripId}-${s.id}`, at: p, kind: "stop", color: s.outcomeCode ? "#008b52" : "#3a57e8", label: String(s.stopSequence), title: `${s.stopSequence}. ${s.outletId} ${s.outletName || ""}` });
      }
    }
    markers.push({ id: row.summary.tripId, at: truck, kind: "truck", color: color(row.state), selected: focus, label: `${row.summary.vehicleId} · ${silentText(row)}`, title: `${row.summary.vehicleId} · ${silentText(row)}`, onClick: () => setSelected(row.summary.tripId) });
  }
  for (const d of depots) markers.push({ id: `depot-${d}`, at: DEPOT_LOCATIONS[d] || DEPOT_LOCATIONS.DEPOT_NORTH, kind: "depot", color: "#232d42", label: `${DEPOT_LABELS[d] || d} ${t("depot")}`, title: `${DEPOT_LABELS[d] || d} ${t("depot")}` });
  const chosen = rows.find((r) => r.summary.tripId === selected);
  return (
    <div className="dp-panel-body">
      <div className="dp-grid-2" style={{ gridTemplateColumns: "minmax(0, 2fr) minmax(280px, 1fr)" }}>
        <div style={{ position: "relative" }}>
          <WaypointMap label={t("Map of trips on the road")} markers={markers} lines={lines} fitKey={rows.map((r) => r.summary.tripId).join()} />
          <div className="dp-map-legend" style={{ zIndex: 500 }}>
            <p style={{ margin: 0 }}><span style={{ color: "#008b52" }}>●</span> {t("On track")}</p>
            <p style={{ margin: 0 }}><span style={{ color: "#d9822b" }}>●</span> {t("At risk of missing a window")}</p>
            <p style={{ margin: 0 }}><span style={{ color: "#c03221" }}>●</span> {t("Needs action now")}</p>
            <p style={{ margin: 0 }}><span style={{ color: "#8a92a6" }}>●</span> {t("No recent update (last known place)")}</p>
            <p style={{ margin: 0 }}><span style={{ color: "#3a57e8" }}>●</span> {t("Selected trip and its stops")}</p>
          </div>
          {chosen && (
            <div className="dp-panel" style={{ position: "absolute", right: 16, bottom: 16, zIndex: 500, width: 300, padding: 16 }}>
              <div className="dp-row dp-row--between"><strong>{chosen.summary.vehicleId}</strong><Tag tone={tone(chosen.state)}>{stateLabel(chosen.state)}</Tag></div>
              <dl className="dp-kv-rows" style={{ marginTop: 8 }}>
                <div><dt>{t("Stops")}</dt><dd>{chosen.summary.completedStops ?? 0} {t("of")} {chosen.summary.stopCount ?? chosen.detail?.stops.length ?? 0}</dd></div>
                <div><dt>{t("Next stop")}</dt><dd>{chosen.next ? `${chosen.next.outletId} · ${clock(chosen.nextEta?.eta || chosen.next.plannedWindowOpen)}` : "—"}</dd></div>
                <div><dt>{t("Last update")}</dt><dd>{chosen.lastUpdate ? clock(chosen.lastUpdate) : t("No driver update yet")}</dd></div>
              </dl>
              <button type="button" className="dp-btn dp-btn--block" style={{ marginTop: 8 }} onClick={() => onOpen(chosen.summary.tripId)}>{t("Open trip")}</button>
            </div>
          )}
        </div>
        <div className="dp-stack">
          <h3 className="dp-h3">{t("Trips on the road")} ({rows.length})</h3>
          <p className="muted" style={{ margin: 0, fontSize: "0.8125rem" }}>{t("Sorted by what needs you first. Select a trip to find it on the map.")}</p>
          {rows.map((row) => (
            <button key={row.summary.tripId} type="button" className="dp-subcard" style={{ textAlign: "left", background: selected === row.summary.tripId ? "#eef1ff" : "#fff", borderColor: selected === row.summary.tripId ? "#3a57e8" : undefined, cursor: "pointer" }} onClick={() => setSelected(row.summary.tripId)}>
              <span className="dp-row dp-row--between"><strong>{row.summary.vehicleId}</strong><Tag tone={tone(row.state)}>{stateLabel(row.state)}</Tag></span>
              <span className="dp-cell-sub">{row.next ? `${t("Next stop")} ${row.next.outletId} · ${clock(row.nextEta?.eta || row.next.plannedWindowOpen)}` : t("Route complete")}</span>
            </button>
          ))}
          <p className="dp-note" style={{ margin: 0, fontSize: "0.8125rem" }}>{t("Outlet positions are approximate (district centre). A truck is shown at its last reported stop, not a GPS position.")}</p>
        </div>
      </div>
    </div>
  );
}

function TripDrawer({ tripId, plan, serviceMinutes, serviceVersion, onClose }: { tripId: string; plan: PlanDetail | null; serviceMinutes: number; serviceVersion: string; onClose: () => void }) {
  const { t } = useLocale();
  const { user } = useAuth();
  const token = user?.access_token || "";
  const [detail, setDetail] = useState<DeliveryTripDetail | null>(null);
  const [messages, setMessages] = useState<TripMessage[]>([]);
  const [history, setHistory] = useState<LatenessProbability[]>([]);
  const [historyState, setHistoryState] = useState<"loading" | "ready" | "unavailable">("loading");
  const [location, setLocation] = useState<LiveLocation | null>(null);
  const [error, setError] = useState("");
  const [body, setBody] = useState("");
  const [stopId, setStopId] = useState("");
  const [sending, setSending] = useState(false);
  const sendingRef = useRef(false);
  const calibrations = evaluatedLatenessCalibrations(history);

  async function open(id: string) {
    setError(""); setHistory([]); setHistoryState("loading");
    try {
      const [trip, messageResult] = await Promise.all([apiJSON<DeliveryTripDetail>(`/delivery/trips/${id}`, token), apiJSON<{ items: TripMessage[] }>(`/delivery/trips/${id}/messages`, token)]);
      setDetail(trip); setMessages(messageResult.items || []);
      try { const h = await apiJSON<{ items: LatenessProbability[] }>(`/delivery/trips/${id}/lateness-history`, token); setHistory(h.items || []); setHistoryState("ready"); }
      catch { setHistoryState("unavailable"); }
    } catch (e) { setHistoryState("unavailable"); setError(errorText(e)); }
  }
  useEffect(() => { if (tripId) void open(tripId); else setDetail(null); }, [tripId]);
  useEffect(() => {
    if (!detail?.tripId || detail.status !== "in_progress") { setLocation(null); return; }
    let active = true;
    let latestRequest = 0;
    const refresh = async () => {
      const request = ++latestRequest;
      try {
        const result = await apiJSON<{ location: LiveLocation | null }>(`/delivery/trips/${encodeURIComponent(detail.tripId)}/location`, token);
        if (active && request === latestRequest) setLocation(result.location || null);
      } catch { if (active && request === latestRequest) setLocation(null); }
    };
    void refresh();
    const timer = window.setInterval(() => { void refresh(); }, 15_000);
    return () => { active = false; window.clearInterval(timer); };
  }, [detail?.tripId, detail?.status, token]);

  async function send(e: FormEvent) {
    e.preventDefault();
    const text = body.trim();
    if (!detail || !text || sendingRef.current) return;
    sendingRef.current = true; setSending(true);
    try { await postTripMessageOnce({ senderId: user?.profile?.sub || "", token, tripId: detail.tripId, body: text, stopId }); setBody(""); await open(detail.tripId); }
    catch (err) { setError(errorText(err)); }
    finally { sendingRef.current = false; setSending(false); }
  }

  const now = Date.now();
  return (
    <Drawer open={Boolean(tripId)} onClose={onClose} title={detail ? `${detail.run?.vehicleId || ""} · ${t("Trip")} ${detail.run?.tripNumber ?? 1}` : t("Trip")} sub={detail ? `${detail.run?.planRef || ""} · ${DEPOT_LABELS[detail.run?.depot || ""] || detail.run?.depot || ""} · ${t(detail.status)}` : undefined}>
      {error && <p className="dp-note dp-note--red" role="alert">{error}</p>}
      {!detail && !error && <p role="status">{t("Loading trip…")}</p>}
      {detail && <>
        {(serviceVersion === fallbackServiceTime.version || historyState === "unavailable" || detail.stops.some((s) => !validArrivalAt(plan?.allocations?.find((a) => a.tripId === detail.tripId && a.orderId === s.orderId)?.plannedArrivalAt))) && <p className="dp-note" role="status">{t(ESTIMATES_UNAVAILABLE_MESSAGE)}</p>}
        <LiveLocationMap key={detail.tripId} location={location} />
        <ol className="dp-checks" style={{ listStyle: "none" }}>
          {detail.stops.map((s, index, stops) => {
            const allocation = plan?.allocations?.find((a) => a.tripId === detail.tripId && a.orderId === s.orderId);
            const previous = previousReportedStop(stops, index);
            const previousAllocation = previous && plan?.allocations?.find((a) => a.tripId === detail.tripId && a.orderId === previous.orderId);
            const estimate = estimateArrival({ plannedArrivalAt: allocation?.plannedArrivalAt, plannedDepartureAt: previousAllocation?.plannedDepartureAt, previousOutcomeAt: previous?.outcomeAt || previous?.outcomeReceivedAt, previousArrivedAt: previous?.arrivedAt, serviceMinutesPerStop: serviceMinutes, windowCloseAt: s.plannedWindowClose, deliveryDate: detail.run.deliveryDate, now });
            const lateness = history.find((item) => item.brand === (s.brand || "") && item.temperatureRequirement === (s.temperatureRequirement || ""));
            const range = calibratedArrivalRange(estimate, lateness);
            return (
              <li key={s.id || s.orderId} className="dp-subcard" style={{ display: "block" }}>
                <div className="dp-row dp-row--between">
                  <strong>{s.stopSequence}. {s.outletName || s.outletId || s.orderRef}</strong>
                  <Tag tone={s.outcomeCode ? (/fail|refus/i.test(s.outcomeCode) ? "red" : "green") : s.arrivedAt ? "primary" : "muted"}>{t(s.outcomeCode || s.status)}</Tag>
                </div>
                <p className="muted" style={{ margin: "4px 0", fontSize: "0.8125rem" }}>{s.orderRef} · {t("Window")} {clock(s.plannedWindowOpen)}–{clock(s.plannedWindowClose)}{isChilled(s.temperatureRequirement) ? ` · ❄ ${t("Chilled")}` : ""}</p>
                <p style={{ margin: 0, fontSize: "0.875rem" }}>{s.arrivedAt ? `${t("Arrived")} ${clock(s.arrivedAt)}` : `${t(estimate.label)} · ${t(estimate.risk || "schedule unavailable")}`}</p>
                {!s.arrivedAt && estimate.kind !== "unknown" && <p className="muted" style={{ margin: 0, fontSize: "0.75rem" }}>{t(estimate.confidence)}{range ? ` · ${t("Calibrated historical arrival range")}: ${clock(range.lower.toISOString())}–${clock(range.upper.toISOString())} · ${t("nominal 80% interval")}; ${range.sampleCount} ${t("paired arrivals")}` : ` · ${t("Arrival range withheld")}: ${t(lateness?.arrivalRangeStatus || (historyState === "unavailable" ? "ARRIVAL_HISTORY_UNAVAILABLE" : "INSUFFICIENT_HISTORY"))}`}</p>}
                {!s.arrivedAt && <p className="muted" style={{ margin: 0, fontSize: "0.75rem" }} aria-live="polite">{historyState === "loading" ? t("Loading arrival history…") : historyState === "unavailable" ? t("Arrival history is unavailable.") : lateness?.probability != null ? `${t("Late arrival probability")}: ${(lateness.probability * 100).toFixed(1)}% · ${lateness.sampleCount} ${t("past stops")}` : `${t("Insufficient history; probability withheld.")} · n=${lateness?.sampleCount ?? 0}`}</p>}
                {s.temperatureReadings?.map((reading) => <p key={reading.operationId} className={reading.evaluation === "OUT_OF_RANGE" ? "dp-cell-sub--red" : "muted"} role={reading.evaluation === "OUT_OF_RANGE" ? "alert" : "status"} style={{ margin: 0, fontSize: "0.75rem" }}>{t(reading.evaluation)} · {reading.valueC.toFixed(1)} °C · {dateTime(reading.occurredAt)} · {reading.actorId}</p>)}
                {(s.loadingShortfallSummary || []).map((sf, i) => <p key={i} className="dp-cell-sub--amber" style={{ margin: 0, fontSize: "0.75rem" }}>{t(sf.type)} · {sf.affectedUnits} {t("units")}{sf.note ? ` · ${sf.note}` : ""}</p>)}
                <p className="muted" style={{ margin: 0, fontSize: "0.75rem" }}>{s.outcomeReceivedAt ? `${t("Outcome received")} ${dateTime(s.outcomeReceivedAt)}` : s.arrivedReceivedAt ? `${t("Arrival received")} ${dateTime(s.arrivedReceivedAt)}` : t("No driver update yet")}</p>
              </li>
            );
          })}
        </ol>
        <p className="muted" style={{ margin: 0, fontSize: "0.75rem" }}>{t("Event-based estimates")} ({ARRIVAL_ESTIMATE_VERSION}; {serviceVersion}) {t("use the published schedule and last completed stop; an arrived stop is projected using its service-time allowance. They do not use GPS or live traffic. Confidence: low. Refreshed")} {dateTime(new Date(now).toISOString())}.</p>
        <p className="muted" style={{ margin: 0, fontSize: "0.75rem" }}>{historyState === "unavailable" ? t("Arrival history is unavailable.") : calibrations.length > 0 ? `${t("Probability calibration (Brier score)")}: ${calibrations.map((item) => `${item.brand}/${item.temperatureRequirement || t("unspecified temperature")} ${item.brierScore!.toFixed(3)}`).join("; ")}. ${t("Lower scores indicate better probability calibration.")}` : t("Calibration is withheld until ten out-of-time predictions are available.")}</p>
        <section aria-labelledby="trip-messages-heading" className="dp-stack">
          <h3 className="dp-h3" id="trip-messages-heading">{t("Trip messages and receipts")}</h3>
          <form onSubmit={send} className="dp-stack" aria-busy={sending}>
            <label className="dp-field">{t("Message for driver")}<textarea value={body} onChange={(e) => setBody(e.target.value)} maxLength={1000} required /></label>
            <label className="dp-field">{t("Related stop (optional)")}<select value={stopId} onChange={(e) => setStopId(e.target.value)}><option value="">{t("Whole trip")}</option>{detail.stops.map((s) => <option key={s.id} value={s.id}>{s.stopSequence}. {s.outletName || s.outletId || s.orderRef}</option>)}</select></label>
            <button type="submit" className="dp-btn" disabled={!body.trim() || sending}>{t("Send message")}</button>
          </form>
          {messages.length === 0 ? <p className="muted" style={{ margin: 0 }}>{t("No trip messages yet.")}</p> : <ul className="dp-checks">{messages.map((m) => <li key={m.id} className="dp-subcard" style={{ display: "block" }}><p style={{ margin: 0 }}>{m.body}</p><p className="muted" style={{ margin: 0, fontSize: "0.75rem" }}>{t("Sent")} {dateTime(m.createdAt)} {t("by")} {m.sentBy}{m.stopId ? ` · ${t("linked to a stop")}` : ` · ${t("whole trip")}`}</p><p role="status" className={m.acknowledgedAt ? "dp-cell-sub--green" : "muted"} style={{ margin: 0, fontSize: "0.75rem" }}>{m.acknowledgedAt ? `${t("Acknowledged by")} ${m.acknowledgedBy} · ${dateTime(m.acknowledgedAt)}` : t("Awaiting driver acknowledgement")}</p></li>)}</ul>}
        </section>
      </>}
    </Drawer>
  );
}

// Vehicle breakdown recovery: stranded stops → rescue options with rule
// checks → confirm and notify. No reassignment is applied until the final step.
function RecoveryDrawer({ incident, plan, rows, onClose, onDone }: { incident: Incident | null; plan: PlanDetail | null; rows: Row[]; onClose: () => void; onDone: (message: string) => void }) {
  const { t } = useLocale();
  const token = useToken();
  const [step, setStep] = useState(1);
  const [proposal, setProposal] = useState<BreakdownProposal | null>(null);
  const [choice, setChoice] = useState<Record<number, string>>({});
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!incident) return;
    setStep(1); setProposal(null); setChoice({}); setError("");
    if (!plan) { setError(t("No plan is loaded for this date, so replacement options cannot be checked.")); return; }
    void apiJSON<BreakdownProposal>(`/planning/plans/${plan.plan.id}/breakdowns/proposals?vehicleId=${encodeURIComponent(incident.vehicleId)}`, token)
      .then((result) => {
        setProposal(result);
        setChoice(Object.fromEntries(result.items.map((item) => [item.tripNumber, item.options.find((o) => o.valid)?.vehicleId || ""])));
        const affected = result.items.flatMap((item) => item.stops.map((stop) => stop.outletId || stop.orderRef));
        setNotice(`Delivery update: vehicle ${incident.vehicleId} is unavailable. We are arranging a replacement for ${affected.join(", ")}. We will confirm revised arrival times shortly.`);
      })
      .catch((e) => setError(errorText(e)));
  }, [incident?.id]);

  async function confirmAll() {
    if (!plan || !incident || !proposal) return;
    setBusy(true); setError("");
    try {
      const drafts: string[] = [];
      for (const item of proposal.items) {
        const replacement = choice[item.tripNumber];
        if (!replacement) continue;
        const result = await apiJSON<{ planVersion: number; noticeDrafts: { orderRef: string; outletId: string; estimatedArrival: string }[] }>(`/planning/plans/${plan.plan.id}/breakdowns/reassign`, token, { method: "POST", body: JSON.stringify({ sourceVehicleId: incident.vehicleId, replacementVehicleId: replacement, tripNumber: item.tripNumber }) });
        drafts.push(...result.noticeDrafts.map((n) => `${n.outletId} ${clock(n.estimatedArrival)}`));
      }
      onDone(`${t("Replacement confirmed · a new plan version was published")}${drafts.length ? ` · ${drafts.join(", ")}` : ""}`);
    } catch (e) { setError(errorText(e)); }
    finally { setBusy(false); }
  }

  const row = rows.find((r) => r.summary.vehicleId === incident?.vehicleId);
  const stopInfo = (orderId: string) => row?.detail?.stops.find((s) => s.orderId === orderId);
  const ranked = (proposal?.items || []).flatMap((item) => item.stops.map((s) => ({ ...s, tripNumber: item.tripNumber, stop: stopInfo(s.orderId) })))
    .sort((a, b) => Number(Boolean(b.chilled) || isChilled(b.stop?.temperatureRequirement)) - Number(Boolean(a.chilled) || isChilled(a.stop?.temperatureRequirement)) || (a.windowClose || a.stop?.plannedWindowClose || "").localeCompare(b.windowClose || b.stop?.plannedWindowClose || "") || (a.urgencyRank ?? 0) - (b.urgencyRank ?? 0));
  const anyChoice = Object.values(choice).some(Boolean);
  const steps = (
    <ol className="dp-steps">
      {[t("Stranded stops"), t("Rescue options"), t("Confirm and notify")].map((label, index) => (
        <li key={label} aria-current={step === index + 1 ? "step" : undefined} className={step > index + 1 ? "is-done" : undefined}><span className="dp-step-num">{step > index + 1 ? "✓" : index + 1}</span>{label}</li>
      ))}
    </ol>
  );
  return (
    <Drawer open={Boolean(incident)} onClose={onClose} eyebrow={t("Vehicle breakdown recovery")} title={incident ? `${incident.vehicleId} · ${t(incident.type)}` : ""} steps={steps}
      footer={<>
        {step > 1 && <button type="button" className="dp-btn dp-btn--secondary" onClick={() => setStep(step - 1)}>{t("Back")}</button>}
        {step === 1 && <button type="button" className="dp-btn" disabled={!proposal} onClick={() => setStep(2)}>→ {t("Find rescue options")}</button>}
        {step === 2 && <button type="button" className="dp-btn" disabled={!anyChoice} onClick={() => setStep(3)}>→ {t("Use selected option")}</button>}
        {step === 3 && <><span className="muted dp-spacer" style={{ fontSize: "0.8125rem" }}>ⓘ {t("Nothing is sent until you confirm.")}</span><button type="button" className="dp-btn" disabled={busy || !anyChoice} onClick={() => void confirmAll()}>✓ {t("Confirm replacement and publish version")}</button></>}
      </>}>
      {error && <p className="dp-note dp-note--red" role="alert">{error}</p>}
      {incident && step === 1 && <>
        <div className="dp-banner" style={{ alignItems: "flex-start" }}>
          <span className="dp-banner-icon" aria-hidden="true">!</span>
          <div><p className="dp-banner-title" style={{ fontSize: "1rem" }}>{t(incident.type)} · {dateTime(incident.reportedAt)}</p><p className="dp-banner-text">{incident.description}</p>{row && <p className="dp-banner-text">{row.summary.completedStops ?? 0} {t("of")} {row.summary.stopCount ?? 0} {t("stops were delivered before it stopped.")}</p>}</div>
        </div>
        <h3 className="dp-h3">{t("Stranded orders, most urgent first")}</h3>
        {!proposal && !error && <p role="status">{t("Checking replacement feasibility…")}</p>}
        {ranked.map((s, index) => (
          <div key={s.allocationId} className="dp-subcard dp-row" style={{ alignItems: "flex-start" }}>
            <span className="dp-list-icon" aria-hidden="true">{index + 1}</span>
            <div className="dp-spacer">
              <p className="dp-list-title">{s.outletId} · {s.orderRef}</p>
              <div className="dp-row" style={{ gap: 6, margin: "4px 0" }}>{isChilled(s.stop?.temperatureRequirement) ? <Tag tone="cool">❄ {t("Chilled")}</Tag> : <Tag tone="primary">{t("Ambient")}</Tag>}<Tag>{t("Trip")} {s.tripNumber}</Tag></div>
              <p className="dp-list-text">{t("Window")} {clock(s.stop?.plannedWindowOpen)}–{clock(s.stop?.plannedWindowClose)} · {t("Stop")} {s.stopSequence}</p>
            </div>
          </div>
        ))}
        {(proposal?.items.length || 0) > 1 && <Note>{t("More than one trip of this vehicle is affected. Each trip gets its own rescue option.")}</Note>}
      </>}
      {incident && step === 2 && proposal && <>
        <p style={{ margin: 0, fontSize: "0.875rem" }}>{t("Each option is checked against every vehicle rule. Only an option that passes all checks can be confirmed.")}</p>
        {proposal.items.map((item) => (
          <fieldset key={item.tripNumber} className="dp-stack" style={{ border: 0, padding: 0, margin: 0 }}>
            <legend className="dp-section-label">{t("Trip")} {item.tripNumber} · {item.stops.length} {t("affected stop(s)")}</legend>
            {item.options.map((option, index) => (
              <label key={option.vehicleId} className={`dp-option${option.valid && index === item.options.findIndex((o) => o.valid) ? " dp-option--recommended" : ""}`}>
                <input type="radio" name={`trip-${item.tripNumber}`} value={option.vehicleId} disabled={!option.valid} checked={choice[item.tripNumber] === option.vehicleId} onChange={() => setChoice({ ...choice, [item.tripNumber]: option.vehicleId })} />
                <div className="dp-spacer">
                  <div className="dp-row dp-row--between"><p className="dp-option-title">{t("Send")} {option.vehicleId}</p>{option.valid ? (index === item.options.findIndex((o) => o.valid) ? <Tag tone="green">{t("Recommended")}</Tag> : <Tag tone="green">{t("Feasible")}</Tag>) : <Tag tone="amber">{t("Blocked")}</Tag>}</div>
                  <ul className="dp-checks" style={{ marginTop: 8 }}>
                    {option.valid ? <Check state="ok">{t("All vehicle rules pass")} · {option.projectedStops} {t("stops")}</Check> : option.failures.map((f) => <Check key={f.reasonCode} state="bad">{t(f.reasonCode)}</Check>)}
                  </ul>
                </div>
              </label>
            ))}
            {item.options.length === 0 && <Note tone="amber">{t("No replacement vehicle is free. Defer these orders from Plan and allocate with a reason.")}</Note>}
          </fieldset>
        ))}
      </>}
      {incident && step === 3 && <>
        <Note tone="green" title={`${t("Rescue plan")}: ${Object.values(choice).filter(Boolean).join(", ")} ${t("takes over from")} ${incident.vehicleId}`}>{t("A new plan version is published and the replacement driver gets the new route.")}</Note>
        <h3 className="dp-h3">{t("When you confirm")}</h3>
        <ul className="dp-checks">
          <Check state="todo">{t("The replacement driver gets the new route on their phone.")}</Check>
          <Check state="todo">{incident.vehicleId} {t("stays blocked for planning until the incident is closed.")}</Check>
          <Check state="todo">{t("The breakdown and handover are added to each order's timeline.")}</Check>
        </ul>
        <label className="dp-field">{t("Draft outlet notice")}<textarea value={notice} onChange={(e) => setNotice(e.target.value)} maxLength={1200} /><span>{t("Draft only; no message was sent.")}</span></label>
      </>}
    </Drawer>
  );
}
