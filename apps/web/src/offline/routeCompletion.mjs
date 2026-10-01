export function hasQueuedRouteCompletion(queue, tripId) {
  return queue.some((item) => item.tripId === tripId && item.type === "ROUTE_COMPLETED");
}
