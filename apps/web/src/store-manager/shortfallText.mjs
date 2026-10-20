const KINDS = { MISSING: "missing", DAMAGED: "damaged", WRONG_ITEM: "wrong item", WRONG: "wrong item" };

/** A loading shortfall as a sentence for the store, never the record it came from. */
export function shortfallText(summary = [], t = (s) => s) {
  const list = Array.isArray(summary) ? summary : [];
  return list
    .map((s) => {
      const units = Number(s?.affectedUnits ?? 0);
      const kind = KINDS[String(s?.type || "MISSING").toUpperCase()] || String(s?.type || "").toLowerCase().replace(/_/g, " ") || "missing";
      const base = `${units} ${t(units === 1 ? "unit" : "units")} ${t(kind)}`;
      return s?.note ? `${base} · ${s.note}` : base;
    })
    .join("; ");
}
