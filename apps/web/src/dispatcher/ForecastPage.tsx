import { useEffect, useState } from "react";
import { apiJSON } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";

type Forecast = {
  generatedAt: string; forecastVersion: string; method: string; historyWeeks: number; driftModelVersion: string; backtestModelVersion: string;
  inputDrift: { depot: string; brand: string; previousOrderCount: number; recentOrderCount: number; changePercent?: number; backtestAPEPercent?: number; status: string }[];
  weekly: { weekStarting: string; depot: string; brand: string; chilledOrders: number; ambientOrders: number; estimatedWeightKg: number; estimatedVolumeM3: number; estimate: boolean }[];
  serviceMinutesPerStop: number; serviceEstimateVersion: string; serviceEstimateSource: string;
  serviceTimeBacktestVersion?: string; serviceTimeBacktestWindowStart?: string; serviceTimeBacktestWindowEnd?: string;
  serviceTimeEvaluation?: { depot: string; brand: string; actualStopCount: number; configuredMinutes: number; meanObservedMinutes?: number; meanAbsoluteErrorMinutes?: number; status: string }[];
  capacity: { depot: string; projectedWeightKg: number; projectedVolumeM3: number; estimatedWeightCapacityKg: number; estimatedVolumeCapacityM3: number; pressure: string }[];
};


