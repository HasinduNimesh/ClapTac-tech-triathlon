import { FormEvent, useEffect, useRef, useState } from "react";
import { ApiError, apiJSON } from "../api/client";
import { todayInSriLanka } from "../api/date.mjs";
import { ArrivalPrediction, DeliveryTripDetail, DeliveryTripSummary, LatenessProbability, TruckCheckout } from "../api/delivery";
import { depotLabel } from "../api/loading";
import { useAuth } from "../auth/AuthContext";
import { PlanDetail } from "../api/planning";
import { ARRIVAL_ESTIMATE_VERSION, estimateArrival, previousReportedStop } from "./arrivalEstimate.mjs";
import { evaluatedLatenessCalibrations } from "./latenessCalibration.mjs";
import { calibratedArrivalRange } from "./arrivalRange.mjs";
import { useLocale } from "../i18n";
import { singleFlightMessagePost } from "./singleFlightMessagePost.mjs";

type TripMessage = { id: string; tripId: string; stopId?: string; body: string; sentBy: string; createdAt: string; acknowledgedBy?: string; acknowledgedAt?: string };
type ServiceTimeEstimate = { minutes: number; version: string };
const fallbackServiceTime: ServiceTimeEstimate = { minutes: 20, version: "fixed_20m_v1" };
const postTripMessageOnce = singleFlightMessagePost(async ({ token, tripId, body, stopId }) =>
  apiJSON(`/delivery/trips/${tripId}/messages`, token, { method: "POST", body: JSON.stringify({ body, stopId: stopId || undefined }) }),
);

