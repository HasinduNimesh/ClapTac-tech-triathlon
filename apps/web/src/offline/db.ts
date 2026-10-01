import Dexie, { Table } from "dexie";
import { DeliveryTripDetail, DeliveryTripSummary } from "../api/delivery";
import { canPurgeCompletedCache } from "./queueRetention.mjs";
import { clearableCompletedTripIds } from "./driverDataPrivacy.mjs";
import { bindSingleOwner } from "./singleOwner.mjs";

export type QueueItem = {
  id?: number;
  operationId: string;
  type: "START" | "ARRIVED" | "PROOF_UPLOAD" | "STOP_OUTCOME" | "ROUTE_COMPLETED" | "TEMPERATURE_READING" | "CUSTODY_RECORD";
  tripId: string;
  stopId?: string;
  orderId?: string;
  dependsOnOperationId?: string;
  payload?: Record<string, unknown>;
  blob?: Blob;
  mimeType?: string;
  proofType?: string;
  receiverName?: string;
  createdAt: string;
};

export type MetaRow = { key: string; value: string };

class DriverDB extends Dexie {
  trips!: Table<DeliveryTripSummary, string>;
  details!: Table<DeliveryTripDetail, string>;
  queue!: Table<QueueItem, number>;
  meta!: Table<MetaRow, string>;

  constructor() {
    super("waypoint-driver");
    this.version(1).stores({
      trips: "tripId",
      details: "tripId",
      queue: "++id, operationId, type, tripId",
      meta: "key",
    });
  }
}

export const driverDb = new DriverDB();

export async function bindDriverOwner(ownerId: string): Promise<boolean> {
  if (!ownerId.trim()) return false;
  return driverDb.transaction("rw", driverDb.meta, async () => bindSingleOwner({
    get: async () => (await driverDb.meta.get("ownerId"))?.value || "",
    set: async (value: string) => { await driverDb.meta.put({ key: "ownerId", value }); },
  }, ownerId));
}

async function requireOwner(ownerId: string) {
  if (!await bindDriverOwner(ownerId)) {
    throw new Error("This device's saved driver data belongs to another account. Sign in with the original driver account to recover it.");
  }
}

export async function setPaused(ownerId: string, paused: boolean) {
  await requireOwner(ownerId);
  await driverDb.meta.put({ key: "paused", value: paused ? "1" : "0" });
}

export async function isPaused(ownerId: string) {
  await requireOwner(ownerId);
  const row = await driverDb.meta.get("paused");
  return row?.value === "1";
}

export async function enqueue(ownerId: string, item: QueueItem) {
  await requireOwner(ownerId);
  await driverDb.queue.add(item);
}

export async function peekQueue(ownerId: string) {
  await requireOwner(ownerId);
  return driverDb.queue.orderBy("id").first();
}

export async function shiftQueue(ownerId: string, id: number) {
  await requireOwner(ownerId);
  await driverDb.queue.delete(id);
}

export async function listQueue(ownerId: string) {
  await requireOwner(ownerId);
  return driverDb.queue.orderBy("id").toArray();
}

export async function getCachedTrips(ownerId: string) {
  await requireOwner(ownerId);
  await purgeExpiredCompletedCache(ownerId);
  return driverDb.trips.toArray();
}

export async function putCachedTrips(ownerId: string, trips: DeliveryTripSummary[]) {
  await requireOwner(ownerId);
  await driverDb.trips.bulkPut(trips);
}

export async function getCachedDetail(ownerId: string, tripId: string) {
  await requireOwner(ownerId);
  await purgeExpiredCompletedCache(ownerId);
  return driverDb.details.get(tripId);
}

export async function putCachedDetail(ownerId: string, detail: DeliveryTripDetail, serverConfirmed = false) {
  await requireOwner(ownerId);
  await driverDb.transaction("rw", driverDb.details, driverDb.meta, async () => {
    await driverDb.details.put(detail);
    if (serverConfirmed && detail.run.status === "completed" && detail.run.completedAt) {
      await driverDb.meta.put({ key: `serverCompletedAt:${encodeURIComponent(detail.tripId)}`, value: detail.run.completedAt });
    }
  });
}

export async function purgeExpiredCompletedCache(ownerId: string, now = Date.now()) {
  await requireOwner(ownerId);
  return driverDb.transaction("rw", driverDb.trips, driverDb.details, driverDb.queue, driverDb.meta, async () => {
    const markers = await driverDb.meta.where("key").startsWith("serverCompletedAt:").toArray();
    const purged: string[] = [];
    for (const marker of markers) {
      const tripId = decodeURIComponent(marker.key.slice("serverCompletedAt:".length));
      const detail = await driverDb.details.get(tripId);
      const queued = await driverDb.queue.where("tripId").equals(tripId).count();
      if (!detail || detail.run.status !== "completed" || detail.run.completedAt !== marker.value ||
          !canPurgeCompletedCache({ serverConfirmedAt: marker.value, hasPendingQueue: queued > 0 }, now)) continue;
      await driverDb.details.delete(tripId);
      await driverDb.trips.delete(tripId);
      await driverDb.meta.delete(marker.key);
      purged.push(tripId);
    }
    return purged;
  });
}

export async function readDriverDataForExport(ownerId: string) {
  await requireOwner(ownerId);
  return driverDb.transaction("r", driverDb.trips, driverDb.details, driverDb.queue, async () => ({
    trips: await driverDb.trips.toArray(),
    details: await driverDb.details.toArray(),
    queue: await driverDb.queue.orderBy("id").toArray(),
  }));
}

/** Clear server-confirmed completed cache only; never clear pending work or active routes. */
export async function clearCompletedDriverCache(ownerId: string) {
  await requireOwner(ownerId);
  return driverDb.transaction("rw", driverDb.trips, driverDb.details, driverDb.queue, driverDb.meta, async () => {
    const pendingQueueCount = await driverDb.queue.count();
    const markers = await driverDb.meta.where("key").startsWith("serverCompletedAt:").toArray();
    const details = await driverDb.details.toArray();
    const completedTripIds = clearableCompletedTripIds({ pendingQueueCount, markers, details });
    if (completedTripIds.length === 0) return { clearedTripIds: [], pendingQueueCount };
    for (const tripId of completedTripIds) {
      await driverDb.details.delete(tripId);
      await driverDb.trips.delete(tripId);
      await driverDb.meta.delete(`serverCompletedAt:${encodeURIComponent(tripId)}`);
    }
    return { clearedTripIds: completedTripIds, pendingQueueCount };
  });
}
