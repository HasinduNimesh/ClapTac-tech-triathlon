export const WORKSHOP_PENDING: "workshop_pending";
export type RecoveryTrip = { tripNumber: number; replacement?: string };
export type RecoveryResult = { status?: string; vehicleInWorkshop?: boolean; noticeDrafts?: { orderRef: string; outletId: string; estimatedArrival: string }[]; planVersion?: number };
export type RecoveryRun<R> = { state: "complete" | "workshop_pending" | "failed"; done: number[]; results: R[]; error?: unknown };
export function isWorkshopPending(error: unknown): boolean;
export function isRecoveryConfirmed(result: RecoveryResult | undefined | null): boolean;
export function runRecovery<R extends RecoveryResult>(trips: RecoveryTrip[], alreadyDone: number[], reassign: (trip: RecoveryTrip) => Promise<R>): Promise<RecoveryRun<R>>;
