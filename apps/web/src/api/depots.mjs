// The dataset names depots "Peliyagoda" and "Kandy"; user profiles and the depot selector use
// DEPOT_NORTH and DEPOT_SOUTH. Everything that compares or looks up a depot goes through depotCode.
const DEPOT_ALIASES = { DEPOT_NORTH: "DEPOT_NORTH", PELIYAGODA: "DEPOT_NORTH", DEPOT_SOUTH: "DEPOT_SOUTH", KANDY: "DEPOT_SOUTH" };

export function depotCode(depot) {
  const key = String(depot ?? "").trim().toUpperCase();
  return DEPOT_ALIASES[key] || key;
}

// Approximate depot positions: there is no depots table, so these two are fixed in code.
export const DEPOT_LOCATIONS = {
  DEPOT_NORTH: [6.9608, 79.8857], // Peliyagoda distribution centre
  DEPOT_SOUTH: [7.2955, 80.6356], // Kandy regional hub
};

/**
 * Where a depot is on the map, whatever name it arrives under ("Peliyagoda", "DEPOT_NORTH", ...), or
 * undefined when it is not one we know. Never a stand-in: a trip from an unknown depot is drawn without
 * a depot, not from the wrong one.
 */
export function depotPosition(depot) {
  return DEPOT_LOCATIONS[depotCode(depot)];
}
