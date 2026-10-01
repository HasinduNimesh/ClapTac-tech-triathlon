import { ApiError, apiJSON } from "../api/client";
import { DeliveryTripDetail } from "../api/delivery";
import { isPaused, listQueue, putCachedDetail, setPaused, shiftQueue } from "./db";
import { drainFIFOQueue } from "./fifoQueue.mjs";
import { singleFlight } from "./singleFlight.mjs";

const base = import.meta.env.VITE_API_BASE_URL || "/api/v1";

export type SyncBanner = { kind: "ok" | "offline" | "paused" | "error" | "syncing"; text: string; queueItemId?: number };

type SyncResult = { operationId: string; status: string; originalStatus?: string; detail?: string };

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
  // The queue is FIFO so dependent proof, outcome, and completion operations stay ordered.
  return drainFIFOQueue({
    list: () => listQueue(ownerId),
    remove: (id: number) => shiftQueue(ownerId, id),
    apply: async (item: Awaited<ReturnType<typeof listQueue>>[number]) => {
      if (item.type === "PROOF_UPLOAD") {
        const form = new FormData();
        form.append("type", item.proofType || "PHOTO");
        form.append("file", item.blob || new Blob(), item.proofType === "SIGNATURE" ? "signature.png" : "photo.jpg");
        if (item.mimeType) form.append("mimeType", item.mimeType);
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
        const body = {
          operations: [
            {
              operationId: item.operationId,
              type: item.type,
              tripId: item.tripId,
              stopId: item.stopId,
              occurredAt: item.payload?.occurredAt,
              dependsOnOperationId: item.dependsOnOperationId,
              payload: item.payload,
            },
          ],
        };
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
          return { applied: false, detail: reason };
        }
      }
      return { applied: true };
    },
    onFailure: async (e: unknown, _item: unknown, count: number): Promise<SyncBanner> => {
      if (e instanceof ApiError && e.status === 401) {
        await setPaused(ownerId, true);
        return { kind: "paused", text: "Sync paused (401). Queue kept." };
      }
      if (e instanceof TypeError || !navigator.onLine) {
        return { kind: "offline", text: `Offline · ${count} queued` };
      }
      if (e instanceof ApiError && e.status >= 500) {
        return { kind: "offline", text: `Delivery service unavailable · ${count} unsynced item(s) remain saved on this device` };
      }
      return { kind: "error", text: e instanceof Error ? e.message : String(e) };
    },
  });
}

export async function cacheDetail(detail: DeliveryTripDetail, ownerId: string, serverConfirmed = false) {
  await putCachedDetail(ownerId, detail, serverConfirmed);
}

export async function resumeSync(token: string, ownerId: string) {
  await setPaused(ownerId, false);
  return drainQueue(token, ownerId);
}
