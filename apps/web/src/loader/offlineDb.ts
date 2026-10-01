import Dexie, { Table } from "dexie";
import { LoadingTripDetail, LoadingTripSummary } from "../api/loading";
import { bindSingleOwner } from "../offline/singleOwner.mjs";
import { LoadingQueueOperation } from "./offlineState.mjs";

type TripDay = { date: string; trips: LoadingTripSummary[] };
type Owner = { key: string; value: string };

class LoaderDB extends Dexie {
  days!: Table<TripDay, string>;
  details!: Table<LoadingTripDetail, string>;
  queue!: Table<LoadingQueueOperation, number>;
  meta!: Table<Owner, string>;

  constructor() {
    super("waypoint-loader");
    this.version(1).stores({ days: "date", details: "tripId", queue: "++id, operationId, type, tripId", meta: "key" });
  }
}

export const loaderDb = new LoaderDB();

export async function bindLoaderOwner(ownerId: string): Promise<boolean> {
  if (!ownerId.trim()) return false;
  return loaderDb.transaction("rw", loaderDb.meta, async () => bindSingleOwner({
    get: async () => (await loaderDb.meta.get("ownerId"))?.value || "",
    set: async (value: string) => { await loaderDb.meta.put({ key: "ownerId", value }); },
  }, ownerId));
}

async function requireOwner(ownerId: string) {
  if (!await bindLoaderOwner(ownerId)) throw new Error("This device's saved loader data belongs to another account. Sign in with the original loader account to recover it.");
}

export async function cacheLoaderTrips(ownerId: string, date: string, trips: LoadingTripSummary[]) {
  await requireOwner(ownerId);
  await loaderDb.days.put({ date, trips });
}

export async function getCachedLoaderTrips(ownerId: string, date: string) {
  await requireOwner(ownerId);
  return (await loaderDb.days.get(date))?.trips || [];
}

export async function cacheLoaderDetail(ownerId: string, detail: LoadingTripDetail) {
  await requireOwner(ownerId);
  await loaderDb.details.put(detail);
}

export async function getCachedLoaderDetail(ownerId: string, tripId: string) {
  await requireOwner(ownerId);
  return loaderDb.details.get(tripId);
}

export async function enqueueLoader(ownerId: string, operation: LoadingQueueOperation) {
  await requireOwner(ownerId);
  await loaderDb.queue.add(operation);
}

export async function listLoaderQueue(ownerId: string) {
  await requireOwner(ownerId);
  return loaderDb.queue.orderBy("id").toArray();
}

export async function removeLoaderQueueItem(ownerId: string, id: number) {
  await requireOwner(ownerId);
  await loaderDb.queue.delete(id);
}
