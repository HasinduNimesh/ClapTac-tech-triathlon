export function splitTrackingResults<O extends { requestedDeliveryDate: string; orderRef: string }, T extends { order: O }>(
  orders: O[],
  settled: PromiseSettledResult<T>[],
): { rows: T[]; unavailable: O[] };
