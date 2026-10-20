/**
 * Turns a failed request into a sentence a person can act on. The server answers with problem+json
 * ({"title":"Conflict","status":409,"detail":"conflict: plan version changed"}), and browsers throw
 * "TypeError: Failed to fetch" when the network is down. Neither should reach the screen as it is,
 * and a 5xx detail (a database message, say) should never reach it at all.
 */

const NETWORK = /failed to fetch|networkerror|load failed|network request failed|the internet connection appears to be offline|aborted/i;
// A server detail is only shown when it reads like a sentence for a person, not like an internal message.
const INTERNAL = /pq:|sql|panic|runtime error|nil pointer|goroutine|stack|exception|traceback|[{}<>]|\bat \w+\.\w+|\bconstraint\b|violates|syntax error|deadline exceeded|context canceled|connection refused|no such host|dial tcp/i;
const PREFIX = /^(invalid|conflict|forbidden|not found|unauthorized|bad request|unprocessable|unavailable)\s*:\s*/i;

export const NETWORK_MESSAGE = "Waypoint can't be reached. Check your connection and try again.";

/** The detail of a problem+json body when it is readable by a person, else "". */
export function readableDetail(body) {
  let detail = "";
  try {
    const parsed = JSON.parse(body);
    if (parsed && typeof parsed === "object") detail = String(parsed.detail || parsed.error || parsed.message || "");
  } catch {
    detail = typeof body === "string" ? body : "";
  }
  detail = detail.replace(PREFIX, "").trim();
  if (!detail || detail.length > 200 || INTERNAL.test(detail) || /^\s*[[{]/.test(detail)) return "";
  return detail.charAt(0).toUpperCase() + detail.slice(1) + (/[.!?]$/.test(detail) ? "" : ".");
}

export function friendlyMessage(status, body = "") {
  if (!status) return NETWORK_MESSAGE;
  if (status === 401) return "Your sign-in has ended. Sign in again.";
  if (status === 429) return "Too many requests. Wait a moment and try again.";
  if (status >= 500) return "Something went wrong on our side. Please try again in a moment.";
  const detail = readableDetail(body);
  if (status === 403) return detail || "You don't have permission to do that.";
  if (status === 404) return "We couldn't find that. It may have been removed or moved.";
  if (status === 409) return detail || "This changed while you were working. Reload and try again.";
  return detail || "That wasn't accepted. Check what you entered and try again.";
}

/** Any thrown value to a sentence for the screen. */
export function describeError(error) {
  if (error && typeof error === "object" && "status" in error && typeof error.status === "number") {
    return friendlyMessage(error.status, String(error.message ?? ""));
  }
  const message = error instanceof Error ? error.message : typeof error === "string" ? error : "";
  if (error instanceof TypeError || NETWORK.test(message)) return NETWORK_MESSAGE;
  const trimmed = message.trim();
  if (!trimmed) return "Something went wrong. Please try again.";
  if (/^\s*[[{]/.test(trimmed)) return readableDetail(trimmed) || "Something went wrong. Please try again.";
  if (INTERNAL.test(trimmed) && trimmed.length > 200) return "Something went wrong. Please try again.";
  return trimmed;
}
