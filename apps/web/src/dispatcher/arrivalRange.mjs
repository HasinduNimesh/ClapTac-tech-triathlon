export function calibratedArrivalRange(estimate, history) {
  if (!estimate?.eta || estimate.kind === "unknown" || history?.arrivalRangeStatus !== "CALIBRATED") return undefined;
  const lowerOffset = history.arrivalOffsetP10Minutes;
  const upperOffset = history.arrivalOffsetP90Minutes;
  if (!Number.isFinite(lowerOffset) || !Number.isFinite(upperOffset) || lowerOffset > upperOffset) return undefined;
  const center = Date.parse(estimate.eta);
  if (!Number.isFinite(center)) return undefined;
  return {
    lower: new Date(center + lowerOffset * 60_000),
    upper: new Date(center + upperOffset * 60_000),
    sampleCount: history.arrivalRangeSamples,
    holdoutCount: history.arrivalRangeHoldouts,
    coverage: history.arrivalRangeCoverage,
    version: history.arrivalRangeVersion,
  };
}
