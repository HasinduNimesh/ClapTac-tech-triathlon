// Pure rules for when the language helpers (order text helper, dashboard assistant) count as unavailable.
// The screens work by hand either way, so "unavailable" is a calm state, never an error.

/** Normalises the assistants status response. Anything but an explicit true counts as off. */
export function helperStatusFrom(body) {
  return { order: body?.orderAssistant === true, dashboard: body?.dashboardAssistant === true };
}

/** True when a failed helper call means "the helper is switched off" (HTTP 503 or agent_unavailable). */
export function isHelperUnavailable(error) {
  if (!error || typeof error !== "object") return false;
  if (error.status === 503) return true;
  return typeof error.message === "string" && error.message.includes("agent_unavailable");
}
