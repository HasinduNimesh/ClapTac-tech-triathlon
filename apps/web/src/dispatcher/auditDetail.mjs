const LABELS = {
  affectedUnits: "Units affected", expectedUnits: "Expected units", receivedUnits: "Received units", issueType: "Reason", status: "Status",
  runId: "Run", planRef: "Plan", planVersion: "Plan version", vehicleId: "Vehicle", orderRef: "Order", decision: "Decision", note: "Note",
  reasonCode: "Reason", sealNumber: "Seal", chilledTemperatureC: "Chilled temperature (°C)", units: "Units",
};

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const words = (key) => key.replace(/([a-z0-9])([A-Z])/g, "$1 $2").replace(/_/g, " ").toLowerCase().replace(/^./, (c) => c.toUpperCase());

function show(value) {
  if (value === null || value === undefined || value === "") return "";
  if (typeof value === "boolean") return value ? "yes" : "no";
  if (typeof value === "number") return String(value);
  if (typeof value === "string") {
    if (UUID.test(value)) return value.slice(0, 8);
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
