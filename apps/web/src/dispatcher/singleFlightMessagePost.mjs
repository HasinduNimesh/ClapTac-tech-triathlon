/** Coalesce identical message POSTs while one request is still pending in this tab. */
export function singleFlightMessagePost(post) {
  const pending = new Map();
  return (request) => {
    const key = JSON.stringify([request.senderId, request.tripId, request.stopId || "", request.body.trim()]);
    if (pending.has(key)) return pending.get(key);
    let current;
    current = Promise.resolve().then(() => post(request)).finally(() => {
      if (pending.get(key) === current) pending.delete(key);
    });
    pending.set(key, current);
    return current;
  };
}
