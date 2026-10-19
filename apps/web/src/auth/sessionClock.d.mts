export const WARN_MS: number;
export function sessionPhase(expiresAtMs: number, now: number, rejected?: boolean): "active" | "expiring" | "expired";
export function minutesLeft(expiresAtMs: number, now: number): number;
export function canRenew(scope: string | undefined): boolean;
export function msUntilChange(expiresAtMs: number, now: number): number | null;
