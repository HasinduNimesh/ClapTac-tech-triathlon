import { FormEvent, useEffect, useRef, useState } from "react";
import { ApiError, apiJSON } from "../api/client";
import { todayLocal } from "../api/date";
import { DeliveryStop, DeliveryTripDetail, DeliveryTripSummary, newOperationId } from "../api/delivery";
import { depotLabel } from "../api/loading";
import { useAuth } from "../auth/AuthContext";
import { shouldUseCachedDriverData } from "./cacheFallback.mjs";
import { installSafeStopLock } from "./safeStopLifecycle.mjs";
import { useLocale } from "../i18n";
import { bindDriverOwner, clearCompletedDriverCache, enqueue, getCachedDetail, getCachedTrips, isPaused, listQueue, putCachedTrips, readDriverDataForExport, setPaused } from "../offline/db";
import { createDriverDataExport } from "../offline/driverDataPrivacy.mjs";
import { offlineQueueHealth } from "../offline/queueHealth.mjs";
import { queueRetentionWarning } from "../offline/queueRetention.mjs";
import { hasQueuedRouteCompletion } from "../offline/routeCompletion.mjs";
import { cacheDetail, drainQueue, resumeSync, SyncBanner } from "../offline/sync";
import { createSingleFlightAction } from "./singleFlightAction.mjs";

function stopLabel(stop: DeliveryStop) {
  return stop.outletName || stop.outletId || stop.orderRef || `Stop ${stop.stopSequence}`;
}
type TripMessage = { id: string; body: string; stopId?: string; createdAt: string; acknowledgedBy?: string; acknowledgedAt?: string };

