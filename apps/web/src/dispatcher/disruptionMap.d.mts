import type { DisruptionRisk, DistrictWeather, WeatherForecast } from "../api/planning";
import type { RiskSeverity } from "./liveAlerts.mjs";
export const SEVERITY_COLORS: Record<RiskSeverity, string>;
export const RAIN_COLOR: string;
export const RAIN_DRAW_MM: number;
export const WEATHER_SOURCE: string;
export type DisruptionArea = {
  id: string;
  kind: "risk" | "rain";
  at: [number, number];
  label: string;
  radiusM: number;
  color: string;
  dashed?: boolean;
  severity?: RiskSeverity;
  risks: (DisruptionRisk & { effectiveSeverity: RiskSeverity })[];
  rain?: DistrictWeather;
};
export function disruptionAreas(risks: DisruptionRisk[] | undefined | null, forecast: WeatherForecast | undefined | null, options?: { rain?: boolean }): DisruptionArea[];
export function pendingWeatherSuggestions(forecast: WeatherForecast | undefined | null, risks: DisruptionRisk[] | undefined | null): (DistrictWeather & { suggestion: NonNullable<DistrictWeather["suggestion"]> })[];
export function suggestionAsRisk(district: DistrictWeather, forecast: WeatherForecast): {
  deliveryDate: string; scope: "DISTRICT"; scopeKey: string; riskType: string; severity: string; summary: string; source: string; sourceReference: string; confidence: number;
};
