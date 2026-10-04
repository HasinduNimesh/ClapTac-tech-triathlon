// Which depot a dispatcher is looking at. They start on their own depot (from their profile), or on all
// depots when they have none; switching is remembered for this browser tab only, per person, so the next
// person to sign in starts on their own depot.
const key = (userId) => `waypoint.dispatcher.depot.${userId}`;

/** The depot the screens show: the person's choice if they made one this tab, else their own depot, else all (""). */
export function effectiveDepot(homeDepot, choice) {
  return choice === undefined ? homeDepot || "" : choice;
}

/** A choice saved earlier in this tab, or undefined when there is none. "" is a real choice (all depots). */
export function readDepotChoice(storage, userId) {
  if (!userId) return undefined;
  try {
    const value = storage?.getItem(key(userId));
    return value === null || value === undefined ? undefined : value;
  } catch {
    return undefined;
  }
}

export function saveDepotChoice(storage, userId, depot) {
  if (!userId) return;
  try {
    storage?.setItem(key(userId), depot);
  } catch {
    // The choice still applies until the page is reloaded.
  }
}
