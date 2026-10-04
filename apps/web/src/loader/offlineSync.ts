import { ApiError, apiJSON } from "../api/client";
import { drainFIFOQueue } from "../offline/fifoQueue.mjs";
import { listLoaderQueue, removeLoaderQueueItem } from "./offlineDb";
import { LoadingQueueOperation } from "./offlineState.mjs";

export type LoaderSyncState = { kind: "ok" | "offline" | "error" | "syncing" | "paused"; text: string };
const active = new Map<string, Promise<LoaderSyncState>>();

async function drainSerial(token: string, ownerId: string): Promise<LoaderSyncState> {
  const pending = await listLoaderQueue(ownerId);
  if (!pending.length) return navigator.onLine ? { kind: "ok", text: "No pending loading changes" } : { kind: "offline", text: "Offline · no saved loading changes" };
  if (!navigator.onLine) return { kind: "offline", text: `Offline · ${pending.length} loading change(s) saved on this device` };
  const result = await drainFIFOQueue({
    list: () => listLoaderQueue(ownerId),
    remove: (id: number) => removeLoaderQueueItem(ownerId, id),
    apply: async (item: LoadingQueueOperation) => {
      const headers = { "Idempotency-Key": item.operationId };
      try {
        if (item.type === "START") {
          await apiJSON(`/loading/trips/${encodeURIComponent(item.tripId)}/start`, token, { method: "POST", headers: { ...headers, ...(item.planVersion ? { "If-Match": String(item.planVersion) } : {}) } });
        } else if (item.type === "ORDER_LOADED") {
          await apiJSON(`/loading/trips/${encodeURIComponent(item.tripId)}/orders/${encodeURIComponent(item.orderId || "")}/loaded`, token, { method: "PUT", headers });
        } else if (item.type === "ISSUE_CREATE") {
          await apiJSON(`/loading/trips/${encodeURIComponent(item.tripId)}/orders/${encodeURIComponent(item.orderId || "")}/issues`, token, {
            method: "POST", headers,
            body: JSON.stringify(item.payload || {}),
          });
        } else if (item.type === "CUSTODY_RECORD") {
          await apiJSON(`/orders/${encodeURIComponent(item.orderId || "")}/custody`, token, { method:"POST", headers, body:JSON.stringify({...item.payload,idempotencyKey:item.operationId}) });
        } else if (item.type === "READY") {
          await apiJSON(`/loading/trips/${encodeURIComponent(item.tripId)}/ready`, token, { method: "POST", headers, body: JSON.stringify({ chilledTemperatureC: item.payload?.chilledTemperatureC, sealNumber: item.payload?.sealNumber }) });
        }
        return { applied: true };
      } catch (error) {
        if (error instanceof ApiError && error.status === 401) return { applied: false, detail: "Sign-in expired. Sign in again; saved loading work is preserved." };
        if (error instanceof ApiError) return { applied: false, detail: `${error.status}: ${error.message}` };
        throw error;
      }
    },
    onFailure: (error: unknown, _item: unknown, count: number): LoaderSyncState => {
      if (error instanceof TypeError || !navigator.onLine) return { kind: "offline", text: `Connection lost · ${count} loading change(s) remain saved on this device` };
      return { kind: "error", text: `${error instanceof Error ? error.message : String(error)} · ${count} loading change(s) remain saved on this device` };
    },
  });
  if (result.kind === "ok" && (await listLoaderQueue(ownerId)).length > 0) return drainSerial(token, ownerId);
  return result;
}

export async function drainLoaderQueue(token: string, ownerId: string): Promise<LoaderSyncState> {
  const existing = active.get(ownerId);
  if (existing) return existing;
  let task: Promise<LoaderSyncState>;
  task = Promise.resolve().then(async () => {
    const locks = navigator.locks;
    if (!locks?.request) return drainSerial(token, ownerId);
    return locks.request(`waypoint-loader-sync:${ownerId}`, { mode: "exclusive" }, () => drainSerial(token, ownerId));
  }).finally(() => { if (active.get(ownerId) === task) active.delete(ownerId); });
  active.set(ownerId, task);
  return task;
}
