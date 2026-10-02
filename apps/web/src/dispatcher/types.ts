export type Outlet = {
  id: string; brand: string; name: string; district: string; depot: string; dockType: string; parkingConstraint: string;
  mallWindow: boolean; windowOpenTime: string; windowCloseTime: string; accessInstructions?: string;
};
export type Vehicle = { id: string; type: string; temp: string; weightCapacityKg: number; volumeCapacityM3: number; fuelType: string; kmPerL: number; weeklyFuelQuotaL: number; homeDepot: string; version: number };
export type Availability = { date: string; vehicleId: string; status: string; reason?: string };
export type Incident = { id: string; vehicleId: string; date: string; tripId?: string; type: string; description: string; affectedStops: string[]; status: string; reportedBy?: string; reportedAt?: string };
export type ReceiptIssue = { orderRef: string; outletId: string; receipt: { receivedUnits: number; expectedUnits: number; status: string; confirmedAt: string }; issue: { issueType: string; affectedUnits: number; note: string; createdAt: string } };
export type AuditEvent = { event_id: string; actor_id: string; action: string; resource_type: string; resource_id: string; timestamp: string; source: string; reason?: string; new_state?: Record<string, unknown> };

export function outletMap(items?: Outlet[]) { return new Map((items || []).map((o) => [o.id, o])); }
export function isVanOnly(outlet?: Outlet) { return Boolean(outlet && /van/i.test(outlet.parkingConstraint || "")); }
export function vehicleKind(vehicle?: { type: string; temp: string }) {
  if (!vehicle) return "";
  return `${vehicle.type}${vehicle.temp ? ` · ${vehicle.temp}` : ""}`;
}
export function isRefrigerated(vehicle?: { temp: string }) { return Boolean(vehicle && /chill|refriger|frozen|multi/i.test(vehicle.temp || "")); }
export function availabilityOf(vehicleId: string, items?: Availability[]) { return (items || []).find((a) => a.vehicleId === vehicleId)?.status || "available"; }
