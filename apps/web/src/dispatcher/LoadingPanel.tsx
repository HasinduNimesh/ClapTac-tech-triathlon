import { FormEvent, useState } from "react";
import { ApiError, apiJSON } from "../api/client";
import { todayLocal } from "../api/date";
import { depotLabel, LoadingTripDetail, LoadingTripSummary } from "../api/loading";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";

export function LoadingPanel() {
  const { user } = useAuth();
  const { t } = useLocale();
  const token = user?.access_token || "";
  const [date, setDate] = useState(todayLocal);
  const [trips, setTrips] = useState<LoadingTripSummary[]>([]);
  const [detail, setDetail] = useState<LoadingTripDetail | null>(null);
  const [error, setError] = useState("");

  async function load(e?: FormEvent) {
    e?.preventDefault();
    setError("");
    try {
      const body = await apiJSON<{ items: LoadingTripSummary[] }>(`/loading/trips?date=${date}`, token);
      setTrips(body.items || []);
      setDetail(null);
    } catch (err) {
      setError(err instanceof ApiError ? `${err.status}: ${err.message}` : String(err));
    }
  }

  async function openTrip(tripId: string) {
    setError("");
    try {
      setDetail(await apiJSON<LoadingTripDetail>(`/loading/trips/${tripId}`, token));
    } catch (err) {
      setError(err instanceof ApiError ? `${err.status}: ${err.message}` : String(err));
    }
  }

  return (
    <section className="card">
      <h2>{t("Loading status")}</h2>
      <p className="muted">{t("Read-only. Loaders start, record shortfalls, and mark ready.")}</p>
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
            <th>{t("Loaded / short / pending")}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {trips.map((trip) => (
            <tr key={trip.tripId}>
              <td>
                {trip.planRef} · {trip.vehicleId}
              </td>
              <td>{depotLabel(trip.depot)}</td>
              <td>{t(trip.loadingStatus || "")}</td>
              <td>
                {trip.loadedCount ?? 0} / {trip.shortfallCount ?? 0} / {trip.pendingCount ?? 0}
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
            {detail.planRef} · {depotLabel(detail.depot)} · {detail.status}
          </h3>
          <table>
            <thead>
              <tr>
                <th>{t("Suggested")}</th>
                <th>{t("Order")}</th>
                <th>{t("Status")}</th>
                <th>{t("Issues")}</th>
              </tr>
            </thead>
            <tbody>
              {(detail.orders || []).map((o) => (
                <tr key={o.orderId}>
                  <td>{o.suggestedLoadSequence}</td>
                  <td>{o.orderRef || o.orderId}</td>
                  <td>{t(o.status)}</td>
                  <td>{(o.issues || []).map((i) => `${t(i.type)} ${i.affectedUnits}`).join(", ") || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
