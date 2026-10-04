// Reading and checking an outlet's position as a dispatcher types or pastes it. The limits match
// shared-service, which is what actually enforces them.
export const LATITUDE_RANGE = [5.5, 10.0];
export const LONGITUDE_RANGE = [79.3, 82.2];

const NUMBER = "[-+]?\\d{1,3}(?:\\.\\d+)?";
// "6.93441, 79.84281" (what a map app copies), "6.93441 79.84281" or "6.93441;79.84281".
const PAIR = new RegExp(`^\\s*(${NUMBER})\\s*[,;\\s]\\s*(${NUMBER})\\s*$`);

/**
 * Turns pasted text into a position.
 * Returns { ok: true, latitude, longitude } or { ok: false, message } where message is a key for the
 * caller to translate.
 */
export function parseCoordinates(text) {
  const match = PAIR.exec(String(text ?? ""));
  if (!match) return { ok: false, message: "Enter latitude and longitude in Sri Lanka, for example 6.93441, 79.84281." };
  const latitude = Number(match[1]);
  const longitude = Number(match[2]);
  if (inSriLanka(latitude, longitude)) return { ok: true, latitude, longitude };
  if (inSriLanka(longitude, latitude)) return { ok: false, message: "Latitude and longitude look swapped. Latitude comes first, for example 6.93441, 79.84281." };
  return { ok: false, message: "That position is not in Sri Lanka. Enter latitude then longitude, for example 6.93441, 79.84281." };
}

function inSriLanka(latitude, longitude) {
  return latitude >= LATITUDE_RANGE[0] && latitude <= LATITUDE_RANGE[1] && longitude >= LONGITUDE_RANGE[0] && longitude <= LONGITUDE_RANGE[1];
}

/** The pair as it is shown in the field, matching what parseCoordinates reads back. */
export function formatCoordinates(latitude, longitude) {
  return `${Number(latitude.toFixed(6))}, ${Number(longitude.toFixed(6))}`;
}

/**
 * What saving the outlet should do to its position, given what is in the field.
 * Returns { change: "keep" } | { change: "clear" } | { change: "set", latitude, longitude } | { change: "invalid", message }.
 */
export function locationChange(text, current) {
  const typed = String(text ?? "").trim();
  const exact = current && current.latitude != null && current.longitude != null && !current.locationApproximate;
  if (!typed) return exact ? { change: "clear" } : { change: "keep" };
  const parsed = parseCoordinates(typed);
  if (!parsed.ok) return { change: "invalid", message: parsed.message };
  if (exact && Math.abs(parsed.latitude - current.latitude) < 1e-9 && Math.abs(parsed.longitude - current.longitude) < 1e-9) return { change: "keep" };
  return { change: "set", latitude: parsed.latitude, longitude: parsed.longitude };
}

/** A link a dispatcher can open to check the point is the shop. */
export function openStreetMapLink(latitude, longitude) {
  return `https://www.openstreetmap.org/?mlat=${latitude}&mlon=${longitude}#map=18/${latitude}/${longitude}`;
}

/** Searches by the outlet's name when no exact position is known. */
export function openStreetMapSearchLink(name, district) {
  const query = [name, district, "Sri Lanka"].filter(Boolean).join(", ");
  return `https://www.openstreetmap.org/search?query=${encodeURIComponent(query)}`;
}
