import type { LoadingTripDetail } from "../api/loading";

export type LoadingQueueOperation = {
  id?: number;
  operationId: string;
  type: "START" | "ORDER_LOADED" | "ISSUE_CREATE" | "READY" | "CUSTODY_RECORD";
  tripId: string;
  planVersion?: number;
  orderId?: string;
  payload?: { type?: string; affectedUnits?: number; note?: string; stage?: string; sealId?: string; serialNumbers?: string[]; condition?: string; evidenceRef?: string; receiverName?: string; chilledTemperatureC?: number; sealNumber?: string };
  createdAt: string;
};

export function shouldQueueLoadingFailure(error: unknown, online?: boolean): boolean;
export function applyLoadingQueueItem<T extends LoadingTripDetail>(detail: T, item: LoadingQueueOperation): T;
export function replayLoadingQueue<T extends LoadingTripDetail>(detail: T, items: LoadingQueueOperation[]): T;
