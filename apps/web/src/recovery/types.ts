export type RecoveryMove = {
  orderId: string; orderRef: string; outletId: string; outletName: string; vehicleId: string;
  chilled: boolean; arrival: string; serviceStart: string; departure: string; delayMinutes: number; deferred: boolean; reason?: string;
};
export type RecoveryCandidate = {
  id: string; strategy: "single" | "split" | "partial" | "defer"; recommended: boolean; moves: RecoveryMove[];
  recovered: number; deferred: number; chilledDeferred: number; vehicles: number; delayMinutes: number; distanceKm: number;
};
export type RecoveryCase = {
  id: string; planId: string; planVersion: number; depot: string; readyAt: string; createdAt: string; expiresAt: string;
  incident: { id: string; vehicleId: string; description: string; date: string };
  status: "pending" | "superseded" | "approved"; candidates: RecoveryCandidate[];
  preferences: { outletId: string; candidateId: string; note: string; updatedAt: string }[];
  selectedId?: string; publishedVersion?: number; approvalNote?: string;
};
