export function isFresh(timestamp: string, now: number, minutes?: number): boolean;
export function drivenKm(distanceM: number): number;
export function estimatedLitres(distanceM: number, kmPerL: number | undefined): number | null;
export function formatKm(km: number): string;
export function formatLitres(litres: number): string;
export function livePoint(location: { latitude: number; longitude: number; timestamp: string } | null | undefined, now: number): [number, number] | null;
