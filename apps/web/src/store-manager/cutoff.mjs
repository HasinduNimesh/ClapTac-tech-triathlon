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

/** True once cutoff has passed in Sri Lanka, using an explicitly supplied server-derived time. */
export function cutoffHasPassed(localTime, now) {
  const cutoff = minutesOfDay(localTime);
  if (cutoff === undefined || !(now instanceof Date) || !Number.isFinite(now.getTime())) return undefined;
  const parts = new Intl.DateTimeFormat("en-GB", { timeZone: COLOMBO, hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).formatToParts(now);
  const hour = Number(parts.find((p) => p.type === "hour")?.value);
  const minute = Number(parts.find((p) => p.type === "minute")?.value);
  return hour * 60 + minute >= cutoff;
}

/** Advance a fresh server timestamp with monotonic elapsed time, never the device wall clock. */
export function serverTimeAt(serverTime, receivedAt, monotonicNow) {
  // An explicit offset is required: a timezone-less timestamp would use the device's timezone.
  if (typeof serverTime !== "string" || !/T.*(?:Z|[+-]\d{2}:\d{2})$/i.test(serverTime)) return undefined;
  const timestamp = Date.parse(serverTime);
  const elapsed = monotonicNow - receivedAt;
  // Do not extrapolate through a prolonged network outage or an invalid monotonic sample.
  if (!Number.isFinite(timestamp) || !Number.isFinite(receivedAt) || !Number.isFinite(monotonicNow) || elapsed < 0 || elapsed > 90_000) return undefined;
  return new Date(timestamp + elapsed);
}

/** Fills the {time} placeholder of a translated sentence. */
export function withTime(sentence, time) {
  return sentence.replace("{time}", time);
}
