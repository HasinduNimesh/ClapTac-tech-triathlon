// Saved store-manager dashboards. A dashboard is an ordered list of cards
// built from the outlet's own order, delivery and receipt data. Dashboards are
// saved per signed-in manager in shared-service, so they follow the person
// across devices. Building one by description uses the dashboard assistant
// when it is switched on; applyRequest below is the offline fallback.
import { useCallback, useEffect, useState } from "react";
import { DashboardSpec, SavedDashboard, draftDashboard, listDashboards, saveDashboard } from "../api/assistants";
import { useAuth } from "../auth/AuthContext";

export type CardId = "deadlines" | "receipts" | "short" | "ontime" | "shortByWeek" | "deferrals" | "chilled" | "arrivals";

export type Dashboard = { id: string; name: string; cards: CardId[]; createdAt: string; updatedAt: string; filter?: "all" | "chilled" | "ambient"; version?: number };

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

const fromSaved = (d: SavedDashboard): Dashboard => ({
  id: d.id, name: d.spec.name, cards: d.spec.cards as CardId[], filter: d.spec.filter, createdAt: d.createdAt, updatedAt: d.updatedAt, version: d.version,
});
const toSpec = (d: Dashboard): DashboardSpec => ({ name: d.name.trim(), cards: d.cards, filter: d.filter || "all" });

/** The signed-in manager's saved dashboards from shared-service. */
export function useDashboards() {
  const { user } = useAuth();
  const token = user?.access_token || "";
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);
  const [loaded, setLoaded] = useState(false);
  const reload = useCallback(async () => {
    if (!token) return;
    try { setDashboards(((await listDashboards(token)).items || []).map(fromSaved)); }
    catch { setDashboards([]); }
    finally { setLoaded(true); }
  }, [token]);
  useEffect(() => { void reload(); }, [reload]);
  return { dashboards, loaded, reload };
}

/** Saves a new dashboard or a new version of an existing one; returns the saved copy. */
export async function saveToServer(token: string, draft: Dashboard): Promise<Dashboard> {
  const existing = draft.version ? { id: draft.id, version: draft.version } : undefined;
  return fromSaved((await saveDashboard(token, toSpec(draft), existing)).dashboard);
}

/** Applies a request with the dashboard assistant. Returns null when it is unavailable. */
export async function askAssistant(token: string, draft: Dashboard, text: string, locale: string): Promise<{ draft: Dashboard; reply: string } | null> {
  try {
    const body = await draftDashboard(token, text, toSpec(draft), locale);
    const next = { ...draft, name: body.draft.name, cards: body.draft.cards as CardId[], filter: body.draft.filter, updatedAt: new Date().toISOString() };
    return { draft: next, reply: [body.reply, body.question].filter(Boolean).join(" ") };
  } catch {
    return null;
  }
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
