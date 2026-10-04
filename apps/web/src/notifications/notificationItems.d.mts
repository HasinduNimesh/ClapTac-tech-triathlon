import type { BellItem } from "./bellModel.mjs";
import type { PlanDetail } from "../api/planning";
import type { DockAlert, LoadingTripDetail, LoadingTripSummary } from "../api/loading";
import type { DeliveryTripDetail, DeliveryTripSummary } from "../api/delivery";
import type { Tracking } from "../store-manager/useOrderTrackings";
import type { AuditEvent, Incident, ReceiptIssue } from "../dispatcher/types";

export type NotificationContext = { t?: (source: string) => string; dateTime?: (value?: string) => string };

export type DispatcherAlert = BellItem & {
  tone: "red" | "amber" | "cool" | "primary";
  tag: string;
  text: string;
  action: string;
  open?: "exception" | "dock" | "loop";
  trip?: LoadingTripSummary;
  dockAlert?: DockAlert;
};
export type DispatcherSource = {
  plan?: PlanDetail | null;
  incidents?: Incident[];
  temperature?: AuditEvent[];
  trips?: LoadingTripSummary[];
  dockAlerts?: DockAlert[];
  conflicts?: { id: string; settledAt?: string }[];
  receipts?: ReceiptIssue[];
};
export function deriveDispatcherAlerts(source?: DispatcherSource, ctx?: NotificationContext): DispatcherAlert[];

export const ETA_THRESHOLD_MINUTES: number;
export function deferralKey(row: Tracking): string;
export type EtaChange = { row: Tracking; from: string; to: string; minutes: number };
export function deriveStoreNotices(
  rows?: Tracking[],
  state?: { acknowledged?: Set<string> | string[]; seenEta?: Record<string, string> },
  ctx?: NotificationContext,
): { receipts: Tracking[]; deferred: Tracking[]; etaChanges: EtaChange[]; items: BellItem[]; count: number };
export function nextSeenEta(rows: Tracking[], seenEta?: Record<string, string>): Record<string, string> | null;
export function prunedAcknowledged(rows: Tracking[], acknowledged: Set<string>): Set<string> | null;

export const STORE_MESSAGE_WINDOW_HOURS: number;
export function deriveStoreMessages(
  messages?: { id: number | string; eventType: string; body: string; status?: string; createdAt: string }[],
  now?: number,
  ctx?: NotificationContext,
): BellItem[];

export function deriveLoaderItems(
  input?: { trips?: LoadingTripSummary[]; details?: Record<string, LoadingTripDetail | undefined> },
  ctx?: NotificationContext,
): BellItem[];
export function loaderTripsNeedingDetail(trips?: LoadingTripSummary[]): LoadingTripSummary[];

export function deriveDriverItems(
  input?: {
    trips?: DeliveryTripSummary[];
    details?: Record<string, DeliveryTripDetail | undefined>;
    messages?: Record<string, { id: string; body: string; createdAt: string; acknowledgedAt?: string }[] | undefined>;
    userId?: string;
  },
  ctx?: NotificationContext,
): BellItem[];
export function driverTripsNeedingDetail(trips?: DeliveryTripSummary[]): DeliveryTripSummary[];
