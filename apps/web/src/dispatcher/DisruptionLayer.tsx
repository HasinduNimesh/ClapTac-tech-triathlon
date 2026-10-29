import type { DisruptionArea } from "./disruptionMap.mjs";
import { RAIN_COLOR, SEVERITY_COLORS } from "./disruptionMap.mjs";
import type { MapArea } from "../components/WaypointMap";
import { useLocale } from "../i18n";

export const RISK_TYPE_LABELS: Record<string, string> = { HEAVY_RAIN: "Heavy rain", FLOODING: "Flooding", LANDSLIDE: "Landslide", ROAD_CLOSURE: "Road closure", ROAD_DAMAGE: "Road damage", OTHER: "Other" };

type T = (key: string) => string;

function rainText(area: DisruptionArea, t: T) {
  const r = area.rain;
  if (r?.dayRainMm == null) return "";
  return `${t("Rain forecast")}: ${r.dayRainMm} mm${r.peakHour && r.maxHourlyRainMm ? ` · ${t("heaviest around")} ${r.peakHour} (${r.maxHourlyRainMm} mm)` : ""}${r.rainProbability != null ? ` · ${r.rainProbability}% ${t("chance")}` : ""}`;
}

/** Map circles with a translated tooltip: the place, each recorded risk, then the rain forecast. */
export function toMapAreas(areas: DisruptionArea[], t: T): MapArea[] {
  return areas.map((area) => {
    const lines = [area.label, ...area.risks.map((r) => `${t(RISK_TYPE_LABELS[r.riskType] || r.riskType)} (${t(r.effectiveSeverity.toLowerCase())}) · ${r.summary}`), rainText(area, t)].filter(Boolean);
    return { id: area.id, at: area.at, radiusM: area.radiusM, color: area.color, dashed: area.dashed, title: lines.join(" — ") };
  });
}

export function DisruptionLegend({ rain }: { rain: boolean }) {
  const { t } = useLocale();
  return <>
    <p style={{ margin: 0 }}><span style={{ color: SEVERITY_COLORS.HIGH }}>◯</span> {t("Road or weather risk: high")}</p>
    <p style={{ margin: 0 }}><span style={{ color: SEVERITY_COLORS.MEDIUM }}>◯</span> {t("Road or weather risk: medium or low")}</p>
    {rain && <p style={{ margin: 0 }}><span style={{ color: RAIN_COLOR }}>◌</span> {t("Rain forecast (5 mm or more)")}</p>}
  </>;
}

/** Switches the risk and rain circles on or off, and says when the forecast could not be read. */
export function DisruptionToggle({ shown, onChange, forecastError }: { shown: boolean; onChange: (shown: boolean) => void; forecastError?: string }) {
  const { t } = useLocale();
  return <p className="muted" style={{ margin: 0, fontSize: "0.8125rem", display: "flex", gap: 12, flexWrap: "wrap", alignItems: "center" }}>
    <label style={{ display: "inline-flex", gap: 6, alignItems: "center" }}><input type="checkbox" checked={shown} onChange={(e) => onChange(e.target.checked)} /> {t("Show road risks and rain forecast")}</label>
    {shown && forecastError && <span>{t("Rain forecast unavailable for this date; recorded risks are still shown.")}</span>}
    {shown && !forecastError && <span>{t("Rain from Open-Meteo at each district centre.")}</span>}
  </p>;
}
