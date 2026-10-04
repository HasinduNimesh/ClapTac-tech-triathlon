export const DEPOT_LABELS: Record<string, string> = {
  DEPOT_NORTH: "Peliyagoda",
  DEPOT_SOUTH: "Kandy",
};

// The dataset names depots "Peliyagoda" and "Kandy"; user profiles and the
// depot selector use DEPOT_NORTH and DEPOT_SOUTH. Compare through this.
export { depotCode } from "./depots.mjs";
import { depotCode } from "./depots.mjs";
export function sameDepot(a?: string, b?: string): boolean {
  return depotCode(a) === depotCode(b);
}

export function depotLabel(code?: string): string {
  if (!code) return "";
  return DEPOT_LABELS[code] ? `${DEPOT_LABELS[code]} (${code})` : code;
}

export type LoadingIssue = {
  id: string;
  orderLoadId: string;
  type: "MISSING" | "DAMAGED" | "WRONG_ITEM" | string;
  affectedUnits: number;
  note?: string;
  reportedBy?: string;
  decision?: "PARTIAL_LOAD" | "HOLD" | "MOVE_TO_NEXT_RUN";
  decisionNote?: string;
  decidedBy?: string;
  decidedAt?: string;
  reportedAt?: string;
  hasPhoto?: boolean;
  seenBy?: string;
  seenAt?: string;
};

/** A loader's "tell dispatcher" alert, such as goods staged at the wrong vehicle. */
export type DockAlert = {
  id: string;
  tripId: string;
  depot: string;
  deliveryDate: string;
  type: "WRONG_VEHICLE" | string;
  orderRef: string;
  belongsVehicleId?: string;
  note?: string;
  reportedBy: string;
  createdAt: string;
  resolvedBy?: string;
  resolvedAt?: string;
};

export type LoadingOrder = {
  orderId: string;
  orderRef?: string;
  outletId?: string;
  brand?: string;
  temperatureRequirement?: string;
  expectedUnits?: number;
  stopSequence: number;
  suggestedLoadSequence: number;
  status: string;
  issues?: LoadingIssue[];
};

export type LoadingTripSummary = {
  tripId: string;
  planRef?: string;
  vehicleId?: string;
  tripNumber?: number;
  depot?: string;
  vehicleType?: string;
  vehicleTemperatureCapability?: string;
  stopCount?: number;
  loadingStatus?: string;
  loadedCount?: number;
  shortfallCount?: number;
  pendingCount?: number;
};

export type LoadingTripDetail = LoadingTripSummary & {
  planId?: string;
  deliveryDate?: string;
  status?: string;
  orders?: LoadingOrder[];
  planVersion?: number;
  acknowledgedVersion?: number;
};

export function newIdempotencyKey(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `iss-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function isIncomplete(order: LoadingOrder): boolean {
  if (order.status === "pending") return true;
  if (order.status === "shortfall" && !(order.issues || []).length) return true;
  return false;
}
