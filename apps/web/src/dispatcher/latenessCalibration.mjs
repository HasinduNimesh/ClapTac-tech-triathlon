export function evaluatedLatenessCalibrations(history) {
  return history.filter(item => item.calibrationStatus === "EVALUATED" && item.brierScore != null);
}
