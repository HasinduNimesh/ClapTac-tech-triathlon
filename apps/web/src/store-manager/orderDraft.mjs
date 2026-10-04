// Turns order lines (from the "paste your order" helper and the manager's own
// answers) into the totals the order form submits. Ambient and chilled goods
// stay separate because they need separate orders.

export function lineFromProduct(product, quantity, sourceText = "") {
  const packs = Math.trunc(Number(quantity));
  if (!product || !Number.isFinite(packs) || packs <= 0 || packs > 999) return null;
  return {
    productId: product.id,
    name: product.name,
    pack: product.pack,
    unitsPerPack: product.unitsPerPack,
    quantity: packs,
    weightKg: Math.round(packs * product.packWeightKg * 100) / 100,
    volumeM3: Math.round(packs * product.packVolumeM3 * 1000) / 1000,
    temperature: product.temperature,
    sourceText,
  };
}

// The form accepts two decimals; round volume up so a small order never becomes 0.
const formKg = (value) => Math.round(value * 100) / 100;
const formM3 = (value) => Math.ceil(Math.round(value * 1000) / 10) / 100;

export function formFills(lines, previousOrder, includePrevious) {
  const groups = new Map();
  const group = (temperature) => {
    if (!groups.has(temperature)) groups.set(temperature, { temperature, orderUnits: 0, weightKg: 0, volumeM3: 0, lines: [], fromOrderRef: "" });
    return groups.get(temperature);
  };
  if (includePrevious && previousOrder && previousOrder.orderUnits > 0) {
    const g = group(previousOrder.temperatureRequirement === "chilled" ? "chilled" : "ambient");
    g.orderUnits += Number(previousOrder.orderUnits) || 0;
    g.weightKg += Number(previousOrder.orderWeightKg) || 0;
    g.volumeM3 += Number(previousOrder.orderVolumeM3) || 0;
    g.fromOrderRef = previousOrder.orderRef || "";
  }
  for (const line of lines || []) {
    const g = group(line.temperature === "chilled" ? "chilled" : "ambient");
    g.orderUnits += line.quantity;
    g.weightKg += line.weightKg;
    g.volumeM3 += line.volumeM3;
    g.lines.push(line);
  }
  return ["ambient", "chilled"]
    .filter((t) => groups.has(t))
    .map((t) => {
      const g = groups.get(t);
      return { ...g, orderWeightKg: formKg(g.weightKg), orderVolumeM3: formM3(g.volumeM3) };
    });
}
