import { PlanDetail, PlanVehicle } from "../api/planning";
import { isChilled, pct } from "./useApi";

// The planning service sends null (not []) for lists a fresh, ungenerated plan
// does not have yet. Every screen reads plans through this so it never crashes.
export function normalizePlan(detail: PlanDetail): PlanDetail {
  return {
    ...detail,
    trips: detail.trips || [],
    allocations: detail.allocations || [],
    deferrals: detail.deferrals || [],
    unallocated: detail.unallocated || [],
    vehicles: detail.vehicles || [],
    orders: detail.orders || [],
    publication: detail.publication ? { ...detail.publication, acknowledgements: detail.publication.acknowledgements || [], reminders: detail.publication.reminders || [] } : detail.publication,
  };
}

export type TripLoad = {
  tripId: string;
  tripNumber: number;
  status: string;
  vehicle?: PlanVehicle;
  weightKg: number;
  volumeM3: number;
  weightPct: number;
  volumePct: number;
  orderCount: number;
  chilledOrders: number;
  firstArrival?: string;
  lastDeparture?: string;
  coolingMismatch: boolean;
  overCapacity: boolean;
  /** First planned arrival to last planned departure, in minutes. */
  tripMinutes?: number;
  fresh: boolean;
  /** Chilled goods on a Fresh trip longer than the Fresh budget. */
  coolingRisk: boolean;
};

/** Fresh deliveries run 03:30-08:00: each Fresh trip must fit in 270 minutes. */
export const FRESH_TRIP_BUDGET_MINUTES = 270;

const reefer = (v?: PlanVehicle) => Boolean(v && /chill|refriger|frozen|multi|reefer/i.test(v.temp || ""));

// FR-51: sums what is actually loaded on one trip from the plan's own
// allocations and orders, so the load meter always matches what was assigned
// rather than depending on a separate backend aggregate. Scoped by tripId,
// not vehicleId: a vehicle can run two sequential trips, each with its own
// capacity, so summing across both trips before comparing to one trip's
// capacity would overstate the load (two 70%-full trips reading as 140%).
export function tripLoads(raw: PlanDetail): TripLoad[] {
  const detail = normalizePlan(raw);
  const ordersById = new Map(detail.orders.map((o) => [o.id, o]));
  return (detail.trips || []).map((trip) => {
    const vehicle = (detail.vehicles || []).find((v) => v.id === trip.vehicleId);
    let weightKg = 0, volumeM3 = 0, orderCount = 0, chilledOrders = 0, fresh = false;
    const arrivals: string[] = [], departures: string[] = [];
    for (const a of detail.allocations) {
      if (a.tripId !== trip.id) continue;
      const order = ordersById.get(a.orderId);
      if (!order) continue;
      weightKg += order.orderWeightKg;
      volumeM3 += order.orderVolumeM3;
      orderCount += 1;
      if (isChilled(order.temperatureRequirement)) chilledOrders += 1;
      if (/fresh/i.test(order.brand)) fresh = true;
      if (a.plannedArrivalAt) arrivals.push(a.plannedArrivalAt);
      if (a.plannedDepartureAt) departures.push(a.plannedDepartureAt);
    }
    const weightPct = vehicle ? pct(weightKg, vehicle.weightCapacityKg) : 0;
    const volumePct = vehicle ? pct(volumeM3, vehicle.volumeCapacityM3) : 0;
    const firstArrival = arrivals.sort()[0], lastDeparture = departures.sort().slice(-1)[0];
    const tripMinutes = firstArrival && lastDeparture ? Math.round((Date.parse(lastDeparture) - Date.parse(firstArrival)) / 60000) : undefined;
    return {
      tripId: trip.id, tripNumber: trip.tripNumber, status: trip.status, vehicle, weightKg, volumeM3, weightPct, volumePct, orderCount, chilledOrders,
      firstArrival, lastDeparture, tripMinutes, fresh,
      coolingMismatch: chilledOrders > 0 && !reefer(vehicle),
      overCapacity: weightPct > 100 || volumePct > 100,
      coolingRisk: fresh && chilledOrders > 0 && (tripMinutes || 0) > FRESH_TRIP_BUDGET_MINUTES,
    };
  });
}

export type PlanCheck = { key: string; label: string; ok: boolean; detail: string };

export function planChecks(raw: PlanDetail, loads: TripLoad[], t: (s: string) => string): PlanCheck[] {
  const detail = normalizePlan(raw);
  const unallocated = detail.unallocated?.length || 0;
  const total = detail.orders?.length || 0;
  const overloaded = loads.filter((l) => l.overCapacity);
  const cooling = loads.filter((l) => l.coolingMismatch);
  const fuelOver = (detail.vehicles || []).filter((v) => Math.max(v.weekFuelActualL || 0, (v.weekFuelPlannedL || 0) + (v.planFuelL || 0)) > v.weeklyFuelQuotaL);
  const maxTrips = detail.planningPolicy?.maxTripsPerVehicle || 2;
  const tripsByVehicle = new Map<string, number>();
  for (const trip of detail.trips || []) tripsByVehicle.set(trip.vehicleId, (tripsByVehicle.get(trip.vehicleId) || 0) + 1);
  const tripLimit = [...tripsByVehicle.values()].filter((n) => n > maxTrips).length;
  return [
    { key: "assigned", label: t("All orders assigned or deferred"), ok: unallocated === 0, detail: `${total - unallocated} / ${total} ${t("assigned")}` },
    { key: "capacity", label: t("Weight and volume limits"), ok: overloaded.length === 0, detail: overloaded.length ? `${overloaded.length} ${t("trip(s) over capacity")}` : t("All trips within limits") },
    { key: "cooling", label: t("Cooling requirements"), ok: cooling.length === 0, detail: cooling.length ? `${cooling.length} ${t("trip(s) carry chilled goods without refrigeration")}` : t("All temperature-sensitive orders OK") },
    { key: "fuel", label: t("Fuel range"), ok: fuelOver.length === 0 && detail.fuelLedgerAvailable !== false, detail: fuelOver.length ? `${fuelOver.length} ${t("vehicle(s) over weekly quota")}` : detail.fuelLedgerAvailable === false ? t("Fuel ledger unavailable") : t("All trips within vehicle fuel range") },
    { key: "trips", label: t("Two-trip limit"), ok: tripLimit === 0, detail: tripLimit ? `${tripLimit} ${t("vehicle(s) exceed the trip limit")}` : `${t("No vehicles exceed")} ${maxTrips} ${t("trips")}` },
    (() => {
      const long = loads.filter((l) => l.fresh && (l.tripMinutes || 0) > FRESH_TRIP_BUDGET_MINUTES);
      return { key: "freshtime", label: t("Fresh trip time"), ok: long.length === 0, detail: long.length ? `${long.length} ${t("Fresh trip(s) over 270 minutes")}${long.some((l) => l.coolingRisk) ? ` · ${t("chilled goods at risk")}` : ""}` : t("All Fresh trips within 270 minutes") };
    })(),
    { key: "policy", label: t("Planning policy"), ok: detail.policySignalAvailable !== false, detail: detail.policySignalAvailable === false ? t("Safe defaults in use") : t("Current policy loaded") },
  ];
}