export function DriverTripsPage() {
  const { user, profile } = useAuth();
  const { t } = useLocale();
  const token = user?.access_token || "";
  const ownerId = profile?.userId || "";
  const [ownerState, setOwnerState] = useState<"loading" | "ready" | "blocked">("loading");
  const [date, setDate] = useState(todayLocal);
  const [trips, setTrips] = useState<DeliveryTripSummary[]>([]);
  const [detail, setDetail] = useState<DeliveryTripDetail | null>(null);
  const [stop, setStop] = useState<DeliveryStop | null>(null);
  const [error, setError] = useState("");
  const [banner, setBanner] = useState<SyncBanner>({ kind: "ok", text: "" });
  const [reason, setReason] = useState("");
  const [onboard, setOnboard] = useState<Record<string, "yes" | "no">>({});
  const [note, setNote] = useState("");
  const [returnGoods, setReturnGoods] = useState("");
  const [returnUnits, setReturnUnits] = useState("");
  const [returnResolution, setReturnResolution] = useState<"NEXT_RUN" | "REQUEST_DEFERRAL">("NEXT_RUN");
  const [temperatureC, setTemperatureC] = useState("");
  const [techCustody, setTechCustody] = useState<Record<string,{sealId:string;serials:string;condition:string}>>({});
  const [proofIds, setProofIds] = useState<Record<string, string>>({});
  const [photoProofIds, setPhotoProofIds] = useState<Record<string,string>>({});
  const [receiverName, setReceiverName] = useState("");
  const [incidentCategory, setIncidentCategory] = useState("VEHICLE");
  const [incidentDescription, setIncidentDescription] = useState("");
  const [queueCount, setQueueCount] = useState(0);
  const [privacyNotice, setPrivacyNotice] = useState("");
  const [queueRetention, setQueueRetention] = useState<ReturnType<typeof queueRetentionWarning>>(null);
  const [showSummary, setShowSummary] = useState(false);
  const [endedLocally, setEndedLocally] = useState(false);
  const [summarySource, setSummarySource] = useState<"device" | "server">("device");
  const [safeStopped, setSafeStopped] = useState(false);
  const [transitionBusy, setTransitionBusy] = useState(false);
  const [messages, setMessages] = useState<TripMessage[]>([]);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const drawing = useRef(false);
  const transitionLock = useRef(createSingleFlightAction());
  const detailRef = useRef<DeliveryTripDetail | null>(detail);
  const queueHealthReport = useRef({ ownerId: "", attemptedAt: 0, inFlight: false });
  detailRef.current = detail;

  // FR-25 review fix: receiverName is proof metadata for whichever stop is
  // currently open. Without this, switching stops after typing a name for
  // stop A silently attributes that name to stop B's proof too.
  useEffect(() => {
    setReceiverName("");
  }, [stop?.id]);

  useEffect(() => {
    let active = true;
    setOwnerState("loading");
    if (!ownerId) return () => { active = false; };
    void bindDriverOwner(ownerId).then((bound) => {
      if (active) setOwnerState(bound ? "ready" : "blocked");
    }).catch(() => { if (active) setOwnerState("blocked"); });
    return () => { active = false; };
  }, [ownerId]);

  useEffect(() => {
    return installSafeStopLock({ document, window, onLock: setSafeStopped });
  }, []);

  useEffect(() => {
    if (ownerState !== "ready" || !ownerId || !token) return;
    const reportHealth = async () => {
      if (!navigator.onLine || queueHealthReport.current.inFlight) return;
      const now = Date.now();
      if (queueHealthReport.current.ownerId === ownerId && now - queueHealthReport.current.attemptedAt < 15 * 60 * 1000) return;
      queueHealthReport.current.inFlight = true;
      try {
        if (await isPaused(ownerId) || !navigator.onLine) return;
        const buckets = offlineQueueHealth(await listQueue(ownerId));
        queueHealthReport.current = { ownerId, attemptedAt: now, inFlight: true };
        await apiJSON("/delivery/telemetry/offline-queue", token, { method: "POST", body: JSON.stringify(buckets) });
      } catch {
        // Queue telemetry is best-effort; it must never block route recovery.
      } finally {
        queueHealthReport.current.inFlight = false;
      }
    };
    const timer = window.setInterval(() => { void reportHealth(); }, 15 * 60 * 1000);
    window.addEventListener("online", reportHealth);
    void reportHealth();
    return () => {
      window.clearInterval(timer);
      window.removeEventListener("online", reportHealth);
    };
  }, [token, ownerState, ownerId]);

  async function refreshBanner() {
    if (ownerState !== "ready" || !ownerId) return;
    const queued = await listQueue(ownerId);
    setQueueCount(queued.length);
    setQueueRetention(queueRetentionWarning(queued));
    const paused = await isPaused(ownerId);
    if (paused) {
      setBanner({ kind: "paused", text: `Sync paused (401). ${queued.length} kept in IndexedDB.` });
      return;
    }
    if (!navigator.onLine) {
      setBanner({ kind: "offline", text: `No connection · ${queued.length} item(s) saved on this device` });
      return;
    }
    if (queued.length) setBanner({ kind: "syncing", text: `Syncing ${queued.length} saved item(s)…` });
      const result = await drainQueue(token, ownerId);
      setBanner(result);
      const remaining = await listQueue(ownerId);
      setQueueCount(remaining.length);
      setQueueRetention(queueRetentionWarning(remaining));
    const currentDetail = detailRef.current;
    if (result.kind === "ok" && queued.length > 0 && remaining.length === 0 && currentDetail) {
      await refreshTripFromServer(currentDetail.tripId);
    }
  }

  async function refreshTripFromServer(tripId: string) {
    if (!navigator.onLine) return false;
    try {
      const latest = await apiJSON<DeliveryTripDetail>(`/delivery/trips/${tripId}`, token);
      setDetail(latest);
      if (stop) setStop(latest.stops.find((item) => item.id === stop.id) || null);
      await cacheDetail(latest, ownerId, true);
      setSummarySource("server");
      const queue = await listQueue(ownerId);
      setQueueCount(queue.length);
      setQueueRetention(queueRetentionWarning(queue));
      setError("");
      return true;
    } catch {
      setSummarySource("device");
      setError(t("Updates are saved, but the server summary could not be refreshed. Retry when connected."));
      return false;
    }
  }

  async function markServiceUnavailable() {
    const queued = await listQueue(ownerId);
    setQueueCount(queued.length);
    setQueueRetention(queueRetentionWarning(queued));
    setBanner({ kind: "offline", text: `Delivery service unavailable · ${queued.length} unsynced item(s) remain saved on this device` });
  }

  useEffect(() => {
    const onOff = () => {
      void refreshBanner();
    };
    window.addEventListener("online", onOff);
    window.addEventListener("offline", onOff);
    void refreshBanner();
    return () => {
      window.removeEventListener("online", onOff);
      window.removeEventListener("offline", onOff);
    };
  }, [token, ownerState, ownerId]);

  async function run(fn: () => Promise<void>) {
    setError("");
    try {
      await fn();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        await setPaused(ownerId, true);
        setBanner({ kind: "paused", text: "Sync paused (401). Queue kept." });
        return;
      }
      if (e instanceof TypeError || (e instanceof ApiError && e.status >= 500)) {
        await markServiceUnavailable();
        setError(t("Could not reach the delivery service. Showing saved trip data where available."));
        return;
      }
      setError(e instanceof ApiError ? `${e.status}: ${t(e.message)}` : t("Update failed. Please try again."));
    }
  }

  async function loadTrips(e?: FormEvent) {
    e?.preventDefault();
    await run(async () => {
      try {
        const body = await apiJSON<{ items: DeliveryTripSummary[] }>(`/delivery/trips?date=${date}`, token);
        const items = body.items || [];
        setTrips(items);
        await putCachedTrips(ownerId, items);
        setDetail(null);
        setStop(null);
      } catch (err) {
        if (!shouldUseCachedDriverData(err)) throw err;
        const cached = await getCachedTrips(ownerId);
        if (cached.length) {
          setTrips(cached);
          throw err;
        }
        throw err;
      }
      await refreshBanner();
    });
  }

  async function openTrip(tripId: string) {
    setOnboard({});
    await run(async () => {
      try {
        const body = await apiJSON<DeliveryTripDetail>(`/delivery/trips/${tripId}`, token);
        setDetail(body);
        setSummarySource("server");
        await cacheDetail(body, ownerId, true);
        if (navigator.onLine) {
          const messageBody = await apiJSON<{items: TripMessage[]}>(`/delivery/trips/${tripId}/messages`,token);
          setMessages(messageBody.items || []);
        }
      } catch (err) {
        if (!shouldUseCachedDriverData(err)) throw err;
        const cached = await getCachedDetail(ownerId, tripId);
        if (cached) {
          setDetail(cached);
          setSummarySource("device");
          if (err instanceof ApiError && err.status === 401) {
            await setPaused(ownerId, true);
            setBanner({ kind: "paused", text: "Sync paused (401). Queue kept." });
          } else if (err instanceof TypeError || (err instanceof ApiError && err.status >= 500)) {
            await markServiceUnavailable();
          }
        } else {
          throw err;
        }
      }
      const queued = await listQueue(ownerId);
      const completionQueued = hasQueuedRouteCompletion(queued, tripId);
      setQueueCount(queued.length);
      setQueueRetention(queueRetentionWarning(queued));
      setEndedLocally(completionQueued);
      setShowSummary(completionQueued);
      setStop(null);
    });
  }

  async function acknowledgeMessage(messageId:string) {
    if (!detail) return;
    await run(async()=>{
      const result=await apiJSON<{message:TripMessage}>(`/delivery/trips/${detail.tripId}/messages/${messageId}/ack`,token,{method:"POST"});
      setMessages(items=>items.map(m=>m.id===messageId?result.message:m));
    });
  }

  async function acknowledgePlan() {
    if (!detail?.run?.planId || !detail.currentPlanVersion) return;
    if (detail.currentPlanVersion !== detail.run.planVersion) { setError(t("This route uses a superseded plan. Ask dispatch to refresh the trip instructions before starting.")); return; }
    try {
      await apiJSON(`/planning/plans/${detail.run.planId}/acknowledgements`,token,{method:"POST",body:JSON.stringify({version:detail.currentPlanVersion})});
      setError(""); await openTrip(detail.tripId);
    } catch(e) { setError(e instanceof ApiError ? `${e.status}: ${t(e.message)}` : t("Update failed. Please try again.")); }
  }

  async function checkOutTruck() {
    if (!detail?.run?.planVersion || !detail.loadList?.length || detail.currentPlanVersion !== detail.run.planVersion) return;
    const items = detail.loadList;
    if (!items.every(item => onboard[item.orderId])) return;
    await run(async () => {
      const result = await apiJSON<{ checkout: NonNullable<DeliveryTripDetail["checkout"]> }>(
        "/delivery/trips/" + encodeURIComponent(detail.tripId) + "/checkout", token, {
          method: "POST",
          body: JSON.stringify({
            planVersion: detail.run.planVersion,
            confirmedOrderIds: items.filter(item => onboard[item.orderId] === "yes").map(item => item.orderId),
          }),
        });
      setDetail(current => current?.tripId === detail.tripId ? { ...current, checkout: result.checkout } : current);
    });
  }

  async function startRun() {
    if (!detail || detail.checkout?.status !== "confirmed" || detail.checkout.planVersion !== detail.run.planVersion) return;
    await run(async () => {
      const operationId = newOperationId();
      try {
        const body = await apiJSON<DeliveryTripDetail>(`/delivery/trips/${detail.tripId}/start`, token, {
          method: "POST",
          body: JSON.stringify({ operationId }),
          headers: { "Idempotency-Key": operationId },
        });
        setDetail(body);
        await cacheDetail(body, ownerId);
      } catch (e) {
        if (e instanceof ApiError) {
          if (e.status === 401) { await setPaused(ownerId, true); setBanner({ kind: "paused", text: "Sync paused (401). Queue kept." }); return; }
          else if (e.status < 500) { setError(`${e.status}: ${t(e.message)}`); return; }
        }
        await enqueue(ownerId, { operationId, type: "START", tripId: detail.tripId, createdAt: new Date().toISOString() });
        const started = { ...detail, status: "in_progress", run: { ...detail.run, status: "in_progress" } };
        setDetail(started);
        setSummarySource("device");
        await cacheDetail(started, ownerId);
        await refreshBanner();
      }
    });
  }

  async function arrive() {
    if (!detail || !stop) return;
    await transitionLock.current(async () => {
      setTransitionBusy(true);
      try {
        if(!await queueTechCustody("DISPATCHED"))return;
        const operationId = newOperationId();
        const occurredAt = new Date().toISOString();
        await enqueue(ownerId, {
          operationId,
          type: "ARRIVED",
          tripId: detail.tripId,
          stopId: stop.id,
          payload: { occurredAt },
          createdAt: occurredAt,
        });
        setStop({ ...stop, status: "arrived", arrivedAt: occurredAt });
        setSummarySource("device");
        const updated = { ...detail, stops: detail.stops.map((s) => s.id === stop.id ? { ...s, status: "arrived", arrivedAt: occurredAt } : s) };
        setDetail(updated);
        await cacheDetail(updated, ownerId);
        await refreshBanner();
      } finally {
        setTransitionBusy(false);
      }
    });
  }

  async function queueTechCustody(stage:"DISPATCHED"|"DELIVERED",evidenceRef="") {
    if(!stop||stop.brand?.toLowerCase()!=="tech")return true;
    const fields=techCustody[stop.id]||{sealId:"",serials:"",condition:""};
    const serialNumbers=fields.serials.split(/[\n,;]/).map(value=>value.trim()).filter(Boolean);
    if(!fields.sealId.trim()||!serialNumbers.length||!fields.condition.trim()) { setError(t("Tech custody requires a seal ID, serial number, and condition.")); return false; }
    if(stage==="DELIVERED"&&!evidenceRef){setError(t("A condition photo is required for high-value Tech delivery."));return false;}
    const operationId=newOperationId();const createdAt=new Date().toISOString();
    await enqueue(ownerId,{operationId,type:"CUSTODY_RECORD",tripId:detail?.tripId||"",stopId:stop.id,orderId:stop.orderId,payload:{stage,sealId:fields.sealId.trim(),serialNumbers,condition:fields.condition.trim(),evidenceRef},createdAt});
    await refreshBanner();
    return true;
  }

  async function queueProof(blob: Blob, mimeType: string, proofType: "SIGNATURE" | "PHOTO") {
    if (!detail || !stop) return;
    const operationId = newOperationId();
    await enqueue(ownerId, {
      operationId,
      type: "PROOF_UPLOAD",
      tripId: detail.tripId,
      stopId: stop.id,
      blob,
      mimeType,
      proofType,
      receiverName: receiverName.trim() || undefined,
      createdAt: new Date().toISOString(),
    });
    setProofIds((prev) => ({ ...prev, [stop.id]: operationId }));
    if(proofType==="PHOTO")setPhotoProofIds(prev=>({...prev,[stop.id]:operationId}));
    setSummarySource("device");
    await refreshBanner();
    return operationId;
  }

  async function captureSignature() {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/png"));
    if (!blob) return;
    await queueProof(blob, "image/png", "SIGNATURE");
  }

  async function onPhoto(file: File | undefined) {
    if (!file) return;
    await queueProof(file, file.type || "image/jpeg", "PHOTO");
  }

  async function recordTemperature() {
    if (!detail || !stop || !Number.isFinite(Number(temperatureC)) || temperatureC.trim()==="") { setError(t("Enter a valid temperature in °C.")); return; }
    const valueC=Number(temperatureC);
    if(valueC < -40 || valueC > 100) { setError(t("Temperature must be between -40 and 100 °C.")); return; }
    const operationId=newOperationId(); const occurredAt=new Date().toISOString();
    const reading={operationId,valueC,unit:"C" as const,occurredAt,actorId:profile?.userId||"",source:"manual" as const,evaluation:"PENDING_SYNC" as const};
    await enqueue(ownerId,{operationId,type:"TEMPERATURE_READING",tripId:detail.tripId,stopId:stop.id,payload:{valueC,occurredAt},createdAt:occurredAt});
    const updated={...detail,stops:detail.stops.map((s)=>s.id===stop.id?{...s,temperatureReadings:[...(s.temperatureReadings||[]),reading]}:s)};
    setDetail(updated);setStop(updated.stops.find((s)=>s.id===stop.id)||null);setTemperatureC("");setSummarySource("device");await cacheDetail(updated,ownerId);await refreshBanner();
  }

  async function reportIncident(e: FormEvent) {
    e.preventDefault();
    if (!detail || incidentDescription.trim() === "") { setError(t("Describe the incident before reporting it.")); return; }
    const operationId = newOperationId();
    const occurredAt = new Date().toISOString();
    await enqueue(ownerId, {
      operationId,
      type: "INCIDENT_REPORT",
      tripId: detail.tripId,
      stopId: stop?.id,
      payload: { category: incidentCategory, description: incidentDescription.trim(), occurredAt },
      createdAt: occurredAt,
    });
    setIncidentDescription("");
    setSummarySource("device");
    await refreshBanner();
  }

  async function outcome(code: "DELIVERED" | "PARTIAL" | "NOT_DELIVERED" | "FAILED" | "REFUSED") {
    if (!detail || !stop) return;
    await transitionLock.current(async () => {
      setTransitionBusy(true);
      try {
        if (["NOT_DELIVERED", "FAILED", "REFUSED"].includes(code) && !reason) {
          setError(t("Choose a reason for the unsuccessful delivery."));
          return;
        }
        if (code === "REFUSED" && (!returnGoods.trim() || !Number.isInteger(Number(returnUnits)) || Number(returnUnits) < 1)) {
          setError(t("Enter the returned goods and quantity."));
          return;
        }
        const plannedUnits = detail.loadList?.find(item => item.orderId === stop.orderId)?.expectedUnits;
        if (code === "REFUSED" && plannedUnits !== undefined && Number(returnUnits) > plannedUnits) {
          setError(`${t("Quantity returning")}: 1-${plannedUnits}`);
          return;
        }
        const queued = await listQueue(ownerId);
        const proof = [...queued].reverse().find((q) => q.type === "PROOF_UPLOAD" && q.stopId === stop.id);
        const depends = proofIds[stop.id] || proof?.operationId;
        if ((code === "DELIVERED" || code === "PARTIAL") && !depends) {
          setError(t("Capture a signature or photo before DELIVERED or PARTIAL."));
          return;
        }
        if(stop.brand?.toLowerCase()==="tech"&&(code==="DELIVERED"||code==="PARTIAL")){
          const photo=[...queued].reverse().find(q=>q.type==="PROOF_UPLOAD"&&q.stopId===stop.id&&q.proofType==="PHOTO");
          const photoReference=photoProofIds[stop.id]||photo?.operationId;
          if(!photoReference){setError(t("A condition photo is required for high-value Tech delivery."));return;}
          if(!await queueTechCustody("DELIVERED",photoReference))return;
        }
        const operationId = newOperationId();
        await enqueue(ownerId, {
          operationId,
          type: "STOP_OUTCOME",
          tripId: detail.tripId,
          stopId: stop.id,
          dependsOnOperationId: depends,
          payload: { code, reason, note, occurredAt: new Date().toISOString(), ...(code === "REFUSED" ? { returnedGoods: { goods: returnGoods.trim(), units: Number(returnUnits), resolution: returnResolution } } : {}) },
          createdAt: new Date().toISOString(),
        });
        setStop({ ...stop, status: "completed", outcomeCode: code });
        setSummarySource("device");
        const updated = { ...detail, stops: detail.stops.map((s) => s.id === stop.id ? { ...s, status: "completed", outcomeCode: code, outcomeReason: reason } : s) };
        setDetail(updated);
        await cacheDetail(updated, ownerId);
        setReason("");
        setNote("");
        setReturnGoods("");
        setReturnUnits("");
        await refreshBanner();
      } finally {
        setTransitionBusy(false);
      }
    });
  }

  async function completeRun() {
    if (!detail) return;
    const unresolved = detail.stops.filter((s) => s.status !== "completed");
    if (unresolved.length) {
      setError(`${unresolved.length} stop(s) still need an outcome.`);
      setShowSummary(true);
      return;
    }
    await enqueue(ownerId, {
      operationId: newOperationId(),
      type: "ROUTE_COMPLETED",
      tripId: detail.tripId,
      createdAt: new Date().toISOString(),
    });
    setSummarySource("device");
    await refreshBanner();
    setEndedLocally(true);
    setShowSummary(true);
  }

  async function exportSavedData() {
    try {
      setError("");
      setPrivacyNotice("");
      const data = await readDriverDataForExport(ownerId);
      const content = await createDriverDataExport({ subject: ownerId, ...data });
      const url = URL.createObjectURL(new Blob([content], { type: "application/json" }));
      const link = document.createElement("a");
      link.href = url;
      link.download = "waypoint-driver-offline-data.json";
      link.click();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      setPrivacyNotice(t("Saved delivery data downloaded."));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  async function clearCompletedCache() {
    if (!window.confirm(t("Clear completed trip cache from this device? Pending offline work and active routes will be kept."))) return;
    try {
      setError("");
      setPrivacyNotice("");
      const result = await clearCompletedDriverCache(ownerId);
      if (result.pendingQueueCount > 0) {
        setPrivacyNotice(t("Pending offline work prevents cache clearing. Export or sync it first."));
        return;
      }
      if (result.clearedTripIds.length === 0) {
        setPrivacyNotice(t("No completed trip cache was available to clear."));
        return;
      }
      const removed = new Set(result.clearedTripIds);
      setTrips(current => current.filter(trip => !removed.has(trip.tripId)));
      if (detail && removed.has(detail.tripId)) {
        setDetail(null);
        setStop(null);
        setMessages([]);
      }
      setPrivacyNotice(t("Completed trip cache cleared."));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  function draw(e: React.PointerEvent<HTMLCanvasElement>) {
    const canvas = canvasRef.current;
    if (!safeStopped) return;
    if (!canvas || !drawing.current) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const r = canvas.getBoundingClientRect();
    ctx.lineTo(e.clientX - r.left, e.clientY - r.top);
    ctx.stroke();
  }

  const inProgress = detail?.status === "in_progress" || detail?.run?.status === "in_progress";
  const completed = detail?.status === "completed" || detail?.run?.status === "completed";
  const nextStop = detail?.stops.filter((s) => s.status !== "completed").sort((a, b) => a.stopSequence - b.stopSequence)[0];
  const driverAcknowledged = !!profile?.userId && !!detail?.planAcknowledgements?.some((a) => a.actorId===profile.userId && a.actorRole==="DRIVER") && detail?.currentPlanVersion===detail?.run?.planVersion;

  if (ownerState === "loading") return <p className="card" role="status">{t("Checking this device's driver account…")}</p>;
  if (ownerState === "blocked") return <section className="card" role="alert"><h2>{t("Driver data is locked to another account")}</h2><p>{t("This device's saved driver data belongs to another account. Sign in with the original driver account to recover it.")}</p></section>;

  return (
    <section className="card loader-shell">
      <h2>{t("Driver")}</h2>
      <p className="muted">{profile?.vehicleId ? `${t("Vehicle")} ${profile.vehicleId}` : t("Vehicle from profile")} · {t("FIFO offline queue")}</p>
      <p className="muted">{t("Do not use this screen while the vehicle is moving. Pull over and park safely first.")}</p>
      <label className="driver-safe-stop-toggle">
        <input type="checkbox" checked={safeStopped} onChange={(event) => setSafeStopped(event.target.checked)} />
        <span>{t("Confirm safely stopped to continue")}</span>
      </label>
      {safeStopped && <p className="status-ok" role="status">{t("Driver actions unlocked. Uncheck to lock them before moving; this app does not detect vehicle motion.")}</p>}
      <fieldset className="driver-safe-actions" disabled={!safeStopped}>
      {banner.text && <p className={banner.kind === "ok" ? "status-ok" : banner.kind === "syncing" ? "status-syncing" : "status-bad"} role="status">{t(banner.text)}</p>}
      {queueRetention && <p className={queueRetention.level === "critical" ? "status-bad" : "status-syncing"} role="alert">{t(queueRetention.text)}</p>}
      {token && banner.kind !== "paused" && <button type="button" className="tap" onClick={async () => { setBanner({ kind: "syncing", text: t("Syncing saved work…") }); setBanner(await drainQueue(token, ownerId)); const queue = await listQueue(ownerId); setQueueCount(queue.length); setQueueRetention(queueRetentionWarning(queue)); }}>{t("Sync Now")}</button>}
      {banner.kind === "paused" && (
        <button type="button" className="tap" onClick={async () => { if (token) { setBanner(await resumeSync(token, ownerId)); const queue = await listQueue(ownerId); setQueueCount(queue.length); setQueueRetention(queueRetentionWarning(queue)); } }}>
          {t("Resume sync")}
        </button>
      )}
      {banner.kind === "error" && (
        <button type="button" className="tap" onClick={async () => { if (token) { setBanner(await drainQueue(token, ownerId)); const queue = await listQueue(ownerId); setQueueCount(queue.length); setQueueRetention(queueRetentionWarning(queue)); } }}>
          {t("Retry sync")}
        </button>
      )}
      {privacyNotice && <p className="status-ok" role="status">{privacyNotice}</p>}
      <div className="row">
        <button type="button" className="tap" onClick={() => void exportSavedData()}>{t("Export saved driver data")}</button>
        <button type="button" className="tap" onClick={() => void clearCompletedCache()}>{t("Clear completed trip cache")}</button>
      </div>
      <p className="muted">{t("Offline work is not cleared. Active trips are kept.")}</p>
      {error && <p className="status-bad" role="alert">{error}</p>}

      <form onSubmit={loadTrips} className="row">
        <label>
          {t("Delivery date")}
          <input type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
        </label>
        <button type="submit" className="tap">
          {t("Show trips")}
        </button>
      </form>

      {!detail && (
        <ul className="trip-list">
          {trips.map((trip) => (
            <li key={trip.tripId}>
              <button type="button" className="trip-card tap" onClick={() => openTrip(trip.tripId)}>
                <strong>
                  {trip.planRef || t("Plan")}{trip.tripNumber && trip.tripNumber > 0 ? ` · ${t("Trip")} ${trip.tripNumber}` : ""}
                </strong>
                <span>
                  {trip.vehicleId} · {depotLabel(trip.depot)}
                </span>
                <span>
                  {t(trip.status || trip.loadingStatus || "")} · {trip.completedStops ?? 0}/{trip.stopCount ?? 0} {t("stops")}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}

      {detail && !stop && (
        <>
          <button type="button" className="linkish" onClick={() => setDetail(null)}>
            ← {t("Trips")}
          </button>
          <h3>
            {detail.run?.planRef} · {detail.run?.vehicleId}
          </h3>
          {!completed && <>
            <p className="muted">{t("Plan version")} {detail.currentPlanVersion || detail.run?.planVersion || t("unavailable")} · {driverAcknowledged ? t("Acknowledged") : t("Acknowledge before starting this route")}</p>
            {detail.currentPlanVersion !== detail.run?.planVersion ? <p className="status-bad">{t("This prepared route is stale. Dispatch must refresh its trip instructions.")}</p> : !driverAcknowledged && <button type="button" className="tap" onClick={acknowledgePlan}>{t("Acknowledge current plan")}</button>}
          </>}
          <p>
            {depotLabel(detail.run?.depot)} · {t(detail.status || "")}
          </p>
          <p className="muted">{t("Route saved for offline use")}</p>
          {navigator.onLine && <section className="message-panel" aria-labelledby="driver-messages-heading">
            <h3 id="driver-messages-heading">{t("Dispatcher messages")}</h3>
            {messages.length===0?<p>{t("No trip messages.")}</p>:<ul>{messages.map(m=><li key={m.id}><p>{m.body}</p><small>{m.stopId?t("Stop-specific instruction"):t("Trip-wide instruction")} · {new Date(m.createdAt).toLocaleString()}</small>{m.acknowledgedAt?<p className="status-ok" role="status">{t("Acknowledged")}</p>:<button type="button" className="tap" onClick={()=>void acknowledgeMessage(m.id)}>{t("Acknowledge receipt")}</button>}</li>)}</ul>}
          </section>}
          <section aria-labelledby="driver-incident-heading">
            <h3 id="driver-incident-heading">{t("Report an incident")}</h3>
            <p className="muted">{t("Report a vehicle, road, outlet, goods, or safety problem. Dispatch sees this in the trip's history.")}</p>
            <form onSubmit={reportIncident} className="row">
              <select aria-label={t("Incident category")} value={incidentCategory} onChange={(e) => setIncidentCategory(e.target.value)}>
                <option value="VEHICLE">{t("Vehicle")}</option>
                <option value="ROAD">{t("Road")}</option>
                <option value="OUTLET">{t("Outlet")}</option>
                <option value="GOODS">{t("Goods")}</option>
                <option value="SAFETY">{t("Safety")}</option>
                <option value="OTHER">{t("Other")}</option>
              </select>
              <input aria-label={t("Incident description")} placeholder={t("What happened?")} value={incidentDescription} onChange={(e) => setIncidentDescription(e.target.value)} maxLength={1000} />
              <button type="submit" className="tap">{t("Report")}</button>
            </form>
          </section>
          <button type="button" className="tap" onClick={() => setShowSummary((show) => !show)}>
            {showSummary ? t("Back to route") : t("End-of-day summary")}
          </button>
          {showSummary ? (
            <div className="end-summary" aria-live="polite">
              <h3>{t("Trip summary")}</h3>
              <p className={summarySource === "server" ? "status-ok" : "status-syncing"} role="status">{summarySource === "server" ? t("Summary refreshed from the server") : t("Summary reflects saved work on this device")}</p>
              <button type="button" className="tap" onClick={() => void refreshTripFromServer(detail.tripId)} disabled={!navigator.onLine || queueCount > 0}>{t("Refresh server summary")}</button>
              <p>{detail.stops.filter((s) => s.outcomeCode === "DELIVERED" || s.outcomeCode === "PARTIAL").length} {t("delivered/partial")} · {detail.stops.filter((s) => s.status !== "completed" || ["NOT_DELIVERED", "FAILED", "REFUSED"].includes(s.outcomeCode || "")).length} {t("unresolved or needs follow-up")}</p>
              <ul className="load-list">
                {detail.stops.map((s) => <li key={s.id} className="load-item"><strong>{s.stopSequence}. {stopLabel(s)}</strong><span>{t(s.status)}{s.outcomeCode ? ` · ${t(s.outcomeCode)}` : ""}{s.outcomeReason ? ` · ${t(s.outcomeReason)}` : ""}</span></li>)}
              </ul>
              <p className={queueCount ? "status-bad" : "status-ok"}>
                {queueCount ? t(`${queueCount} upload(s)/event(s) pending on this device`) : t("No pending uploads · changes synced")}
              </p>
              {endedLocally && <p className={queueCount ? "muted" : "status-ok"}>
                {queueCount ? t("Trip completion is queued until sync confirms it.") : t("Trip completion synced with the server.")}
              </p>}
            </div>
          ) : <>
          {nextStop && <button type="button" className="next-stop tap" onClick={() => setStop(nextStop)} disabled={!inProgress}>
            <strong>{t("Next stop ·")} {nextStop.stopSequence}. {stopLabel(nextStop)}</strong>
            <span>{nextStop.plannedWindowOpen ? `${t("Window")} ${nextStop.plannedWindowOpen}–${nextStop.plannedWindowClose || ""} · ` : ""}{t("Tap only when safely stopped")}</span>
          </button>}
          {!inProgress && !completed && <section className="card" aria-label={t("Truck check-out")}>
            <h4>{t("Truck check-out")}</h4>
            {!detail.loadList?.length || detail.currentPlanVersion !== detail.run.planVersion ? <p role="alert">{t("Current load list unavailable. Refresh the trip before departure.")}</p> :
              <ul>{detail.loadList.map(item => <li key={item.orderId}>
                <span>{item.stopSequence}. {item.orderRef || item.orderId} - {item.expectedUnits} {t("units")}</span>
                <label><input type="radio" name={"onboard-" + item.orderId} checked={onboard[item.orderId] === "yes"} onChange={() => setOnboard(current => ({ ...current, [item.orderId]: "yes" }))} />{t("On board")}</label>
                <label><input type="radio" name={"onboard-" + item.orderId} checked={onboard[item.orderId] === "no"} onChange={() => setOnboard(current => ({ ...current, [item.orderId]: "no" }))} />{t("Missing")}</label>
              </li>)}</ul>}
            {detail.checkout?.status === "blocked" && <p role="alert" className="status-bad">{t("Check-out blocked. Loader and dispatcher alerted.")}</p>}
            {detail.checkout?.status === "confirmed" && detail.checkout.planVersion === detail.run.planVersion && <p role="status" className="status-ok">{t("Check-out recorded")} - {new Date(detail.checkout.checkedAt).toLocaleString()}</p>}
            {detail.checkout?.status !== "confirmed" && <button type="button" className="tap" onClick={checkOutTruck} disabled={!navigator.onLine || !driverAcknowledged || !detail.loadList?.length || !detail.loadList.every(item => onboard[item.orderId]) || detail.currentPlanVersion !== detail.run.planVersion}>{t("Confirm check-out")}</button>}
          </section>}
          {!inProgress && !completed && (
            <button type="button" className="tap primary" onClick={startRun} disabled={!driverAcknowledged || detail.checkout?.status !== "confirmed" || detail.checkout.planVersion !== detail.run.planVersion}>
              {t("Start run")}
            </button>
          )}
          <ul className="load-list">
            {(detail.stops || [])
              .slice()
              .sort((a, b) => a.stopSequence - b.stopSequence)
              .map((s) => (
                <li key={s.id} className={`load-item status-${s.status}`}>
                  <button type="button" className="tap" onClick={() => setStop(s)} disabled={!inProgress && !completed}>
                    <strong>
                      {s.stopSequence}. {stopLabel(s)}
                    </strong>
                    <span>
                      {t(s.status)}
                      {s.outcomeCode ? ` · ${t(s.outcomeCode)}` : ""}
                    </span>
                    {(s.loadingShortfallSummary || []).length > 0 && (
                      <span className="muted">{t("Loading shortfall on board")}</span>
                    )}
                  </button>
                </li>
              ))}
          </ul>
          {inProgress && (
            <button type="button" className="tap primary" onClick={completeRun}>
              {t("Finish trip")}
            </button>
          )}
          </>}
        </>
      )}

      {detail && stop && (
        <>
          <button type="button" className="linkish" onClick={() => setStop(null)}>
            ← {t("Stops")}
          </button>
          <h3>
            {stopLabel(stop)}{stop.orderRef ? ` · ${stop.orderRef}` : ""}
          </h3>
          <p>
            {t(stop.status)}
            {stop.outcomeCode ? ` · ${t(stop.outcomeCode)}` : ""}
          </p>
          <dl className="stop-guidance">
            {stop.plannedWindowOpen && <><dt>{t("Delivery window")}</dt><dd>{stop.plannedWindowOpen}–{stop.plannedWindowClose || ""}</dd></>}
            {stop.district && <><dt>{t("District")}</dt><dd>{stop.district}</dd></>}
            {stop.dockType && <><dt>{t("Access")}</dt><dd>{t(stop.dockType)}</dd></>}
            {stop.parkingConstraint && <><dt>{t("Parking")}</dt><dd>{t(stop.parkingConstraint.replace(/_/g, " "))}</dd></>}
            {stop.accessInstructions && <><dt>{t("Landmark and final approach")}</dt><dd className="driver-access-instructions">{stop.accessInstructions}</dd></>}
            {stop.accessInstructionsUpdatedAt && <><dt>{t("Access note last updated")}</dt><dd>{new Date(stop.accessInstructionsUpdatedAt).toLocaleDateString("en-LK", { timeZone: "Asia/Colombo" })}</dd></>}
            {stop.temperatureRequirement === "chilled" && <><dt>{t("Cold-chain limits")}</dt><dd>{stop.chilledTemperatureMinC !== undefined && stop.chilledTemperatureMaxC !== undefined ? `${stop.chilledTemperatureMinC}–${stop.chilledTemperatureMaxC} °C` : t("No outlet limits configured · readings will be flagged for review")}</dd></>}
          </dl>
          {stop.brand?.toLowerCase()==="tech"&&<fieldset className="scan-panel"><legend>{t("High-value Tech custody")}</legend><p>{t("Check the seal and serial numbers against the loading record. Record the condition at handoff; take a condition photo of the sealed item for the custody record.")}</p>
            <label>{t("Seal ID")}<input value={techCustody[stop.id]?.sealId||""} onChange={e=>setTechCustody(v=>({...v,[stop.id]:{sealId:e.target.value,serials:v[stop.id]?.serials||"",condition:v[stop.id]?.condition||""}}))} maxLength={100}/></label>
            <label>{t("Serial number(s), comma separated")}<textarea value={techCustody[stop.id]?.serials||""} onChange={e=>setTechCustody(v=>({...v,[stop.id]:{sealId:v[stop.id]?.sealId||"",serials:e.target.value,condition:v[stop.id]?.condition||""}}))} maxLength={12000}/></label>
            <label>{t("Delivery condition")}<input value={techCustody[stop.id]?.condition||""} onChange={e=>setTechCustody(v=>({...v,[stop.id]:{sealId:v[stop.id]?.sealId||"",serials:v[stop.id]?.serials||"",condition:e.target.value}}))} maxLength={500}/></label>
          </fieldset>}
          {stop.status === "pending" && (
            <button type="button" className="tap primary" disabled={transitionBusy} onClick={arrive}>
              {t("Arrive")}
            </button>
          )}
          {stop.status === "arrived" && (
            <>
              {stop.temperatureRequirement === "chilled" && <section aria-label={t("Chilled goods temperature")}><h4>{t("Chilled goods temperature")}</h4><p className="muted">{t("Use a calibrated manual thermometer at the outlet. This records evidence; configured outlet limits determine whether it is in range.")}</p><label>{t("Temperature (°C)")}<input type="number" min="-40" max="100" step="0.1" inputMode="decimal" value={temperatureC} onChange={(e)=>setTemperatureC(e.target.value)} /></label><button type="button" className="tap" onClick={recordTemperature}>{t("Record temperature")}</button><ul aria-live="polite">{(stop.temperatureReadings||[]).map((reading)=><li key={reading.operationId} className={reading.evaluation==="OUT_OF_RANGE"?"status-bad":""} role={reading.evaluation==="OUT_OF_RANGE"?"alert":"status"}>{reading.valueC.toFixed(1)} °C · {t(reading.evaluation)} · {new Date(reading.occurredAt).toLocaleString("en-LK",{timeZone:"Asia/Colombo"})} {reading.actorId&&`· ${reading.actorId}`}</li>)}</ul></section>}
          <p className="muted">{t("Capture proof for delivered or partial orders. PNG/JPEG only. Not delivered requires a reason.")}</p>
              {proofIds[stop.id] && <p className={queueCount ? "muted" : "status-ok"} role="status">{queueCount ? t("Proof saved on this device · waiting to sync") : t("Proof synced")}</p>}
              <label>{t("Recipient name (optional)")}<input type="text" maxLength={120} value={receiverName} onChange={(e) => setReceiverName(e.target.value)} disabled={!safeStopped} /></label>
              <canvas
                ref={canvasRef}
                className="sig"
                role="img"
                aria-label={t("Signature drawing area. Draw the receiver signature here.")}
                width={320}
                height={140}
                onPointerDown={(e) => {
                  if (!safeStopped) return;
                  drawing.current = true;
                  const canvas = canvasRef.current;
                  const ctx = canvas?.getContext("2d");
                  if (!canvas || !ctx) return;
                  const r = canvas.getBoundingClientRect();
                  ctx.beginPath();
                  ctx.moveTo(e.clientX - r.left, e.clientY - r.top);
                }}
                onPointerMove={draw}
                onPointerUp={() => {
                  drawing.current = false;
                }}
              />
              <div className="row">
                <button type="button" className="tap" onClick={captureSignature}>
                  {t("Save signature")}
                </button>
                <label className="tap">
                  {t("Photo")}
                  <input type="file" accept="image/png,image/jpeg" capture="environment" className="visually-hidden" onChange={(e) => onPhoto(e.target.files?.[0])} />
                </label>
              </div>
              <label>
                {t("Reason code (required for not delivered)")}
                <select value={reason} onChange={(e) => setReason(e.target.value)}>
                  <option value="">{t("Select a reason")}</option>
                  <option value="OUTLET_CLOSED">{t("Outlet closed")}</option>
                  <option value="ACCESS_BLOCKED">{t("Access blocked")}</option>
                  <option value="RECEIVER_UNAVAILABLE">{t("Receiver unavailable")}</option>
                  <option value="GOODS_REJECTED">{t("Goods rejected")}</option>
                  <option value="VEHICLE_ISSUE">{t("Vehicle issue")}</option>
                  <option value="OTHER">{t("Other")}</option>
                </select>
              </label>
              <label>
                {t("Optional note")}
                <input value={note} onChange={(e) => setNote(e.target.value)} />
              </label>
              <fieldset>
                <legend>{t("Rejected goods / take-back")}</legend>
                <label>{t("Goods or items being returned")}<input value={returnGoods} onChange={(e) => setReturnGoods(e.target.value)} maxLength={200} /></label>
                <label>{t("Quantity returning")}<input type="number" min="1" max={detail.loadList?.find(item => item.orderId === stop.orderId)?.expectedUnits} step="1" value={returnUnits} onChange={(e) => setReturnUnits(e.target.value)} /></label>
                <label>{t("Follow-up choice")}
                  <select value={returnResolution} onChange={(e) => setReturnResolution(e.target.value as "NEXT_RUN" | "REQUEST_DEFERRAL")}>
                    <option value="NEXT_RUN">{t("Re-attempt on next run")}</option>
                    <option value="REQUEST_DEFERRAL">{t("Request dispatcher deferral")}</option>
                  </select>
                </label>
              </fieldset>
              <div className="row">
                <button type="button" className="tap" disabled={transitionBusy} onClick={() => outcome("DELIVERED")}>
                  {t("Delivered")}
                </button>
                <button type="button" className="tap" disabled={transitionBusy} onClick={() => outcome("PARTIAL")}>
                  {t("Partial")}
                </button>
                <button type="button" className="tap" disabled={transitionBusy} onClick={() => outcome("FAILED")}>
                  {t("Failed")}
                </button>
                <button type="button" className="tap" disabled={transitionBusy} onClick={() => outcome("REFUSED")}>
                  {t("Refused")}
                </button>
              </div>
            </>
          )}
        </>
      )}
      </fieldset>
    </section>
  );
}
