type ShortfallTrip = { openShortfallCount?: number; shortfallCount?: number; openChilledShortfall?: boolean; deliveryDate?: string };
export function openShortfallCount(trip?: ShortfallTrip | null): number;
export function hasOpenShortfall(trip?: ShortfallTrip | null): boolean;
export function tripsWithOpenShortfall<T extends ShortfallTrip>(trips?: T[]): T[];
export function isChilledShortfall(trip?: ShortfallTrip | null): boolean;
export function shortfallSeverity(trip?: ShortfallTrip | null): "critical" | "high";
export function shortfallLink(trip?: ShortfallTrip | null, fallbackDate?: string): string;
