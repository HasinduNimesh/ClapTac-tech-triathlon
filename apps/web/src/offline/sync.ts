import { ApiError, apiJSON } from "../api/client";
import { DeliveryTripDetail } from "../api/delivery";
import { isPaused, listQueue, putCachedDetail, putSyncConflictNotice, setPaused, shiftQueue } from "./db";
import { drainFIFOQueue } from "./fifoQueue.mjs";
import { singleFlight } from "./singleFlight.mjs";
import { orderForSync, readSyncConflict, syncOperationBody } from "./syncQueue.mjs";
import { createSyncProgress } from "./syncProgress.mjs";

const base = import.meta.env.VITE_API_BASE_URL || "/api/v1";

export type SyncBanner = { kind: "ok" | "offline" | "paused" | "error" | "syncing"; text: string; queueItemId?: number };

type SyncResult = { operationId: string; status: string; originalStatus?: string; detail?: string; conflict?: { recordedPlanVersion?: number; currentPlanVersion?: number; detail?: string } };

/** What the sync loop is sending or could not send right now (drives the per-stop status and Retry). */
export const syncProgress = createSyncProgress();

function authHeaders(token: string, extra?: HeadersInit): HeadersInit {
  return { Authorization: `Bearer ${token}`, ...(extra || {}) };
}

// The Web Lock coordinates tabs sharing the same driver's IndexedDB queue;
// never put the bearer token in the lock name.
const drainQueueOnce = singleFlight(drainQueueSerial, (_token: string, ownerId: string) => ownerId);

export async function drainQueue(token: string, ownerId: string): Promise<SyncBanner> {
  // Recheck after a coalesced drain: an action can enqueue more work while an
  // online event or Sync Now is already draining the earlier queue contents.
  const result = await drainQueueOnce(token, ownerId);
  if (result.kind === "ok" && (await listQueue(ownerId)).length > 0) return drainQueue(token, ownerId);
  return result;
}