export function ForecastPage() {
  const { user } = useAuth();
  const { t } = useLocale();
  const [forecast, setForecast] = useState<Forecast | null>(null);
  const [error, setError] = useState("");
  const inputDrift = forecast?.inputDrift ?? [];
  const weekly = forecast?.weekly ?? [];
  const capacity = forecast?.capacity ?? [];
  const serviceTimeEvaluation = forecast?.serviceTimeEvaluation ?? [];
  useEffect(() => {
    if (!user?.access_token) return;
    void apiJSON<{ forecast: Forecast }>("/orders/forecast", user.access_token)
      .then((r) => setForecast(r.forecast))
      .catch((e: unknown) => setError(e instanceof Error ? e.message : t("Forecast could not be loaded")));
  }, [user?.access_token]);

  return <section>
    <h2>{t("Demand forecast and capacity")}</h2>
    <p role="note">{t("Planning estimates only. Demand uses the previous four complete weeks; sparse history may understate future demand. Confirmed orders and plan constraints remain authoritative.")}</p>
    {error && <p role="alert" className="status-bad">{t("Forecast could not be loaded. Try again later.")} {error}</p>}
    {!forecast && !error && <p role="status">{t("Loading estimates…")}</p>}
    {forecast && <>
      <p>{t("Forecast version")} {forecast.forecastVersion} · {forecast.method} · {t("refreshed")} {new Date(forecast.generatedAt).toLocaleString()}</p>
      <article><h3>{t("Service-time estimate")} · {forecast.serviceEstimateVersion}</h3><p>{forecast.serviceMinutesPerStop} {t("minutes per stop")} · {t(forecast.serviceEstimateSource)}</p><p>{t("This deterministic allowance is a planning input, not an observed arrival promise.")}</p></article>
      <article>
        <h3>{t("Observed service-time evaluation")} · {forecast.serviceTimeBacktestVersion || "unavailable"}</h3>
        <p>{t("Compares the configured allowance with completed delivered/partial stop durations from arrival to outcome. It reports evaluation only and never changes the plan.")} {t("Arrival-to-outcome durations outside 0–240 minutes are excluded. Error metrics are hidden below ten valid stops.")} {forecast.serviceTimeBacktestWindowStart || "—"} – {forecast.serviceTimeBacktestWindowEnd || "—"}</p>
        {serviceTimeEvaluation.length === 0 ? <p>{t("No completed stop-duration history is available for this evaluation window.")}</p> : <div className="table-scroll"><table><thead><tr><th>{t("Depot")}</th><th>{t("Brand")}</th><th>{t("Observed stops")}</th><th>{t("Configured minutes")}</th><th>{t("Mean observed minutes")}</th><th>{t("Mean absolute error minutes")}</th><th>{t("Evaluation status")}</th></tr></thead><tbody>{serviceTimeEvaluation.map((row) => <tr key={`${row.depot}-${row.brand}`}><td>{row.depot}</td><td>{row.brand}</td><td>{row.actualStopCount}</td><td>{row.configuredMinutes}</td><td>{row.meanObservedMinutes == null ? "—" : row.meanObservedMinutes.toFixed(1)}</td><td>{row.meanAbsoluteErrorMinutes == null ? "—" : row.meanAbsoluteErrorMinutes.toFixed(1)}</td><td>{t(row.status)}</td></tr>)}</tbody></table></div>}
      </article>
      <article>
        <h3>{t("Input drift monitor")} · {forecast.driftModelVersion}</h3>
        <p>{t("Compares confirmed-order counts in the latest four complete weeks with the prior four weeks. Flags are monitoring signals, not predictions or automatic planning changes.")}</p>
        <p>{t("Backtest uses the earlier four-week count as a baseline for the later four weeks. Error is withheld when actual holdout count is below ten.")} · {forecast.backtestModelVersion}</p>
        {inputDrift.length === 0 ? <p>{t("Insufficient order history to monitor input drift.")}</p> : <div className="table-scroll"><table><thead><tr><th>{t("Depot")}</th><th>{t("Brand")}</th><th>{t("Previous four weeks")}</th><th>{t("Latest four weeks")}</th><th>{t("Change")}</th><th>{t("Holdout error")}</th><th>{t("Drift status")}</th></tr></thead><tbody>{inputDrift.map((d) => <tr key={`${d.depot}-${d.brand}`}><td>{d.depot}</td><td>{d.brand}</td><td>{d.previousOrderCount}</td><td>{d.recentOrderCount}</td><td>{d.changePercent == null ? "—" : `${d.changePercent.toFixed(1)}%`}</td><td>{d.backtestAPEPercent == null ? "—" : `${d.backtestAPEPercent.toFixed(1)}%`}</td><td>{t(d.status)}</td></tr>)}</tbody></table></div>}
      </article>
      <h3>{t("Weekly demand by depot and brand")}</h3>
      {weekly.length === 0 ? <p>{t("No confirmed order history is available for an estimate.")}</p> : <div className="table-scroll"><table><thead><tr><th>{t("Week starting")}</th><th>{t("Depot")}</th><th>{t("Brand")}</th><th>{t("Chilled orders")}</th><th>{t("Ambient orders")}</th><th>{t("Estimated weight")}</th><th>{t("Estimated volume")}</th></tr></thead><tbody>{weekly.map((b, i) => <tr key={`${b.weekStarting}-${b.depot}-${b.brand}-${i}`}><td>{b.weekStarting}</td><td>{b.depot}</td><td>{b.brand}</td><td>{b.chilledOrders}</td><td>{b.ambientOrders}</td><td>{b.estimatedWeightKg.toFixed(2)} kg</td><td>{b.estimatedVolumeM3.toFixed(3)} m³</td></tr>)}</tbody></table></div>}
      <h3>{t("Estimated capacity pressure")}</h3>
      <p>{t("Pressure compares estimated demand with configured vehicle weight and volume capacity across five operating days. It does not reserve vehicles.")}</p>
      {capacity.length === 0 ? <p>{t("Fleet capacity data is unavailable for comparison.")}</p> : <div className="table-scroll"><table><thead><tr><th>{t("Depot")}</th><th>{t("Demand weight")}</th><th>{t("Weight capacity")}</th><th>{t("Demand volume")}</th><th>{t("Volume capacity")}</th><th>{t("Pressure")}</th></tr></thead><tbody>{capacity.map((c) => <tr key={c.depot}><td>{c.depot}</td><td>{c.projectedWeightKg.toFixed(1)} kg</td><td>{c.estimatedWeightCapacityKg.toFixed(1)} kg</td><td>{c.projectedVolumeM3.toFixed(3)} m³</td><td>{c.estimatedVolumeCapacityM3.toFixed(3)} m³</td><td>{t(c.pressure)}</td></tr>)}</tbody></table></div>}
    </>}
  </section>;
}
