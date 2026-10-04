export const SAMPLE_TRIP_PREFIX: string;
export const SAMPLE_STOPS_PER_TRIP: number;
export function sampleReferenceTime(date: string, now?: number): number;
export function isSampleTrip(tripId: string | undefined | null): boolean;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function buildSampleTrips(input: { outlets?: { id: string; name?: string; brand?: string; district?: string; depot: string; windowOpenTime?: string; windowCloseTime?: string; latitude?: number; longitude?: number; locationApproximate?: boolean }[]; vehicles?: { id: string; homeDepot: string }[]; date: string; now?: number }): any[];
