export type VehicleSort = "number-asc" | "number-desc" | "weight-desc" | "volume-desc";
export function vehicleNumber(id: string | null | undefined): string;
export function browseVehicles<T extends { id: string; weightCapacityKg: number; volumeCapacityM3: number }>(vehicles: T[], query?: string, sort?: VehicleSort): T[];
