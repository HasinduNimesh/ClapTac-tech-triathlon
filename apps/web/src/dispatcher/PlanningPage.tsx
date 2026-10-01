import { FormEvent, useState } from "react";
import { ApiError, apiJSON } from "../api/client";
import { todayInSriLanka } from "../api/date.mjs";
import { newOperationId } from "../api/delivery";
import { DEFER_REASONS, FuelLedger, Plan, PlanDetail } from "../api/planning";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { resolveFuelAttempt } from "./fuelSubmission.mjs";
import { DisruptionRiskPanel } from "./DisruptionRiskPanel";

type FleetIncident={id:string;vehicleId:string;date:string;tripId?:string;type:string;description:string;affectedStops:string[];status:string};
type BreakdownProposal={planId:string;vehicleId:string;items:{tripNumber:number;stops:{allocationId:string;orderId:string;orderRef:string;outletId:string;stopSequence:number}[];options:{vehicleId:string;valid:boolean;projectedStops:number;failures:{reasonCode:string}[]}[]}[];confirmed:boolean};

function planTime(value?: string) {
  return value ? new Date(value).toLocaleTimeString("en-LK", { timeZone: "Asia/Colombo", hour: "2-digit", minute: "2-digit", hour12: false }) : "—";
}

export function PlanningPage() {
  const { user } = useAuth();
  const { t } = useLocale();
  const token = user?.access_token || "";
  const [date, setDate] = useState(todayInSriLanka);
  const [detail, setDetail] = useState<PlanDetail | null>(null);
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const [assignOrderId, setAssignOrderId] = useState("");
  const [deferOrderId, setDeferOrderId] = useState("");
  const [vehicleId, setVehicleId] = useState("");
  const [tripNumber, setTripNumber] = useState("1");
  const [reason, setReason] = useState("MANUAL_DISPATCHER_DEFERRAL");
  const [comment, setComment] = useState("");
  const [fuelLedger, setFuelLedger] = useState<FuelLedger | null>(null);
  const [fuelError, setFuelError] = useState("");
  const [fuelVehicleId, setFuelVehicleId] = useState("");
  const [fuelDate, setFuelDate] = useState(todayInSriLanka);
  const [fuelLiters, setFuelLiters] = useState("");
  const [fuelReceipt, setFuelReceipt] = useState("");
  const [fuelSubmitting, setFuelSubmitting] = useState(false);
  const [fuelAttempt, setFuelAttempt] = useState<{signature:string;operationId:string}|null>(null);
  const [incidents,setIncidents]=useState<FleetIncident[]>([]);const [incidentVehicle,setIncidentVehicle]=useState("");const [breakdown,setBreakdown]=useState<BreakdownProposal|null>(null);const [noticeDraft,setNoticeDraft]=useState("");const [confirmedNoticeDrafts,setConfirmedNoticeDrafts]=useState<string[]>([]);

  async function loadIncidents(){try{const data=await apiJSON<{items:FleetIncident[]}>("/fleet/incidents?openOnly=true",token);setIncidents(data.items||[]);setIncidentVehicle(current=>current||data.items?.[0]?.vehicleId||"");}catch{/* Fleet incident feed is supplementary to the plan view. */}}

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
    setError("");
    setInfo("");
    try {
      await fn();
      setInfo(t(label));
    } catch (e) {
      setError(e instanceof ApiError ? `${e.status}: ${e.message}` : String(e));
    }
  }

  async function refresh(id?: string) {
    const path = id ? `/planning/plans/${id}` : `/planning/plans?date=${date}`;
    const body = await apiJSON<PlanDetail>(path, token);
    setDetail(body);
    await loadFuelLedger(fuelDate);
    await loadIncidents();
    return body;
  }

  async function previewBreakdown(){if(!detail||!incidentVehicle)return;await run("Replacement feasibility checked",async()=>{const result=await apiJSON<BreakdownProposal>(`/planning/plans/${detail.plan.id}/breakdowns/proposals?vehicleId=${encodeURIComponent(incidentVehicle)}`,token);setBreakdown(result);setConfirmedNoticeDrafts([]);const affected=result.items.flatMap(item=>item.stops.map(stop=>stop.outletId||stop.orderRef));setNoticeDraft(`Delivery update: vehicle ${incidentVehicle} is unavailable. We are arranging a replacement for ${affected.join(", ")}. We will confirm revised arrival times shortly.`);});}

  async function confirmBreakdown(tripNumber:number,replacementVehicleId:string){if(!detail||!incidentVehicle)return;await run("Replacement confirmed · a new plan version was published",async()=>{const result=await apiJSON<{planVersion:number;noticeDrafts:{orderRef:string;outletId:string;estimatedArrival:string}[]}>(`/planning/plans/${detail.plan.id}/breakdowns/reassign`,token,{method:"POST",body:JSON.stringify({sourceVehicleId:incidentVehicle,replacementVehicleId,tripNumber})});setConfirmedNoticeDrafts(result.noticeDrafts.map(n=>`${n.orderRef} · ${n.outletId}: revised arrival ${new Date(n.estimatedArrival).toLocaleTimeString("en-LK",{timeZone:"Asia/Colombo",hour:"2-digit",minute:"2-digit",hour12:false})}`));setBreakdown(null);await refresh(detail.plan.id);});}

  async function createPlan(e: FormEvent) {
    e.preventDefault();
    await run("Plan loaded", async () => {
      const created = await apiJSON<{ plan: Plan }>("/planning/plans", token, {
        method: "POST",
        body: JSON.stringify({ deliveryDate: date }),
      });
      await refresh(created.plan.id);
    });
  }

  async function generate() {
    if (!detail) return;
    await run("Generated", async () => {
      await apiJSON(`/planning/plans/${detail.plan.id}/generate`, token, { method: "POST" });
      await refresh(detail.plan.id);
    });
  }

  async function reset() {
    if (!detail) return;
    await run("Reset", async () => {
      await apiJSON(`/planning/plans/${detail.plan.id}/reset`, token, { method: "POST" });
      await refresh(detail.plan.id);
    });
  }

  async function confirm() {
    if (!detail) return;
    await run("Confirmed", async () => {
      await apiJSON(`/planning/plans/${detail.plan.id}/confirm`, token, { method: "POST" });
      await refresh(detail.plan.id);
    });
  }

  async function revisePlan() {
    if (!detail) return;
    await run("Revision opened · field acknowledgement is required for the next version", async () => {
      await apiJSON(`/planning/plans/${detail.plan.id}/revise`,token,{method:"POST"});
      await refresh(detail.plan.id);
    });
  }

  async function assign(e: FormEvent) {
    e.preventDefault();
    if (!detail) return;
    await run("Assigned", async () => {
      await apiJSON(`/planning/plans/${detail.plan.id}/allocations`, token, {
        method: "POST",
        body: JSON.stringify({ orderId: assignOrderId, vehicleId, tripNumber: Number(tripNumber) }),
      });
      await refresh(detail.plan.id);
      setAssignOrderId("");
    });
  }

  async function deferOrder(e: FormEvent) {
    e.preventDefault();
    if (!detail) return;
    await run("Deferred", async () => {
      await apiJSON(`/planning/plans/${detail.plan.id}/deferrals`, token, {
        method: "POST",
        body: JSON.stringify({ orderId: deferOrderId, reasonCode: reason, comment }),
      });
      await refresh(detail.plan.id);
      setDeferOrderId("");
    });
  }

  async function removeAlloc(id: string) {
    if (!detail) return;
    await run("Removed", async () => {
      await apiJSON(`/planning/plans/${detail.plan.id}/allocations/${id}`, token, { method: "DELETE" });
      await refresh(detail.plan.id);
    });
  }

  async function recordFuel(e: FormEvent) {
    e.preventDefault();
    if (!detail || fuelSubmitting) return;
    setFuelSubmitting(true);
    const entry={ vehicleId:fuelVehicleId, date:fuelDate, liters:Number(fuelLiters), receiptRef:fuelReceipt };
    const attempt=resolveFuelAttempt(fuelAttempt,entry,newOperationId);
    setFuelAttempt(attempt);
    try {
      await apiJSON("/fleet/fuel/entries", token, {
        method: "POST",
        headers: { "Idempotency-Key": attempt.operationId },
        body: JSON.stringify(entry),
      });
      await refresh(detail.plan.id);
      setFuelAttempt(null);
      setFuelLiters("");
      setFuelReceipt("");
      setInfo(t("Actual fuel entry recorded"));
    } catch (e) {
      setError(e instanceof ApiError ? `${e.status}: ${e.message}` : String(e));
    } finally {
      setFuelSubmitting(false);
    }
  }

  const confirmed = detail?.plan.status === "confirmed";

  return (
    <section className="card">
      <h2>{t("Daily Plan")}</h2>
      {error && <p className="status-bad" role="alert">{error}</p>}
      {info && <p className="status-ok" role="status">{info}</p>}
      <form onSubmit={createPlan} className="row">
        <label>
          {t("Delivery date")}
          <input type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
        </label>
        <button type="submit">{t("Create or load plan")}</button>
      </form>
      <DisruptionRiskPanel date={date} token={token} />
      {detail && (
        <>
          <p>
            {detail.plan.planRef} · {detail.plan.deliveryDate} · {detail.plan.status}
            {detail.plan.generatedAt ? " · generated" : ""}
          </p>
          {detail.publication?.version ? <p className="muted">{t("Published version")} {detail.publication.version} · {detail.publication.acknowledgements.length} {t("field acknowledgement(s) recorded")}</p> : null}
          <p className={detail.fairness?.signalAvailable ? "muted" : "status-bad"}>
            {t("Fairness policy")}: {detail.fairness?.policy || t("Delivery history unavailable; review before generating.")}
          </p>
          {detail.policySignalAvailable === false && <p className="status-bad" role="status">{t("Planning policy service is unavailable. Safe defaults are in use; review policy before locking this plan.")}</p>}
          <div className="row">
            <button type="button" onClick={generate} disabled={confirmed}>
              {t("Generate")}
            </button>
            <button type="button" onClick={reset} disabled={confirmed}>
              {t("Reset")}
            </button>
            <button type="button" onClick={confirm} disabled={confirmed}>
              {t("Confirm")}
            </button>
            {confirmed && <button type="button" onClick={revisePlan}>{t("Revise published plan")}</button>}
          </div>

          <h3>{t("Unallocated")}</h3>
          <table>
            <thead>
              <tr>
                <th>{t("Order")}</th>
                <th>{t("Reason")}</th>
              </tr>
            </thead>
            <tbody>
              {(detail.unallocated || []).map((u) => (
                <tr key={u.orderId}>
                  <td>{u.orderRef || u.orderId}</td>
                  <td>{u.reasonCode}</td>
                </tr>
              ))}
            </tbody>
          </table>

          <h3>{t("Fairness priority signals")}</h3>
          <p className="muted">{t("Priority affects processing order only; hard vehicle and delivery constraints still decide feasibility.")}</p>
          <table>
            <thead><tr><th>{t("Order")}</th><th>{t("Outlet")}</th><th>{t("Previous deferrals")}</th><th>{t("Days since served")}</th><th>{t("Priority score")}</th></tr></thead>
            <tbody>
              {[...(detail.orders || [])].sort((a, b) => b.fairnessScore - a.fairnessScore || a.orderRef.localeCompare(b.orderRef)).map((order) => (
                <tr key={order.id}>
                  <td>{order.orderRef}</td>
                  <td>{order.outletId}</td>
                  <td>{order.outletDeferralCount || 0}</td>
                  <td>{detail.fairness?.signalAvailable ? order.lastServedAt ? order.daysSinceLastServed : t("No prior successful delivery") : t("Unavailable")}</td>
                  <td>{order.fairnessScore}</td>
                </tr>
              ))}
            </tbody>
          </table>

          <h3>{t("Weekly fuel forecast")} · {t("week of")} {detail.plan.deliveryDate}</h3>
          <p className="muted">{t("Quota checks reserve the larger of actual use or other confirmed-plan estimates, then add this plan’s estimate.")}</p>
          {detail.fuelLedgerAvailable === false && <p className="status-bad" role="status">{t("The actual fuel ledger or confirmed-plan totals could not be loaded. Quota estimates may be incomplete; retry before locking the plan.")}</p>}
          {fuelError && <p className="status-bad" role="status">{fuelError}</p>}
          <table>
            <thead><tr><th>{t("Vehicle")}</th><th>{t("Other confirmed plans")}</th><th>{t("This plan estimate")}</th><th>{t("Actual to date")}</th><th>{t("Weekly quota")}</th><th>{t("Forecast / status")}</th></tr></thead>
            <tbody>
              {(detail.vehicles || []).map((vehicle) => {
                const actual = vehicle.weekFuelActualL || 0;
                const planned = (vehicle.weekFuelPlannedL || 0) + (vehicle.planFuelL || 0);
                const forecast = Math.max(actual, planned);
                return <tr key={vehicle.id}>
                  <td>{vehicle.id}</td>
                  <td>{(vehicle.weekFuelPlannedL || 0).toFixed(1)} L</td>
                  <td>{(vehicle.planFuelL || 0).toFixed(1)} L</td>
                  <td>{actual.toFixed(1)} L</td>
                  <td>{vehicle.weeklyFuelQuotaL.toFixed(1)} L</td>
                  <td>{forecast.toFixed(1)} L · {forecast > vehicle.weeklyFuelQuotaL ? t("over quota") : t("within quota")}</td>
                </tr>;
              })}
            </tbody>
          </table>
          <h3>{t("Record actual fuel use")}</h3>
          <p className="muted">{t("Fuel entries are append-only. Select the date the consumption occurred; future dates are rejected.")}</p>
          <form onSubmit={recordFuel} className="row">
            <label>{t("Vehicle")}
              <select value={fuelVehicleId} onChange={(e) => setFuelVehicleId(e.target.value)} required>
                <option value="">{t("Select vehicle")}</option>
                {(detail.vehicles || []).map((vehicle) => <option key={vehicle.id} value={vehicle.id}>{vehicle.id}</option>)}
              </select>
            </label>
            <label>{t("Consumption date")}<input type="date" value={fuelDate} onChange={(e) => { setFuelDate(e.target.value); if (e.target.value) void loadFuelLedger(e.target.value); }} required /></label>
            <label>{t("Liters used")}<input type="number" min="0.001" max="10000" step="0.001" value={fuelLiters} onChange={(e) => setFuelLiters(e.target.value)} required /></label>
            <label>{t("Receipt reference or note")}<input value={fuelReceipt} onChange={(e) => setFuelReceipt(e.target.value)} maxLength={120} required /></label>
            <button type="submit" disabled={fuelSubmitting}>{fuelSubmitting ? t("Recording…") : t("Record actual fuel")}</button>
          </form>
          {fuelLedger && <p className="muted">{t("Actual fuel ledger for the selected date’s week:")} {fuelLedger.weekStart} {t("to")} {fuelLedger.weekEnd}.</p>}
          {fuelLedger?.items?.length ? <table>
            <thead><tr><th>{t("Vehicle")}</th><th>{t("Actual this week")}</th><th>{t("Weekly quota")}</th><th>{t("Remaining")}</th></tr></thead>
            <tbody>{fuelLedger.items.map((item) => <tr key={item.vehicleId}><td>{item.vehicleId}</td><td>{item.actualLitersL.toFixed(1)} L</td><td>{item.weeklyQuotaL.toFixed(1)} L</td><td>{Math.max(0, item.weeklyQuotaL - item.actualLitersL).toFixed(1)} L{item.actualLitersL > item.weeklyQuotaL ? ` · ${t("over quota")}` : ""}</td></tr>)}</tbody>
          </table> : null}
          {fuelLedger?.entries?.length ? <table>
            <thead><tr><th>{t("Date")}</th><th>{t("Vehicle")}</th><th>{t("Liters")}</th><th>{t("Receipt / note")}</th><th>{t("Recorded by")}</th></tr></thead>
            <tbody>{fuelLedger.entries.map((entry) => <tr key={entry.id}><td>{entry.date}</td><td>{entry.vehicleId}</td><td>{entry.liters.toFixed(3)} L</td><td>{entry.receiptRef || entry.note || "—"}</td><td>{entry.recordedBy}</td></tr>)}</tbody>
          </table> : null}

          {incidents.length>0&&<section className="card">
            <h3>{t("Open vehicle incidents")}</h3>
            <p>{t("Report an incident in Master data first. The vehicle is blocked for planning on the incident date.")}</p>
            <label>{t("Incident vehicle")}<select value={incidentVehicle} onChange={e=>{setIncidentVehicle(e.target.value);setBreakdown(null);setNoticeDraft("");setConfirmedNoticeDrafts([]);}}>{incidents.map(i=><option key={i.id} value={i.vehicleId}>{i.vehicleId} · {t(i.type)} · {i.date}{i.tripId?` · ${i.tripId}`:""}</option>)}</select></label>
            <button type="button" onClick={()=>void previewBreakdown()}>{t("Check replacement feasibility")}</button>
            {breakdown&&<div><p>{t("Feasible replacement options by affected trip:")}</p>{breakdown.items.map(item=><div key={item.tripNumber}><h4>{t("Trip")} {item.tripNumber} · {item.stops.length} {t("affected stop(s)")}</h4><p>{item.stops.map(s=>`${s.stopSequence}. ${s.orderRef||s.orderId} (${s.outletId})`).join(" · ")}</p><ul>{item.options.map(option=><li key={option.vehicleId}><strong>{option.vehicleId}</strong> · {option.valid?`${t("Feasible")} · ${option.projectedStops} ${t("stops")}`:`${t("Blocked")} · ${option.failures.map(f=>t(f.reasonCode)).join(", ")}`}{option.valid&&<button type="button" onClick={()=>void confirmBreakdown(item.tripNumber,option.vehicleId)}>{t("Confirm replacement and publish version")}</button>}</li>)}</ul></div>)}
              <label>{t("Draft outlet notice")}<textarea value={noticeDraft} onChange={e=>setNoticeDraft(e.target.value)} maxLength={1200}/></label>
              <p className="muted">{t("No reassignment has been applied. A passing constraint check enables confirmation and revised-arrival notice drafts.")}</p>
            </div>}
          </section>}
          {!breakdown&&noticeDraft&&<section className="card" aria-live="polite"><h3>{t("Reassignment notice drafts")}</h3><p>{t("Draft only; no message was sent.")}</p><label>{t("Draft outlet notice")}<textarea value={noticeDraft} onChange={e=>setNoticeDraft(e.target.value)} maxLength={1200}/></label>{confirmedNoticeDrafts.length>0&&<><h4>{t("Revised arrival estimates")}</h4><ul>{confirmedNoticeDrafts.map((draft,index)=><li key={`${index}-${draft}`}>{draft}</li>)}</ul></>}</section>}
          <h3>{t("Allocations")}</h3>
          <table>
            <thead>
              <tr>
                <th>{t("Order")}</th>
                <th>{t("Vehicle")}</th>
                <th>{t("Seq")}</th>
                <th>{t("Arrive")}</th>
                <th>{t("Service")}</th>
                <th>{t("Depart")}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {(detail.allocations || []).map((a) => (
                <tr key={a.id}>
                  <td>{a.orderId}</td>
                  <td>
                    {a.vehicleId}
                  </td>
                  <td>{a.sequence}</td>
                  <td>{planTime(a.plannedArrivalAt)}</td>
                  <td>{planTime(a.plannedServiceStartAt)}</td>
                  <td>{planTime(a.plannedDepartureAt)}</td>
                  <td>
                    {!confirmed && (
                      <button type="button" onClick={() => removeAlloc(a.id)}>
                        {t("Remove")}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          <h3>{t("Deferrals")}</h3>
          <table>
            <thead>
              <tr>
                <th>{t("Order")}</th>
                <th>{t("Reason")}</th>
              </tr>
            </thead>
            <tbody>
              {(detail.deferrals || []).map((d) => (
                <tr key={d.id}>
                  <td>{d.orderId}</td>
                  <td>{d.reasonCode}</td>
                </tr>
              ))}
            </tbody>
          </table>

          <h3>{t("Assign")}</h3>
          <form onSubmit={assign} className="row">
            <select aria-label={t("Order to assign")} value={assignOrderId} onChange={(e) => setAssignOrderId(e.target.value)} required disabled={confirmed || !detail.unallocated?.length}>
              <option value="">{t("Select unallocated order")}</option>
              {(detail.unallocated || []).map((order) => <option key={order.orderId} value={order.orderId}>{order.orderRef || order.orderId} · {t(order.reasonCode)}</option>)}
            </select>
            <select aria-label={t("Assignment vehicle")} value={vehicleId} onChange={(e) => setVehicleId(e.target.value)} required>
              <option value="">{t("Vehicle")}</option>
              {(detail.vehicles || []).map((v) => (
                <option key={v.id} value={v.id}>
                  {v.id} {v.type}/{v.temp} {v.status}
                </option>
              ))}
            </select>
            <select aria-label={t("Trip number")} value={tripNumber} onChange={(e) => setTripNumber(e.target.value)}>
              <option value="1">{t("Trip")} 1</option>
              <option value="2">{t("Trip")} 2</option>
            </select>
              <button type="submit" disabled={confirmed || !detail.unallocated?.length}>
              {t("Assign")}
            </button>
          </form>

          <h3>{t("Defer")}</h3>
          <form onSubmit={deferOrder} className="row">
            <select aria-label={t("Order to defer")} value={deferOrderId} onChange={(e) => setDeferOrderId(e.target.value)} required disabled={confirmed || !detail.unallocated?.length}>
              <option value="">{t("Select unallocated order")}</option>
              {(detail.unallocated || []).map((order) => <option key={order.orderId} value={order.orderId}>{order.orderRef || order.orderId} · {t(order.reasonCode)}</option>)}
            </select>
            <select aria-label={t("Deferral reason")} value={reason} onChange={(e) => setReason(e.target.value)}>
              {DEFER_REASONS.map((r) => (
                <option key={r} value={r}>
                  {t(r)}
                </option>
              ))}
            </select>
            <input aria-label={t("Deferral comment")} placeholder={t("Comment")} value={comment} onChange={(e) => setComment(e.target.value)} />
            <button type="submit" disabled={confirmed || !detail.unallocated?.length}>
              {t("Defer")}
            </button>
          </form>
        </>
      )}
    </section>
  );
}
