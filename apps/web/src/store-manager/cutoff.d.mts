export function minutesOfDay(localTime: string | undefined | null): number | undefined;
export function formatCutoff(localTime: string | undefined | null, locale?: string): string | undefined;
export function cutoffHasPassed(localTime: string | undefined | null, now?: Date): boolean | undefined;
export function serverTimeAt(serverTime: string | undefined, receivedAt: number, monotonicNow: number): Date | undefined;
export function withTime(sentence: string, time: string): string;
