export const LATITUDE_RANGE: [number, number];
export const LONGITUDE_RANGE: [number, number];
export type CoordinateParse = { ok: true; latitude: number; longitude: number } | { ok: false; message: string };
export function parseCoordinates(text: string): CoordinateParse;
export function formatCoordinates(latitude: number, longitude: number): string;
export type LocationChange = { change: "keep" } | { change: "clear" } | { change: "set"; latitude: number; longitude: number } | { change: "invalid"; message: string };
export function locationChange(text: string, current?: { latitude?: number | null; longitude?: number | null; locationApproximate?: boolean } | null): LocationChange;
export function openStreetMapLink(latitude: number, longitude: number): string;
export function openStreetMapSearchLink(name: string, district: string): string;
