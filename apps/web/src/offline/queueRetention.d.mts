export type QueueRetentionWarning = {
  level: "warning" | "urgent" | "critical";
  ageDays: number;
  text: string;
};

export function queueRetentionWarning(
  items: Array<{ createdAt?: string }>,
  now?: number,
): QueueRetentionWarning | null;

export function canPurgeCompletedCache(
  input: { serverConfirmedAt?: string; hasPendingQueue: boolean },
  now?: number,
): boolean;
