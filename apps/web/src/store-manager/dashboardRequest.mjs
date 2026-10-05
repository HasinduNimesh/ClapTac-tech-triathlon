// Reads a plain-words request ("add deferrals, drop arrivals, only chilled") and applies it to a dashboard
// draft. It only chooses and orders cards from the fixed list over the outlet's own data. It is the fallback
// used when the language helper is off, so it has to be predictable on its own.

export const CARD_CATALOGUE = [
  { id: "deadlines", title: "Report-by deadlines", keywords: /deadline|report.?by|claim|before.*close|due/i },
  { id: "receipts", title: "Receipts to confirm", keywords: /receipt|confirm|count/i },
  { id: "short", title: "Short this week", keywords: /short|missing|shortage|damage|broken/i },
  { id: "ontime", title: "On-time arrivals", keywords: /on.?time|late|arriv|window|punctual|delay/i },
  { id: "shortByWeek", title: "Items short by week", keywords: /by week|trend|chart|graph|weekly|history|over time/i },
  { id: "deferrals", title: "Deferrals by reason", keywords: /defer|later run|postpone|reason|not delivered|skipped/i },
  { id: "chilled", title: "Chilled deliveries", keywords: /chill|cold|frozen|dairy|refriger/i },
  { id: "arrivals", title: "Arrivals vs my window", keywords: /arrivals? vs|my window|receiving window|eta|when.*(arrive|coming)|today'?s arrivals/i },
];

// Starting points offered as one-click chips.
export const TEMPLATES = [
  { name: "Receipts and shortages", cards: ["deadlines", "receipts", "short"], filter: "all" },
  { name: "Delivery performance", cards: ["ontime", "arrivals", "deferrals"], filter: "all" },
  { name: "Chilled watch", cards: ["chilled", "receipts", "shortByWeek"], filter: "chilled" },
  { name: "Weekly overview", cards: ["receipts", "short", "ontime", "shortByWeek", "deferrals"], filter: "all" },
];

const ALL_IDS = CARD_CATALOGUE.map((c) => c.id);
const REMOVE = /\b(remove|drop|hide|delete|without|get rid of|take out|stop showing|don'?t (?:show|need|want)|do not (?:show|need|want))\b|^\s*no\s/i;
const FIRST = /\b(first|top|start with|lead with|above)\b/i;
const LAST = /\b(last|bottom|end|below|at the end)\b/i;
const RESET = /\b(start over|start again|clear( it| all| everything)?|reset|empty it|from scratch)\b/i;
const EVERYTHING = /\b(everything|all (?:the )?cards|all of them|full overview|complete overview|the lot)\b/i;

const title = (id) => CARD_CATALOGUE.find((c) => c.id === id).title;

// When a specific phrase matches, the broader card it also contains words of is not what the person meant.
function mentionedCards(text) {
  let ids = CARD_CATALOGUE.filter((c) => c.keywords.test(text)).map((c) => c.id);
  if (ids.includes("shortByWeek")) ids = ids.filter((id) => id !== "short");
  if (ids.includes("arrivals")) ids = ids.filter((id) => id !== "ontime" || /on.?time|late|punctual/i.test(text));
  return ids;
}

// "only chilled", "just the ambient goods", "all goods". Returns the new filter or null if not mentioned.
function goodsFilter(text) {
  if (/\b(only|just)\b[^.]*\bchill|\bchilled (?:goods|orders|items) only\b|\bfor chilled\b/i.test(text)) return "chilled";
  if (/\b(only|just)\b[^.]*\bambient|\bambient (?:goods|orders|items) only\b|\bfor ambient\b/i.test(text)) return "ambient";
  if (/\ball goods\b|\bkeep all\b|\bboth\b|\bchilled and ambient\b|\beverything\b/i.test(text)) return "all";
  return null;
}

function suggestName(cards, filter) {
  const prefix = filter === "chilled" ? "Chilled " : filter === "ambient" ? "Ambient " : "";
  if (cards.includes("receipts") || cards.includes("short") || cards.includes("deadlines")) return `${prefix}Receipts and shortages`.trim();
  if (cards.includes("deferrals")) return `${prefix}Deferrals`.trim();
  if (cards.includes("chilled")) return "Chilled deliveries";
  return `${prefix}Deliveries`.trim();
}

const list = (ids) => ids.map((id) => title(id).toLowerCase()).join(", ");

export function applyRequest(draft, text) {
  const now = new Date().toISOString();
  let cards = [...draft.cards];
  let filter = draft.filter || "all";
  const added = [];
  const removed = [];
  let moved = false;
  let understood = false;

  if (RESET.test(text) && !mentionedCards(text).length) {
    removed.push(...cards);
    cards = [];
    understood = true;
  }
  if (EVERYTHING.test(text) && !REMOVE.test(text)) {
    for (const id of ALL_IDS) if (!cards.includes(id)) { cards.push(id); added.push(id); }
    understood = true;
  }

  // "add deferrals and drop arrivals" is two instructions; each clause carries its own verb.
  const clauses = text.split(/\s*(?:,|;|\bbut\b|\bthen\b|\band also\b|\band\b(?=\s+(?:remove|drop|hide|delete|add|show|put|move|without|only|just)))\s*/i).filter(Boolean);
  for (const clause of clauses) {
    const goods = goodsFilter(clause);
    if (goods) { filter = goods; understood = true; }
    let mentioned = mentionedCards(clause);
    // "only chilled" is about the goods, not a request for the chilled-deliveries card.
    if (goods && goods !== "all" && !/\bdeliver|\bcard\b/i.test(clause)) mentioned = mentioned.filter((id) => id !== "chilled");
    if (!mentioned.length) continue;
    understood = true;
    if (REMOVE.test(clause)) {
      // A bare "arrivals" means both arrival cards when removing.
      if (mentioned.includes("ontime") && !/on.?time|late|punctual/i.test(clause)) mentioned = [...mentioned, "arrivals"];
      for (const id of mentioned) if (cards.includes(id)) { cards = cards.filter((c) => c !== id); removed.push(id); }
      continue;
    }
    for (const id of mentioned) if (!cards.includes(id)) { cards.push(id); added.push(id); }
    if (FIRST.test(clause)) { cards = [...mentioned, ...cards.filter((c) => !mentioned.includes(c))]; moved = true; }
    else if (LAST.test(clause)) { cards = [...cards.filter((c) => !mentioned.includes(c)), ...mentioned]; moved = true; }
  }

  if (!understood) {
    return {
      draft,
      reply: `I did not catch that. I can show ${list(ALL_IDS)}. Try "add deferrals", "remove arrivals", "put receipts first" or "only chilled".`,
    };
  }
  if (!cards.length && !removed.length) cards = ["receipts", "short"];

  const named = !draft.name || draft.name === "New dashboard" ? suggestName(cards, filter) : draft.name;
  const parts = [];
  if (added.length) parts.push(`added ${list(added)}`);
  if (removed.length) parts.push(`removed ${list(removed)}`);
  if (moved) parts.push("changed the order");
  if (filter !== (draft.filter || "all")) parts.push(filter === "all" ? "now covers all goods" : `now covers ${filter} goods only`);
  const summary = parts.length ? parts.join(", ") : "nothing needed to change";
  const reply = cards.length
    ? `Done: ${summary}. "${named}" shows ${list(cards)}. Save it when it looks right, or ask for another change.`
    : `Done: ${summary}. The dashboard is empty. Tell me what to show, for example "receipts to confirm".`;
  return { draft: { ...draft, name: named, cards, filter, updatedAt: now }, reply };
}

export function moveCard(cards, id, direction) {
  const from = cards.indexOf(id);
  const to = from + direction;
  if (from < 0 || to < 0 || to >= cards.length) return cards;
  const next = [...cards];
  [next[from], next[to]] = [next[to], next[from]];
  return next;
}
