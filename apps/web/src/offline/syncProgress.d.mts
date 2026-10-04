export type SyncProgressSnapshot = { sendingIds: string[]; failed: Record<string, string> };
export type SyncProgress = {
  snapshot(): SyncProgressSnapshot;
  subscribe(listener: (snapshot: SyncProgressSnapshot) => void): () => void;
  markSending(operationId: string): void;
  markSaved(operationId: string): void;
  markFailed(operationId: string, reason?: string): void;
  clearFailed(operationId?: string): void;
};
export function createSyncProgress(): SyncProgress;
