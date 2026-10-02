export type TimelineStep = {
  key: string;
  label: string;
  detail: string;
  warn?: boolean;
  state: "done" | "current" | "upcoming";
};
type TrackingLike = {
  stage: string;
  planning?: { plannedArrivalAt?: string };
  order?: { requestedDeliveryDate?: string };
};
export function colomboDate(value: string | number | Date): string;
export function colomboTime(value: string | number | Date): string;
export function formatDay(dateOnly: string | undefined): string;
export function isDeferred(stage: string): boolean;
export function isActiveRun(stage: string): boolean;
export function needsReceipt(stage: string): boolean;
export function isReceiptConfirmed(stage: string): boolean;
export function isNotDelivered(stage: string): boolean;
export function statusLabel(stage: string): string;
export function statusTone(stage: string): "on-route" | "deferred" | "delivered" | "default";
export function arrivesOn(tracking: TrackingLike | undefined, date: string): boolean;
export function sortByArrival(a: TrackingLike, b: TrackingLike): number;
export function timelineSteps(tracking: TrackingLike | undefined): TimelineStep[];
