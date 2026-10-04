import { useState } from "react";
import { todayInSriLanka } from "../api/date.mjs";
import { DEPOT_LABELS, sameDepot } from "../api/loading";
import { useLocale } from "../i18n";
import { useDepot } from "./DispatcherLayout";
import { operatingDaysPerWeek } from "./forecastCapacity.mjs";
import { isRefrigerated, Vehicle } from "./types";
import { BRAND_COLORS, ChipGroup, DpHero, Note, Panel, Stat, StatRow, Tag } from "./ui";
import { dateTime, dayLabel, useApi } from "./useApi";

type Forecast = {
  generatedAt: string; forecastVersion: string; method: string; historyWeeks: number; driftModelVersion: string; backtestModelVersion: string;
  inputDrift: { depot: string; brand: string; previousOrderCount: number; recentOrderCount: number; changePercent?: number; backtestAPEPercent?: number; status: string }[];
  weekly: { weekStarting: string; depot: string; brand: string; chilledOrders: number; ambientOrders: number; estimatedWeightKg: number; estimatedVolumeM3: number; estimate: boolean }[];
  serviceMinutesPerStop: number; serviceEstimateVersion: string; serviceEstimateSource: string;
  serviceTimeBacktestVersion?: string; serviceTimeBacktestWindowStart?: string; serviceTimeBacktestWindowEnd?: string;
  serviceTimeEvaluation?: { depot: string; brand: string; actualStopCount: number; configuredMinutes: number; meanObservedMinutes?: number; meanAbsoluteErrorMinutes?: number; status: string }[];
  capacity: { depot: string; projectedWeightKg: number; projectedVolumeM3: number; estimatedWeightCapacityKg: number; estimatedVolumeCapacityM3: number; pressure: string }[];
};
type CalendarDay = { date: string; isOperating: boolean };
type Week = { start: string; byBrand: Record<string, number>; total: number; chilled: number };
const BRANDS = ["Fresh", "Style", "Tech"];
const RANGE = 0.1;

function addDays(date: string, days: number) { const d = new Date(`${date}T00:00:00Z`); d.setUTCDate(d.getUTCDate() + days); return d.toISOString().slice(0, 10); }
function weekNo(date: string) { const d = new Date(`${date}T00:00:00Z`); const day = (d.getUTCDay() + 6) % 7; d.setUTCDate(d.getUTCDate() - day + 3); const first = new Date(Date.UTC(d.getUTCFullYear(), 0, 4)); return 1 + Math.round(((d.getTime() - first.getTime()) / 86400000 - 3 + ((first.getUTCDay() + 6) % 7)) / 7); }
const approx = (v: number) => `≈ ${Math.round(v).toLocaleString("en-LK")} m³`;

