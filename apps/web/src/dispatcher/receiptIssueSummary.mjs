// One line saying what a store reported for a delivered order. The wording follows the issue type: a count that
// differs from the driver's record is not necessarily a shortage, and damaged goods did arrive.

export function receiptIssueSummary(issueType, receivedUnits, expectedUnits, affectedUnits, t) {
  const counted = `${receivedUnits} ${t("of")} ${expectedUnits}`;
  switch (issueType) {
    case "MISSING":
      return `${counted} · ${affectedUnits} ${t("short")}`;
    case "DAMAGED":
      return `${counted} · ${affectedUnits} ${t("damaged")}`;
    case "QUANTITY_MISMATCH":
      return `${counted} · ${t("differs from the driver's record by")} ${affectedUnits}`;
    default:
      return `${counted} · ${affectedUnits} ${t("affected")}`;
  }
}
