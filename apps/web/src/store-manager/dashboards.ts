// Saved store-manager dashboards. A dashboard is an ordered list of cards
// built from the outlet's own order, delivery and receipt data. Dashboards are
// saved per signed-in manager in shared-service, so they follow the person
// across devices. Building one by description uses the dashboard assistant
// when it is switched on; applyRequest below is the offline fallback.
import { useCallback, useEffect, useState } from "react";
import { DashboardSpec, SavedDashboard, draftDashboard, listDashboards, saveDashboard } from "../api/assistants";
import { useAuth } from "../auth/AuthContext";
import { CARD_CATALOGUE as catalogue, applyRequest as applyDashboardRequest } from "./dashboardRequest.mjs";

export type CardId = "deadlines" | "receipts" | "short" | "ontime" | "shortByWeek" | "deferrals" | "chilled" | "arrivals";

export type Dashboard = { id: string; name: string; cards: CardId[]; createdAt: string; updatedAt: string; filter?: "all" | "chilled" | "ambient"; version?: number };

export const CARD_CATALOGUE: { id: CardId; title: string; keywords: RegExp }[] = catalogue;

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

export const applyRequest: (draft: Dashboard, text: string) => { draft: Dashboard; reply: string } = applyDashboardRequest;
