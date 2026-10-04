// Saved store-manager dashboards. A dashboard is an ordered list of cards
// built from the outlet's own order, delivery and receipt data. Dashboards are
// stored per signed-in manager and outlet on this device.

export type CardId = "deadlines" | "receipts" | "short" | "ontime" | "shortByWeek" | "deferrals" | "chilled" | "arrivals";

export type Dashboard = { id: string; name: string; cards: CardId[]; createdAt: string; updatedAt: string; filter?: "all" | "chilled" | "ambient" };

export const CARD_CATALOGUE: { id: CardId; title: string; keywords: RegExp }[] = [
  { id: "deadlines", title: "Report-by deadlines", keywords: /deadline|report.?by|claim|before.*close/i },
  { id: "receipts", title: "Receipts to confirm", keywords: /receipt|confirm|count/i },
  { id: "short", title: "Short this week", keywords: /short|missing|shortage|damage/i },
  { id: "ontime", title: "On-time arrivals", keywords: /on.?time|late|arriv|window|punctual/i },
  { id: "shortByWeek", title: "Items short by week", keywords: /by week|trend|chart|weekly|history/i },
  { id: "deferrals", title: "Deferrals by reason", keywords: /defer|later run|postpone|reason/i },
  { id: "chilled", title: "Chilled deliveries", keywords: /chill|cold|frozen|dairy|refriger/i },
  { id: "arrivals", title: "Arrivals vs my window", keywords: /arrivals? vs|my window|receiving window|eta/i },
];

export const DEFAULT_ID = "overview";

function key(userId: string, outletId: string) { return `sm-dashboards:${userId}:${outletId}`; }

export function loadDashboards(userId: string, outletId: string): Dashboard[] {
  try {
    const raw = window.localStorage.getItem(key(userId, outletId));
    const parsed = raw ? (JSON.parse(raw) as Dashboard[]) : [];
    return Array.isArray(parsed) ? parsed.filter((d) => d && typeof d.id === "string" && Array.isArray(d.cards)) : [];
  } catch {
    return [];
  }
}

export function saveDashboards(userId: string, outletId: string, items: Dashboard[]) {
  try { window.localStorage.setItem(key(userId, outletId), JSON.stringify(items)); } catch { /* the dashboard still shows for this visit */ }
}

export function newDashboardId() { return `dash-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`; }

export type AssistantTurn = { from: "assistant" | "manager"; text: string };

// Turns a plain-words request into a change to the draft. It only chooses and
// orders cards over the outlet's data; it never changes orders or receipts.
export function applyRequest(draft: Dashboard, text: string): { draft: Dashboard; reply: string } {
  const lower = text.toLowerCase();
  let cards = [...draft.cards];
  const mentioned = CARD_CATALOGUE.filter((c) => c.keywords.test(text)).map((c) => c.id);
  const remove = /\b(remove|drop|hide|without|no)\b/i.test(text);
  const first = /\b(first|top|start with|lead with)\b/i.test(text);
  let filter = draft.filter || "all";
  if (/\b(only|just)\b.*\bchill/i.test(lower)) filter = "chilled";
  else if (/\b(only|just)\b.*\bambient/i.test(lower)) filter = "ambient";
  else if (/\ball goods\b|\bkeep all\b|\bboth\b/i.test(lower)) filter = "all";

  if (remove && mentioned.length) {
    cards = cards.filter((c) => !mentioned.includes(c));
  } else if (mentioned.length) {
    for (const id of mentioned) if (!cards.includes(id)) cards.push(id);
    if (first) cards = [...mentioned, ...cards.filter((c) => !mentioned.includes(c))];
  }
  if (!cards.length) cards = ["receipts", "short"];

  const named = !draft.name || draft.name === "New dashboard" ? suggestName(cards) : draft.name;
  const titles = cards.map((c) => CARD_CATALOGUE.find((x) => x.id === c)!.title.toLowerCase());
  const reply = mentioned.length === 0 && !/only|all goods|keep all/i.test(lower)
    ? `I can show ${CARD_CATALOGUE.map((c) => c.title.toLowerCase()).join(", ")}. Which would help?`
    : `Done. "${named}" now shows ${titles.join(", ")}${filter === "all" ? "" : ` for ${filter} goods only`}. Save it when it looks right, or ask for another change.`;
  return { draft: { ...draft, name: named, cards, filter, updatedAt: new Date().toISOString() }, reply };
}

function suggestName(cards: CardId[]) {
  if (cards.includes("receipts") || cards.includes("short") || cards.includes("deadlines")) return "Receipts and shortages";
  if (cards.includes("deferrals")) return "Deferrals";
  if (cards.includes("chilled")) return "Chilled deliveries";
  return "Deliveries";
}
