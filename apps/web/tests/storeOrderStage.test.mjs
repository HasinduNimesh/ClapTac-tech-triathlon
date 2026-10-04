import test from "node:test";
import assert from "node:assert/strict";
import {
  arrivesOn, colomboDate, colomboTime, formatDay, needsReceipt, sortByArrival, statusLabel, statusTone, timelineSteps,
} from "../src/store-manager/orderStage.mjs";

test("stage labels and tones follow the backend tracking stages", () => {
  assert.equal(statusLabel("OUT_FOR_DELIVERY"), "On route");
  assert.equal(statusLabel("RECEIPT_CONFIRMED_WITH_ISSUE"), "Receipt confirmed with issue");
  assert.equal(statusTone("DEFERRED"), "deferred");
  assert.equal(statusTone("DELIVERED"), "delivered");
  assert.equal(statusLabel("REFUSED"), "Rejected delivery");
  assert.equal(statusTone("REFUSED"), "deferred");
  assert.equal(statusTone("CONFIRMED"), "default");
});

test("receipt is only requested after a delivered or partial outcome", () => {
  assert.equal(needsReceipt("DELIVERED"), true);
  assert.equal(needsReceipt("PARTIAL"), true);
  assert.equal(needsReceipt("RECEIPT_CONFIRMED"), false);
  assert.equal(needsReceipt("OUT_FOR_DELIVERY"), false);
});

test("arrival day uses the Asia/Colombo calendar, not UTC", () => {
  // 19:00 UTC on 29 Sep is 00:30 on 30 Sep in Colombo.
  const late = "2026-09-29T19:00:00Z";
  assert.equal(colomboDate(late), "2026-09-30");
  assert.equal(colomboTime(late), "00:30");
  const tracking = { stage: "OUT_FOR_DELIVERY", planning: { plannedArrivalAt: late }, order: { requestedDeliveryDate: "2026-09-29" } };
  assert.equal(arrivesOn(tracking, "2026-09-30"), true);
  assert.equal(arrivesOn(tracking, "2026-09-29"), false);
});

test("only planned or active-run orders can arrive today", () => {
  const base = { planning: {}, order: { requestedDeliveryDate: "2026-09-30" } };
  assert.equal(arrivesOn({ ...base, stage: "PLANNED" }, "2026-09-30"), true);
  assert.equal(arrivesOn({ ...base, stage: "CONFIRMED" }, "2026-09-30"), false);
  assert.equal(arrivesOn({ ...base, stage: "DEFERRED" }, "2026-09-30"), false);
  assert.equal(arrivesOn({ ...base, stage: "DELIVERED" }, "2026-09-30"), false);
});

test("orders sort by earliest planned arrival with unplanned last", () => {
  const rows = [
    { id: "c", planning: {} },
    { id: "b", planning: { plannedArrivalAt: "2026-09-30T05:00:00Z" } },
    { id: "a", planning: { plannedArrivalAt: "2026-09-30T01:00:00Z" } },
  ];
  assert.deepEqual(rows.sort(sortByArrival).map((row) => row.id), ["a", "b", "c"]);
});

test("timeline marks progress and surfaces deferral and failed delivery", () => {
  const states = (stage) => timelineSteps({ stage }).map((step) => step.state).join(",");
  assert.equal(states("CONFIRMED"), "done,current,upcoming,upcoming,upcoming");
  assert.equal(states("OUT_FOR_DELIVERY"), "done,done,done,current,upcoming");
  assert.equal(states("RECEIPT_CONFIRMED"), "done,done,done,done,done");
  assert.equal(timelineSteps({ stage: "DEFERRED" })[1].warn, true);
  const failed = timelineSteps({ stage: "NOT_DELIVERED" });
  assert.equal(failed.length, 4);
  assert.equal(failed[3].label, "Not delivered");
  const refused = timelineSteps({ stage: "REFUSED" });
  assert.equal(refused.length, 4);
  assert.equal(refused[3].label, "Rejected delivery");
  assert.equal(refused[3].warn, true);
});

test("day labels are formatted from date-only strings without timezone drift", () => {
  assert.match(formatDay("2026-09-29"), /^Tue 29 Sep/);
  assert.equal(formatDay(""), "—");
});
