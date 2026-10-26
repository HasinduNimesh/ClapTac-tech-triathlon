const referenceOrder = new Intl.Collator("en", { numeric: true, sensitivity: "base" });

export const orderSortOptions = [
  ["newest", "Newest first"],
  ["oldest", "Oldest first"],
  ["delivery", "Delivery date: earliest first"],
  ["reference", "Order number"],
];

export const orderStages = [
  "CONFIRMED", "PLANNED", "READY_FOR_DEPARTURE", "OUT_FOR_DELIVERY", "DEFERRED",
  "DELIVERED", "PARTIAL", "REFUSED", "NOT_DELIVERED", "FAILED",
  "RECEIPT_CONFIRMED", "RECEIPT_CONFIRMED_WITH_ISSUE",
];

function dateOrder(left, right, descending = false) {
  const a = Date.parse(left);
  const b = Date.parse(right);
  // Undated legacy orders stay last in either direction.
  if (!Number.isFinite(a)) return Number.isFinite(b) ? 1 : 0;
  if (!Number.isFinite(b)) return -1;
  return descending ? b - a : a - b;
}

/** Filter a copy; polling and other consumers retain the original order array. */
export function browseOrders(rows, { sort = "newest", stage = "all", goods = "all", query = "" } = {}) {
  const search = query.trim().toLowerCase();
  return rows.filter(({ order, stage: current }) =>
    (stage === "all" || current === stage) &&
    (goods === "all" || order.temperatureRequirement.toLowerCase() === goods) &&
    (!search || order.orderRef.toLowerCase().includes(search)),
  ).sort((a, b) => {
    const reference = referenceOrder.compare(a.order.orderRef, b.order.orderRef) || a.order.id.localeCompare(b.order.id);
    if (sort === "reference") return reference;
    if (sort === "delivery") return dateOrder(a.order.requestedDeliveryDate, b.order.requestedDeliveryDate) || reference;
    const descending = sort !== "oldest";
    return dateOrder(a.order.createdAt, b.order.createdAt, descending) || (descending ? -reference : reference);
  });
}
