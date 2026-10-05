import type { CatalogProduct, DraftLine } from "./orderDraft.mjs";
export const MAX_PACKS: number;
export function addProduct(lines: DraftLine[], product: CatalogProduct | undefined, packs: number | string, sourceText?: string): DraftLine[];
export function setPacks(lines: DraftLine[], productId: string, packs: number | string): DraftLine[];
export function removeLine(lines: DraftLine[], productId: string): DraftLine[];
export function totals(lines: DraftLine[]): { units: number; weightKg: number; volumeM3: number };
export function requestLines(lines: DraftLine[]): { productId: string; packQty: number }[];
export function linesSource(lines: DraftLine[]): "form" | "text_helper";
export function pickable(products: CatalogProduct[], temperature: string, lines: DraftLine[]): CatalogProduct[];
