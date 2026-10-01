/** Share one in-flight operation in this tab and serialize it across tabs when Web Locks are available. */
export function singleFlight(run, keyOf = () => "default", lockManager = globalThis.navigator?.locks) {
  const active = new Map();
  return (...args) => {
    const key = keyOf(...args);
    if (active.has(key)) return active.get(key);
    let current;
    current = Promise.resolve().then(() => {
      if (!lockManager?.request) return run(...args);
      return lockManager.request(`waypoint-delivery-sync:${key}`, { mode: "exclusive" }, () => run(...args));
    }).finally(() => {
      if (active.get(key) === current) active.delete(key);
    });
    active.set(key, current);
    return current;
  };
}
