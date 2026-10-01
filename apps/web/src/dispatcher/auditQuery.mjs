function sriLankaTimestamp(value, endOfMinute = false) {
  if (!value) return "";
  const localValue = value.length === 16 ? `${value}:${endOfMinute ? "59.999" : "00"}` : value;
  const timestamp = new Date(`${localValue}+05:30`);
  if (!Number.isFinite(timestamp.getTime())) throw new Error("Enter a valid date and time.");
  return timestamp.toISOString();
}

export function buildAuditSearchParams(filters, offset = 0) {
  const params = new URLSearchParams({ limit: "50", offset: String(offset) });
  for (const key of ["query", "action", "resourceType", "resourceId", "actorId"]) {
    const value = filters[key]?.trim();
    if (value) params.set(key === "query" ? "q" : key, value);
  }
  const from = sriLankaTimestamp(filters.from);
  const to = sriLankaTimestamp(filters.to, true);
  if (from && to && Date.parse(to) < Date.parse(from)) throw new Error("To must be after From.");
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  return params;
}
