import { FormEvent, useEffect, useState } from "react";
import { PriorityReviews } from "../automations/PriorityReviews";
import { Link } from "react-router-dom";
import { ApiError, apiJSON } from "../api/client";
import { todayInSriLanka } from "../api/date.mjs";
import { newOperationId } from "../api/delivery";
import { DEPOT_LABELS } from "../api/loading";
import { DEFER_REASONS, FuelLedger, Plan, PlanDetail, PlanOrder } from "../api/planning";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { resolveFuelAttempt } from "./fuelSubmission.mjs";
import { DisruptionRiskPanel } from "./DisruptionRiskPanel";
import { Outlet, outletMap } from "./types";
import { FRESH_TRIP_BUDGET_MINUTES, normalizePlan, planChecks, tripLoads } from "./planModel";
import { DEPOT_LOCATIONS, LatLng, MapLine, MapMarker, WaypointMap } from "../components/WaypointMap";
import { Banner, Check, ChipGroup, DpHero, Drawer, Meter, Note, Panel, Stat, StatRow, Tag, brandTone, meterTone } from "./ui";
import { clock, dateTime, dayLabel, hhmm, isChilled, kg, m3, pct, useApi } from "./useApi";

type Tab = "unallocated" | "trips" | "map" | "deferred" | "constraints";

// Warnings that do not stop the plan being locked.
const ADVISORY = new Set(["policy", "freshtime"]);
const TRIP_COLORS = ["#3a57e8", "#008b52", "#d9822b", "#7c3aed", "#0891b2", "#c03221", "#4b5563", "#be185d"];
type SimulateResult = { allocated: number; unallocated: number; failures?: { orderId: string; reasonCode: string }[]; fairnessPolicy?: string };

