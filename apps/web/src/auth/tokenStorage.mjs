// What the browser keeps of a sign-in. The page's storage can be read by any script that runs on the site, so it
// should hold only what the app uses. The refresh token is the most valuable item (it outlives the access token and
// can be used from anywhere), so it is not kept unless the app is set up to renew with it.

/** The stored user as JSON text, without its refresh token. Anything that is not a JSON object is returned as is. */
export function withoutRefreshToken(text) {
  if (typeof text !== "string") return text;
  let user;
  try { user = JSON.parse(text); } catch { return text; }
  if (!user || typeof user !== "object" || Array.isArray(user) || !("refresh_token" in user)) return text;
  const { refresh_token: _dropped, ...rest } = user;
  return JSON.stringify(rest);
}

/** True for the entries oidc-client-ts uses for the signed-in user. */
export function isUserEntry(key) {
  return typeof key === "string" && key.startsWith("oidc.user:");
}

/**
 * A Storage that writes through to `backing`, but saves the signed-in user without its refresh token when the app
 * cannot use one. Reads are untouched, so the access token and the id token work exactly as before.
 */
export function storageWithoutRefreshToken(backing, keepRefreshToken) {
  return {
    get length() { return backing.length; },
    key: (index) => backing.key(index),
    getItem: (key) => backing.getItem(key),
    removeItem: (key) => backing.removeItem(key),
    clear: () => backing.clear(),
    setItem: (key, value) => backing.setItem(key, !keepRefreshToken && isUserEntry(key) ? withoutRefreshToken(value) : value),
  };
}
