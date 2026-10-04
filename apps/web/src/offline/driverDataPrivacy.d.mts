import type { DeliveryTripDetail, DeliveryTripSummary } from "../api/delivery";
import type { QueueItem } from "./db";

export function createDriverDataExport(data: {
  subject: string;
  trips: DeliveryTripSummary[];
  details: DeliveryTripDetail[];
  queue: QueueItem[];
}, exportedAt?: Date): Promise<string>;

export function clearableCompletedTripIds(input: {
  pendingQueueCount: number;
  markers: { key: string; value: string }[];
  details: DeliveryTripDetail[];
}): string[];

export function otherDayTripIds(input: {
  trips?: DeliveryTripSummary[];
  details?: DeliveryTripDetail[];
  queuedTripIds?: string[];
  today: string;
}): string[];