async function drainQueueSerial(token: string, ownerId: string): Promise<SyncBanner> {
  if (!token) {
    return { kind: "paused", text: "Sign in to sync" };
  }
  if (await isPaused(ownerId)) {
    return { kind: "paused", text: "Sync paused (401). IndexedDB queue kept." };
  }
  const initial = await listQueue(ownerId);
  if (initial.length === 0) {
    return navigator.onLine ? { kind: "ok", text: "No pending changes" } : { kind: "offline", text: "Offline · queue empty" };
  }
  // Records go first, then photos; items that cite a photo wait behind it, and
  // route completion goes last (see orderForSync). The head is always the next
  // item to send, and nothing is dropped until the server accepts it.
  const operationByQueueId = new Map<number, string>();
  return drainFIFOQueue({
    list: async () => {
      const ordered = orderForSync(await listQueue(ownerId));
      for (const item of ordered) if (item.id !== undefined) operationByQueueId.set(item.id, item.operationId);
      return ordered;
    },
    remove: async (id: number) => {
      await shiftQueue(ownerId, id);
      const operationId = operationByQueueId.get(id);
      if (operationId) syncProgress.markSaved(operationId);
    },
    apply: async (item: Awaited<ReturnType<typeof listQueue>>[number]) => {
      syncProgress.markSending(item.operationId);
      if (item.type === "PROOF_UPLOAD") {
        const form = new FormData();
        form.append("type", item.proofType || "PHOTO");
        form.append("file", item.blob || new Blob(), item.proofType === "SIGNATURE" ? "signature.png" : "photo.jpg");
        if (item.mimeType) form.append("mimeType", item.mimeType);
        if (item.receiverName) form.append("receiverName", item.receiverName);
        const res = await fetch(`${base}/delivery/trips/${item.tripId}/stops/${item.stopId}/proofs`, {
          method: "POST",
          headers: authHeaders(token, { "Idempotency-Key": item.operationId }),
          body: form,
        });
        if (res.status === 401) {
          await setPaused(ownerId, true);
          throw new ApiError(401, "Sync paused (401). Queue kept.");
        }
        if (!res.ok) {
          throw new ApiError(res.status, await res.text());
        }
      } else if (item.type === "START") {
        await apiJSON(`/delivery/trips/${item.tripId}/start`, token, {
          method: "POST",
          headers: { "Idempotency-Key": item.operationId },
        });
      } else if (item.type === "CUSTODY_RECORD") {
        if (!item.orderId) throw new ApiError(400,"Tech custody event is missing its order reference.");
        await apiJSON(`/orders/${encodeURIComponent(item.orderId)}/custody`,token,{method:"POST",headers:{"Idempotency-Key":item.operationId},body:JSON.stringify({...item.payload,idempotencyKey:item.operationId})});
      } else {
        const body = { operations: [syncOperationBody(item)] };
        const res = await fetch(`${base}/delivery/sync`, {
          method: "POST",
          headers: { ...authHeaders(token), Accept: "application/json", "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        if (res.status === 401) {
          await setPaused(ownerId, true);
          throw new ApiError(401, "Sync paused (401). Queue kept.");
        }
        if (!res.ok) {
          throw new ApiError(res.status, await res.text());
        }
        const response = (await res.json()) as { results?: SyncResult[] };
        const result = response.results?.find((r) => r.operationId === item.operationId);
        const applied = result?.status === "APPLIED" ||
          (result?.status === "DUPLICATE" && result.originalStatus === "APPLIED");
        if (!applied) {
          const reason = result?.detail || `operation status: ${result?.status || "missing result"}`;
          syncProgress.markFailed(item.operationId, reason);
          return { applied: false, detail: reason };
        }
        // The record was kept. When it was made on an older plan, remember the
        // notice so the driver can still read it after the queue entry is gone.
        const conflict = readSyncConflict(result);
        if (conflict) {
          try {
            await putSyncConflictNotice(ownerId, { ...conflict, operationId: item.operationId, tripId: item.tripId, stopId: item.stopId, type: item.type, receivedAt: new Date().toISOString() });
          } catch {
            // The record itself is already accepted; a notice that cannot be saved must not block sync.
          }
        }
      }
      return { applied: true };
    },
    onFailure: async (e: unknown, item: { operationId: string }, count: number): Promise<SyncBanner> => {
      // The record stays queued in every case below. A 401 or a lost connection
      // returns it to "Saved on this phone"; any other failure shows Retry.
      const reason = e instanceof Error ? e.message : String(e);
      if (e instanceof ApiError && e.status === 401) {
        syncProgress.markSaved(item.operationId);
        await setPaused(ownerId, true);
        return { kind: "paused", text: "Sync paused (401). Queue kept." };
      }
      if (!navigator.onLine) {
        syncProgress.markSaved(item.operationId);
        return { kind: "offline", text: `Offline · ${count} queued` };
      }
      syncProgress.markFailed(item.operationId, reason);
      if (e instanceof TypeError) {
        return { kind: "offline", text: `Offline · ${count} queued` };
      }
      if (e instanceof ApiError && e.status >= 500) {
        return { kind: "offline", text: `Delivery service unavailable · ${count} unsynced item(s) remain saved on this device` };
      }
      return { kind: "error", text: reason };
    },
  });
}

export async function cacheDetail(detail: DeliveryTripDetail, ownerId: string, serverConfirmed = false) {
  await putCachedDetail(ownerId, detail, serverConfirmed);
}

/** Retry one record that could not be sent; the queue still sends records before photos. */
export async function retryItem(token: string, ownerId: string, operationId: string) {
  syncProgress.clearFailed(operationId);
  return drainQueue(token, ownerId);
}

export async function resumeSync(token: string, ownerId: string) {
  await setPaused(ownerId, false);
  return drainQueue(token, ownerId);
}
