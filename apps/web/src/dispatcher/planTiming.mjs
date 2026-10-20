/** How many planned arrivals are already behind the clock. Locking such a plan sends drivers a run that cannot be on time. */
export function pastArrivals(allocations = [], now = Date.now()) {
  const times = allocations.map((a) => Date.parse(a?.plannedArrivalAt || "")).filter((ms) => Number.isFinite(ms));
  const past = times.filter((ms) => ms < now).length;
  return { past, total: times.length };
}
