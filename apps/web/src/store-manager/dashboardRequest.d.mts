export type CatalogueEntry = { id: "deadlines" | "receipts" | "short" | "ontime" | "shortByWeek" | "deferrals" | "chilled" | "arrivals"; title: string; keywords: RegExp };
export type Template = { name: string; cards: string[]; filter: string };
type Draft = { id: string; name: string; cards: any[]; createdAt: string; updatedAt: string; filter?: any; version?: number };
export const CARD_CATALOGUE: CatalogueEntry[];
export const TEMPLATES: Template[];
export function applyRequest<D extends Draft>(draft: D, text: string): { draft: D; reply: string };
export function moveCard(cards: string[], id: string, direction: number): string[];
