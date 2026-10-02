// Plain-language deferral explanations for Store Managers (FR-27).
// The deterministic planner's reason codes stay authoritative; this only translates
// them. Strings are English keys that the UI passes through t() for si/ta.
export const deferralMessages = {
  "REFRIGERATION_REQUIRED": {
    "message": "No refrigerated vehicle had space for this chilled order on the delivery day.",
    "nextAction": "Your order stays on the list. Dispatch will try the next feasible run; no action needed unless it is urgent, then call dispatch."
  },
  "VAN_REQUIRED": {
    "message": "Your outlet can only be reached by a van, and no van was free for this order.",
    "nextAction": "Your order stays on the list for the next feasible run. If access has changed, ask dispatch to update your outlet details."
  },
  "WEIGHT_CAPACITY_EXCEEDED": {
    "message": "The available vehicles were already full by weight.",
    "nextAction": "Your order stays on the list for the next feasible run. A smaller order may fit sooner."
  },
  "VOLUME_CAPACITY_EXCEEDED": {
    "message": "The available vehicles were already full by volume.",
    "nextAction": "Your order stays on the list for the next feasible run. A smaller order may fit sooner."
  },
  "DELIVERY_WINDOW_CONFLICT": {
    "message": "No vehicle could arrive inside your delivery window.",
    "nextAction": "Your order stays on the list. Check that your delivery window is correct and ask dispatch to update it if it has changed."
  },
  "FUEL_QUOTA_EXCEEDED": {
    "message": "The vehicles that could serve you had no weekly fuel allowance left.",
    "nextAction": "Your order stays on the list and will be planned once fuel allowance is available."
  },
  "DEPOT_MISMATCH": {
    "message": "No vehicle from your depot was available for this order.",
    "nextAction": "Your order stays on the list for the next feasible run."
  },
  "TRIP_LIMIT_REACHED": {
    "message": "All vehicles had already reached their maximum trips for the day.",
    "nextAction": "Your order stays on the list for the next feasible run."
  },
  "VEHICLE_UNAVAILABLE": {
    "message": "The vehicles that could serve you were unavailable, for example in the workshop.",
    "nextAction": "Your order stays on the list for the next feasible run."
  },
  "NO_ELIGIBLE_VEHICLE": {
    "message": "No vehicle could take this order on the delivery day.",
    "nextAction": "Your order stays on the list for the next feasible run."
  },
  "MANUAL_DISPATCHER_DEFERRAL": {
    "message": "Dispatch decided to move this order to a later run.",
    "nextAction": "Check the note from dispatch if one is shown, or contact dispatch for the new date."
  }
};

export const genericDeferralMessage = {
  "message": "This order could not be delivered on the requested day.",
  "nextAction": "Your order stays on the list. Contact dispatch if you need it urgently."
};

export function deferralExplanation(reasonCode) {
  return deferralMessages[String(reasonCode || "").toUpperCase()] ?? genericDeferralMessage;
}
