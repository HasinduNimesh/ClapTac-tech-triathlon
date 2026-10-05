// The item list on the order form. The server is the authority on weight and volume (it prices every line from
// the catalog), so these totals are only the preview the manager sees while building the order.
import { lineFromProduct } from "./orderDraft.mjs";

export const MAX_PACKS = 999;

/** Adds packs of a product, merging with an existing line. Returns the new list, or the same list if refused. */
export function addProduct(lines, product, packs, sourceText = "") {
  const wanted = Math.trunc(Number(packs));
  if (!product || !Number.isFinite(wanted) || wanted <= 0) return lines;
  const existing = lines.find((l) => l.productId === product.id);
  const total = Math.min(MAX_PACKS, (existing ? existing.quantity : 0) + wanted);
  const base = lineFromProduct(product, total, existing ? existing.sourceText : sourceText);
  if (!base) return lines;
  // Same precision as the server (kg to 3 places, m³ to 4), so the preview matches what is saved.
  const line = { ...base, weightKg: Math.round(total * product.packWeightKg * 1000) / 1000, volumeM3: Math.round(total * product.packVolumeM3 * 10000) / 10000 };
  return existing ? lines.map((l) => (l.productId === product.id ? line : l)) : [...lines, line];
}

/** Sets the packs of one line; zero or less removes it. Weight and volume are scaled from the line's own rate. */
export function setPacks(lines, productId, packs) {
  const wanted = Math.trunc(Number(packs));
  if (!Number.isFinite(wanted) || wanted <= 0) return removeLine(lines, productId);
  const quantity = Math.min(MAX_PACKS, wanted);
  return lines.map((l) => {
    if (l.productId !== productId) return l;
    const perPackKg = l.weightKg / l.quantity;
    const perPackM3 = l.volumeM3 / l.quantity;
    return { ...l, quantity, weightKg: Math.round(quantity * perPackKg * 1000) / 1000, volumeM3: Math.round(quantity * perPackM3 * 10000) / 10000 };
  });
}

export function removeLine(lines, productId) {
  return lines.filter((l) => l.productId !== productId);
}

export function totals(lines) {
  const units = lines.reduce((sum, l) => sum + l.quantity, 0);
  const weight = lines.reduce((sum, l) => sum + l.weightKg, 0);
  const volume = lines.reduce((sum, l) => sum + l.volumeM3, 0);
  return {
    units,
    weightKg: Math.round(weight * 1000) / 1000,
    // Round up like the server, so a small order never shows less space than it will be given.
    volumeM3: Math.ceil(Math.round(volume * 10000) / 10) / 1000,
  };
}

/** What the server accepts: product and packs only. Prices, names and weights are never sent. */
export function requestLines(lines) {
  return lines.map((l) => ({ productId: l.productId, packQty: l.quantity }));
}

/** Where the lines came from, for the audit trail: the text helper if any line was read from text. */
export function linesSource(lines) {
  return lines.some((l) => l.sourceText) ? "text_helper" : "form";
}

/** Products the manager can add right now: the chosen goods type, and not already at the pack limit. */
export function pickable(products, temperature, lines) {
  return products.filter((p) => p.temperature === temperature && !(lines.find((l) => l.productId === p.id)?.quantity >= MAX_PACKS));
}

/** Turns the server's suggestion into form lines, using the products the manager can already pick from. */
export function suggestionLines(products, suggestion) {
  let lines = [];
  const reasons = {};
  for (const s of suggestion?.lines || []) {
    const product = products.find((p) => p.id === s.productId);
    if (!product) continue;
    lines = addProduct(lines, product, s.packs);
    reasons[s.productId] = s;
  }
  return { lines, reasons };
}

const SEASONS = { avurudu: "Sinhala and Tamil New Year", christmas: "Christmas season" };

/** One plain sentence saying why a quantity was suggested, in the user's language via the translate function. */
export function reasonSentence(line, coverDays, t) {
  const rate = line.perDay >= 10 ? String(Math.round(line.perDay)) : line.perDay.toFixed(1);
  let text = t(line.basis === "history" ? "About {rate} boxes a day from your last orders" : "About {rate} boxes a day from the store's usual sales").replace("{rate}", rate);
  if (coverDays > 1) text += `, ${t("for {days} days").replace("{days}", String(coverDays))}`;
  if (line.upliftPercent > 0) text += `, ${t("{percent}% more for {season}").replace("{percent}", String(line.upliftPercent)).replace("{season}", t(SEASONS[line.season] || "the season"))}`;
  return text;
}
