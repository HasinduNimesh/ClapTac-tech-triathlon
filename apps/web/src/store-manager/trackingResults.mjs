/**
 * Pairs each order with the outcome of loading its tracking. An order whose tracking could not be
 * loaded is kept (as `unavailable`) instead of being dropped, so a placed order never vanishes from
 * the screen just because the planning or delivery lookup behind it is failing.
 *
 * `settled` is Promise.allSettled output in the same order as `orders`.
 */
export function splitTrackingResults(orders, settled) {
  const rows = [];
  const unavailable = [];
  orders.forEach((order, index) => {
    const result = settled[index];
    if (result && result.status === "fulfilled") rows.push(result.value);
    else unavailable.push(order);
  });
  rows.sort((a, b) => b.order.requestedDeliveryDate.localeCompare(a.order.requestedDeliveryDate) || b.order.orderRef.localeCompare(a.order.orderRef));
  unavailable.sort((a, b) => b.requestedDeliveryDate.localeCompare(a.requestedDeliveryDate) || b.orderRef.localeCompare(a.orderRef));
  return { rows, unavailable };
}
