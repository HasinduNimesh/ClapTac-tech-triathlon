// When a session ends, signing in again should bring the person back to the page they were on. The page is kept in
// this tab only, used once, and only if it is a page inside the role's own area: a stored value is never trusted
// as a place to send someone.

export const RETURN_KEY = "waypoint.returnTo";

const AREAS = { STORE_MANAGER: "/store-manager", DISPATCHER: "/dispatcher" };

function safe(path) {
  if (typeof path !== "string" || path.length > 300) return false;
  if (!path.startsWith("/") || path.startsWith("//") || path.includes("\\") || /[\u0000-\u001f]/.test(path)) return false;
  if (path.startsWith("/login") || path.startsWith("/auth/")) return false;
  return true;
}

export function rememberReturn(storage, path) {
  try {
    if (safe(path)) storage.setItem(RETURN_KEY, path);
  } catch {
    // Storage can be blocked; the person then lands on their usual start page.
  }
}

/** The remembered page if it belongs to this role's area, else null. It is removed either way. */
export function takeReturn(storage, role) {
  let path = null;
  try {
    path = storage.getItem(RETURN_KEY);
    storage.removeItem(RETURN_KEY);
  } catch {
    return null;
  }
  const area = AREAS[role];
  if (!area || !safe(path)) return null;
  return path === area || path.startsWith(`${area}/`) || path.startsWith(`${area}?`) ? path : null;
}
