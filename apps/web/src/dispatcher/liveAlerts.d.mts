import type { DisruptionRisk } from "../api/planning";
export type RiskSeverity = "LOW" | "MEDIUM" | "HIGH";
export function effectiveSeverity(risk: DisruptionRisk | undefined | null): RiskSeverity | undefined;
export function disruptionsForTrip(
  risks: DisruptionRisk[] | undefined | null,
  trip: { tripId: string; vehicleId?: string; depot?: string; planRef?: string; remainingDistricts?: string[] },
): (DisruptionRisk & { effectiveSeverity: RiskSeverity })[];
export type GroupableAction = { key: string; severity: "critical" | "high" | "medium" | "low"; tripId?: string; tripLabel?: string };
export function groupActionsByTrip<T extends GroupableAction>(actions: T[]): { key: string; tripId?: string; label?: string; severity: T["severity"]; items: T[] }[];