export function ForecastPage() {
  const { t } = useLocale();
  const { depot: shellDepot } = useDepot();
  const [depot, setDepot] = useState<string>(shellDepot);
  const today = todayInSriLanka();
  const result = useApi<{ forecast: Forecast }>("/orders/forecast");
  const forecast = result.data?.forecast;
  const policy = useApi<{ policy: { maxTripsPerVehicle: number } }>("/shared/policies/current");
  const calendar = useApi<{ items: CalendarDay[] }>(`/shared/calendar?from=${today}&to=${addDays(today, 41)}`);
  const fleet = useApi<{ items: Vehicle[] }>("/fleet/vehicles");

  const buckets = (forecast?.weekly || []).filter((b) => !depot || sameDepot(b.depot, depot));
  const weeksMap = new Map<string, Week>();
  for (const b of buckets) {
    const w = weeksMap.get(b.weekStarting) || { start: b.weekStarting, byBrand: {}, total: 0, chilled: 0 };
    w.byBrand[b.brand] = (w.byBrand[b.brand] || 0) + b.estimatedVolumeM3;
    w.total += b.estimatedVolumeM3;
    const orders = b.chilledOrders + b.ambientOrders;
    w.chilled += orders > 0 ? b.estimatedVolumeM3 * (b.chilledOrders / orders) : 0;
    weeksMap.set(b.weekStarting, w);
  }
  const weeks = [...weeksMap.values()].sort((a, b) => a.start.localeCompare(b.start));
  const next = weeks[0];
  const busiest = [...weeks].sort((a, b) => b.total - a.total)[0];
  const avg = weeks.length ? weeks.reduce((s, w) => s + w.total, 0) / weeks.length : 0;
  const capacityRows = (forecast?.capacity || []).filter((c) => !depot || sameDepot(c.depot, depot));
  const weeklyCapacity = capacityRows.reduce((s, c) => s + c.estimatedVolumeCapacityM3, 0);
  const pressureOf = (w: Week) => (weeklyCapacity > 0 ? w.total / weeklyCapacity : 0);
  const pressureTag = (p: number) => p > 1 ? <Tag tone="red">{t("Over capacity")}</Tag> : p > 0.9 ? <Tag tone="amber">{t("Tight")}</Tag> : <Tag tone="green">{t("Normal")}</Tag>;
  const overWeeks = weeks.filter((w) => pressureOf(w) > 1);
  // Refrigerated trips a day: estimated chilled volume per operating day over
  // the average reefer's volume, against the depot's reefers at the policy's trips per vehicle.
  const reefers = (fleet.data?.items || []).filter((v) => isRefrigerated(v) && (!depot || sameDepot(v.homeDepot, depot)));
  const reeferM3 = reefers.length ? reefers.reduce((s, v) => s + v.volumeCapacityM3, 0) / reefers.length : 0;
  // Trips a vehicle may run a day is the planning policy's (Master data); operating days a week come from
  // the operating calendar. If either is not known the refrigerated-trip maths is skipped, not guessed.
  const tripsPerVehicle = policy.data?.policy?.maxTripsPerVehicle;
  const operatingDays = operatingDaysPerWeek(calendar.data?.items);
  const reeferTripsAvailable = tripsPerVehicle ? reefers.length * tripsPerVehicle : 0;
  const reeferTripsNeeded = (w: Week) => (reeferM3 > 0 && operatingDays ? Math.ceil(w.chilled / operatingDays / reeferM3) : 0);
  const reeferShort = [...weeks].map((w) => ({ w, need: reeferTripsNeeded(w) })).filter((x) => reeferTripsAvailable > 0 && x.need > reeferTripsAvailable).sort((a, b) => b.need - a.need)[0];
  const max = Math.max(1, ...weeks.map((w) => w.total * (1 + RANGE)));

  const days = calendar.data?.items || [];
  const weekOfDay = (date: string) => weeks.filter((w) => w.start <= date).slice(-1)[0];
  const dayState = (day: CalendarDay) => {
    if (!day.isOperating) return "off";
    const w = weekOfDay(day.date);
    if (!w || !avg) return "normal";
    return w.total > avg * 1.2 ? "peak" : w.total > avg * 1.05 ? "busy" : "normal";
  };
  const firstDay = days[0]?.date || today;
  const lead = (new Date(`${firstDay}T00:00:00Z`).getUTCDay() + 6) % 7;

  return (
    <>
      <DpHero title={t("Demand forecast")} subtitle={t("Estimated order volume for the next weeks, by depot and brand, to plan vehicles, drivers and refrigerated capacity.")}>
        <div className="dp-row" style={{ background: "#fff", padding: 4, borderRadius: 8 }}>
          <ChipGroup label={t("Depot")} value={depot} onChange={setDepot} options={[...Object.entries(DEPOT_LABELS).map(([code, label]) => ({ value: code, label })), { value: "", label: t("Both depots") }]} />
        </div>
      </DpHero>
      {forecast && <StatRow cols={4}>
        <Stat icon="▥" label={`${t("Estimated volume next week")}${next ? ` (W${weekNo(next.start)})` : ""}`} value={next ? approx(next.total) : "—"} sub={next ? `${t("Likely range")} ${Math.round(next.total * (1 - RANGE))}–${Math.round(next.total * (1 + RANGE))} m³` : t("No estimate yet")} />
        <Stat icon="❄" iconTone="cool" label={t("Estimated chilled volume")} value={next ? approx(next.chilled) : "—"} sub={next && next.total ? `${Math.round((next.chilled / next.total) * 100)}% ${t("of the total")}` : ""} />
        <Stat icon="▦" iconTone="amber" label={t("Busiest week ahead")} value={busiest ? `W${weekNo(busiest.start)} · ${dayLabel(busiest.start)}` : "—"} sub={busiest ? approx(busiest.total) : ""} subTone="amber" />
        {reeferShort
          ? <Stat icon="❄" iconTone="red" variant="red" label={`${t("Refrigerated shortfall in")} W${weekNo(reeferShort.w.start)}`} value={`≈ ${reeferShort.need - reeferTripsAvailable} ${t("trips a day")}`} sub={`${t("Needs")} ${reeferShort.need} ${t("reefer trips a day")}, ${reeferTripsAvailable} ${t("available")} · ${overWeeks.length} ${t("week(s) over capacity")}`} subTone="red" />
          : <Stat icon="▣" iconTone={overWeeks.length ? "red" : "green"} label={t("Weeks over estimated capacity")} value={overWeeks.length} sub={reeferTripsAvailable ? `${t("No refrigerated shortfall")} · ${reeferTripsAvailable} ${t("reefer trips a day available")}` : weeklyCapacity ? `${t("Weekly capacity")} ${approx(weeklyCapacity)}` : t("Fleet capacity data is unavailable for comparison.")} subTone={overWeeks.length ? "red" : "green"} />}
      </StatRow>}
      {!forecast && <div style={{ height: 24 }} />}
      <div className={`dp-body${forecast ? "" : " dp-body--flush"}`}>
        <Note title={<>{t("Estimates, not orders")} <Tag tone="primary">{t("Estimate")}</Tag></>}>{t("Planning estimates only. Demand uses the previous four complete weeks; sparse history may understate future demand. Confirmed orders and plan constraints remain authoritative.")}</Note>
        {result.error && <p role="alert" className="dp-note dp-note--red">{t("Forecast could not be loaded. Try again later.")} {result.error}</p>}
        {!forecast && !result.error && <p role="status">{t("Loading estimates…")}</p>}
        {forecast && <>
          <div className="dp-grid-2">
            <Panel title={t("Weekly demand by brand")} sub={`${t("Estimated total volume per week, m³")} · ${depot ? DEPOT_LABELS[depot] : t("Both depots")}`} actions={<ul className="dp-legend" style={{ flexDirection: "row", gap: 14 }}>{BRANDS.map((b) => <li key={b}><span className="dp-swatch" style={{ background: BRAND_COLORS[b.toLowerCase()] }} />{t(b)}</li>)}<li>│ {t("Likely range")} (±10%)</li></ul>}>
              {weeks.length === 0 ? <p className="muted">{t("No confirmed order history is available for an estimate.")}</p> : <>
                <div className="dp-chart" role="img" aria-label={t("Weekly demand by brand")}>
                  {weeks.map((w) => {
                    const peak = busiest && w.start === busiest.start;
                    return (
                      <div key={w.start} className={`dp-chart-col${peak ? " dp-chart-col--peak" : ""}`}>
                        {peak && <span className="dp-chart-tag" style={{ color: "#c2410c" }}>{t("Peak")}</span>}
                        <span className="dp-chart-value">≈{Math.round(w.total)}</span>
                        <div className="dp-chart-bar" style={{ height: `${(w.total / max) * 82}%` }}>
                          {BRANDS.map((b) => <span key={b} style={{ height: `${w.total ? ((w.byBrand[b] || 0) / w.total) * 100 : 0}%`, background: BRAND_COLORS[b.toLowerCase()] }} />)}
                        </div>
                        <span className="dp-chart-label">W{weekNo(w.start)}<br /><span className="muted" style={{ fontWeight: 400 }}>{dayLabel(w.start)}</span></span>
                      </div>
                    );
                  })}
                </div>
                <div className="dp-grid-3" style={{ marginTop: 16, gridTemplateColumns: "repeat(4, minmax(0,1fr))" }}>
                  {BRANDS.map((b) => <div key={b} className="dp-subcard"><p className="dp-stat-label"><span className="dp-swatch" style={{ display: "inline-block", background: BRAND_COLORS[b.toLowerCase()] }} /> {t(b)} · {t("all weeks")}</p><p className="dp-stat-value">{approx(weeks.reduce((s, w) => s + (w.byBrand[b] || 0), 0))}</p></div>)}
                  <div className="dp-subcard"><p className="dp-stat-label">❄ {t("Chilled, all weeks")}</p><p className="dp-stat-value">{approx(weeks.reduce((s, w) => s + w.chilled, 0))}</p></div>
                </div>
              </>}
            </Panel>
            <Panel title={t("Busy-day calendar")} sub={t("How full each operating day is likely to be")}>
              <div className="dp-calendar" aria-label={t("Busy-day calendar")}>
                {[t("Mon"), t("Tue"), t("Wed"), t("Thu"), t("Fri"), t("Sat"), t("Sun")].map((d) => <span key={d} className="dp-cal-head">{d}</span>)}
                {Array.from({ length: lead }, (_, i) => <span key={`lead-${i}`} />)}
                {days.map((day) => { const s = dayState(day); return <span key={day.date} className={`dp-cal-day${s === "off" ? " dp-cal-day--off" : s === "busy" ? " dp-cal-day--busy" : s === "peak" ? " dp-cal-day--peak" : ""}${day.date === today ? " dp-cal-day--today" : ""}`} title={`${day.date} · ${t(s === "off" ? "No deliveries" : s === "peak" ? "Peak" : s === "busy" ? "Busy" : "Normal")}`}>{Number(day.date.slice(8))}</span>; })}
              </div>
              {calendar.error && <p className="muted">{t("Operating calendar is unavailable.")}</p>}
              <ul className="dp-legend" style={{ flexDirection: "row", flexWrap: "wrap", gap: 12, marginTop: 12 }}>
                <li><span className="dp-swatch" style={{ background: "#e9ecef" }} />{t("Normal")}</li>
                <li><span className="dp-swatch" style={{ background: "#fff4de" }} />{t("Busy")}</li>
                <li><span className="dp-swatch" style={{ background: "#fceeeb" }} />{t("Peak")}</li>
                <li><span className="dp-swatch" style={{ background: "transparent", border: "1px solid #b3bac8" }} />{t("No deliveries")}</li>
              </ul>
              {busiest && <Note tone="red" title={`${t("Peak week")} · W${weekNo(busiest.start)}`}>{`${approx(busiest.total)} ${t("estimated")}, ${approx(busiest.chilled)} ${t("of it chilled.")} ${t("Move chilled orders earlier in the week where outlets can take them.")}`}</Note>}
              <p className="muted" style={{ fontSize: "0.75rem", margin: "8px 0 0" }}>{t("Busy days are derived from each week's estimated volume against the average week.")}</p>
            </Panel>
          </div>
          <Panel title={t("Capacity pressure by week")} sub={t("Estimated volume against what the fleet can move. Pressure compares estimated demand with configured vehicle weight and volume capacity across five operating days. It does not reserve vehicles.")} flush actions={<span className="muted">{t("Volumes in m³")}</span>}>
            {weeks.length === 0 ? <p className="dp-empty">{t("No confirmed order history is available for an estimate.")}</p> : (
              <div className="dp-table-wrap"><table className="dp-table">
                <thead><tr><th>{t("Week")}</th><th>{t("Starts")}</th>{BRANDS.map((b) => <th key={b}>{t(b)}</th>)}<th>{t("Total (est.)")}</th><th>{t("Chilled (est.)")}</th><th>{t("Load vs capacity")}</th><th>{reeferTripsAvailable ? `${t("Reefer trips a day vs")} ${reeferTripsAvailable}` : t("Reefer trips a day")}</th><th>{t("Pressure")}</th></tr></thead>
                <tbody>{weeks.map((w) => { const p = pressureOf(w); return (
                  <tr key={w.start} className={p > 1 ? "is-alert" : undefined}>
                    <td><strong>W{weekNo(w.start)}</strong></td><td>{w.start}</td>
                    {BRANDS.map((b) => <td key={b}>{Math.round(w.byBrand[b] || 0)}</td>)}
                    <td>{approx(w.total)}</td><td>{approx(w.chilled)}</td>
                    <td style={{ minWidth: 140 }}>{weeklyCapacity ? `${Math.round(p * 100)}%` : "—"}<div className="dp-meter-track" style={{ marginTop: 4 }}><span className="dp-meter-fill" style={{ width: `${Math.min(100, p * 100)}%`, background: p > 1 ? "#c03221" : p > 0.9 ? "#d9822b" : "#008b52" }} /></div></td>
                    <td>{reeferM3 ? <span className={reeferTripsNeeded(w) > reeferTripsAvailable ? "dp-cell-sub--red" : undefined}>{reeferTripsNeeded(w)} / {reeferTripsAvailable}</span> : "—"}</td>
                    <td>{weeklyCapacity ? pressureTag(p) : "—"}</td>
                  </tr>
                ); })}</tbody>
              </table></div>
            )}
            <div className="dp-panel-body" style={{ paddingTop: 12 }}><p className="dp-note dp-note--cool" style={{ margin: 0 }}>ⓘ {t("These are estimates from past orders, with a typical error of about ±10%. Actual orders are confirmed each day at the 4 PM cutoff and always take priority over the forecast.")}</p></div>
          </Panel>
          {capacityRows.length > 0 && <Panel title={t("Estimated capacity pressure")} flush>
            <div className="dp-table-wrap"><table className="dp-table">
              <thead><tr><th>{t("Depot")}</th><th>{t("Demand weight")}</th><th>{t("Weight capacity")}</th><th>{t("Demand volume")}</th><th>{t("Volume capacity")}</th><th>{t("Pressure")}</th></tr></thead>
              <tbody>{capacityRows.map((c) => <tr key={c.depot}><td>{DEPOT_LABELS[c.depot] || c.depot}</td><td>{c.projectedWeightKg.toFixed(1)} kg</td><td>{c.estimatedWeightCapacityKg.toFixed(1)} kg</td><td>{c.projectedVolumeM3.toFixed(3)} m³</td><td>{c.estimatedVolumeCapacityM3.toFixed(3)} m³</td><td>{t(c.pressure)}</td></tr>)}</tbody>
            </table></div>
          </Panel>}
          <section className="dp-panel">
            <details className="dp-details">
              <summary>{t("Model checks")} · {t("Forecast version")} {forecast.forecastVersion} · {forecast.method} · {t("refreshed")} {dateTime(forecast.generatedAt)}</summary>
              <div className="dp-stack">
                <h3 className="dp-h3">{t("Service-time estimate")} · {forecast.serviceEstimateVersion}</h3>
                <p style={{ margin: 0 }}>{forecast.serviceMinutesPerStop} {t("minutes per stop")} · {t(forecast.serviceEstimateSource)}. {t("This deterministic allowance is a planning input, not an observed arrival promise.")}</p>
                <h3 className="dp-h3">{t("Observed service-time evaluation")} · {forecast.serviceTimeBacktestVersion || "unavailable"}</h3>
                <p className="muted" style={{ margin: 0 }}>{t("Compares the configured allowance with completed delivered/partial stop durations from arrival to outcome. It reports evaluation only and never changes the plan.")} {t("Arrival-to-outcome durations outside 0–240 minutes are excluded. Error metrics are hidden below ten valid stops.")} {forecast.serviceTimeBacktestWindowStart || "—"} – {forecast.serviceTimeBacktestWindowEnd || "—"}</p>
                {(forecast.serviceTimeEvaluation || []).length === 0 ? <p style={{ margin: 0 }}>{t("No completed stop-duration history is available for this evaluation window.")}</p> : <div className="dp-table-wrap"><table className="dp-table"><thead><tr><th>{t("Depot")}</th><th>{t("Brand")}</th><th>{t("Observed stops")}</th><th>{t("Configured minutes")}</th><th>{t("Mean observed minutes")}</th><th>{t("Mean absolute error minutes")}</th><th>{t("Evaluation status")}</th></tr></thead><tbody>{forecast.serviceTimeEvaluation!.map((row) => <tr key={`${row.depot}-${row.brand}`}><td>{row.depot}</td><td>{row.brand}</td><td>{row.actualStopCount}</td><td>{row.configuredMinutes}</td><td>{row.meanObservedMinutes == null ? "—" : row.meanObservedMinutes.toFixed(1)}</td><td>{row.meanAbsoluteErrorMinutes == null ? "—" : row.meanAbsoluteErrorMinutes.toFixed(1)}</td><td>{t(row.status)}</td></tr>)}</tbody></table></div>}
                <h3 className="dp-h3">{t("Input drift monitor")} · {forecast.driftModelVersion}</h3>
                <p className="muted" style={{ margin: 0 }}>{t("Compares confirmed-order counts in the latest four complete weeks with the prior four weeks. Flags are monitoring signals, not predictions or automatic planning changes.")} {t("Backtest uses the earlier four-week count as a baseline for the later four weeks. Error is withheld when actual holdout count is below ten.")} · {forecast.backtestModelVersion}</p>
                {forecast.inputDrift.length === 0 ? <p style={{ margin: 0 }}>{t("Insufficient order history to monitor input drift.")}</p> : <div className="dp-table-wrap"><table className="dp-table"><thead><tr><th>{t("Depot")}</th><th>{t("Brand")}</th><th>{t("Previous four weeks")}</th><th>{t("Latest four weeks")}</th><th>{t("Change")}</th><th>{t("Holdout error")}</th><th>{t("Drift status")}</th></tr></thead><tbody>{forecast.inputDrift.map((d) => <tr key={`${d.depot}-${d.brand}`}><td>{d.depot}</td><td>{d.brand}</td><td>{d.previousOrderCount}</td><td>{d.recentOrderCount}</td><td>{d.changePercent == null ? "—" : `${d.changePercent.toFixed(1)}%`}</td><td>{d.backtestAPEPercent == null ? "—" : `${d.backtestAPEPercent.toFixed(1)}%`}</td><td>{t(d.status)}</td></tr>)}</tbody></table></div>}
              </div>
            </details>
          </section>
        </>}
      </div>
    </>
  );
}
