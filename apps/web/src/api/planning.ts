export type Plan = {
  id: string;
  planRef: string;
  deliveryDate: string;
  status: string;
  createdBy: string;
  generatedAt?: string;
  currentVersion?: number;
  publishedAt?: string;
};

export type Allocation = {
  id: string;
  planId: string;
  orderId: string;
  tripId: string;
  vehicleId: string;
  sequence: number;
  plannedArrivalAt?: string;
  plannedServiceStartAt?: string;
  plannedDepartureAt?: string;
};

export type Deferral = {
  id: string;
  orderId: string;
  outletId: string;
  reasonCode: string;
  comment?: string;
  nextRunTarget?: string;
};

export type OtherLimitingFactor = {
  reasonCode: string;
  vehicleTripsBlocked: number;
};

export type Unallocated = {
  orderId: string;
  orderRef?: string;
  reasonCode: string;
  details?: {
    otherLimitingFactors?: OtherLimitingFactor[];
    vehicleTripsEvaluated?: number;
    primaryBlockedVehicleTrips?: number;
  };
};

export type PlanVehicle = {
  id: string;
  type: string;
  temp: string;
  homeDepot: string;
  status: string;
  weightCapacityKg: number;
  volumeCapacityM3: number;
  weeklyFuelQuotaL: number;
  weekFuelUsedL?: number;
  weekFuelPlannedL?: number;
  weekFuelActualL?: number;
  planFuelL?: number;
};

export type FuelLedger = {
  weekOf: string;
  weekStart: string;
  weekEnd: string;
  items: { vehicleId: string; weeklyQuotaL: number; actualLitersL: number }[];
  entries: { id: string; vehicleId: string; date: string; liters: number; receiptRef?: string; note?: string; recordedBy: string; createdAt: string }[];
};

export type DisruptionRisk = {
  id: string;
  deliveryDate: string;
  scope: "DEPOT" | "DISTRICT" | "ROUTE";
  scopeKey: string;
  riskType: "HEAVY_RAIN" | "FLOODING" | "LANDSLIDE" | "ROAD_CLOSURE" | "ROAD_DAMAGE" | "OTHER";
  severity: "LOW" | "MEDIUM" | "HIGH";
  summary: string;
  source: string;
  sourceReference?: string;
  confidence: number;
  createdBy: string;
  createdAt: string;
  overrideDecision?: "ACKNOWLEDGED" | "OVERRIDE" | "DISMISSED";
  overrideSeverity?: "LOW" | "MEDIUM" | "HIGH";
  overrideReason?: string;
  overriddenBy?: string;
  overriddenAt?: string;
};

export type PlanOrder = {
  id: string;
  orderRef: string;
  sourceSystem?: string;
  outletId: string;
  brand: string;
  temperatureRequirement: string;
  orderWeightKg: number;
  orderVolumeM3: number;
  outletDeferralCount: number;
  daysSinceLastServed: number;
  lastServedAt?: string;
  fairnessScore: number;
  deferredLastRun?: boolean;
  lastDeferralDate?: string;
};

export type PlanDetail = {
  plan: Plan;
  trips: { id: string; vehicleId: string; tripNumber: number; status: string }[];
  allocations: Allocation[];
  deferrals: Deferral[];
  unallocated: Unallocated[];
  vehicles: PlanVehicle[];
  orders: PlanOrder[];
  fairness?: {
    signalAvailable: boolean;
    policy: string;
    asOf: string;
  };
  fuelLedgerAvailable?: boolean;
  policySignalAvailable?: boolean;
  unallocatedReasonsAvailable?: boolean;
  planningPolicy?: { version: number; cutoffLocalTime: string; deferralWeightPoints: number; maxDeferralCount: number; maxUnservedDays: number; maxTripsPerVehicle: number };
  publication?: { version: number; contentHash: string; publishedBy: string; publishedAt: string; acknowledgements: {actorId:string;actorRole:string;acknowledgedAt:string}[] };
};

export const DEFER_REASONS = [
  "NO_ELIGIBLE_VEHICLE",
  "WEIGHT_CAPACITY_EXCEEDED",
  "VOLUME_CAPACITY_EXCEEDED",
  "REFRIGERATION_REQUIRED",
  "VAN_REQUIRED",
  "DEPOT_MISMATCH",
  "DELIVERY_WINDOW_CONFLICT",
  "FUEL_QUOTA_EXCEEDED",
  "TRIP_LIMIT_REACHED",
  "VEHICLE_UNAVAILABLE",
  "MANUAL_DISPATCHER_DEFERRAL",
];
