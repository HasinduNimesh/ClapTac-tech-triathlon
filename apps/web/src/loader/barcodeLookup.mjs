export function findLoadOrderByCode(orders, rawValue) {
  const code = String(rawValue ?? "").trim().toLocaleLowerCase();
  if (!code) return undefined;
  return orders.find(order => order.orderId.toLocaleLowerCase() === code || String(order.orderRef ?? "").toLocaleLowerCase() === code);
}

export function resolveLoadOrderCode(orders, rawValue, isLoading) {
  const order = findLoadOrderByCode(orders, rawValue);
  if (!order) return { kind: "not_found" };
  if (!isLoading) return { kind: "not_loading", order };
  if ((order.issues || []).length > 0) return { kind: "unresolved_shortfall", order };
  return { kind: "ready", order };
}
