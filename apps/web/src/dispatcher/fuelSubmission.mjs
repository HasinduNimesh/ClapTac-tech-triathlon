function fingerprint(entry) {
  return JSON.stringify({
    vehicleId: entry.vehicleId,
    date: entry.date,
    liters: Number(entry.liters),
    receiptRef: entry.receiptRef.trim(),
  });
}

/** Reuse an idempotency key for retries of the same in-flight fuel entry. */
export function resolveFuelAttempt(previous, entry, createOperationId) {
  const signature = fingerprint(entry);
  if (previous?.signature === signature) return previous;
  return { signature, operationId: createOperationId() };
}
