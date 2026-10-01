import assert from "node:assert/strict";
import test from "node:test";
import { dateInTimeZone, todayInSriLanka } from "../src/api/date.mjs";

test("delivery date follows Sri Lanka calendar day across UTC midnight boundaries", () => {
  assert.equal(todayInSriLanka(Date.parse("2026-09-29T18:29:00Z")), "2026-09-29");
  assert.equal(todayInSriLanka(Date.parse("2026-09-29T18:30:00Z")), "2026-09-30");
});

test("calendar date helper uses the requested time zone", () => {
  const instant = Date.parse("2026-09-29T23:30:00Z");
  assert.equal(dateInTimeZone(instant, "Asia/Colombo"), "2026-09-30");
  assert.equal(dateInTimeZone(instant, "UTC"), "2026-09-29");
});
