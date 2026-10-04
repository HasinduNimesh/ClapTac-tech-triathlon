export declare const SILENT_TRIP_THRESHOLD_MINUTES: number;
export declare const CHILLED_ALLOWED_MINUTES_DEFAULT: number;

export interface SilentTripStatus {
  isSilent: boolean;
  elapsedMinutes: number;
  timeStr: string;
  lastKnownPlace: string;
  label: string;
}

export interface ChilledTripStatus {
  isChilled: boolean;
  isChilledLong: boolean;
  onBoardMinutes: number;
  allowedMinutes: number;
  label: string;
}

export interface TripWatchAlert {
  id: string;
  tripId: string;
  vehicleId: string;
  type: "SILENT_TRIP" | "CHILLED_OVERAGE";
  title: string;
  message: string;
  severity: "warning" | "amber";
  acknowledged: boolean;
  lastKnownPlace?: string;
  timeStr?: string;
  chilledMinutes?: number;
  allowedMinutes?: number;
}

export declare function formatWatchTime(timeVal: string | number | Date | undefined): string;
export declare function evaluateTripSilentStatus(trip: Record<string, any>, now?: number): SilentTripStatus;
export declare function evaluateChilledOnBoardStatus(trip: Record<string, any>): ChilledTripStatus;
export declare function enrichTripWithWatch<T extends Record<string, any>>(trip: T, now?: number): T & {
  isSilent: boolean;
  silentTime: string;
  silentLabel: string;
  lastKnownPlace: string;
  isChilled: boolean;
  isChilledLong: boolean;
  chilledMinutes: number;
  chilledAllowedMinutes: number;
  chilledLabel: string;
};
export declare function generateNeedsActionAlerts(
  trips: Array<Record<string, any>>,
  acknowledgedAlertIds?: Set<string>,
  now?: number
): TripWatchAlert[];
