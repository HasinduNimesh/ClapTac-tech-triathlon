export type LatLng = [number, number];
export function depotCode(depot?: string | null): string;
export const DEPOT_LOCATIONS: Record<string, LatLng>;
export function depotPosition(depot?: string | null): LatLng | undefined;
