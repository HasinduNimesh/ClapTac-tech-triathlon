// What a dispatcher sees about how far a truck has driven and what that suggests about fuel. Distance comes from the
// driver phone's position reports; fuel is an estimate from the vehicle's rated km per litre, not a measurement.
// Litres actually put in the tank are recorded separately in the fuel ledger.

const FRESH_MINUTES = 5;

/** A live position counts only while it is recent; after that the truck is drawn at its last reported stop. */
export function isFresh(timestamp, now, minutes = FRESH_MINUTES) {
  const at = new Date(timestamp).getTime();
  return Number.isFinite(at) && now - at >= 0 - 60_000 && now - at <= minutes * 60_000;
}

export function drivenKm(distanceM) {
  const metres = Number(distanceM);
  return Number.isFinite(metres) && metres > 0 ? metres / 1000 : 0;
}

/** Litres this distance would use at the vehicle's rated efficiency. Null when the rating is missing or not usable. */
export function estimatedLitres(distanceM, kmPerL) {
  const rated = Number(kmPerL);
  if (!Number.isFinite(rated) || rated <= 0) return null;
  return drivenKm(distanceM) / rated;
}

export function formatKm(km) {
  return `${km.toLocaleString("en-US", { minimumFractionDigits: km < 10 ? 1 : 0, maximumFractionDigits: km < 10 ? 1 : 0 })} km`;
}

export function formatLitres(litres) {
  return `${litres.toLocaleString("en-US", { minimumFractionDigits: 1, maximumFractionDigits: 1 })} L`;
}

/** The truck's drawn position: its fresh live report, else null so the caller falls back to the last stop. */
export function livePoint(location, now) {
  if (!location || !isFresh(location.timestamp, now)) return null;
  const { latitude, longitude } = location;
  return Number.isFinite(latitude) && Number.isFinite(longitude) ? [latitude, longitude] : null;
}
