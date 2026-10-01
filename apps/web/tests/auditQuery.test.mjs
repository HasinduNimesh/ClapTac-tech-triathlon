import assert from "node:assert/strict";
import test from "node:test";
import { buildAuditSearchParams } from "../src/dispatcher/auditQuery.mjs";

test("audit query includes normalized text, actor/resource filters, date range, and paging", () => {
  const params = buildAuditSearchParams({
    query: "  ORD-12 ", action: "ORDER_DEFERRED", resourceType: "ORDER", resourceId: " ORD-12 ", actorId: "USR-2",
    from: "2026-09-30T00:00", to: "2026-09-30T23:59",
  }, 50);
  assert.equal(params.get("q"), "ORD-12");
  assert.equal(params.get("action"), "ORDER_DEFERRED");
  assert.equal(params.get("resourceType"), "ORDER");
  assert.equal(params.get("resourceId"), "ORD-12");
  assert.equal(params.get("actorId"), "USR-2");
  assert.equal(params.get("from"), "2026-09-29T18:30:00.000Z");
  assert.equal(params.get("to"), "2026-09-30T18:29:59.999Z");
  assert.equal(params.get("offset"), "50");
});

test("blank filters are omitted and malformed local timestamps are rejected", () => {
  const params = buildAuditSearchParams({ query: " ", action: "", resourceType: "", resourceId: "", actorId: "", from: "", to: "" });
  assert.equal(params.has("q"), false);
  assert.equal(params.has("from"), false);
  assert.throws(() => buildAuditSearchParams({ query:"", action:"", resourceType:"", resourceId:"", actorId:"", from:"not-a-date", to:"" }), /valid date and time/);
  assert.throws(() => buildAuditSearchParams({ query:"", action:"", resourceType:"", resourceId:"", actorId:"", from:"2026-09-30T12:00", to:"2026-09-30T11:00" }), /To must be after From/);
});
