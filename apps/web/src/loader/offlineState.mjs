export function shouldQueueLoadingFailure(error, online = true) {
  if (!online || error instanceof TypeError) return true;
  return Number.isInteger(error?.status) && error.status >= 500;
}

export function applyLoadingQueueItem(detail, item) {
  const next = structuredClone(detail);
  if (item.type === "START") {
    next.status = "in_progress";
    next.loadingStatus = "in_progress";
  } else if (item.type === "ORDER_LOADED") {
    const order = next.orders?.find((candidate) => candidate.orderId === item.orderId);
    if (order && !(order.issues || []).length) {
      order.status = "loaded";
    }
  } else if (item.type === "ISSUE_CREATE") {
    const order = next.orders?.find((candidate) => candidate.orderId === item.orderId);
    if (order) {
      order.status = "shortfall";
      order.issues = [...(order.issues || []), {
        id: `local-${item.operationId}`,
        type: item.payload.type,
        affectedUnits: item.payload.affectedUnits,
        note: item.payload.note,
        reportedBy: "device queue",
      }];
    }
  } else if (item.type === "READY") {
    next.status = "ready";
    next.loadingStatus = "ready";
  }
  const orders = next.orders || [];
  next.loadedCount = orders.filter((order) => order.status === "loaded").length;
  next.shortfallCount = orders.filter((order) => order.status === "shortfall").length;
  next.pendingCount = orders.filter((order) =>
    order.status === "pending" || (order.status === "shortfall" && !(order.issues || []).length),
  ).length;
  return next;
}

export function replayLoadingQueue(detail, items) {
  return items.reduce((current, item) => applyLoadingQueueItem(current, item), detail);
}
