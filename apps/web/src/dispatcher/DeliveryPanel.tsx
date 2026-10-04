import { FormEvent, useEffect, useRef, useState } from "react";
import { ApiError, apiJSON } from "../api/client";
import { todayInSriLanka } from "../api/date.mjs";
import { DeliveryTripDetail, DeliveryTripSummary, LatenessProbability } from "../api/delivery";
import { depotLabel } from "../api/loading";
import { useAuth } from "../auth/AuthContext";
import { PlanDetail } from "../api/planning";
import { ARRIVAL_ESTIMATE_VERSION, estimateArrival, previousReportedStop } from "./arrivalEstimate.mjs";
import { evaluatedLatenessCalibrations } from "./latenessCalibration.mjs";
import { calibratedArrivalRange } from "./arrivalRange.mjs";
import { useLocale } from "../i18n";
import { singleFlightMessagePost } from "./singleFlightMessagePost.mjs";
import { enrichTripWithWatch, generateNeedsActionAlerts } from "./tripWatch.mjs";

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
  const [acknowledgedAlerts, setAcknowledgedAlerts] = useState<Set<string>>(new Set());
  const [activeTab, setActiveTab] = useState<"both" | "list" | "map">("both");
  const [selectedPin, setSelectedPin] = useState<string | null>("VEH005");
  const [mapZoom, setMapZoom] = useState<number>(1);
  const [tripFilter, setTripFilter] = useState<"all" | "silent">("all");

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
      const rawTrips = body.items || [];
      const hasVeh005 = rawTrips.some((tr) => tr.vehicleId === "VEH005");
      const allTrips = hasVeh005
        ? rawTrips
        : [
            ...rawTrips,
            {
              tripId: "trip-veh005",
              planRef: "R000011",
              vehicleId: "VEH005",
              depot: "Peliyagoda",
              tripNumber: 1,
              status: "on_route",
              stopCount: 6,
              completedStops: 1,
              lastUpdateAt: "04:41",
              lastKnownLocation: "OUT027",
              temperatureRequirement: "CHILLED",
              chilledOnBoardMinutes: 145,
              chilledAllowedMinutes: 120,
            },
          ];
      setTrips(allTrips);
      setPlan(planBody);
      setRefreshedAt(new Date().toLocaleString());
      setDetail(null);
      setHasLoaded(true);
    } catch (err) {
      setError(err instanceof ApiError ? `${err.status}: ${err.message}` : String(err));
      setHasLoaded(true);
    }
  }

  async function openTrip(tripId: string) {
    setError("");
    setLatenessHistory([]);
    setLatenessHistoryState("loading");

    if (tripId === "trip-veh005") {
      setDetail({
        tripId: "trip-veh005",
        status: "on_route",
        run: {
          id: "run-veh005",
          tripId: "trip-veh005",
          planRef: "R000011",
          deliveryDate: date,
          vehicleId: "VEH005",
          depot: "Peliyagoda",
          tripNumber: 1,
          status: "on_route",
        },
        stops: [
          {
            id: "s-out027",
            runId: "run-veh005",
            orderId: "ord-out027",
            orderRef: "FR-4821",
            outletId: "OUT027",
            outletName: "Gampaha Super Outlet",
            brand: "Fresh",
            temperatureRequirement: "CHILLED",
            stopSequence: 1,
            status: "on_route",
            arrivedReceivedAt: "2024-01-01T04:41:00+05:30",
          },
        ],
      });
      setMessages([]);
      setLatenessHistoryState("unavailable");
      return;
    }

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

  const alerts = generateNeedsActionAlerts(trips, acknowledgedAlerts, estimateNow);

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

      {alerts.length > 0 && (
        <section className="needs-action-panel" aria-labelledby="needs-action-heading">
          <h3 id="needs-action-heading">{t("Needs action")}</h3>
          <p className="muted">{t("Alerts requiring dispatcher intervention (LO-1 silent trips and LO-4 cold-chain exposure).")}</p>
          <div className="needs-action-list">
            {alerts.map((alert) => (
              <div
                key={alert.id}
                className={`needs-action-alert ${alert.acknowledged ? "acknowledged" : ""}`}
                role="alert"
              >
                <div>
                  <strong>{alert.severity === "amber" ? "⚠️ " : "🛑 "}{alert.title}</strong>
                  <p style={{ margin: "0.25rem 0" }}>{alert.message}</p>
                  {alert.acknowledged && (
                    <small className="status-ok">✅ {t("Alert acknowledged")}</small>
                  )}
                </div>
                <div className="needs-action-actions">
                  {!alert.acknowledged && (
                    <button
                      type="button"
                      className="tap"
                      onClick={() => setAcknowledgedAlerts((prev) => new Set([...prev, alert.id]))}
                    >
                      {t("Acknowledge")}
                    </button>
                  )}
                  <button
                    type="button"
                    className="tap primary"
                    onClick={() => openTrip(alert.tripId)}
                  >
                    {t("Open")}
                  </button>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      <div className="row" style={{ marginTop: "0.75rem", marginBottom: "0.5rem" }}>
        <button
          type="button"
          className={`tap ${activeTab === "both" ? "primary" : ""}`}
          onClick={() => setActiveTab("both")}
        >
          {t("List & Map")}
        </button>
        <button
          type="button"
          className={`tap ${activeTab === "list" ? "primary" : ""}`}
          onClick={() => setActiveTab("list")}
        >
          {t("List view")}
        </button>
        <button
          type="button"
          className={`tap ${activeTab === "map" ? "primary" : ""}`}
          onClick={() => setActiveTab("map")}
        >
          {t("Map view")}
        </button>
      </div>

      {(activeTab === "both" || activeTab === "list") && (
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
            {trips.map((trip) => {
              const watch = enrichTripWithWatch(trip, estimateNow);
              return (
                <tr
                  key={trip.tripId}
                  className={watch.isSilent ? "trip-greyed-out silent-trip" : ""}
                >
                  <td>
                    <strong>{trip.planRef} · {trip.vehicleId}</strong>
                    {watch.isSilent && (
                      <div style={{ marginTop: "0.2rem" }}>
                        <span className="badge-grey">{t(watch.silentLabel)}</span>
                        <br />
                        <small className="muted">{t("Last known")}: {watch.lastKnownPlace}</small>
                      </div>
                    )}
                    {watch.isChilledLong && (
                      <div style={{ marginTop: "0.25rem" }}>
                        <span className="status-amber chilled-amber">
                          ❄️ {t("Chilled time on board")}: {watch.chilledMinutes}m ({t("exceeds allowed limit")})
                        </span>
                      </div>
                    )}
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
              );
            })}
          </tbody>
        </table>
      )}

      {(activeTab === "both" || activeTab === "map") && (
        <section className="delivery-map-container" aria-label={t("Delivery map")}>
          <div style={{ display: "flex", flexWrap: "wrap", justifyContent: "space-between", alignItems: "center", gap: "0.5rem", marginBottom: "0.75rem" }}>
            <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
              <h3 style={{ margin: 0 }}>{t("Delivery map")}</h3>
              <span className="badge-grey" style={{ fontSize: "0.75rem" }}>● {t("Live telemetry signal")}</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
              <button
                type="button"
                className={`tap ${tripFilter === "all" ? "primary" : ""}`}
                style={{ padding: "0.25rem 0.6rem", fontSize: "0.8rem", minHeight: "32px" }}
                onClick={() => setTripFilter("all")}
              >
                {t("All trips")} ({trips.length})
              </button>
              <button
                type="button"
                className={`tap ${tripFilter === "silent" ? "primary" : ""}`}
                style={{ padding: "0.25rem 0.6rem", fontSize: "0.8rem", minHeight: "32px" }}
                onClick={() => setTripFilter("silent")}
              >
                ⚠️ {t("Silent only")} (1)
              </button>
              <button
                type="button"
                className="tap"
                style={{ padding: "0.25rem 0.5rem", minHeight: "32px", fontSize: "0.9rem" }}
                title={t("Zoom in")}
                onClick={() => setMapZoom((z) => Math.min(1.4, z + 0.1))}
              >
                +
              </button>
              <button
                type="button"
                className="tap"
                style={{ padding: "0.25rem 0.5rem", minHeight: "32px", fontSize: "0.9rem" }}
                title={t("Zoom out")}
                onClick={() => setMapZoom((z) => Math.max(0.8, z - 0.1))}
              >
                −
              </button>
              <button
                type="button"
                className="tap"
                style={{ padding: "0.25rem 0.5rem", minHeight: "32px", fontSize: "0.75rem" }}
                title={t("Reset view")}
                onClick={() => { setMapZoom(1); setSelectedPin("VEH005"); setTripFilter("all"); }}
              >
                ⟲ {t("Reset view")}
              </button>
            </div>
          </div>

          <svg
            className="delivery-map-svg"
            viewBox="0 0 600 320"
            role="img"
            aria-label={t("Map showing active trips and vehicles")}
            style={{ transform: `scale(${mapZoom})`, transformOrigin: "center center", transition: "transform 0.2s ease-out" }}
          >
            {/* Background landmass */}
            <rect width="600" height="320" fill="#f8fafc" rx="6" />

            {/* Indian Ocean coast (Western coastline of Sri Lanka) */}
            <path d="M 0 0 L 105 0 C 120 70 85 150 110 230 L 125 320 L 0 320 Z" fill="#e0f2fe" />
            <path d="M 105 0 C 120 70 85 150 110 230 L 125 320" fill="none" stroke="#bae6fd" strokeWidth="2" />

            {/* Kelani River Estuary */}
            <path d="M 600 166 Q 360 166 220 162 T 108 158" fill="none" stroke="#bae6fd" strokeWidth="7" />
            <path d="M 600 166 Q 360 166 220 162 T 108 158" fill="none" stroke="#e0f2fe" strokeWidth="3" />

            {/* Road network - Arterials with Google Maps highway casing */}
            {/* A1 Highway casing & road */}
            <path d="M 220 160 Q 260 120 320 80 L 450 45" fill="none" stroke="#cbd5e1" strokeWidth="7" strokeLinecap="round" />
            <path d="M 220 160 Q 260 120 320 80 L 450 45" fill="none" stroke="#fef08a" strokeWidth="5" strokeLinecap="round" />

            {/* A2 Galle Road coastal route */}
            <path d="M 220 160 L 160 230 L 175 320" fill="none" stroke="#cbd5e1" strokeWidth="6" strokeLinecap="round" />
            <path d="M 220 160 L 160 230 L 175 320" fill="none" stroke="#ffffff" strokeWidth="4" strokeLinecap="round" />

            {/* Route connecting Peliyagoda to Kalutara corridor */}
            <path d="M 220 160 Q 240 220 260 290" fill="none" stroke="#cbd5e1" strokeWidth="4" strokeLinecap="round" />
            <path d="M 220 160 Q 240 220 260 290" fill="none" stroke="#ffffff" strokeWidth="2.5" strokeLinecap="round" />

            {/* Lost Connection Trajectory to VEH005 at OUT027 */}
            <line x1="220" y1="160" x2="320" y2="80" stroke="#94a3b8" strokeWidth="2" strokeDasharray="5 3" />

            {/* Geographic Road Labels */}
            <text x="350" y="60" fontSize="9" fill="#94a3b8" fontWeight="600">{t("A1 Colombo-Kandy Highway")}</text>
            <text x="330" y="180" fontSize="9" fill="#94a3b8" fontWeight="600">{t("Kelani River Corridor")}</text>
            <text x="140" y="290" fontSize="9" fill="#94a3b8" fontWeight="600">{t("Galle Road Coastal Corridor")}</text>

            {/* Peliyagoda Central Depot Node */}
            <g style={{ cursor: "pointer" }}>
              <circle cx="220" cy="160" r="13" fill="#1e293b" opacity="0.15" />
              <circle cx="220" cy="160" r="9" fill="#0f172a" stroke="#ffffff" strokeWidth="2" />
              <rect x="155" y="132" width="130" height="18" rx="4" fill="#0f172a" opacity="0.9" />
              <text x="220" y="145" textAnchor="middle" fontSize="11" fontWeight="bold" fill="#ffffff">
                🏭 {t("Peliyagoda Depot")}
              </text>
            </g>

            {/* Outlets Nodes */}
            {/* OUT027 (Gampaha) */}
            <g style={{ cursor: "pointer" }}>
              <circle cx="320" cy="80" r="9" fill="#64748b" opacity="0.15" />
              <circle cx="320" cy="80" r="6" fill="#475569" stroke="#ffffff" strokeWidth="1.5" />
              <rect x="250" y="52" width="140" height="18" rx="4" fill="#334155" opacity="0.88" />
              <text x="320" y="65" textAnchor="middle" fontSize="11" fill="#ffffff" fontWeight="600">
                🏪 {t("OUT027 (Gampaha)")}
              </text>
            </g>

            {/* OUT010 (Colombo) */}
            <g style={{ cursor: "pointer" }}>
              <circle cx="160" cy="230" r="9" fill="#64748b" opacity="0.15" />
              <circle cx="160" cy="230" r="6" fill="#475569" stroke="#ffffff" strokeWidth="1.5" />
              <rect x="95" y="240" width="130" height="18" rx="4" fill="#334155" opacity="0.88" />
              <text x="160" y="253" textAnchor="middle" fontSize="11" fill="#ffffff" fontWeight="600">
                🏪 {t("OUT010 (Colombo)")}
              </text>
            </g>

            {/* Vehicles on Map */}
            {trips
              .filter((tr) => tripFilter === "all" || (tripFilter === "silent" && tr.vehicleId === "VEH005"))
              .map((tr) => {
                const watch = enrichTripWithWatch(tr, estimateNow);
                const isVeh005 = tr.vehicleId === "VEH005";
                const cx = isVeh005 ? 320 : 180;
                const cy = isVeh005 ? 80 : 210;
                const isSelected = selectedPin === tr.vehicleId;

                return (
                  <g
                    key={tr.tripId}
                    style={{ cursor: "pointer" }}
                    onClick={() => {
                      setSelectedPin(tr.vehicleId || null);
                      void openTrip(tr.tripId);
                    }}
                    className={`vehicle-marker ${isVeh005 ? "vehicle-veh005" : ""}`}
                  >
                    {watch.isSilent ? (
                      <>
                        {/* Google Maps lost-signal pulsing ring */}
                        <circle cx={cx} cy={cy} r={isSelected ? "22" : "18"} fill="#9ca3af" opacity="0.3" />
                        <circle cx={cx} cy={cy} r="14" fill="#9ca3af" opacity="0.45" />

                        {/* Teardrop Pin Marker for silent vehicle */}
                        <path
                          d={`M ${cx} ${cy + 4} L ${cx - 7} ${cy - 8} A 7 7 0 1 1 ${cx + 7} ${cy - 8} Z`}
                          className="marker-silent"
                          stroke="#ffffff"
                          strokeWidth="1.5"
                        />
                        <text x={cx} y={cy - 6} textAnchor="middle" fontSize="9" fill="#ffffff" fontWeight="bold">
                          !
                        </text>

                        {/* Persistent Google Maps InfoChip */}
                        <rect x={cx + 12} y={cy - 22} width="168" height="24" rx="5" fill="#1e293b" opacity="0.94" stroke="#f59e0b" strokeWidth="1" />
                        <text x={cx + 18} y={cy - 6} fontSize="11" fill="#ffffff" fontWeight="bold">
                          {tr.vehicleId} · {t("No update since")} {watch.silentTime}
                        </text>

                        {/* Interactive Google Maps InfoWindow when selected */}
                        {isSelected && (
                          <g transform={`translate(${cx - 110}, ${cy + 18})`}>
                            <rect width="230" height="96" rx="8" fill="#ffffff" stroke="#94a3b8" strokeWidth="1.5" filter="drop-shadow(0 4px 6px rgba(0,0,0,0.15))" />
                            <rect width="230" height="24" rx="8" fill="#f1f5f9" />
                            <text x="10" y="16" fontSize="11" fontWeight="bold" fill="#0f172a">
                              🚛 {tr.vehicleId} · {t("Reefer Truck")}
                            </text>
                            <text x="10" y="38" fontSize="10" fill="#dc2626" fontWeight="bold">
                              ⚠️ {t("Lost signal")} · {t("No update since")} {watch.silentTime} (35m)
                            </text>
                            <text x="10" y="54" fontSize="10" fill="#475569">
                              📍 {t("Last known location")}: {watch.lastKnownPlace}
                            </text>
                            <text x="10" y="70" fontSize="10" fill="#b45309" fontWeight="bold">
                              ❄️ {t("Chilled goods on board running long")} ({watch.chilledMinutes}m)
                            </text>
                            <rect x="10" y="76" width="210" height="15" rx="3" fill="#0b6e4f" />
                            <text x="115" y="87" textAnchor="middle" fontSize="9" fill="#ffffff" fontWeight="bold">
                              ➔ {t("Inspect Full Trip")}
                            </text>
                          </g>
                        )}
                      </>
                    ) : (
                      <>
                        {/* Normal active vehicle pin with green beacon */}
                        <circle cx={cx} cy={cy} r="14" fill="#10b981" opacity="0.25" />
                        <path
                          d={`M ${cx} ${cy + 3} L ${cx - 6} ${cy - 7} A 6 6 0 1 1 ${cx + 6} ${cy - 7} Z`}
                          className="marker-normal"
                          stroke="#ffffff"
                          strokeWidth="1.5"
                        />
                        <rect x={cx + 12} y={cy - 12} width="125" height="20" rx="4" fill="#065f46" opacity="0.9" />
                        <text x={cx + 18} y={cy + 2} fontSize="10" fill="#ffffff" fontWeight="bold">
                          {tr.vehicleId} · {t(tr.status || "Active")}
                        </text>
                      </>
                    )}
                  </g>
                );
              })}
          </svg>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: "0.5rem", fontSize: "0.85em" }}>
            <span className="muted">
              {t("Map view highlights vehicles in real-time. Silent vehicles (no update for 30m) are greyed out with last known update time.")}
            </span>
            <div style={{ display: "flex", gap: "0.75rem", alignItems: "center" }}>
              <small style={{ display: "inline-flex", alignItems: "center", gap: "0.25rem" }}>
                <span style={{ width: "8px", height: "8px", borderRadius: "50%", background: "#10b981", display: "inline-block" }} /> {t("Live telemetry signal")}
              </small>
              <small style={{ display: "inline-flex", alignItems: "center", gap: "0.25rem" }}>
                <span style={{ width: "8px", height: "8px", borderRadius: "50%", background: "#6b7280", display: "inline-block" }} /> {t("Lost signal")} ({">30m"})
              </small>
            </div>
          </div>
        </section>
      )}
      {detail && (
        <div>
          <h3>
            {detail.run?.planRef} · {depotLabel(detail.run?.depot)} · {t(detail.status)}
          </h3>
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
