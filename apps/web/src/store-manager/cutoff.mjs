// The order cutoff is a policy value a dispatcher can change (Master data), so screens read it from
// the order service instead of stating a time of their own.

const COLOMBO = "Asia/Colombo";

/** "16:00" or "16:00:00" -> minutes after midnight, or undefined when it is not a time. */
export function minutesOfDay(localTime) {
  const match = /^(\d{1,2}):(\d{2})(?::\d{2})?$/.exec(String(localTime ?? "").trim());
  if (!match) return undefined;
  const hours = Number(match[1]);
  const minutes = Number(match[2]);
  return hours < 24 && minutes < 60 ? hours * 60 + minutes : undefined;
}

/** The cutoff as a person reads it in their language: "4:00 PM". Undefined when there is no usable cutoff. */
export function formatCutoff(localTime, locale = "en") {
  const total = minutesOfDay(localTime);
  if (total === undefined) return undefined;
  const tag = locale === "si" ? "si-LK" : locale === "ta" ? "ta-LK" : "en-LK";
  // Format the clock reading itself; UTC keeps the viewer's own time zone out of it.
  return new Intl.DateTimeFormat(tag, { hour: "numeric", minute: "2-digit", hour12: true, timeZone: "UTC" }).format(new Date(Date.UTC(2000, 0, 1, Math.floor(total / 60), total % 60)));
}

/** True once today's cutoff has passed in Sri Lanka. Undefined when the cutoff is not known. */
export function cutoffHasPassed(localTime, now = new Date()) {
  const cutoff = minutesOfDay(localTime);
  if (cutoff === undefined) return undefined;
  const parts = new Intl.DateTimeFormat("en-GB", { timeZone: COLOMBO, hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).formatToParts(now);
  const hour = Number(parts.find((p) => p.type === "hour")?.value);
  const minute = Number(parts.find((p) => p.type === "minute")?.value);
  return hour * 60 + minute >= cutoff;
}

/** Fills the {time} placeholder of a translated sentence. */
export function withTime(sentence, time) {
  return sentence.replace("{time}", time);
}
