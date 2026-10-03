export const ESTIMATES_UNAVAILABLE_MESSAGE = "Estimates unavailable, using standard times";

export function validArrivalAt(value) {
  return typeof value === "string" && value.trim() !== "" && Number.isFinite(Date.parse(value));
}

export function standardArrivalAt(planned, snapshot) {
  if (validArrivalAt(planned)) return planned;
  if (validArrivalAt(snapshot)) return snapshot;
  return undefined;
}

export function validServiceMinutes(value) {
  return Number.isFinite(value) && value > 0;
}

export function hasUsablePrediction(items) {
  return Array.isArray(items) && items.some((item) => item?.status === "ESTIMATED") &&
    items.every((item) => item?.status !== "ESTIMATED" ||
      (Number.isFinite(item.probability) && item.probability >= 0 && item.probability <= 1));
}
