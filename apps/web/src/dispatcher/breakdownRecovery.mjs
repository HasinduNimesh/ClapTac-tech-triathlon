// State logic for the dispatcher's breakdown recovery. A recovery is only
// complete when every affected trip has a published revision AND the broken
// vehicle is in the workshop. If the workshop update fails the server answers
// 502 with the stable code "workshop_pending" and commits nothing for that
// trip, so the UI must show an unresolved state and retry - never "confirmed".

export const WORKSHOP_PENDING = "workshop_pending";

/** True when a failed reassign call means "retry: the vehicle is not in the workshop yet". */
export function isWorkshopPending(error) {
  if (!error || error.status !== 502) return false;
  try {
    return JSON.parse(error.message)?.code === WORKSHOP_PENDING;
  } catch {
    return false;
  }
}

/** A reassign result counts only when the plan is confirmed and the vehicle is in the workshop. */
export function isRecoveryConfirmed(result) {
  return result?.status === "confirmed" && result?.vehicleInWorkshop === true;
}

/**
 * Applies each chosen trip replacement in order, skipping trips already done in
 * an earlier attempt (the server would answer "affected trip not found" for
 * them). Stops at the first failure and reports why, so the caller can show a
 * Retry instead of claiming success.
 *
 * Returns { state: "complete" | "workshop_pending" | "failed", done, results, error? }
 * where `done` is every completed trip number so far and `results` holds only
 * the results gathered by this call.
 */
export async function runRecovery(trips, alreadyDone, reassign) {
  const done = [...alreadyDone];
  const results = [];
  for (const trip of trips) {
    if (!trip.replacement || done.includes(trip.tripNumber)) continue;
    let result;
    try {
      result = await reassign(trip);
    } catch (error) {
      return { state: isWorkshopPending(error) ? "workshop_pending" : "failed", done, results, error };
    }
    if (!isRecoveryConfirmed(result)) {
      return { state: "failed", done, results, error: new Error("The server did not confirm the vehicle is in the workshop.") };
    }
    done.push(trip.tripNumber);
    results.push(result);
  }
  return { state: "complete", done, results };
}
