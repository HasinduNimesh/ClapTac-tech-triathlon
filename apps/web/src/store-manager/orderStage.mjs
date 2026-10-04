import { validArrivalAt } from "../api/estimateAvailability.mjs";

const COLOMBO = "Asia/Colombo";

export function colomboDate(value) {
  if (!validArrivalAt(value) && !(typeof value === "number" && Number.isFinite(value))) return "";
  return new Intl.DateTimeFormat("en-CA", { timeZone: COLOMBO, year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date(value));
}

export function colomboTime(value) {
  if (!validArrivalAt(value) && !(typeof value === "number" && Number.isFinite(value))) return "";
  return new Intl.DateTimeFormat("en-GB", { timeZone: COLOMBO, hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(new Date(value));
}

export function formatDay(dateOnly) {
  if (!dateOnly) return "—";
  const [year, month, day] = String(dateOnly).slice(0, 10).split("-").map(Number);
  if (!year || !month || !day) return String(dateOnly);
  return new Intl.DateTimeFormat("en-GB", { timeZone: "UTC", weekday: "short", day: "numeric", month: "short" }).format(new Date(Date.UTC(year, month - 1, day)));
}

export const isDeferred = (stage) => stage === "DEFERRED";
export const isActiveRun = (stage) => stage === "READY_FOR_DEPARTURE" || stage === "OUT_FOR_DELIVERY";
export const needsReceipt = (stage) => stage === "DELIVERED" || stage === "PARTIAL";
export const isReceiptConfirmed = (stage) => stage === "RECEIPT_CONFIRMED" || stage === "RECEIPT_CONFIRMED_WITH_ISSUE";
export const isNotDelivered = (stage) => stage === "NOT_DELIVERED" || stage === "FAILED" || stage === "REFUSED";

export function statusLabel(stage) {
  switch (stage) {
    case "CONFIRMED": return "Confirmed";
    case "PLANNED": return "Planned";
    case "DEFERRED": return "Deferred";
    case "READY_FOR_DEPARTURE": return "Ready to depart";
    case "OUT_FOR_DELIVERY": return "On route";
    case "DELIVERED": return "Delivered";
    case "PARTIAL": return "Partially delivered";
    case "REFUSED": return "Rejected delivery";
    case "NOT_DELIVERED":
    case "FAILED": return "Not delivered";
    case "RECEIPT_CONFIRMED": return "Receipt confirmed";
    case "RECEIPT_CONFIRMED_WITH_ISSUE": return "Receipt confirmed with issue";
    default: return "Confirmed";
  }
}

export function statusTone(stage) {
  if (stage === "OUT_FOR_DELIVERY" || stage === "READY_FOR_DEPARTURE") return "on-route";
  if (stage === "DEFERRED" || stage === "NOT_DELIVERED" || stage === "FAILED" || stage === "REFUSED" || stage === "PARTIAL" || stage === "RECEIPT_CONFIRMED_WITH_ISSUE") return "deferred";
  if (stage === "DELIVERED" || stage === "RECEIPT_CONFIRMED") return "delivered";
  return "default";
}

export function arrivesOn(tracking, date) {
  if (!tracking) return false;
  const { stage, planning, order } = tracking;
  if (!isActiveRun(stage) && stage !== "PLANNED") return false;
  if (validArrivalAt(planning?.plannedArrivalAt)) return colomboDate(planning.plannedArrivalAt) === date;
  return order?.requestedDeliveryDate === date;
}

export function sortByArrival(a, b) {
  const left = validArrivalAt(a.planning?.plannedArrivalAt) ? a.planning.plannedArrivalAt : "9999";
  const right = validArrivalAt(b.planning?.plannedArrivalAt) ? b.planning.plannedArrivalAt : "9999";
  return left.localeCompare(right);
}

const LEVEL = {
  CONFIRMED: 1,
  DEFERRED: 1,
  PLANNED: 2,
  READY_FOR_DEPARTURE: 2,
  OUT_FOR_DELIVERY: 3,
  DELIVERED: 4,
  PARTIAL: 4,
  NOT_DELIVERED: 4,
  FAILED: 4,
  REFUSED: 4,
  RECEIPT_CONFIRMED: 5,
  RECEIPT_CONFIRMED_WITH_ISSUE: 5,
};

export function timelineSteps(tracking) {
  const stage = tracking?.stage || "CONFIRMED";
  const level = LEVEL[stage] ?? 1;
  const deferred = isDeferred(stage);
  const notDelivered = isNotDelivered(stage);
  const steps = [
    { key: "placed", label: "Order placed", detail: "Request acknowledged" },
    {
      key: "planned",
      label: deferred ? "Deferred to a later run" : stage === "CONFIRMED" ? "Awaiting dispatch planning" : "Trip planned",
      detail: deferred ? "Dispatch is reviewing the next available run" : stage === "CONFIRMED" ? "Confirmed at cutoff, in the dispatcher queue" : "Valid trip assigned",
      warn: deferred,
    },
    {
      key: "onroute",
      label: "On route",
      detail: stage === "READY_FOR_DEPARTURE" ? "Loaded and ready to depart" : "Latest driver event informs the ETA",
    },
    {
      key: "delivered",
      label: stage === "REFUSED" ? "Rejected delivery" : notDelivered ? "Not delivered" : stage === "PARTIAL" ? "Partially delivered" : "Delivered",
      detail: notDelivered ? "The driver could not complete this delivery" : "Driver recorded the delivery outcome",
      warn: notDelivered || stage === "PARTIAL",
    },
  ];
  if (!notDelivered) {
    steps.push({
      key: "receipt",
      label: stage === "RECEIPT_CONFIRMED_WITH_ISSUE" ? "Receipt confirmed with issue" : "Receipt confirmed",
      detail: "Store confirms the quantity received",
    });
  }
  return steps.map((step, index) => ({
    ...step,
    state: index < level ? "done" : index === level ? "current" : "upcoming",
  }));
}
