export type FIFOQueueResult = {
  kind: "ok" | "offline" | "paused" | "error" | "syncing";
  text: string;
  queueItemId?: number;
};

export type FIFOQueueItem = {
  id?: number;
  operationId: string;
  type: string;
};

export function drainFIFOQueue<T extends FIFOQueueItem>(dependencies: {
  list: () => Promise<T[]>;
  apply: (item: T) => Promise<{ applied: boolean; detail?: string }>;
  remove: (id: number) => Promise<unknown>;
  onFailure: (error: unknown, item: T, remaining: number) => FIFOQueueResult | Promise<FIFOQueueResult>;
}): Promise<FIFOQueueResult>;
