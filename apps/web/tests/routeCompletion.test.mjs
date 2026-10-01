import assert from "node:assert/strict";
import test from "node:test";
import { hasQueuedRouteCompletion } from "../src/offline/routeCompletion.mjs";

test("restores the completion summary only for a queued completion of this trip", () => {
  const queue = [
    { type: "ARRIVED", tripId: "trip-1" },
    { type: "ROUTE_COMPLETED", tripId: "trip-2" },
  ];

  assert.equal(hasQueuedRouteCompletion(queue, "trip-2"), true);
  assert.equal(hasQueuedRouteCompletion(queue, "trip-1"), false);
  assert.equal(hasQueuedRouteCompletion(queue, "trip-3"), false);
  assert.equal(hasQueuedRouteCompletion([], "trip-2"), false);
});
