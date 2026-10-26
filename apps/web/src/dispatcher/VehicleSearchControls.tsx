import { useId, useRef } from "react";
import { useLocale } from "../i18n";
import type { VehicleSort } from "./vehicleBrowser.mjs";
import "./vehicleBrowser.css";

export function VehicleSearchControls({ query, onQuery, sort, onSort, count, total }: {
  query: string; onQuery: (query: string) => void;
  sort: VehicleSort; onSort: (sort: VehicleSort) => void;
  count: number; total: number;
}) {
  const { t } = useLocale();
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  return <div className="dp-vehicle-search">
    <div className="dp-vehicle-search-controls">
      <label className="dp-field">{t("Search vehicle number")}
        <input ref={input} type="search" value={query} placeholder={t("Enter all or part of a vehicle number")}
          aria-describedby={`${id}-results`} onChange={(e) => onQuery(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") e.preventDefault(); }} />
      </label>
      <label className="dp-field">{t("Sort vehicles by")}
        <select value={sort} onChange={(e) => onSort(e.target.value as VehicleSort)}>
          <option value="number-asc">{t("Vehicle number: ascending")}</option>
          <option value="number-desc">{t("Vehicle number: descending")}</option>
          <option value="weight-desc">{t("Weight capacity: highest first")}</option>
          <option value="volume-desc">{t("Volume capacity: highest first")}</option>
        </select>
      </label>
      {query && <button type="button" className="dp-btn dp-btn--secondary" onClick={() => { onQuery(""); input.current?.focus(); }}>{t("Clear search")}</button>}
    </div>
    <p id={`${id}-results`} className="muted" role="status">
      {t("{count} of {total} vehicles match").replace("{count}", String(count)).replace("{total}", String(total))}
      {count === 0 && ` · ${t("Try another vehicle number or clear your filters.")}`}
    </p>
  </div>;
}