// FR-53: the next-run target must be strictly after the plan's own delivery
// date (enforced server-side too), so the date picker's minimum is the day
// after, not the plan date itself.
function dayAfter(dateStr: string) {
  const d = new Date(`${dateStr}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + 1);
  return d.toISOString().slice(0, 10);
}

export function PlanningPage() {
  const { user } = useAuth();
  const { t } = useLocale();
  const token = user?.access_token || "";
  const [date, setDate] = useState(todayInSriLanka);
  const [detail, setDetail] = useState<PlanDetail | null>(null);
  const [loadingPlan, setLoadingPlan] = useState(true);
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const [tab, setTab] = useState<Tab>("unallocated");
  const [expanded, setExpanded] = useState<string>("");
  const [assignOrder, setAssignOrder] = useState<string>("");
  const [assignReason, setAssignReason] = useState("");
  const [vehicleId, setVehicleId] = useState("");
  const [tripNumber, setTripNumber] = useState("1");
  const [deferOrder, setDeferOrder] = useState<string>("");
  const [reason, setReason] = useState("MANUAL_DISPATCHER_DEFERRAL");
  const [comment, setComment] = useState("");
  const [nextRunTarget, setNextRunTarget] = useState("");
  const [whatIf, setWhatIf] = useState<SimulateResult | null>(null);
  const [whatIfOpen, setWhatIfOpen] = useState(false);
  const [fuelLedger, setFuelLedger] = useState<FuelLedger | null>(null);
  const [fuelError, setFuelError] = useState("");
  const [fuelVehicleId, setFuelVehicleId] = useState("");
  const [fuelDate, setFuelDate] = useState(todayInSriLanka);
  const [fuelLiters, setFuelLiters] = useState("");
  const [fuelReceipt, setFuelReceipt] = useState("");
  const [fuelSubmitting, setFuelSubmitting] = useState(false);
  const [fuelAttempt, setFuelAttempt] = useState<{ signature: string; operationId: string } | null>(null);
  const outlets = useApi<{ items: Outlet[] }>("/shared/outlets");
  const outletById = outletMap(outlets.data?.items);

  async function loadFuelLedger(weekOf: string) {
    try {
      setFuelLedger(await apiJSON<FuelLedger>(`/fleet/fuel/ledger?weekOf=${weekOf}`, token));
      setFuelError("");
    } catch {
      setFuelLedger(null);
      setFuelError(t("Actual fuel entries are unavailable. Planning will use confirmed-plan estimates only until the ledger reconnects."));
    }
  }

  async function run(label: string, fn: () => Promise<void>) {
    setError(""); setInfo("");
    try { await fn(); setInfo(t(label)); }
    catch (e) { setError(e instanceof ApiError ? `${e.status}: ${e.message}` : String(e)); }
  }

  async function refresh(id?: string) {
    const body = normalizePlan(await apiJSON<PlanDetail>(id ? `/planning/plans/${id}` : `/planning/plans?date=${date}`, token));
    setDetail(body);
    await loadFuelLedger(fuelDate);
    return body;
  }

  // Open the existing plan for the chosen date, if one was already created.
  useEffect(() => {
    if (!token) return;
    let active = true;
    setLoadingPlan(true);
    apiJSON<PlanDetail>(`/planning/plans?date=${date}`, token)
      .then((body) => { if (active) setDetail(normalizePlan(body)); })
      .catch(() => { if (active) setDetail(null); })
      .finally(() => { if (active) setLoadingPlan(false); });
    return () => { active = false; };
  }, [token, date]);

  async function createPlan(e?: FormEvent) {
    e?.preventDefault();
    await run("Plan loaded", async () => {
      const created = await apiJSON<{ plan: Plan }>("/planning/plans", token, { method: "POST", body: JSON.stringify({ deliveryDate: date }) });
      await refresh(created.plan.id);
    });
  }
  const planId = detail?.plan.id || "";
  const generate = () => run("Generated", async () => { await apiJSON(`/planning/plans/${planId}/generate`, token, { method: "POST" }); await refresh(planId); setWhatIfOpen(false); });
  const reset = () => run("Reset", async () => { await apiJSON(`/planning/plans/${planId}/reset`, token, { method: "POST" }); await refresh(planId); });
  const confirm = () => run("Confirmed", async () => { await apiJSON(`/planning/plans/${planId}/confirm`, token, { method: "POST" }); await refresh(planId); });
  const revisePlan = () => run("Revision opened · field acknowledgement is required for the next version", async () => { await apiJSON(`/planning/plans/${planId}/revise`, token, { method: "POST" }); await refresh(planId); });
  const removeAlloc = (id: string) => run("Removed", async () => { await apiJSON(`/planning/plans/${planId}/allocations/${id}`, token, { method: "DELETE" }); await refresh(planId); });
  const simulate = () => run("What-if comparison ready", async () => { setWhatIf(await apiJSON<SimulateResult>(`/planning/plans/${planId}/simulate`, token, { method: "POST" })); setWhatIfOpen(true); });

  async function assign(e: FormEvent) {
    e.preventDefault();
    await run("Assigned", async () => {
      await apiJSON(`/planning/plans/${planId}/allocations`, token, { method: "POST", body: JSON.stringify({ orderId: assignOrder, vehicleId, tripNumber: Number(tripNumber), reason: assignReason }) });
      await refresh(planId);
      setAssignOrder(""); setAssignReason("");
    });
  }

  async function deferSelected(e: FormEvent) {
    e.preventDefault();
    await run("Deferred", async () => {
      await apiJSON(`/planning/plans/${planId}/deferrals`, token, { method: "POST", body: JSON.stringify({ orderId: deferOrder, reasonCode: reason, comment, nextRunTarget: nextRunTarget || undefined }) });
      await refresh(planId);
      setDeferOrder(""); setNextRunTarget(""); setComment("");
    });
  }

  async function recordFuel(e: FormEvent) {
    e.preventDefault();
    if (!detail || fuelSubmitting) return;
    setFuelSubmitting(true);
    const entry = { vehicleId: fuelVehicleId, date: fuelDate, liters: Number(fuelLiters), receiptRef: fuelReceipt };
    const attempt = resolveFuelAttempt(fuelAttempt, entry, newOperationId);
    setFuelAttempt(attempt);
    try {
      await apiJSON("/fleet/fuel/entries", token, { method: "POST", headers: { "Idempotency-Key": attempt.operationId }, body: JSON.stringify(entry) });
      await refresh(detail.plan.id);
      setFuelAttempt(null); setFuelLiters(""); setFuelReceipt("");
      setInfo(t("Actual fuel entry recorded"));
    } catch (e) {
      setError(e instanceof ApiError ? `${e.status}: ${e.message}` : String(e));
    } finally { setFuelSubmitting(false); }
  }

  const confirmed = detail?.plan.status === "confirmed";
  const loads = detail ? tripLoads(detail) : [];
  const checks = detail ? planChecks(detail, loads, t) : [];
  const blocking = checks.filter((c) => !c.ok && !ADVISORY.has(c.key));
  const ordersById = new Map((detail?.orders || []).map((o) => [o.id, o]));
  const vehiclesUsed = new Set(loads.filter((l) => l.orderCount > 0).map((l) => l.vehicle?.id)).size;
  const totalWeight = loads.reduce((s, l) => s + l.weightKg, 0);
  const totalCapacity = loads.reduce((s, l) => s + (l.vehicle?.weightCapacityKg || 0), 0);
  const totalVolume = loads.reduce((s, l) => s + l.volumeM3, 0);
  const volumeCapacity = loads.reduce((s, l) => s + (l.vehicle?.volumeCapacityM3 || 0), 0);
  const estFuel = (detail?.vehicles || []).reduce((s, v) => s + (v.planFuelL || 0), 0);
  // Planned fuel converts back to the distance it was estimated from.
  const estDistance = (detail?.vehicles || []).reduce((s, v) => s + (v.planFuelL || 0) * (v.kmPerL || 0), 0);
  const assignedCount = detail?.allocations.length || 0;
  const orderTotal = detail?.orders.length || 0;
  const deferOrderData = ordersById.get(deferOrder);
  const deferUnalloc = detail?.unallocated.find((u) => u.orderId === deferOrder);
  const assignOrderData = ordersById.get(assignOrder);
  const assignVehicle = detail?.vehicles.find((v) => v.id === vehicleId);
  const assignLoad = loads.find((l) => l.vehicle?.id === vehicleId && String(l.tripNumber) === tripNumber);
  const depotOf = (o?: PlanOrder) => (o ? outletById.get(o.outletId)?.depot : undefined);
  const outletLabel = (o?: PlanOrder) => { if (!o) return "—"; const outlet = outletById.get(o.outletId); return `${o.outletId}${outlet?.name ? ` ${outlet.name}` : ""}`; };

  const heroTitle = confirmed ? t("Plan Successfully Locked") : t("Plan and allocate");
  const heroSub = confirmed ? t("The plan has been finalized and published to operational teams.") : t("Your daily planning dashboard for a clear view of what’s next, what needs attention, and fleet readiness");

  return (
    <>
      <DpHero title={heroTitle} subtitle={heroSub}>
        <form className="dp-row" onSubmit={createPlan}>
          <label className="visually-hidden" htmlFor="plan-date">{t("Delivery date")}</label>
          <input id="plan-date" className="dp-input" style={{ width: 170 }} type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
          <button type="submit" className="dp-btn dp-btn--ghost-light">{detail ? t("Reload plan") : t("Create or load plan")}</button>
        </form>
      </DpHero>
      {detail && !confirmed && (
        <StatRow cols={5}>
          <Stat icon="▤" label={t("Orders to plan")} value={orderTotal} sub={`${detail.unallocated.length} ${t("with issues")}`} subTone={detail.unallocated.length ? "red" : "green"} />
          <Stat icon="▣" label={t("Vehicles allocated")} value={`${vehiclesUsed} / ${detail.vehicles.length}`} sub={`${detail.vehicles.length - vehiclesUsed} ${t("unassigned")}`} />
          <Stat icon="✓" iconTone="green" label={t("Orders allocated")} value={assignedCount} sub={`${pct(assignedCount, orderTotal)}% ${t("coverage")}`} subTone="green" />
          <Stat icon="!" iconTone="red" variant={blocking.length ? "red" : undefined} label={t("Blocking issues")} value={blocking.length} sub={blocking.length ? t("Plan cannot be locked") : t("All checks passed")} subTone={blocking.length ? "red" : "green"} />
          <Stat icon="◷" label={t("Plan last generated")} value={detail.plan.generatedAt ? clock(detail.plan.generatedAt) : "—"} sub={`${t("by")} ${detail.plan.createdBy || "—"}`} />
        </StatRow>
      )}
      {detail && confirmed && (
        <StatRow cols={5}>
          <Stat icon="▤" label={t("Plan version")} value={`${detail.plan.planRef}`} sub={`v${detail.publication?.version || detail.plan.currentVersion || 1} · ${dateTime(detail.publication?.publishedAt || detail.plan.publishedAt)}`} />
          <Stat icon="▣" label={t("Vehicles used")} value={`${vehiclesUsed} / ${detail.vehicles.length}`} sub={`${pct(totalWeight, totalCapacity)}% ${t("utilisation")}`} subTone="green" />
          <Stat icon="✓" iconTone="green" label={t("Assigned orders")} value={assignedCount} sub={`${pct(assignedCount, orderTotal)}% ${t("of planned orders")}`} />
          <Stat icon="▤" iconTone="red" label={t("Deferred orders")} value={detail.deferrals.length} sub={`${pct(detail.deferrals.length, orderTotal)}% ${t("deferred to next run")}`} subTone="red" />
          <Stat icon="∕" label={t("Estimated distance")} value={estDistance ? `${Math.round(estDistance).toLocaleString("en-LK")} km` : "—"} sub={t("Across all trips")} />
        </StatRow>
      )}
      {!detail && <div style={{ height: 24 }} />}
      <div className={`dp-body${detail ? "" : " dp-body--flush"}`}>
        {error && <p className="dp-note dp-note--red" role="alert">{error}</p>}
        {info && <p className="dp-note dp-note--green" role="status">{info}</p>}
        <PriorityReviews />
        {!detail && (
          <Panel title={loadingPlan ? t("Loading plan…") : `${t("No plan yet for")} ${dayLabel(date)}`} sub={t("Create the plan to pull in every confirmed order for this date, then generate allocations.")}>
            <div className="dp-row"><button type="button" className="dp-btn" onClick={() => void createPlan()} disabled={loadingPlan}>{t("Create or load plan")}</button></div>
          </Panel>
        )}
        {detail && !confirmed && (blocking.length ? (
          <Banner title={t("Cannot lock plan")} text={`${t("Your plan has")} ${blocking.length} ${t("blocking issue(s) that need to be resolved before it can be locked and sent to drivers.")}`}>
            <button type="button" className="dp-btn" disabled>🔒 {t("Lock plan")}</button>
          </Banner>
        ) : (
          <Banner tone="green" icon="✓" title={t("Plan ready")} text={t("All checks passed. You can lock this plan.")}>
            <button type="button" className="dp-btn" onClick={() => void confirm()}>🔒 {t("Lock plan and send to dispatch")}</button>
          </Banner>
        ))}
        {detail && confirmed && <ConfirmedView detail={detail} loads={loads} onRevise={() => void revisePlan()} />}
        {detail && !confirmed && (
          <div className="dp-grid-2">
            <section className="dp-panel" aria-label={t("Plan workspace")}>
              <div className="dp-panel-head">
                <ChipGroup label={t("Plan views")} value={tab} onChange={setTab} options={[
                  { value: "unallocated", label: `${t("Unallocated orders")} ${detail.unallocated.length}` },
                  { value: "trips", label: `${t("Planned trips")} ${detail.trips.length}` },
                  { value: "map", label: t("Map view") },
                  { value: "deferred", label: `${t("Deferred orders")} ${detail.deferrals.length}` },
                  { value: "constraints", label: t("Fuel, fairness and risks") },
                ]} />
                <div className="dp-row">
                  <button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" onClick={() => void generate()}>{t("Generate")}</button>
                  <button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" onClick={() => void simulate()}>{t("View comparison")}</button>
                  <button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" onClick={() => void reset()}>{t("Reset")}</button>
                </div>
              </div>
              {tab === "unallocated" && <>
                <div className="dp-panel-body" style={{ paddingBottom: 12 }}>
                  <h2 className="dp-panel-title">{t("Unallocated orders")}</h2>
                  <p className="dp-panel-sub">{t("Select orders to allocate to a run, or defer if needed.")}</p>
                  {detail.unallocatedReasonsAvailable === false && <p className="dp-note dp-note--amber" role="status" style={{ marginTop: 12 }}>{t("Why these orders are unallocated could not be loaded right now; the reasons below may be incomplete.")}</p>}
                </div>
                {detail.unallocated.length === 0 ? <p className="dp-empty">{t("No unallocated orders.")}</p> : (
                  <div className="dp-table-wrap">
                    <table className="dp-table">
                      <thead><tr><th>{t("Order")}</th><th>{t("Outlet")}</th><th>{t("Brand")}</th><th>{t("Weight / volume")}</th><th>{t("Reason")}</th><th>{t("Prev.")}</th><th>{t("Action")}</th></tr></thead>
                      <tbody>
                        {detail.unallocated.map((u) => {
                          const order = ordersById.get(u.orderId);
                          return (
                            <tr key={u.orderId} className={order?.deferredLastRun ? "is-alert" : undefined}>
                              <td><span className="dp-cell-main">{u.orderRef || order?.orderRef || u.orderId}</span><span className="dp-cell-sub">{order ? `${isChilled(order.temperatureRequirement) ? t("Chilled") : t("Ambient")} · ${m3(order.orderVolumeM3)}` : ""}</span></td>
                              <td><span className="dp-cell-main">{outletLabel(order)}</span><span className="dp-cell-sub">{DEPOT_LABELS[depotOf(order) || ""] || ""}</span></td>
                              <td>{order && <Tag tone={brandTone(order.brand)}>{t(order.brand)}</Tag>}</td>
                              <td><span className="dp-cell-main">{order ? kg(order.orderWeightKg) : "—"}</span><span className="dp-cell-sub">{order ? m3(order.orderVolumeM3) : ""}</span></td>
                              <td><span className="dp-cell-main">{t(u.reasonCode)}</span>{typeof u.details?.primaryBlockedVehicleTrips === "number" && typeof u.details?.vehicleTripsEvaluated === "number" && <span className="dp-cell-sub">{t("blocked")} {u.details.primaryBlockedVehicleTrips}/{u.details.vehicleTripsEvaluated} {t("vehicle-trips tried")}</span>}{(u.details?.otherLimitingFactors || []).map((f) => <span className="dp-cell-sub" key={f.reasonCode}>{t(f.reasonCode)} · {f.vehicleTripsBlocked}</span>)}</td>
                              <td>{order?.outletDeferralCount || 0}</td>
                              <td><div className="dp-row" style={{ gap: 6 }}>
                                <button type="button" className="dp-btn dp-btn--sm" onClick={() => { setAssignOrder(u.orderId); setVehicleId(""); }} aria-label={`${t("Assign")} ${u.orderRef || u.orderId}`}>{t("Assign")}</button>
                                <button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" onClick={() => { setDeferOrder(u.orderId); setReason(DEFER_REASONS.includes(u.reasonCode) ? u.reasonCode : "MANUAL_DISPATCHER_DEFERRAL"); }} aria-label={`${t("Defer")} ${u.orderRef || u.orderId}`}>{t("Defer")}</button>
                              </div></td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
              </>}
              {tab === "trips" && <>
                <div className="dp-panel-body" style={{ paddingBottom: 12 }}>
                  <h2 className="dp-panel-title">{t("Planned trips")} ({detail.trips.length})</h2>
                  <p className="dp-panel-sub">{t("What this plan has loaded onto each vehicle trip so far, against its capacity.")}</p>
                </div>
                <div className="dp-table-wrap">
                  <table className="dp-table">
                    <thead><tr><th>{t("Trip no.")}</th><th>{t("Vehicle")}</th><th>{t("Orders")}</th><th>{t("Load utilization")}</th><th>{t("Trip time")}</th><th>{t("Status")}</th><th>{t("Stops")}</th></tr></thead>
                    <tbody>
                      {loads.map((load) => {
                        const label = load.vehicle?.id || "—";
                        const open = expanded === load.tripId;
                        const allocations = detail.allocations.filter((a) => a.tripId === load.tripId).sort((a, b) => a.sequence - b.sequence);
                        return [
                          <tr key={load.tripId} className={load.overCapacity || load.coolingMismatch ? "is-alert" : undefined}>
                            <td><span className="dp-cell-main">{detail.plan.planRef}-{load.tripNumber}</span><span className="dp-cell-sub">{t("Trip")} {load.tripNumber}</span></td>
                            <td><span className="dp-cell-main">{label}</span><span className="dp-cell-sub">{load.vehicle ? `${load.vehicle.type} · ${load.vehicle.temp}` : ""}</span></td>
                            <td>{load.orderCount}{load.chilledOrders > 0 && <span className="dp-cell-sub">❄ {load.chilledOrders} {t("chilled")}</span>}</td>
                            <td style={{ minWidth: 180 }}>
                              <Meter label={t("Weight")} valueText={`${load.weightPct}%`} pct={load.weightPct} tone={meterTone(load.weightPct)} ariaLabel={`${t("Weight")} ${label} ${t("Trip")} ${load.tripNumber}`} />
                              <Meter label={t("Volume")} valueText={`${load.volumePct}%`} pct={load.volumePct} tone={meterTone(load.volumePct)} ariaLabel={`${t("Volume")} ${label} ${t("Trip")} ${load.tripNumber}`} />
                            </td>
                            <td><span className="dp-cell-main">{load.firstArrival ? `${clock(load.firstArrival)} – ${clock(load.lastDeparture)}` : "—"}</span>{load.tripMinutes != null && <span className={`dp-cell-sub${load.fresh && load.tripMinutes > FRESH_TRIP_BUDGET_MINUTES ? " dp-cell-sub--red" : ""}`}>{load.tripMinutes}{load.fresh ? ` / ${FRESH_TRIP_BUDGET_MINUTES}` : ""} {t("min")}</span>}</td>
                            <td>{load.overCapacity ? <Tag tone="red">{t("Over capacity")}</Tag> : load.coolingMismatch ? <Tag tone="red">{t("Cooling mismatch")}</Tag> : load.coolingRisk ? <Tag tone="amber">{t("Cooling risk")}</Tag> : load.orderCount ? <Tag tone="green">{t("Ready")}</Tag> : <Tag>{t("Empty")}</Tag>}</td>
                            <td><button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" aria-expanded={open} onClick={() => setExpanded(open ? "" : load.tripId)}>{open ? t("Hide") : t("Show")} ({allocations.length})</button></td>
                          </tr>,
                          open && <tr key={`${load.tripId}-stops`}><td colSpan={7} style={{ background: "#fafbfe" }}>
                            <table className="dp-table">
                              <thead><tr><th>{t("Seq")}</th><th>{t("Order")}</th><th>{t("Outlet")}</th><th>{t("Arrive")}</th><th>{t("Service")}</th><th>{t("Depart")}</th><th>{t("Action")}</th></tr></thead>
                              <tbody>{allocations.map((a) => { const order = ordersById.get(a.orderId); return (
                                <tr key={a.id}><td>{a.sequence}</td><td>{order?.orderRef || a.orderId}</td><td>{outletLabel(order)}</td><td>{clock(a.plannedArrivalAt)}</td><td>{clock(a.plannedServiceStartAt)}</td><td>{clock(a.plannedDepartureAt)}</td>
                                  <td><button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" onClick={() => void removeAlloc(a.id)} aria-label={`${t("Remove")} ${order?.orderRef || a.orderId}`}>{t("Remove")}</button></td></tr>
                              ); })}</tbody>
                            </table>
                          </td></tr>,
                        ];
                      })}
                    </tbody>
                  </table>
                </div>
              </>}
              {tab === "map" && (() => {
                const at = (outletId?: string): LatLng | undefined => { const o = outletId ? outletById.get(outletId) : undefined; return o?.latitude != null && o.longitude != null ? [o.latitude, o.longitude] : undefined; };
                const markers: MapMarker[] = [];
                const lines: MapLine[] = [];
                const depots = new Set<string>();
                loads.filter((l) => l.orderCount > 0).forEach((load, i) => {
                  const color = TRIP_COLORS[i % TRIP_COLORS.length];
                  const depot = load.vehicle?.homeDepot || "DEPOT_NORTH";
                  depots.add(depot);
                  const stops = detail.allocations.filter((a) => a.tripId === load.tripId).sort((a, b) => a.sequence - b.sequence);
                  const points = stops.map((a) => at(ordersById.get(a.orderId)?.outletId)).filter((p): p is LatLng => Boolean(p));
                  lines.push({ id: load.tripId, points: [DEPOT_LOCATIONS[depot] || DEPOT_LOCATIONS.DEPOT_NORTH, ...points], color, weight: 3 });
                  stops.forEach((a) => { const order = ordersById.get(a.orderId); const p = at(order?.outletId); if (p) markers.push({ id: a.id, at: p, kind: "stop", color, label: `${load.vehicle?.id || ""}·${a.sequence}`, title: `${load.vehicle?.id} ${t("Trip")} ${load.tripNumber} · ${a.sequence}. ${outletLabel(order)}` }); });
                });
                for (const u of detail.unallocated) { const order = ordersById.get(u.orderId); const p = at(order?.outletId); if (p) markers.push({ id: `u-${u.orderId}`, at: p, kind: "stop", color: "#c03221", title: `${u.orderRef || order?.orderRef} · ${t("Unallocated")} · ${t(u.reasonCode)}` }); }
                for (const d of depots) markers.push({ id: `depot-${d}`, at: DEPOT_LOCATIONS[d] || DEPOT_LOCATIONS.DEPOT_NORTH, kind: "depot", color: "#232d42", label: `${DEPOT_LABELS[d] || d}`, title: `${DEPOT_LABELS[d] || d}` });
                return <div className="dp-panel-body dp-stack">
                  <WaypointMap label={t("Map of planned trips")} markers={markers} lines={lines} height={480} fitKey={`${detail.plan.id}-${loads.length}-${detail.allocations.length}`} />
                  <p className="muted" style={{ margin: 0, fontSize: "0.8125rem" }}>{t("Each colour is one vehicle trip in stop order; red points are unallocated orders. Outlet positions are approximate (district centre).")}</p>
                </div>;
              })()}
              {tab === "deferred" && <>
                <div className="dp-panel-body" style={{ paddingBottom: 12 }}>
                  <h2 className="dp-panel-title">{t("Deferred orders")} ({detail.deferrals.length})</h2>
                  <p className="dp-panel-sub">{t("A deferral needs a reason, a decider and a next run before it is accepted. The store sees the same reason.")}</p>
                </div>
                {detail.deferrals.length === 0 ? <p className="dp-empty">{t("No orders deferred in this plan.")}</p> : (
                  <div className="dp-table-wrap"><table className="dp-table">
                    <thead><tr><th>{t("Order")}</th><th>{t("Outlet")}</th><th>{t("Reason")}</th><th>{t("Comment")}</th><th>{t("Next run")}</th></tr></thead>
                    <tbody>{detail.deferrals.map((d) => <tr key={d.id}><td>{ordersById.get(d.orderId)?.orderRef || d.orderId}</td><td>{d.outletId}</td><td>{t(d.reasonCode)}</td><td>{d.comment || "—"}</td><td>{d.nextRunTarget ? dayLabel(d.nextRunTarget) : t("Next available run")}</td></tr>)}</tbody>
                  </table></div>
                )}
              </>}
              {tab === "constraints" && (
                <div className="dp-panel-body dp-stack">
                  <h2 className="dp-panel-title">{t("Fairness priority signals")}</h2>
                  <p className={detail.fairness?.signalAvailable ? "muted" : "dp-note dp-note--amber"} style={{ margin: 0 }}>{t("Fairness policy")}: {detail.fairness?.policy || t("Delivery history unavailable; review before generating.")}</p>
                  <p className="muted" style={{ margin: 0 }}>{t("Priority affects processing order only; hard vehicle and delivery constraints still decide feasibility.")}</p>
                  <div className="dp-table-wrap"><table className="dp-table">
                    <thead><tr><th>{t("Order")}</th><th>{t("Outlet")}</th><th>{t("Previous deferrals")}</th><th>{t("Days since served")}</th><th>{t("Priority score")}</th></tr></thead>
                    <tbody>{[...detail.orders].sort((a, b) => b.fairnessScore - a.fairnessScore || a.orderRef.localeCompare(b.orderRef)).slice(0, 15).map((order) => (
                      <tr key={order.id}><td>{order.orderRef}</td><td>{order.outletId}</td><td>{order.outletDeferralCount || 0}</td><td>{detail.fairness?.signalAvailable ? order.lastServedAt ? order.daysSinceLastServed : t("No prior successful delivery") : t("Unavailable")}</td><td>{order.fairnessScore}</td></tr>
                    ))}</tbody>
                  </table></div>
                  <h2 className="dp-panel-title">{t("Weekly fuel forecast")} · {t("week of")} {detail.plan.deliveryDate}</h2>
                  <p className="muted" style={{ margin: 0 }}>{t("Quota checks reserve the larger of actual use or other confirmed-plan estimates, then add this plan’s estimate.")}</p>
                  {detail.fuelLedgerAvailable === false && <p className="dp-note dp-note--red" role="status">{t("The actual fuel ledger or confirmed-plan totals could not be loaded. Quota estimates may be incomplete; retry before locking the plan.")}</p>}
                  {fuelError && <p className="dp-note dp-note--amber" role="status">{fuelError}</p>}
                  <div className="dp-table-wrap"><table className="dp-table">
                    <thead><tr><th>{t("Vehicle")}</th><th>{t("Other confirmed plans")}</th><th>{t("This plan estimate")}</th><th>{t("Actual to date")}</th><th>{t("Weekly quota")}</th><th>{t("Forecast / status")}</th></tr></thead>
                    <tbody>{detail.vehicles.map((vehicle) => { const actual = vehicle.weekFuelActualL || 0; const planned = (vehicle.weekFuelPlannedL || 0) + (vehicle.planFuelL || 0); const forecast = Math.max(actual, planned); const over = forecast > vehicle.weeklyFuelQuotaL; return (
                      <tr key={vehicle.id} className={over ? "is-alert" : undefined}><td>{vehicle.id}</td><td>{(vehicle.weekFuelPlannedL || 0).toFixed(1)} L</td><td>{(vehicle.planFuelL || 0).toFixed(1)} L</td><td>{actual.toFixed(1)} L</td><td>{vehicle.weeklyFuelQuotaL.toFixed(1)} L</td><td>{forecast.toFixed(1)} L · <Tag tone={over ? "red" : "green"}>{over ? t("over quota") : t("within quota")}</Tag></td></tr>
                    ); })}</tbody>
                  </table></div>
                  <h2 className="dp-panel-title">{t("Record actual fuel use")}</h2>
                  <p className="muted" style={{ margin: 0 }}>{t("Fuel entries are append-only. Select the date the consumption occurred; future dates are rejected.")}</p>
                  <form onSubmit={recordFuel} className="dp-filters">
                    <label className="dp-field">{t("Vehicle")}<select value={fuelVehicleId} onChange={(e) => setFuelVehicleId(e.target.value)} required><option value="">{t("Select vehicle")}</option>{detail.vehicles.map((v) => <option key={v.id} value={v.id}>{v.id}</option>)}</select></label>
                    <label className="dp-field">{t("Consumption date")}<input type="date" value={fuelDate} onChange={(e) => { setFuelDate(e.target.value); if (e.target.value) void loadFuelLedger(e.target.value); }} required /></label>
                    <label className="dp-field">{t("Liters used")}<input type="number" min="0.001" max="10000" step="0.001" value={fuelLiters} onChange={(e) => setFuelLiters(e.target.value)} required /></label>
                    <label className="dp-field">{t("Receipt reference or note")}<input value={fuelReceipt} onChange={(e) => setFuelReceipt(e.target.value)} maxLength={120} required /></label>
                    <button type="submit" className="dp-btn" disabled={fuelSubmitting}>{fuelSubmitting ? t("Recording…") : t("Record actual fuel")}</button>
                  </form>
                  {fuelLedger && <p className="muted" style={{ margin: 0 }}>{t("Actual fuel ledger for the selected date’s week:")} {fuelLedger.weekStart} {t("to")} {fuelLedger.weekEnd}.</p>}
                  {fuelLedger?.entries?.length ? <div className="dp-table-wrap"><table className="dp-table">
                    <thead><tr><th>{t("Date")}</th><th>{t("Vehicle")}</th><th>{t("Liters")}</th><th>{t("Receipt / note")}</th><th>{t("Recorded by")}</th></tr></thead>
                    <tbody>{fuelLedger.entries.map((entry) => <tr key={entry.id}><td>{entry.date}</td><td>{entry.vehicleId}</td><td>{entry.liters.toFixed(3)} L</td><td>{entry.receiptRef || entry.note || "—"}</td><td>{entry.recordedBy}</td></tr>)}</tbody>
                  </table></div> : null}
                  <DisruptionRiskPanel date={date} token={token} />
                </div>
              )}
            </section>
            <aside className="dp-stack" aria-label={t("Plan summary")}>
              <Panel title={t("Plan summary")} actions={<span className="muted">{dayLabel(detail.plan.deliveryDate)}</span>}>
                <dl className="dp-kv">
                  <div><dt>{t("assigned orders")}</dt><dd>{assignedCount}</dd></div>
                  <div><dt>{t("deferred orders")}</dt><dd>{detail.deferrals.length}</dd></div>
                  <div><dt>{t("total trips")}</dt><dd>{detail.trips.length}</dd></div>
                  <div><dt>{t("vehicles used")}</dt><dd>{vehiclesUsed}</dd></div>
                  <div><dt>{t("total load")}</dt><dd>{(totalWeight / 1000).toFixed(1)} / {(totalCapacity / 1000).toFixed(1)} t</dd></div>
                  <div><dt>{t("estimated fuel")}</dt><dd>{Math.round(estFuel)} L</dd></div>
                  <div><dt>{t("Capacity usage (weight)")}</dt><dd>{pct(totalWeight, totalCapacity)}%</dd></div>
                  <div><dt>{t("Capacity usage (volume)")}</dt><dd>{pct(totalVolume, volumeCapacity)}%</dd></div>
                </dl>
                <p className="muted" style={{ margin: "12px 0 0", fontSize: "0.8125rem" }}>{`${t("Volume")} ${m3(totalVolume)} / ${m3(volumeCapacity)} · ${detail.plan.planRef} · ${t(detail.plan.status)}`}</p>
              </Panel>
              <Panel title={blocking.length ? `${t("Blocking issues")} (${blocking.length})` : t("All checks passed")} sub={blocking.length ? t("Resolve these issues to lock the plan.") : t("Your plan meets all operational constraints.")}>
                <ul className="dp-checks">
                  {checks.map((check) => <Check key={check.key} state={check.ok ? "ok" : ADVISORY.has(check.key) ? "warn" : "bad"}><strong>{check.label}</strong> · <span className={check.ok ? "dp-cell-sub--green" : "dp-cell-sub--red"}>{check.detail}</span></Check>)}
                </ul>
                <div className="dp-stack" style={{ marginTop: 16 }}>
                  <button type="button" className="dp-btn dp-btn--block" onClick={() => void confirm()} disabled={blocking.length > 0}>🔒 {t("Lock plan and send to dispatch")}</button>
                  <button type="button" className="dp-btn dp-btn--secondary dp-btn--block" onClick={() => setTab("trips")}>{t("Review plan summary")}</button>
                </div>
              </Panel>
              {detail.publication?.version ? <Note tone="cool">{t("Published version")} {detail.publication.version} · {detail.publication.acknowledgements.length} {t("field acknowledgement(s) recorded")}</Note> : null}
            </aside>
          </div>
        )}
      </div>

      <Drawer open={Boolean(deferOrder && detail)} onClose={() => setDeferOrder("")} narrow title={t("Edit deferral reason")} sub={t("Update the reason for deferring this order and add any notes.")}
        footer={<><button type="button" className="dp-btn dp-btn--secondary" onClick={() => setDeferOrder("")}>{t("Cancel")}</button><button type="submit" form="defer-form" className="dp-btn">{t("Confirm deferral")}</button></>}>
        {deferOrderData && detail && <form id="defer-form" className="dp-stack" onSubmit={deferSelected}>
          <div className="dp-subcard">
            <div className="dp-row dp-row--between"><span className="muted">{t("Selected order")}</span>{deferOrderData.deferredLastRun && <Tag tone="amber">{t("Deferred last run")}</Tag>}</div>
            <p className="dp-stat-value">{deferOrderData.orderRef}</p>
            <dl className="dp-kv">
              <div><dt>{t("Outlet")}</dt><dd>{outletLabel(deferOrderData)}</dd></div>
              <div><dt>{t("Brand")}</dt><dd><Tag tone={brandTone(deferOrderData.brand)}>{t(deferOrderData.brand)}</Tag></dd></div>
              <div><dt>{t("Cooling requirement")}</dt><dd>{isChilled(deferOrderData.temperatureRequirement) ? t("Chilled") : t("Ambient")}</dd></div>
              <div><dt>{t("Weight / volume")}</dt><dd>{kg(deferOrderData.orderWeightKg)} · {m3(deferOrderData.orderVolumeM3)}</dd></div>
            </dl>
          </div>
          {deferOrderData.deferredLastRun && <Note tone="red" live>{t("Warning: this outlet was already deferred on its last run")}{deferOrderData.lastDeferralDate ? ` (${deferOrderData.lastDeferralDate})` : ""}.</Note>}
          {deferUnalloc && <div className="dp-subcard dp-row dp-row--between">
            <div><h3 className="dp-h3">{t("System suggested reason")}</h3><p className="muted" style={{ margin: 0 }}>{t(deferUnalloc.reasonCode)}</p></div>
            <button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" onClick={() => setReason(DEFER_REASONS.includes(deferUnalloc.reasonCode) ? deferUnalloc.reasonCode : "MANUAL_DISPATCHER_DEFERRAL")}>↵ {t("Use this reason")}</button>
          </div>}
          <label className="dp-field">{t("Deferral reason")}<select aria-label={t("Deferral reason")} value={reason} onChange={(e) => setReason(e.target.value)}>{DEFER_REASONS.map((r) => <option key={r} value={r}>{t(r)}</option>)}</select></label>
          <label className="dp-field">{t("Additional notes (optional)")}<textarea aria-label={t("Deferral comment")} value={comment} onChange={(e) => setComment(e.target.value)} maxLength={500} placeholder={t("Comment")} /><span>{comment.length} / 500</span></label>
          <label className="dp-field">{t("Next run recommendation")}<input aria-label={t("Expected next-run date")} type="date" value={nextRunTarget} onChange={(e) => setNextRunTarget(e.target.value)} min={dayAfter(detail.plan.deliveryDate)} /><span>{t("Based on available capacity and current demand. Leave empty for the next available run.")}</span></label>
          <div className="dp-subcard"><h3 className="dp-h3">{t("Current priority")}</h3><p style={{ margin: "4px 0 0" }}>{t("Priority score")} {deferOrderData.fairnessScore} · {deferOrderData.outletDeferralCount || 0} {t("previous deferral(s)")}</p><p className="muted" style={{ margin: 0, fontSize: "0.8125rem" }}>{t("Repeat deferrals raise this outlet's priority on the next plan.")}</p></div>
        </form>}
      </Drawer>

      <Drawer open={Boolean(assignOrder && detail)} onClose={() => setAssignOrder("")} title={t("Manual adjustment")} sub={t("You are making manual changes to the plan. All compatibility and capacity checks run again when you assign.")}
        footer={<><button type="button" className="dp-btn dp-btn--secondary" onClick={() => setAssignOrder("")}>{t("Cancel")}</button><button type="submit" form="assign-form" className="dp-btn">{t("Apply adjustment")}</button></>}>
        {assignOrderData && detail && <form id="assign-form" className="dp-stack" onSubmit={assign}>
          <div className="dp-subcard">
            <div className="dp-row dp-row--between"><p className="dp-stat-value" style={{ margin: 0 }}>{assignOrderData.orderRef}</p><Tag tone={isChilled(assignOrderData.temperatureRequirement) ? "cool" : "primary"}>{isChilled(assignOrderData.temperatureRequirement) ? t("Chilled") : t("Ambient")}</Tag></div>
            <p className="muted" style={{ margin: 0 }}>{outletLabel(assignOrderData)} · {kg(assignOrderData.orderWeightKg)} · {m3(assignOrderData.orderVolumeM3)}</p>
            <p className="muted" style={{ margin: 0 }}>{t("Delivery window")}: {(() => { const o = outletById.get(assignOrderData.outletId); return o ? `${hhmm(o.windowOpenTime)}–${hhmm(o.windowCloseTime)}` : "—"; })()}</p>
          </div>
          <div className="dp-filters">
            <label className="dp-field">{t("Vehicle")}<select aria-label={t("Assignment vehicle")} value={vehicleId} onChange={(e) => setVehicleId(e.target.value)} required><option value="">{t("Vehicle")}</option>{detail.vehicles.map((v) => <option key={v.id} value={v.id}>{v.id} · {v.type}/{v.temp} · {t(v.status)}</option>)}</select></label>
            <label className="dp-field">{t("Trip number")}<select aria-label={t("Trip number")} value={tripNumber} onChange={(e) => setTripNumber(e.target.value)}><option value="1">{t("Trip")} 1</option><option value="2">{t("Trip")} 2</option></select></label>
          </div>
          <label className="dp-field">{t("Reason for this manual assignment")}<input aria-label={t("Reason for this manual assignment")} placeholder={t("Why are you assigning this manually?")} value={assignReason} onChange={(e) => setAssignReason(e.target.value)} required minLength={3} maxLength={500} /></label>
          {assignVehicle && (() => {
            const weightAfter = (assignLoad?.weightKg || 0) + assignOrderData.orderWeightKg;
            const volumeAfter = (assignLoad?.volumeM3 || 0) + assignOrderData.orderVolumeM3;
            const wPct = pct(weightAfter, assignVehicle.weightCapacityKg), vPct = pct(volumeAfter, assignVehicle.volumeCapacityM3);
            const coolingOk = !isChilled(assignOrderData.temperatureRequirement) || /chill|refriger|frozen|multi|reefer/i.test(assignVehicle.temp);
            const depotOk = !depotOf(assignOrderData) || depotOf(assignOrderData) === assignVehicle.homeDepot;
            return (
              <div className="dp-trip-card" style={{ fontSize: "0.875rem" }}>
                <div className="dp-row dp-row--between"><strong>{assignVehicle.id} · {t("Trip")} {tripNumber}</strong><Tag tone={wPct > 100 || vPct > 100 || !coolingOk ? "red" : "green"}>{wPct > 100 || vPct > 100 || !coolingOk ? t("Attention") : t("On track")}</Tag></div>
                <Meter label={t("Weight after this order")} valueText={`${wPct}%`} pct={wPct} tone={meterTone(wPct)} ariaLabel={t("Weight after this order")} />
                <Meter label={t("Volume after this order")} valueText={`${vPct}%`} pct={vPct} tone={meterTone(vPct)} ariaLabel={t("Volume after this order")} />
                <ul className="dp-checks">
                  <Check state={coolingOk ? "ok" : "bad"}>{coolingOk ? t("Cooling suitable") : t("Chilled goods in non-refrigerated vehicle")}</Check>
                  <Check state={depotOk ? "ok" : "bad"}>{depotOk ? t("Depot match") : t("Depot mismatch")}</Check>
                  <Check state={wPct <= 100 && vPct <= 100 ? "ok" : "bad"}>{wPct <= 100 && vPct <= 100 ? t("Within weight and volume limits") : t("Vehicle overloaded")}</Check>
                </ul>
                <p className="muted" style={{ margin: 0 }}>{t("The server checks delivery windows, fuel and the trip limit again when you apply.")}</p>
              </div>
            );
          })()}
        </form>}
      </Drawer>

      <Drawer open={whatIfOpen && Boolean(whatIf) && Boolean(detail)} onClose={() => setWhatIfOpen(false)} title={t("What-if comparison")} sub={t("Compare your current plan with a freshly generated alternative before choosing.")}
        footer={<><button type="button" className="dp-btn dp-btn--secondary" onClick={() => setWhatIfOpen(false)}>{t("Keep current plan")}</button><button type="button" className="dp-btn" onClick={() => void generate()}>{t("Apply alternative plan")}</button></>}>
        {whatIf && detail && (() => {
          const altDeferred = whatIf.unallocated;
          const better = altDeferred < detail.unallocated.length;
          return <>
            <div className="dp-grid-2 dp-grid-2--even">
              <div className="dp-subcard"><div className="dp-row dp-row--between"><h3 className="dp-h3">{t("Current plan")}</h3><Tag tone="primary">{t("Active")}</Tag></div>
                <dl className="dp-kv-rows"><div><dt>{t("Orders allocated")}</dt><dd>{assignedCount}</dd></div><div><dt>{t("Unallocated orders")}</dt><dd>{detail.unallocated.length}</dd></div><div><dt>{t("Total trips")}</dt><dd>{detail.trips.length}</dd></div><div><dt>{t("Blocking issues")}</dt><dd>{blocking.length}</dd></div></dl></div>
              <div className="dp-subcard"><div className="dp-row dp-row--between"><h3 className="dp-h3">{t("Alternative plan")}</h3><Tag>{t("Scenario A")}</Tag></div>
                <dl className="dp-kv-rows"><div><dt>{t("Orders allocated")}</dt><dd>{whatIf.allocated} <span className={whatIf.allocated >= assignedCount ? "dp-cell-sub--green" : "dp-cell-sub--red"}>{whatIf.allocated - assignedCount >= 0 ? "+" : ""}{whatIf.allocated - assignedCount}</span></dd></div><div><dt>{t("Unallocated orders")}</dt><dd>{altDeferred} <span className={better ? "dp-cell-sub--green" : "dp-cell-sub--red"}>{altDeferred - detail.unallocated.length >= 0 ? "+" : ""}{altDeferred - detail.unallocated.length}</span></dd></div></dl></div>
            </div>
            {(whatIf.failures || []).length > 0 && <div><h3 className="dp-h3">{t("Orders still unallocated in the alternative")}</h3><ul className="dp-checks">{whatIf.failures!.slice(0, 8).map((f) => <Check key={f.orderId} state="warn">{ordersById.get(f.orderId)?.orderRef || f.orderId} · {t(f.reasonCode)}</Check>)}</ul></div>}
            <Note tone={better ? "green" : "primary"} title={better ? t("Recommended: Alternative plan") : t("Recommended: Keep current plan")}>{better ? t("The alternative allocates more orders. Applying it regenerates allocations and replaces manual changes.") : t("The alternative does not allocate more orders than the current plan.")}</Note>
            {whatIf.fairnessPolicy && <p className="muted" style={{ margin: 0, fontSize: "0.8125rem" }}>{whatIf.fairnessPolicy}</p>}
          </>;
        })()}
      </Drawer>
    </>
  );
}

function ConfirmedView({ detail, loads, onRevise }: { detail: PlanDetail; loads: ReturnType<typeof tripLoads>; onRevise: () => void }) {
  const { t } = useLocale();
  const publishedAt = detail.publication?.publishedAt || detail.plan.publishedAt;
  const acks = detail.publication?.acknowledgements || [];
  return (
    <div className="dp-grid-2">
      <Panel title={t("Plan summary")} sub={t("Key trips in this plan. Loaders and drivers get exactly this version; any later change creates a new one.")} flush>
        <div className="dp-table-wrap"><table className="dp-table">
          <thead><tr><th>{t("Trip no.")}</th><th>{t("Vehicle")}</th><th>{t("Orders")}</th><th>{t("Load utilization")}</th><th>{t("Trip time")}</th><th>{t("Status")}</th></tr></thead>
          <tbody>{loads.map((load) => <tr key={load.tripId}><td>{detail.plan.planRef}-{load.tripNumber}</td><td><span className="dp-cell-main">{load.vehicle?.id}</span><span className="dp-cell-sub">{load.vehicle ? `${load.vehicle.type} · ${load.vehicle.temp}` : ""}</span></td><td>{load.orderCount}</td><td>{load.weightPct}% / {load.volumePct}%</td><td>{load.tripMinutes != null ? `${load.tripMinutes}${load.fresh ? ` / ${FRESH_TRIP_BUDGET_MINUTES}` : ""} ${t("min")}` : "—"}</td><td><Tag tone="green">{t("Locked")}</Tag></td></tr>)}</tbody>
        </table></div>
        <div className="dp-panel-body" style={{ paddingTop: 16 }}>
          <Note title={t("What happens next?")}>{t("Drivers and loaders can now view their trips, load lists and assigned orders in their apps. You can monitor progress in the Live operations dashboard.")}</Note>
          <div className="dp-row" style={{ marginTop: 16 }}>
            <button type="button" className="dp-btn dp-btn--secondary" onClick={onRevise}>{t("Revise published plan")}</button>
            <span className="dp-spacer" />
            <Link to="/dispatcher" className="dp-btn dp-btn--secondary">← {t("Back to overview")}</Link>
            <Link to="/dispatcher/live" className="dp-btn">{t("View live operations")} →</Link>
          </div>
        </div>
      </Panel>
      <Panel title={t("Plan publication status")}>
        <ul className="dp-checks">
          <Check state="ok"><strong>{t("Plan locked")}</strong> · {detail.plan.planRef} · {dateTime(publishedAt)}</Check>
          <Check state="ok"><strong>{t("Trips published to drivers")}</strong> · {detail.trips.length} {t("trips shared via driver app")}</Check>
          <Check state="ok"><strong>{t("Load list shared with loaders")}</strong> · {t("Vehicle load lists generated")}</Check>
          <Check state={acks.length ? "ok" : "todo"}><strong>{t("Field acknowledgements")}</strong> · {acks.length} {t("field acknowledgement(s) recorded")}</Check>
          {acks.slice(0, 6).map((ack) => <Check key={`${ack.actorId}-${ack.acknowledgedAt}`} state="ok">{t(ack.actorRole)} · {ack.actorId} · {dateTime(ack.acknowledgedAt)}</Check>)}
          <Check state="ok"><strong>{t("Plan available for live operations")}</strong> · {t("Visible in operations dashboard")}</Check>
        </ul>
      </Panel>
    </div>
  );
}
