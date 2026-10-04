export type ReceiptDiscrepancy = {
  needsIssue: boolean;
  short: number;
  differsFromDriver: boolean;
  driverKnown: boolean;
  driverUnits: number | null;
  affectedUnits: number;
  reason: "shortage" | "driver_mismatch" | null;
  suggestedIssueType: "MISSING" | "QUANTITY_MISMATCH";
};
export function receiptDiscrepancy(input: { ordered: number; received: number; driverDelivered?: number | null }): ReceiptDiscrepancy;