export function DeliveryPanel() {
  const { user } = useAuth();
  const { t } = useLocale();
  const token = user?.access_token || "";
  const [date, setDate] = useState(todayInSriLanka);
  const [trips, setTrips] = useState<DeliveryTripSummary[]>([]);
  const [detail, setDetail] = useState<DeliveryTripDetail | null>(null);
  const [arrivalChanges, setArrivalChanges] = useState<Record<string, ArrivalPrediction>>({});
  const publishingArrivals = useRef(false);
  const [checkoutAlert, setCheckoutAlert] = useState<TruckCheckout | null>(null);
  const [plan, setPlan] = useState<PlanDetail | null>(null);
  const [refreshedAt, setRefreshedAt] = useState("");
  const [error, setError] = useState("");
  const [hasLoaded, setHasLoaded] = useState(false);
  const [messages, setMessages] = useState<TripMessage[]>([]);
  const [latenessHistory, setLatenessHistory] = useState<LatenessProbability[]>([]);
  const [latenessHistoryState, setLatenessHistoryState] = useState<"loading" | "ready" | "unavailable">("loading");
  const latenessCalibrations = evaluatedLatenessCalibrations(latenessHistory);
  const [serviceTime, setServiceTime] = useState<ServiceTimeEstimate>(fallbackServiceTime);
  const [messageBody, setMessageBody] = useState("");
  const [messageStopId, setMessageStopId] = useState("");
  const [messageSending, setMessageSending] = useState(false);
  const messageSendingRef = useRef(false);
  const [estimateNow, setEstimateNow] = useState(Date.now);

  useEffect(() => {
    const refreshEstimateClock = () => setEstimateNow(Date.now());
    const interval = window.setInterval(refreshEstimateClock, 60_000);
    window.addEventListener("focus", refreshEstimateClock);
    document.addEventListener("visibilitychange", refreshEstimateClock);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener("focus", refreshEstimateClock);
      document.removeEventListener("visibilitychange", refreshEstimateClock);
    };
  }, []);

  useEffect(() => {
    void load();
  }, [token]);

  async function load(e?: FormEvent) {
    e?.preventDefault();
    setError("");
    try {
      const body = await apiJSON<{ items: DeliveryTripSummary[] }>(`/delivery/trips?date=${date}`, token);
      let planBody: PlanDetail | null = null;
      try {
        planBody = await apiJSON<PlanDetail>(`/planning/plans?date=${date}`, token);
      } catch {
        // Delivery updates remain useful when a plan version is unavailable;
        // the stop rows then display that an ETA cannot be estimated.
      }
      let serviceTimeBody: { forecast?: { serviceMinutesPerStop?: number; serviceEstimateVersion?: string } } | null = null;
      try {
        serviceTimeBody = await apiJSON<{ forecast?: { serviceMinutesPerStop?: number; serviceEstimateVersion?: string } }>("/orders/forecast", token);
      } catch {
        // The deterministic fallback keeps live ETA estimates available when forecasting is unavailable.
      }
      const configuredMinutes = serviceTimeBody?.forecast?.serviceMinutesPerStop;
      setServiceTime(Number.isFinite(configuredMinutes) && configuredMinutes! >= 0
        ? { minutes: configuredMinutes!, version: serviceTimeBody?.forecast?.serviceEstimateVersion || fallbackServiceTime.version }
        : fallbackServiceTime);
      setTrips(body.items || []);
      setPlan(planBody);
      setRefreshedAt(new Date().toLocaleString());
      setDetail(null);
      setHasLoaded(true);
    } catch (err) {
      setError(err instanceof ApiError ? `${err.status}: ${err.message}` : String(err));
      setHasLoaded(true);
    }
  }

  useEffect(() => {
    if (!detail?.tripId || !token) { setCheckoutAlert(null); return; }
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
  }, [detail?.tripId, token]);

  useEffect(() => {
    if (!detail?.tripId || detail.status !== "in_progress" || !token) return;
    let active = true;
    const tripId = detail.tripId;
    const poll = async () => {
      try {
        const latest = await apiJSON<DeliveryTripDetail>("/delivery/trips/" + encodeURIComponent(tripId), token);
        if (active && latest.tripId === tripId) setDetail(latest);
      } catch { /* The last good trip view remains usable. */ }
    };
    const timer = window.setInterval(() => { void poll(); }, 15_000);
    return () => { active = false; window.clearInterval(timer); };
  }, [detail?.tripId, detail?.status, token]);

  useEffect(() => {
    if (!detail || detail.status !== "in_progress" || !detail.run?.planVersion ||
      detail.currentPlanVersion !== detail.run.planVersion || !plan ||
      (plan.plan.currentVersion != null && plan.plan.currentVersion !== detail.run.planVersion) ||
      publishingArrivals.current) return;
    publishingArrivals.current = true;
    let active = true;
    const publish = async () => {
      try {
        for (const [index, stop] of detail.stops.entries()) {
          if (!active || stop.status !== "pending") continue;
          const previous = previousReportedStop(detail.stops, index);
          const allocation = plan.allocations?.find(item => item.tripId === detail.tripId && item.orderId === stop.orderId);
          const previousAllocation = previous && plan.allocations?.find(item => item.tripId === detail.tripId && item.orderId === previous.orderId);
          const estimate = estimateArrival({
            plannedArrivalAt: allocation?.plannedArrivalAt,
            plannedDepartureAt: previousAllocation?.plannedDepartureAt,
            previousOutcomeAt: previous?.outcomeAt || previous?.outcomeReceivedAt,
            previousArrivedAt: previous?.arrivedAt,
            serviceMinutesPerStop: serviceTime.minutes,
            windowCloseAt: stop.plannedWindowClose,
            deliveryDate: detail.run.deliveryDate,
            now: estimateNow,
          });
          if (!estimate.eta || !estimate.risk) continue;
          const lateness = latenessHistory.find(item => item.brand === (stop.brand || "") && item.temperatureRequirement === (stop.temperatureRequirement || ""));
          const range = calibratedArrivalRange(estimate, lateness);
          try {
            const result = await apiJSON<{ prediction: ArrivalPrediction }>(
              "/delivery/trips/" + encodeURIComponent(detail.tripId) + "/stops/" + encodeURIComponent(stop.id) + "/arrival-prediction", token, {
                method: "POST",
                body: JSON.stringify({
                  planVersion: detail.run.planVersion,
                  sourceEventAt: previous?.outcomeReceivedAt || previous?.arrivedReceivedAt || previous?.outcomeAt || previous?.arrivedAt,
                  estimatedArrivalAt: estimate.eta,
                  arrivalRangeLower: range?.lower.toISOString(),
                  arrivalRangeUpper: range?.upper.toISOString(),
                  lateRisk: estimate.risk,
                }),
              });
            if (active) setArrivalChanges(current => ({ ...current, [stop.id]: result.prediction }));
          } catch { /* A failed publish cannot hide the local ETA. */ }
        }
      } finally { publishingArrivals.current = false; }
    };
    void publish();
    return () => { active = false; };
  }, [detail, plan, serviceTime, latenessHistory, estimateNow, token]);

  async function openTrip(tripId: string) {
    setArrivalChanges({});
    setError("");
    setLatenessHistory([]);
    setLatenessHistoryState("loading");
    try {
      const [trip, messageResult] = await Promise.all([
        apiJSON<DeliveryTripDetail>(`/delivery/trips/${tripId}`, token),
        apiJSON<{ items: TripMessage[] }>(`/delivery/trips/${tripId}/messages`, token),
      ]);
      setDetail(trip);
      setMessages(messageResult.items || []);
      try {
        const history = await apiJSON<{ items: LatenessProbability[] }>(`/delivery/trips/${tripId}/lateness-history`, token);
        setLatenessHistory(history.items || []);
        setLatenessHistoryState("ready");
      } catch {
        // A failed history query must not hide current event-based trip status.
        setLatenessHistoryState("unavailable");
      }
    } catch (err) {
      setLatenessHistoryState("unavailable");
      setError(err instanceof ApiError ? `${err.status}: ${err.message}` : String(err));
    }
  }

  async function sendMessage(e: FormEvent) {
    e.preventDefault();
    const body = messageBody.trim();
    if (!detail || !body || messageSendingRef.current) return;
    messageSendingRef.current = true;
    setMessageSending(true);
    try {
      await postTripMessageOnce({ senderId: user?.profile?.sub || "", token, tripId: detail.tripId, body, stopId: messageStopId });
      setMessageBody("");
      await openTrip(detail.tripId);
    } catch (err) { setError(err instanceof ApiError ? `${err.status}: ${err.message}` : String(err)); }
    finally { messageSendingRef.current = false; setMessageSending(false); }
  }

  return (
    <section className="card">
      <h2>{t("Delivery status")}</h2>
      <p className="muted">{t("Read-only, event-based progress from driver updates; this view does not claim GPS tracking.")}</p>
      {error && <p className="status-bad" role="alert">{error}</p>}
      <form onSubmit={load} className="row">
        <label>
          {t("Delivery date")}
          <input type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
        </label>
        <button type="submit">{t("Refresh")}</button>
      </form>
      <table>
        <thead>
          <tr>
            <th>{t("Trip")}</th>
            <th>{t("Depot")}</th>
            <th>{t("Status")}</th>
            <th>{t("Stops")}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {hasLoaded && trips.length === 0 && <tr><td colSpan={5}>{t("No delivery trips for this date.")}</td></tr>}
          {trips.map((trip) => (
            <tr key={trip.tripId}>
              <td>
                {trip.planRef} · {trip.vehicleId}
              </td>
              <td>{depotLabel(trip.depot)}</td>
              <td>{t(trip.status || "")}</td>
              <td>
                {trip.completedStops ?? 0} / {trip.stopCount ?? 0}
              </td>
              <td>
                <button type="button" onClick={() => openTrip(trip.tripId)}>
                  {t("View")}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {detail && (
        <div>
          <h3>
            {detail.run?.planRef} · {depotLabel(detail.run?.depot)} · {t(detail.status)}
          </h3>
          {checkoutAlert && <p className="status-bad" role="alert">{t("Driver reported missing goods at check-out")}: {checkoutAlert.missingOrderIds.map(id => detail.stops.find(stop => stop.orderId === id)?.orderRef || id).join(", ")}. {t("Check-out blocked. Review this load with the loader.")}</p>}
          <table>
            <thead>
              <tr>
                <th>{t("Stop")}</th>
                <th>{t("Outlet")}</th>
                <th>{t("Status")}</th>
                <th>{t("Outcome")}</th>
                <th>{t("Estimated arrival / window risk")}</th>
                <th>{t("Last reported")}</th>
              </tr>
            </thead>
            <tbody>
              {(detail.stops || []).map((s, index, stops) => {
                const allocation = plan?.allocations?.find((item) => item.tripId === detail.tripId && item.orderId === s.orderId);
                const previous = previousReportedStop(stops, index);
                const previousAllocation = previous && plan?.allocations?.find((item) => item.tripId === detail.tripId && item.orderId === previous.orderId);
                const estimate = estimateArrival({
                  plannedArrivalAt: allocation?.plannedArrivalAt,
                  plannedDepartureAt: previousAllocation?.plannedDepartureAt,
                  previousOutcomeAt: previous?.outcomeAt || previous?.outcomeReceivedAt,
                  previousArrivedAt: previous?.arrivedAt,
                  serviceMinutesPerStop: serviceTime.minutes,
                  windowCloseAt: s.plannedWindowClose,
                  deliveryDate: detail.run.deliveryDate,
                  now: estimateNow,
                });
                const lateness = latenessHistory.find((item) => item.brand === (s.brand || "") && item.temperatureRequirement === (s.temperatureRequirement || ""));
                const arrivalRange = calibratedArrivalRange(estimate, lateness);
                return (
                  <tr key={s.id || s.orderId}>
                    <td>{s.stopSequence}</td>
                    <td>{s.outletName || s.outletId || s.orderRef}</td>
                    <td>{t(s.status)}{s.temperatureReadings?.map((reading) => <p key={reading.operationId} className={reading.evaluation === "OUT_OF_RANGE" ? "status-bad" : "muted"} role={reading.evaluation === "OUT_OF_RANGE" ? "alert" : "status"}>{t(reading.evaluation)} · {reading.valueC.toFixed(1)} °C · {new Date(reading.occurredAt).toLocaleString()} · {reading.actorId}</p>)}</td>
                    <td>{t(s.outcomeCode || "—")}</td>
                    <td>
                      {s.arrivedAt ? `${t("Arrived")} ${new Date(s.arrivedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}` : `${t(estimate.label)} · ${t(estimate.risk || "schedule unavailable")}`}
                      {!s.arrivedAt && estimate.kind !== "unknown" && <small className="muted">{t(estimate.confidence)}</small>}
                      {arrivalChanges[s.id]?.previouslyCommunicatedAt && arrivalChanges[s.id]?.notifiedArrivalAt && <small className="muted">{t("Arrival changed from")} {new Date(arrivalChanges[s.id].previouslyCommunicatedAt!).toLocaleString()} {t("to")} {new Date(arrivalChanges[s.id].notifiedArrivalAt!).toLocaleString()}</small>}
                      {!s.arrivedAt && estimate.kind !== "unknown" && arrivalRange && <small className="muted">{t("Calibrated historical arrival range")}: {arrivalRange.lower.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}–{arrivalRange.upper.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} · {t("nominal 80% interval")}; {arrivalRange.sampleCount} {t("paired arrivals")}; {arrivalRange.holdoutCount} {t("holdout arrivals")}; {arrivalRange.coverage == null ? t("coverage unavailable") : `${(arrivalRange.coverage * 100).toFixed(1)}% ${t("holdout coverage")}`} · {arrivalRange.version} · {t("Historical schedule residuals by depot, brand, and temperature; adjusted around the event-based ETA.")}</small>}
                      {!s.arrivedAt && estimate.kind !== "unknown" && !arrivalRange && <small className="muted">{t("Arrival range withheld")}: {t(lateness?.arrivalRangeStatus || (latenessHistoryState === "unavailable" ? "ARRIVAL_HISTORY_UNAVAILABLE" : "INSUFFICIENT_HISTORY"))} · {lateness?.arrivalRangeSamples ?? 0} {t("paired arrivals")}, {lateness?.arrivalRangeHoldouts ?? 0} {t("holdout arrivals")}</small>}
                      {!s.arrivedAt && <small className="muted" aria-live="polite">{latenessHistoryState === "loading" ? t("Loading arrival history…") : latenessHistoryState === "unavailable" ? t("Arrival history is unavailable.") : lateness?.probability != null ? `${t("Late arrival probability")}: ${(lateness.probability * 100).toFixed(1)}% · ${lateness.sampleCount} ${t("past stops")}` : `${t("Insufficient history; probability withheld.")} · n=${lateness?.sampleCount ?? 0}`}</small>}
                    </td>
                    <td>{s.outcomeReceivedAt ? `${t("Outcome received")} ${new Date(s.outcomeReceivedAt).toLocaleString()}` : s.arrivedReceivedAt ? `${t("Arrival received")} ${new Date(s.arrivedReceivedAt).toLocaleString()}` : t("No driver update yet")}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          <p className="muted">{t("Event-based estimates")} ({ARRIVAL_ESTIMATE_VERSION}; {serviceTime.version}) {t("use the published schedule and last completed stop; an arrived stop is projected using its service-time allowance. They do not use GPS or live traffic. Confidence: low. Refreshed")} {refreshedAt || "—"}.</p>
          <p className="muted">{t("Historical baseline from the past 90 days; not a live-traffic prediction.")}</p>
          <p className="muted">{latenessHistoryState === "unavailable" ? t("Arrival history is unavailable.") : latenessCalibrations.length > 0 ? `${t("Probability calibration (Brier score)")}: ${latenessCalibrations.map(item => `${item.brand}/${item.temperatureRequirement || t("unspecified temperature")} ${item.brierScore!.toFixed(3)} · ${item.calibrationSampleCount} ${t("evaluated arrivals")} · ${item.calibrationVersion}`).join("; ")}. ${t("Lower scores indicate better probability calibration.")}` : t("Calibration is withheld until ten out-of-time predictions are available.")}</p>
          <section aria-labelledby="trip-messages-heading" className="message-panel">
            <h4 id="trip-messages-heading">{t("Trip messages and receipts")}</h4>
            <form onSubmit={sendMessage} className="issue-form" aria-busy={messageSending}>
              <label>{t("Message for driver")}<textarea value={messageBody} onChange={e => setMessageBody(e.target.value)} maxLength={1000} required /></label>
              <label>{t("Related stop (optional)")}<select value={messageStopId} onChange={e => setMessageStopId(e.target.value)}><option value="">{t("Whole trip")}</option>{detail.stops.map(s => <option key={s.id} value={s.id}>{s.stopSequence}. {s.outletName || s.outletId || s.orderRef}</option>)}</select></label>
              <button type="submit" disabled={!messageBody.trim() || messageSending}>{t("Send message")}</button>
            </form>
            {messages.length === 0 ? <p>{t("No trip messages yet.")}</p> : <ul>{messages.map(m => <li key={m.id}><p>{m.body}</p><small>{t("Sent")} {new Date(m.createdAt).toLocaleString()} {t("by")} {m.sentBy}{m.stopId ? ` · ${t("linked to a stop")}` : ` · ${t("whole trip")}`}</small><p role="status">{m.acknowledgedAt ? `${t("Acknowledged by")} ${m.acknowledgedBy} · ${new Date(m.acknowledgedAt).toLocaleString()}` : t("Awaiting driver acknowledgement")}</p></li>)}</ul>}
          </section>
        </div>
      )}
    </section>
  );
}
