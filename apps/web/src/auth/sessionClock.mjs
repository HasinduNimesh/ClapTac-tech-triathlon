// A web sign-in lasts an hour. These helpers say where a session is in that hour, so the app can warn people
// while they can still finish what they are doing instead of failing a request without explanation.

/** The warning starts this long before the session ends. */
export const WARN_MS = 5 * 60_000;

/** active, expiring (warn now) or expired. A refused request counts as expired even if the clock says otherwise. */
export function sessionPhase(expiresAtMs, now, rejected = false) {
  if (rejected) return "expired";
  if (!Number.isFinite(expiresAtMs)) return "active";
  if (now >= expiresAtMs) return "expired";
  if (expiresAtMs - now <= WARN_MS) return "expiring";
  return "active";
}

export function minutesLeft(expiresAtMs, now) {
  return Math.max(1, Math.ceil((expiresAtMs - now) / 60_000));
}

/** Whether the sign-in can renew itself with a refresh token (needs the offline_access scope). */
export function canRenew(scope) {
  return String(scope || "").split(/\s+/).includes("offline_access");
}

/** Milliseconds until the phase can next change, so a timer can wake the app exactly then. Null when it cannot. */
export function msUntilChange(expiresAtMs, now) {
  if (!Number.isFinite(expiresAtMs) || now >= expiresAtMs) return null;
  const untilWarning = expiresAtMs - WARN_MS - now;
  return untilWarning > 0 ? untilWarning : expiresAtMs - now;
}
