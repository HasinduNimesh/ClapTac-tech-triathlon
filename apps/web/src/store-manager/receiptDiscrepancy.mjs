// Decides whether a store receipt needs a discrepancy issue. This mirrors the
// order-service ConfirmReceipt rule exactly:
//   - received below the original order always needs an issue, and
//   - when the driver's delivered count is known, any received count that
//     differs from it needs an issue (including a count ABOVE the driver's).
// When the driver count is unknown only the shortage rule applies.

const count = (v) => (typeof v === "number" && Number.isFinite(v) ? Math.max(0, Math.trunc(v)) : 0);

export function receiptDiscrepancy({ ordered, received, driverDelivered }) {
  const orderedUnits = count(ordered);
  const receivedUnits = count(received);
  const driverKnown = typeof driverDelivered === "number" && Number.isFinite(driverDelivered);
  const driverUnits = driverKnown ? count(driverDelivered) : null;
  const short = Math.max(0, orderedUnits - receivedUnits);
  const differsFromDriver = driverKnown && receivedUnits !== driverUnits;
  const needsIssue = short > 0 || differsFromDriver;
  // Shortage is measured against the original order (as before); a pure
  // driver-count variance is measured against the driver's record.
  const affectedUnits = short > 0 ? short : differsFromDriver ? Math.min(orderedUnits, Math.abs(receivedUnits - driverUnits)) : 0;
  const reason = short > 0 ? "shortage" : differsFromDriver ? "driver_mismatch" : null;
  return { needsIssue, short, differsFromDriver, driverKnown, driverUnits, affectedUnits, reason, suggestedIssueType: reason === "driver_mismatch" ? "QUANTITY_MISMATCH" : "MISSING" };
}
