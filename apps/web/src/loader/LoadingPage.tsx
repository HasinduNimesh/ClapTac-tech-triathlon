import { FormEvent, useEffect, useRef, useState } from "react";
import { ApiError, apiJSON } from "../api/client";
import { TruckCheckout } from "../api/delivery";
import { todayLocal } from "../api/date";
import { depotLabel, isIncomplete, LoadingTripDetail, LoadingTripSummary, newIdempotencyKey } from "../api/loading";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { enqueueLoader, bindLoaderOwner, cacheLoaderDetail, cacheLoaderTrips, getCachedLoaderDetail, getCachedLoaderTrips, listLoaderQueue } from "./offlineDb";
import { drainLoaderQueue, LoaderSyncState } from "./offlineSync";
import { applyLoadingQueueItem, replayLoadingQueue, shouldQueueLoadingFailure, LoadingQueueOperation } from "./offlineState.mjs";
import { resolveLoadOrderCode } from "./barcodeLookup.mjs";
import { acquireBarcodeCamera } from "./barcodeCamera.mjs";

export function LoadingPage() {
  const { user, profile } = useAuth();
  const { t } = useLocale();
  const token = user?.access_token || "";
  const ownerId = profile?.userId || "";
  const [ownerState, setOwnerState] = useState<"loading" | "ready" | "blocked">("loading");
  const [date, setDate] = useState(todayLocal);
  const [trips, setTrips] = useState<LoadingTripSummary[]>([]);
  const [detail, setDetail] = useState<LoadingTripDetail | null>(null);
  const [checkoutAlert, setCheckoutAlert] = useState<TruckCheckout | null>(null);
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const [issueType, setIssueType] = useState("MISSING");
  const [issueUnits, setIssueUnits] = useState("1");
  const [issueNote, setIssueNote] = useState("");
  const [issueOrderId, setIssueOrderId] = useState("");
  const [techCustody, setTechCustody] = useState<Record<string,{sealId:string;serials:string;condition:string;evidenceRef:string}>>({});
  const [scanCode, setScanCode] = useState("");
  const [scannerOpen, setScannerOpen] = useState(false);
  const [scannerMessage, setScannerMessage] = useState("");
  const [online, setOnline] = useState(() => navigator.onLine);
  const [queueCount, setQueueCount] = useState(0);
  const [syncState, setSyncState] = useState<LoaderSyncState>({ kind: "ok", text: "" });
  const videoRef = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    let active = true;
    setOwnerState("loading");
    if (!ownerId) return () => { active = false; };
    void bindLoaderOwner(ownerId).then(async bound => {
      if (!active) return;
      if (!bound) { setOwnerState("blocked"); return; }
      setOwnerState("ready");
      setQueueCount((await listLoaderQueue(ownerId)).length);
      if (navigator.onLine) void drainLoaderQueue(token, ownerId).then(async result => {
        if (!active) return;
        setSyncState(result);
        setQueueCount((await listLoaderQueue(ownerId)).length);
      }).catch(e => { if (active) setError(String(e)); });
    }).catch(() => { if (active) setOwnerState("blocked"); });
    return () => { active = false; };
  }, [ownerId]);

  useEffect(() => {
    if (!detail?.tripId || !token || !online) { setCheckoutAlert(null); return; }
    setCheckoutAlert(null);
    let active = true;
    const tripId = detail.tripId;
    const loadAlert = async () => {
      try {
        const result = await apiJSON<{ checkout: TruckCheckout | null }>("/delivery/trips/" + encodeURIComponent(tripId) + "/checkout", token);
        if (active) setCheckoutAlert(result.checkout?.status === "blocked" ? result.checkout : null);
      } catch { if (active) setCheckoutAlert(null); }
    };
    void loadAlert();
    const timer = window.setInterval(() => { void loadAlert(); }, 15_000);
    return () => { active = false; window.clearInterval(timer); };
  }, [detail?.tripId, token, online]);

  async function refreshFromServer(tripId: string) {
    if (!navigator.onLine) throw new TypeError("Connection lost while refreshing the loading manifest.");
    const latest = await apiJSON<LoadingTripDetail>(`/loading/trips/${encodeURIComponent(tripId)}`, token);
    await cacheLoaderDetail(ownerId, latest);
    const queued = await listLoaderQueue(ownerId);
    setQueueCount(queued.length);
    displayDetail(replayLoadingQueue(latest, queued.filter(item => item.tripId === tripId)));
  }

  function displayDetail(next: LoadingTripDetail) {
    setDetail(next);
    const loadedCount = next.orders?.filter(order => order.status === "loaded").length || 0;
    const shortfallCount = next.orders?.filter(order => order.status === "shortfall").length || 0;
    const pendingCount = next.orders?.filter(order => isIncomplete(order)).length || 0;
    setTrips(current => {
      const updated = current.map(trip => trip.tripId === next.tripId ? {
        ...trip,
        loadingStatus: next.loadingStatus || next.status || trip.loadingStatus,
        loadedCount,
        shortfallCount,
        pendingCount,
      } : trip);
      void cacheLoaderTrips(ownerId, date, updated);
      return updated;
    });
  }

  async function syncPending() {
    if (ownerState !== "ready" || !token || !ownerId) return;
    const result = await drainLoaderQueue(token, ownerId);
    const queued = await listLoaderQueue(ownerId);
    setQueueCount(queued.length);
    setSyncState(result);
    if (result.kind === "ok" && detail) await refreshFromServer(detail.tripId);
  }

  useEffect(() => {
    const onOnline = () => { setOnline(true); void syncPending().catch(e => setError(String(e))); };
    const onOffline = () => { setOnline(false); setSyncState({ kind: "offline", text: "Connection lost. Loading work will be saved on this device." }); };
    window.addEventListener("online", onOnline);
    window.addEventListener("offline", onOffline);
    return () => { window.removeEventListener("online", onOnline); window.removeEventListener("offline", onOffline); };
  }, [ownerState, ownerId, token, detail]);

  async function operationId() {
    return typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `load-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }

  async function performOrQueue(item: LoadingQueueOperation, send: () => Promise<unknown>) {
    if (!detail) return false;
    try {
      await send();
      await refreshFromServer(detail.tripId);
      setSyncState({ kind: "ok", text: "" });
      return false;
    } catch (e) {
      if (!shouldQueueLoadingFailure(e, navigator.onLine)) throw e;
      await enqueueLoader(ownerId, item);
      const next = applyLoadingQueueItem(detail, item);
      await cacheLoaderDetail(ownerId, next);
      const queued = await listLoaderQueue(ownerId);
      displayDetail(next);
      setQueueCount(queued.length);
      setOnline(navigator.onLine);
      setSyncState({ kind: "offline", text: "Loading change saved on this device and will sync in order when connected." });
      return true;
    }
  }

  async function confirmScannedCode(raw: string) {
    const result = resolveLoadOrderCode(orders, raw, inProgress);
    if (result.kind === "not_found") { setScannerMessage(t("Code not found in this trip. Check the trip and enter the order reference manually.")); return; }
    if (result.kind === "not_loading") { setScannerMessage(t("Start loading before confirming scanned orders.")); return; }
    if (result.kind === "unresolved_shortfall") { setScannerMessage(t("This order has an unresolved shortfall. Resolve it before marking loaded.")); return; }
    setScannerOpen(false);
    setScannerMessage(`${t("Matched")} ${result.order.orderRef || result.order.orderId}. ${t("Confirming load…")}`);
    await markLoaded(result.order.orderId);
  }

  useEffect(() => {
    if (!scannerOpen) return;
    let stopCamera: (() => void) | undefined;
    let frame = 0;
    let active = true;
    async function begin() {
      try {
        const Detector = (window as unknown as { BarcodeDetector?: new (options?: { formats: string[] }) => { detect(source: HTMLVideoElement): Promise<{ rawValue: string }[]> } }).BarcodeDetector;
        if (!Detector) { setScannerMessage(t("Camera scanning is unavailable on this browser. Use the manual order-reference field or the Loaded button.")); return; }
        stopCamera = await acquireBarcodeCamera(navigator.mediaDevices, videoRef.current, () => active);
        if (!stopCamera || !active || !videoRef.current) return;
        const detector = new Detector({ formats: ["qr_code", "code_128", "ean_13", "ean_8", "upc_a", "upc_e"] });
        const detect = async () => {
          if (!active || !videoRef.current) return;
          try { const results = await detector.detect(videoRef.current); if (results[0]?.rawValue) { await confirmScannedCode(results[0].rawValue); return; } }
          catch { setScannerMessage(t("Could not read a code. Adjust the camera or enter the order reference manually.")); }
          frame = requestAnimationFrame(() => void detect());
        };
        frame = requestAnimationFrame(() => void detect());
      } catch {
        setScannerMessage(t("Camera permission was denied or unavailable. Use the manual order-reference field or the Loaded button."));
      }
    }
    void begin();
    return () => { active = false; cancelAnimationFrame(frame); stopCamera?.(); };
  }, [scannerOpen, detail?.tripId, t]);

  async function run(label: string, fn: () => Promise<void>) {
    setError("");
    setInfo("");
    try {
      await fn();
      if (label) setInfo(t(label));
    } catch (e) {
      setError(e instanceof ApiError ? `${e.status}: ${e.message}` : String(e));
    }
  }

  async function loadTrips(e?: FormEvent) {
    e?.preventDefault();
    await run("", async () => {
      try {
        const body = await apiJSON<{ items: LoadingTripSummary[] }>(`/loading/trips?date=${date}`, token);
        setTrips(body.items || []);
        await cacheLoaderTrips(ownerId, date, body.items || []);
        setDetail(null);
        setInfo(t("Trips loaded"));
      } catch (e) {
        if (!shouldQueueLoadingFailure(e, navigator.onLine)) throw e;
        const cached = await getCachedLoaderTrips(ownerId, date);
        setTrips(cached);
        setDetail(null);
        setSyncState({ kind: "offline", text: "Showing the most recently saved trip list on this device." });
        setInfo(t("Offline trip list loaded from this device"));
      }
    });
  }

  async function openTrip(tripId: string) {
    await run("", async () => {
      try {
        const body = await apiJSON<LoadingTripDetail>(`/loading/trips/${encodeURIComponent(tripId)}`, token);
        await cacheLoaderDetail(ownerId, body);
        const queued = await listLoaderQueue(ownerId);
        setQueueCount(queued.length);
        displayDetail(replayLoadingQueue(body, queued.filter(item => item.tripId === tripId)));
      } catch (e) {
        if (!shouldQueueLoadingFailure(e, navigator.onLine)) throw e;
        const cached = await getCachedLoaderDetail(ownerId, tripId);
        if (!cached) throw new Error("This trip has not been saved on this device. Connect to load its latest plan and manifest.");
        const queued = await listLoaderQueue(ownerId);
        setQueueCount(queued.length);
        displayDetail(replayLoadingQueue(cached, queued.filter(item => item.tripId === tripId)));
        setSyncState({ kind: "offline", text: "Showing the saved manifest. Check the plan version again before departure." });
      }
    });
  }

  async function start() {
    if (!detail) return;
    await run("", async () => {
      if (!detail.planVersion) throw new Error("Load a published plan version before starting this trip.");
      const item: LoadingQueueOperation = { operationId: await operationId(), type: "START", tripId: detail.tripId, planVersion: detail.planVersion, createdAt: new Date().toISOString() };
      const queued = await performOrQueue(item, () => apiJSON(`/loading/trips/${detail.tripId}/start`, token, { method: "POST", headers: { "Idempotency-Key": item.operationId, "If-Match": String(item.planVersion) } }));
      setInfo(t(queued ? "Loading start saved on this device" : "Loading started"));
    });
  }

  async function markLoaded(orderId: string) {
    if (!detail) return;
    await run("", async () => {
      const order=detail.orders?.find(item=>item.orderId===orderId);
      if(order?.brand?.toLowerCase()==="tech"){
        const fields=techCustody[orderId]||{sealId:"",serials:"",condition:"",evidenceRef:""};
        const serialNumbers=fields.serials.split(/[\n,;]/).map(value=>value.trim()).filter(Boolean);
        if(!fields.sealId.trim()||!serialNumbers.length||!fields.condition.trim())throw new Error(t("Tech custody requires a seal ID, serial number, and loading condition."));
        const custodyKey=await operationId();
        const custody:LoadingQueueOperation={operationId:custodyKey,type:"CUSTODY_RECORD",tripId:detail.tripId,orderId,payload:{stage:"LOADED",sealId:fields.sealId.trim(),serialNumbers,condition:fields.condition.trim(),evidenceRef:fields.evidenceRef.trim()},createdAt:new Date().toISOString()};
        await performOrQueue(custody,()=>apiJSON(`/orders/${encodeURIComponent(orderId)}/custody`,token,{method:"POST",headers:{"Idempotency-Key":custodyKey},body:JSON.stringify({stage:"LOADED",sealId:fields.sealId.trim(),serialNumbers,condition:fields.condition.trim(),evidenceRef:fields.evidenceRef.trim(),idempotencyKey:custodyKey})}));
      }
      const item: LoadingQueueOperation = { operationId: await operationId(), type: "ORDER_LOADED", tripId: detail.tripId, orderId, createdAt: new Date().toISOString() };
      const queued = await performOrQueue(item, () => apiJSON(`/loading/trips/${detail.tripId}/orders/${orderId}/loaded`, token, { method: "PUT", headers: { "Idempotency-Key": item.operationId } }));
      setInfo(t(queued ? "Load confirmation saved on this device" : "Order loaded"));
    });
  }

  async function addIssue(e: FormEvent) {
    e.preventDefault();
    if (!detail || !issueOrderId) return;
    await run("", async () => {
      const affectedUnits = Number(issueUnits);
      if (!Number.isInteger(affectedUnits) || affectedUnits < 1) throw new Error("Affected units must be a positive whole number.");
      const payload = { type: issueType, affectedUnits, note: issueNote.trim() };
      const item: LoadingQueueOperation = { operationId: newIdempotencyKey(), type: "ISSUE_CREATE", tripId: detail.tripId, orderId: issueOrderId, payload, createdAt: new Date().toISOString() };
      const queued = await performOrQueue(item, () => apiJSON(`/loading/trips/${detail.tripId}/orders/${issueOrderId}/issues`, token, {
        method: "POST", headers: { "Idempotency-Key": item.operationId }, body: JSON.stringify(payload),
      }));
      setIssueNote("");
      setInfo(t(queued ? "Shortfall saved on this device" : "Issue recorded"));
    });
  }

  async function removeIssue(orderId: string, issueId: string) {
    if (!detail) return;
    if (!navigator.onLine || queueCount > 0) { setError(t("Connect and sync saved loading work before clearing an issue.")); return; }
    await run("Issue removed", async () => {
      await apiJSON(`/loading/trips/${detail.tripId}/orders/${orderId}/issues/${issueId}`, token, { method: "DELETE" });
      await openTrip(detail.tripId);
    });
  }

  async function ready() {
    if (!detail) return;
    await run("", async () => {
      if (!(detail.planVersion && detail.acknowledgedVersion === detail.planVersion)) throw new Error("Acknowledge the current plan while connected before marking the trip ready.");
      const item: LoadingQueueOperation = { operationId: await operationId(), type: "READY", tripId: detail.tripId, createdAt: new Date().toISOString() };
      const queued = await performOrQueue(item, () => apiJSON(`/loading/trips/${detail.tripId}/ready`, token, { method: "POST", headers: { "Idempotency-Key": item.operationId } }));
      setInfo(t(queued ? "Ready status saved on this device" : "Ready for departure"));
    });
  }

  async function acknowledgePlan() {
    if (!detail?.planId || !detail.planVersion) return;
    await run("Current plan acknowledged", async () => {
      await apiJSON(`/planning/plans/${detail.planId}/acknowledgements`,token,{method:"POST",body:JSON.stringify({version:detail.planVersion})});
      await openTrip(detail.tripId);
    });
  }

  const orders = [...(detail?.orders || [])].sort((a, b) => a.suggestedLoadSequence - b.suggestedLoadSequence);
  const blocked = orders.some(isIncomplete);
  const inProgress = detail?.status === "in_progress" || detail?.loadingStatus === "in_progress";
  const isReady = detail?.status === "ready" || detail?.loadingStatus === "ready";
  const currentPlanAcknowledged = Boolean(detail?.planVersion && detail.acknowledgedVersion === detail.planVersion);

  if (ownerState === "loading") return <section className="card loader-shell"><h2>{t("Loader")}</h2><p role="status">{t("Checking this device's saved loader account…")}</p></section>;
  if (ownerState === "blocked") return <section className="card loader-shell"><h2>{t("Loader")}</h2><p className="status-bad" role="alert">{t("This device's saved loading work belongs to another loader account. Sign in with the original loader account to recover it. The saved queue has not been deleted.")}</p></section>;

  return (
    <section className="card loader-shell">
      <h2>{t("Loader")}</h2>
      <p className="muted">
        {profile?.depot ? depotLabel(profile.depot) : t("Depot from profile")} · {t("Suggested Load Order is last-out first-in guidance.")}
      </p>
      {error && <p className="status-bad" role="alert">{error}</p>}
      {info && <p className="status-ok" role="status">{info}</p>}
      {!online && <p className="status-syncing" role="status">{t("Offline mode · using the most recently saved trip list and manifest when available. Confirm the plan version again before departure.")}</p>}
      {queueCount > 0 && <div className={syncState.kind === "error" ? "status-bad" : "status-syncing"} role="status">
        <p>{queueCount} {t("loading change(s) saved on this device.")} {syncState.text}</p>
        <button type="button" className="tap" onClick={() => void syncPending().catch(e => setError(String(e)))} disabled={!online}>{t("Sync saved loading work")}</button>
      </div>}

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
                  {trip.planRef || t("Plan")} · {t("Trip")} {trip.tripNumber ?? ""}
                </strong>
                <span>
                  {trip.vehicleId} · {depotLabel(trip.depot)}
                </span>
                <span>
                  {trip.loadingStatus || t("pending")} · {trip.loadedCount ?? 0} {t("loaded")} · {trip.shortfallCount ?? 0} {t("short")} ·{" "}
                  {trip.pendingCount ?? trip.stopCount ?? 0} {t("pending")}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}

      {detail && (
        <>
          <button type="button" className="linkish" onClick={() => setDetail(null)}>
            ← {t("Trips")}
          </button>
          <h3>
            {detail.planRef} · {detail.vehicleId}
          </h3>
          <p className="muted">{t("Plan version")} {detail.planVersion || t("unavailable")} · {currentPlanAcknowledged ? t("Acknowledged") : t("Acknowledgement required before departure")}</p>
          {!currentPlanAcknowledged && <button type="button" className="tap" onClick={acknowledgePlan} disabled={!online || !detail.planVersion}>{t("Acknowledge current plan")}</button>}
          <p>
            {depotLabel(detail.depot)} · {t(detail.status || detail.loadingStatus || "pending")} · {detail.loadedCount ?? 0} {t("loaded")} ·{" "}
            {detail.shortfallCount ?? 0} {t("short")} · {detail.pendingCount ?? 0} {t("pending")}
          </p>
          {checkoutAlert && <p className="status-bad" role="alert">{t("Driver reported missing goods at check-out")}: {checkoutAlert.missingOrderIds.map(id => detail.orders?.find(order => order.orderId === id)?.orderRef || id).join(", ")}. {t("Check-out blocked. Review this load with dispatch.")}</p>}
          {detail.status === "pending" && (
            <button type="button" className="tap primary" onClick={start}>
              {t("Start loading")}
            </button>
          )}

          <fieldset className="scan-panel">
            <legend>{t("Barcode or QR verification")}</legend>
            <p>{t("Scan the order label, type a scanned code, or use the per-order Loaded button below.")}</p>
            {inProgress && <button type="button" className="tap" onClick={() => { setScannerMessage(""); setScannerOpen(v => !v); }}>{scannerOpen ? t("Close camera") : t("Scan with camera")}</button>}
            {scannerOpen && <video ref={videoRef} className="scan-video" aria-label={t("Camera view for order barcode scanning")} muted playsInline />}
            <form className="row" onSubmit={e => { e.preventDefault(); void confirmScannedCode(scanCode); }}>
              <label>{t("Order barcode / QR value")}<input value={scanCode} onChange={e => setScanCode(e.target.value)} autoComplete="off" /></label>
              <button className="tap" type="submit" disabled={!inProgress}>{t("Verify and mark loaded")}</button>
            </form>
            {scannerMessage && <p role="status">{scannerMessage}</p>}
          </fieldset>

          <h3>{t("Suggested Load Order")}</h3>
          <ol className="load-list">
            {orders.map((o) => (
              <li key={o.orderId} className={`load-item status-${o.status}`}>
                <div>
                  <strong>
                    #{o.suggestedLoadSequence} {o.orderRef || o.orderId}
                  </strong>
                <span>
                  {o.outletId} · {o.brand} · {o.expectedUnits ?? "?"} {t("units")} · {t("stop")} {o.stopSequence} · {t(o.status)}
                </span>
                </div>
                {o.brand?.toLowerCase()==="tech"&&<fieldset className="scan-panel"><legend>{t("High-value Tech custody")}</legend><p>{t("Record the sealed package and serial identifiers before loading. The same values are checked at delivery and receipt.")}</p>
                  <label>{t("Seal ID")}<input value={techCustody[o.orderId]?.sealId||""} onChange={e=>setTechCustody(v=>({...v,[o.orderId]:{sealId:e.target.value,serials:v[o.orderId]?.serials||"",condition:v[o.orderId]?.condition||"",evidenceRef:v[o.orderId]?.evidenceRef||""}}))} maxLength={100}/></label>
                  <label>{t("Serial number(s), comma separated")}<textarea value={techCustody[o.orderId]?.serials||""} onChange={e=>setTechCustody(v=>({...v,[o.orderId]:{sealId:v[o.orderId]?.sealId||"",serials:e.target.value,condition:v[o.orderId]?.condition||"",evidenceRef:v[o.orderId]?.evidenceRef||""}}))} maxLength={12000}/></label>
                  <label>{t("Loading condition")}<input value={techCustody[o.orderId]?.condition||""} onChange={e=>setTechCustody(v=>({...v,[o.orderId]:{sealId:v[o.orderId]?.sealId||"",serials:v[o.orderId]?.serials||"",condition:e.target.value,evidenceRef:v[o.orderId]?.evidenceRef||""}}))} maxLength={500}/></label>
                  <label>{t("Condition photo reference (optional)")}<input value={techCustody[o.orderId]?.evidenceRef||""} onChange={e=>setTechCustody(v=>({...v,[o.orderId]:{sealId:v[o.orderId]?.sealId||"",serials:v[o.orderId]?.serials||"",condition:v[o.orderId]?.condition||"",evidenceRef:e.target.value}}))} maxLength={500}/></label>
                </fieldset>}
                {inProgress && (
                  <button
                    type="button"
                    className="tap"
                    disabled={(o.issues || []).length > 0}
                    onClick={() => markLoaded(o.orderId)}
                  >
                    {t("Loaded")}
                  </button>
                )}
                {(o.issues || []).length > 0 && (
                  <ul className="issues">
                    {o.issues!.map((iss) => (
                      <li key={iss.id}>
                        {iss.type} · {iss.affectedUnits}
                        {iss.note ? ` · ${iss.note}` : ""}
                        {` · ${iss.decision ? `${t("Dispatcher decision")}: ${t(iss.decision)}${iss.decisionNote ? ` (${iss.decisionNote})` : ""}` : t("Waiting for dispatcher decision")}`}
                        {inProgress && (
                          <button type="button" onClick={() => removeIssue(o.orderId, iss.id)} disabled={!online || queueCount > 0}>
                            {t("Clear")}
                          </button>
                        )}
                      </li>
                    ))}
                  </ul>
                )}
              </li>
            ))}
          </ol>

          {inProgress && (
            <form onSubmit={addIssue} className="issue-form">
              <h3>{t("Record shortfall")}</h3>
              <select aria-label={t("Order with shortfall")} value={issueOrderId} onChange={(e) => setIssueOrderId(e.target.value)} required>
                <option value="">{t("Order")}</option>
                {orders.map((o) => (
                  <option key={o.orderId} value={o.orderId}>
                    {o.orderRef || o.orderId}
                  </option>
                ))}
              </select>
              <select aria-label={t("Shortfall type")} value={issueType} onChange={(e) => setIssueType(e.target.value)}>
                <option value="MISSING">{t("MISSING")}</option>
                <option value="DAMAGED">{t("DAMAGED")}</option>
              </select>
              <input
                type="number"
                min={1}
                value={issueUnits}
                onChange={(e) => setIssueUnits(e.target.value)}
                aria-label={t("Affected units")}
              />
                <input aria-label={t("Shortfall note")} placeholder={t("Note")} value={issueNote} onChange={(e) => setIssueNote(e.target.value)} />
              <button type="submit" className="tap">
                {t("Record issue")}
              </button>
            </form>
          )}

          {inProgress && (
            <button type="button" className="tap primary" onClick={ready} disabled={blocked || !currentPlanAcknowledged}>
              {t("Ready for departure")}
            </button>
          )}
          {isReady && <p className="status-ok">{t("Trip is ready for departure.")}</p>}
        </>
      )}
    </section>
  );
}
