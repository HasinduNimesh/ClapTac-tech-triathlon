export type FuelEntryDraft = { vehicleId: string; date: string; liters: number | string; receiptRef: string };
export type FuelAttempt = { signature: string; operationId: string };
export function resolveFuelAttempt(previous: FuelAttempt | null, entry: FuelEntryDraft, createOperationId: () => string): FuelAttempt;
