// The loader app is a separate app with its own sign-in. When this app has just signed a loader in and sends
// them there, it leaves a timestamp in this tab's sessionStorage. The loader app reads it once and carries on
// to the identity server without its own Sign in screen or a forced re-login. A tab opened fresh has no marker.
export const LOADER_HANDOFF_KEY = "waypoint.loader.handoff";

/** Returns true when the marker was stored. Storage can be blocked, in which case the loader app shows its own Sign in. */
export function markLoaderHandoff(storage = globalThis.sessionStorage, now = Date.now()) {
  try {
    storage.setItem(LOADER_HANDOFF_KEY, String(now));
    return true;
  } catch {
    return false;
  }
}
