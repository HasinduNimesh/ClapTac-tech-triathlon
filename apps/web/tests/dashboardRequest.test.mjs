import test from "node:test";
import assert from "node:assert/strict";
import { applyRequest, moveCard, TEMPLATES, CARD_CATALOGUE } from "../src/store-manager/dashboardRequest.mjs";

const empty = { id: "d", name: "New dashboard", cards: [], filter: "all", createdAt: "", updatedAt: "" };
const withCards = (...cards) => ({ ...empty, name: "Mine", cards });

test("a first request adds the cards it names and suggests a name", () => {
  const r = applyRequest(empty, "Shortages this week");
  assert.deepEqual(r.draft.cards, ["short"]);
  assert.equal(r.draft.name, "Receipts and shortages");
});

test("add and remove in one sentence are handled clause by clause", () => {
  const r = applyRequest(withCards("receipts", "arrivals"), "add deferrals and remove arrivals");
  assert.deepEqual(r.draft.cards, ["receipts", "deferrals"]);
  assert.match(r.reply, /added deferrals by reason/);
  assert.match(r.reply, /removed arrivals vs my window/);
});

test("'no' inside a sentence is not a removal, but a leading 'no' is", () => {
  assert.deepEqual(applyRequest(withCards("receipts"), "show me anything with no stock, deferrals").draft.cards, ["receipts", "deferrals"]);
  assert.deepEqual(applyRequest(withCards("receipts", "short"), "no shortages").draft.cards, ["receipts"]);
});

test("specific phrases win over the broad card they contain words of", () => {
  assert.deepEqual(applyRequest(empty, "shortages by week").draft.cards, ["shortByWeek"]);
  assert.deepEqual(applyRequest(empty, "arrivals vs my window").draft.cards, ["arrivals"]);
  assert.deepEqual(applyRequest(empty, "on-time arrivals").draft.cards, ["ontime"]);
});

test("first and last move the named cards", () => {
  assert.deepEqual(applyRequest(withCards("receipts", "short", "deferrals"), "put deferrals first").draft.cards, ["deferrals", "receipts", "short"]);
  assert.deepEqual(applyRequest(withCards("receipts", "short", "deferrals"), "move receipts to the end").draft.cards, ["short", "deferrals", "receipts"]);
});

test("'only chilled' changes the goods filter and does not add the chilled card", () => {
  const r = applyRequest(withCards("receipts"), "only chilled goods");
  assert.equal(r.draft.filter, "chilled");
  assert.deepEqual(r.draft.cards, ["receipts"]);
  assert.match(r.reply, /chilled goods only/);
  assert.equal(applyRequest(r.draft, "all goods").draft.filter, "all");
  assert.deepEqual(applyRequest(empty, "chilled deliveries").draft.cards, ["chilled"]);
});

test("everything adds every card once; start over empties the dashboard", () => {
  const all = applyRequest(withCards("receipts"), "show everything").draft;
  assert.equal(all.cards.length, CARD_CATALOGUE.length);
  assert.equal(new Set(all.cards).size, CARD_CATALOGUE.length);
  const cleared = applyRequest(all, "start over");
  assert.deepEqual(cleared.draft.cards, []);
  assert.match(cleared.reply, /empty/);
});

test("an unclear request changes nothing and says what to try", () => {
  const draft = withCards("receipts");
  const r = applyRequest(draft, "make it pretty please");
  assert.equal(r.draft, draft);
  assert.match(r.reply, /did not catch that/);
});

test("removing the last card leaves it empty instead of silently re-adding defaults", () => {
  const r = applyRequest(withCards("receipts"), "remove receipts");
  assert.deepEqual(r.draft.cards, []);
});

test("templates only use known cards and filters", () => {
  const ids = new Set(CARD_CATALOGUE.map((c) => c.id));
  for (const t of TEMPLATES) {
    assert.ok(t.cards.length > 0 && t.cards.every((c) => ids.has(c)));
    assert.ok(["all", "chilled", "ambient"].includes(t.filter));
    assert.equal(new Set(t.cards).size, t.cards.length);
  }
});

test("moveCard swaps neighbours and ignores the ends", () => {
  assert.deepEqual(moveCard(["a", "b", "c"], "b", -1), ["b", "a", "c"]);
  assert.deepEqual(moveCard(["a", "b", "c"], "b", 1), ["a", "c", "b"]);
  assert.deepEqual(moveCard(["a", "b", "c"], "a", -1), ["a", "b", "c"]);
  assert.deepEqual(moveCard(["a", "b", "c"], "x", 1), ["a", "b", "c"]);
});
