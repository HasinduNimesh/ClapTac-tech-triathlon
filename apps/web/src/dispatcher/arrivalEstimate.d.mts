export type ArrivalEstimateInput = {
  plannedArrivalAt?: string;
  plannedDepartureAt?: string;
  previousOutcomeAt?: string;
  previousArrivedAt?: string;
  serviceMinutesPerStop?: number;
  windowCloseAt?: string;
  deliveryDate?: string;
  now?: number;
};
export const ARRIVAL_ESTIMATE_VERSION: string;
export function previousReportedStop<T extends { outcomeAt?: string; outcomeReceivedAt?: string; arrivedAt?: string }>(stops: T[], beforeIndex: number): T | undefined;
export function estimateArrival(input: ArrivalEstimateInput): {
  version: string;
  kind: "unknown" | "risk" | "late" | "estimate";
  label: string;
  confidence: string;
  eta?: string;
  risk?: string;
  delayMinutes?: number;
};
