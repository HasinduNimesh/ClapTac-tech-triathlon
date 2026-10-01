/** Keep cached route work available on connection/server failure or expired auth.
 * Never turn an explicit authorization, validation, conflict, or missing-trip
 * response into an offline cache hit.
 */
export function shouldUseCachedDriverData(error) {
  if (error instanceof TypeError) return true;
  const status = error?.status;
  return Number.isInteger(status) && (status === 401 || status >= 500);
}
