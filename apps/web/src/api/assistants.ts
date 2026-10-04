import { useEffect, useState } from "react";
import { apiJSON } from "./client";
import { helperStatusFrom } from "../store-manager/helperAvailability.mjs";
import type { CatalogProduct, DraftLine, PreviousOrder } from "../store-manager/orderDraft.mjs";

export type OrderQuestion = {
  kind: "choose_product" | "quantity" | "not_in_catalog" | "previous_not_found" | "date";
  sourceText: string;
  question: string;
  options: CatalogProduct[];
  suggestedQuantity: number | null;
};

export type OrderDraftResponse = {
  lines: DraftLine[];
  questions: OrderQuestion[];
  previousOrder: PreviousOrder | null;
  neededBy: string | null;
  products: CatalogProduct[];
  submitted: false;
};

/** Saved-dashboard spec shared with shared-service and the dashboard assistant. */
export type DashboardSpec = { name: string; cards: string[]; filter: "all" | "chilled" | "ambient" };

export type DashboardDraftResponse = {
  draft: DashboardSpec;
  question: string | null;
  reply: string;
  rejected: string[];
  saved: false;
};

export type SavedDashboard = { id: string; spec: DashboardSpec; version: number; createdAt: string; updatedAt: string };

export const draftOrder = (token: string, message: string) =>
  apiJSON<OrderDraftResponse>("/agent/order-assistant/drafts", token, { method: "POST", body: JSON.stringify({ message }) });

export const draftDashboard = (token: string, message: string, draft: DashboardSpec, locale: string) =>
  apiJSON<DashboardDraftResponse>("/agent/dashboard-assistant/drafts", token, { method: "POST", body: JSON.stringify({ message, draft, locale }) });

export const listDashboards = (token: string) => apiJSON<{ items: SavedDashboard[] }>("/shared/dashboards", token);

export const saveDashboard = (token: string, spec: DashboardSpec, existing?: { id: string; version: number }) =>
  apiJSON<{ dashboard: SavedDashboard }>(existing ? `/shared/dashboards/${existing.id}` : "/shared/dashboards", token, {
    method: existing ? "PUT" : "POST",
    body: JSON.stringify(existing ? { spec, version: existing.version } : { spec }),
  });

/** Whether the language helpers are switched on. Screens work the same way without them. */
export function useHelpersAvailable(token: string | undefined) {
  const [available, setAvailable] = useState<{ order: boolean; dashboard: boolean } | null>(null);
  useEffect(() => {
    if (!token) return;
    let live = true;
    apiJSON<{ orderAssistant: boolean; dashboardAssistant: boolean }>("/agent/assistants/status", token)
      .then((s) => { if (live) setAvailable(helperStatusFrom(s)); })
      .catch(() => { if (live) setAvailable({ order: false, dashboard: false }); });
    return () => { live = false; };
  }, [token]);
  return available;
}
