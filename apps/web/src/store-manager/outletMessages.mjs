// Presentation rules for the messages (SMS notices) the system queued for a store. Labels are
// returned as English source strings; the page passes them through t().

const TYPE_LABELS = {
  LOAD_SHORTFALL: "SHORT LOAD",
  DEFERRAL: "DEFERRED",
  ARRIVAL_CHANGE: "ETA CHANGE",
  MAJOR_DELAY: "DELAY",
  DELIVERY_REJECTED: "DELIVERY REFUSED",
};

export function messageTypeLabel(eventType) {
  return TYPE_LABELS[eventType] || "NOTICE";
}

// PENDING/SENDING/QUEUED are waiting for an SMS provider; FAILED/UNKNOWN never reached one. Without a
// provider (local environments) messages simply stay queued or are marked not sent, so the wording
// stays neutral and the message text is always shown.
export function messageStatusLabel(status) {
  switch (status) {
    case "SENT": return "Sent";
    case "DELIVERED": return "Delivered";
    case "FAILED":
    case "UNKNOWN": return "Not sent";
    default: return "Queued";
  }
}

export function messageTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat("en-GB", { timeZone: "Asia/Colombo", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(date);
}

export const messageLabelKeys = [...Object.values(TYPE_LABELS), "NOTICE", "Sent", "Delivered", "Not sent", "Queued"];
