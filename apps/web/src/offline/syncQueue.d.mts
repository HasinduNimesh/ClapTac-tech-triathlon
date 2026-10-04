export type SyncQueueItem = {
  id?: number;
  operationId: string;
  type: string;
  tripId?: string;
  stopId?: string;
  dependsOnOperationId?: string;
  payload?: Record<string, unknown>;
  planVersion?: number;
};
export type SyncConflict = { recordedPlanVersion: number; currentPlanVersion: number; detail: string };
export type SyncConflictNotice = SyncConflict & { operationId: string; tripId: string; stopId?: string; type: string; receivedAt: string };
export type StopSyncState = "none" | "saved" | "sending" | "failed" | "sent";

type PlanDetail = { currentPlanVersion?: number; run?: { planVersion?: number } } | null | undefined;

export function planVersionShown(detail: PlanDetail): number | undefined;
export function stampPlanVersion<T extends { planVersion?: number }>(item: T, detail: PlanDetail): T;
export function orderForSync<T extends SyncQueueItem>(items: T[]): T[];
export function syncOperationBody(item: SyncQueueItem): Record<string, unknown>;
export function classifySyncResult(result: unknown): { applied: boolean; retry: boolean; detail: string };
export function readSyncConflict(result: unknown): SyncConflict | null;
export function conflictMessage(conflict: { recordedPlanVersion: number; currentPlanVersion: number }, t?: (text: string) => string): string;
export function conflictsForStop<T extends { tripId: string; stopId?: string }>(notices: T[], tripId: string, stopId?: string): T[];
export function deriveStopSyncStatus(input: {
  stopId: string;
  stopStatus?: string;
  queue: SyncQueueItem[];
  sendingIds?: string[];
  failed?: Record<string, string>;
}): { state: StopSyncState; waiting: number; failedItems: { operationId: string; type: string; reason: string }[] };
export function waitingCounts(queue: SyncQueueItem[]): { total: number; photos: number; records: number };
export const SYNC_STATE_LABELS: Record<string, string>;
export function syncStateLabel(state: string): string;
