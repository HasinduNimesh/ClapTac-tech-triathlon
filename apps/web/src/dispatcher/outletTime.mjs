export function timeInputValue(value) {
  if (typeof value !== "string") return "";
  const match = value.match(/^(\d{2}:\d{2})(?::\d{2}(?:\.\d+)?)?$/);
  return match?.[1] || "";
}
