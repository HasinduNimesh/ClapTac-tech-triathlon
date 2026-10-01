export type QueueHealthBuckets = {
  ageBucket: "none" | "unknown" | "lt24h" | "1d_7d" | "7d_30d" | "30d_plus";
  countBucket: "none" | "one" | "2_5" | "6_plus";
};

export function offlineQueueHealth(
  items: Array<{ createdAt?: string }> | null | undefined,
  now?: number,
): QueueHealthBuckets;
