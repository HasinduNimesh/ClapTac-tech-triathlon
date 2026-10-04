// In-memory record of what the sync loop is doing right now, so the driver
// screen can show "Sending" and "Could not send / Retry" per record. Nothing
// here is persisted: the queue itself in IndexedDB is the source of truth.

export function createSyncProgress() {
  const sending = new Set();
  const failed = new Map();
  const listeners = new Set();
  let current = { sendingIds: [], failed: {} };

  function publish() {
    current = { sendingIds: [...sending], failed: Object.fromEntries(failed) };
    listeners.forEach((listener) => listener(current));
  }

  return {
    snapshot: () => current,
    subscribe(listener) {
      listeners.add(listener);
      return () => { listeners.delete(listener); };
    },
    markSending(operationId) {
      sending.add(operationId);
      failed.delete(operationId);
      publish();
    },
    markSaved(operationId) {
      sending.delete(operationId);
      failed.delete(operationId);
      publish();
    },
    markFailed(operationId, reason) {
      sending.delete(operationId);
      failed.set(operationId, reason || "");
      publish();
    },
    clearFailed(operationId) {
      if (operationId === undefined) failed.clear();
      else failed.delete(operationId);
      publish();
    },
  };
}
