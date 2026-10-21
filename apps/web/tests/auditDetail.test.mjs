import test from "node:test";
import assert from "node:assert/strict";
import { auditDetail } from "../src/dispatcher/auditDetail.mjs";
import { shortfallText } from "../src/store-manager/shortfallText.mjs";

test("an audit row's state reads as words, not JSON", () => {
  assert.equal(auditDetail({ new_state: { affectedUnits: 1, issueType: "MISSING" } }), "Units affected: 1 · Reason: missing");
  assert.equal(auditDetail({ new_state: { expectedUnits: 5, receivedUnits: 4, status: "confirmed_with_issue" } }), "Expected units: 5 · Received units: 4 · Status: confirmed with issue");
  assert.equal(auditDetail({ new_state: { runId: "7ee95863-9ffa-42b1-83db-96ca3cdb5ece" } }), "Run: 7ee95863");
});

test("a reason wins, and an empty state is a dash", () => {
  assert.equal(auditDetail({ reason: "Vehicle broke down", new_state: { a: 1 } }), "Vehicle broke down");
  assert.equal(auditDetail({}), "—");
  assert.equal(auditDetail({ new_state: {} }), "—");
});

test("unknown fields get readable labels and nothing prints braces", () => {
  const text = auditDetail({ new_state: { someNewField: true, items: [1, 2], nested: { a: 1 } } });
  assert.equal(text, "Some new field: yes · Items: 2 items");
  assert.doesNotMatch(text, /[{}"]/);
});

test("a store sees a shortfall as a sentence", () => {
  assert.equal(shortfallText([{ type: "MISSING", affectedUnits: 1, note: "one box missing" }]), "1 unit missing · one box missing");
  assert.equal(shortfallText([{ type: "DAMAGED", affectedUnits: 3 }, { type: "MISSING", affectedUnits: 2 }]), "3 units damaged; 2 units missing");
  assert.equal(shortfallText(undefined), "");
});

test("a time in the state is written as a time, not as ISO text", () => {
  assert.equal(auditDetail({ new_state: { scheduledAt: "2026-10-20T16:35:00Z", status: "completed" } }), "Scheduled at: 20 Oct, 22:05 · Status: completed");
  assert.equal(auditDetail({ new_state: { note: "2026-13-45T99:99:99Z" } }), "Note: 2026-13-45T99:99:99Z");
});
