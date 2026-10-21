const LABELS = {
  affectedUnits: "Units affected", expectedUnits: "Expected units", receivedUnits: "Received units", issueType: "Reason", status: "Status",
  runId: "Run", planRef: "Plan", planVersion: "Plan version", vehicleId: "Vehicle", orderRef: "Order", decision: "Decision", note: "Note",
  reasonCode: "Reason", sealNumber: "Seal", chilledTemperatureC: "Chilled temperature (°C)", units: "Units",
};

const ISO = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2})$/;

/** An ISO time as "20 Oct, 22:05" in Sri Lanka time, the way the rest of the audit page writes times. */
function clock(value) {
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString("en-GB", { timeZone: "Asia/Colombo", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", hour12: false });
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const words = (key) => key.replace(/([a-z0-9])([A-Z])/g, "$1 $2").replace(/_/g, " ").toLowerCase().replace(/^./, (c) => c.toUpperCase());

function show(value) {
  if (value === null || value === undefined || value === "") return "";
  if (typeof value === "boolean") return value ? "yes" : "no";
  if (typeof value === "number") return String(value);
  if (typeof value === "string") {
    if (UUID.test(value)) return value.slice(0, 8);
    if (ISO.test(value)) return clock(value);
    // Codes such as MISSING or confirmed_with_issue read better as plain words; references like ORD000017 stay as they are.
    if (/^[A-Z]+(_[A-Z]+)*$/.test(value) || /^[a-z]+(_[a-z]+)+$/.test(value)) return value.toLowerCase().replace(/_/g, " ");
    return value;
  }
  if (Array.isArray(value)) return `${value.length} ${value.length === 1 ? "item" : "items"}`;
  return "";
}

/** What an audit row says happened, as words. A reason, when there is one, is the whole answer. */
export function auditDetail(event, t = (s) => s) {
  if (event?.reason) return event.reason;
  const state = event?.new_state && typeof event.new_state === "object" ? event.new_state : {};
  const parts = Object.entries(state)
    .map(([key, value]) => [key, show(value)])
    .filter(([, shown]) => shown !== "")
    .map(([key, shown]) => `${t(LABELS[key] || words(key))}: ${shown}`);
  return parts.length ? parts.join(" · ") : "—";
}
