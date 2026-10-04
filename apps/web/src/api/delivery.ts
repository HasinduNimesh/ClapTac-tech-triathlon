export type DeliveryStop = {
  id: string;
  runId: string;
  orderId: string;
  orderRef?: string;
  outletId?: string;
  outletName?: string;
  brand?: string;
  temperatureRequirement?: string;
  chilledTemperatureMinC?: number;
  chilledTemperatureMaxC?: number;
  temperatureReadings?: TemperatureReading[];
  district?: string;
  // The outlet's position; approximate means only the district centre is known.
  latitude?: number;
  longitude?: number;
  locationApproximate?: boolean;
  dockType?: string;
  parkingConstraint?: string;
  accessInstructions?: string;
  accessInstructionsUpdatedAt?: string;
  plannedWindowOpen?: string;
  plannedWindowClose?: string;
  stopSequence: number;
  loadingStatus?: string;
  loadingShortfallSummary?: { type: string; affectedUnits: number; note?: string }[];
  status: string;
  arrivedAt?: string;
  arrivedReceivedAt?: string;
  outcomeCode?: string;
  outcomeReason?: string;
  outcomeNote?: string;
  outcomeAt?: string;
  outcomeReceivedAt?: string;
};

export type TemperatureReading = { operationId:string; valueC:number; unit:"C"; occurredAt:string; receivedAt?:string; actorId:string; source:"manual"|"iot"; evaluation:"IN_RANGE"|"OUT_OF_RANGE"|"LIMITS_UNCONFIGURED"|"PENDING_SYNC"; minC?:number; maxC?:number; note?:string };

export type DeliveryRun = {
  id: string;
  tripId: string;
  planRef?: string;
  deliveryDate?: string;
  vehicleId?: string;
  depot?: string;
  tripNumber?: number;
  status: string;
  planId?: string;
  planVersion?: number;
  acknowledgedVersion?: number;
  startedAt?: string;
  completedAt?: string;
};

export type DeliveryTripSummary = {
  tripId: string;
  planRef?: string;
  vehicleId?: string;
  depot?: string;
  tripNumber?: number;
  status?: string;
  loadingStatus?: string;
  stopCount?: number;
  completedStops?: number;
  runId?: string;
};

export type DeliveryTripDetail = {
  tripId: string;
  status: string;
  run: DeliveryRun;
  stops: DeliveryStop[];
  currentPlanVersion?: number;
  planAcknowledgements?: {actorId:string;actorRole:string;acknowledgedAt:string}[];
};

export type LatenessProbability = {
  depot: string;
  brand: string;
  temperatureRequirement: string;
  sampleCount: number;
  lateCount: number;
  probability?: number;
  status: "ESTIMATED" | "INSUFFICIENT_HISTORY";
  modelVersion: string;
  historyDays: number;
  definition: string;
  calibrationSampleCount: number;
  brierScore?: number;
  calibrationStatus: "EVALUATED" | "INSUFFICIENT_HOLDOUT";
  calibrationVersion: string;
  arrivalRangeStatus: "CALIBRATED" | "INSUFFICIENT_HISTORY" | "INSUFFICIENT_HOLDOUT" | "POOR_COVERAGE";
  arrivalRangeVersion: string;
  arrivalRangeSamples: number;
  arrivalRangeHoldouts: number;
  arrivalRangeCoverage?: number;
  arrivalOffsetP10Minutes?: number;
  arrivalOffsetP90Minutes?: number;
};

export function newOperationId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `op-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}
