import { useMemo, useState } from "react";
import { useLocale } from "../i18n";
import { browseOrders, orderSortOptions, orderStages, type OrderSort } from "./orderBrowser.mjs";
import { formatDay, statusLabel, statusTone } from "./orderStage.mjs";
import type { Tracking } from "./useOrderTrackings";
import "./orderBrowser.css";

export function OrderBrowserPanel({ rows, selectedId, onSelect, open }: {
  rows: Tracking[];
  selectedId: string;
  onSelect: (id: string) => void;
  open: boolean;
}) {
  const { t } = useLocale();
  const [sort, setSort] = useState<OrderSort>("newest");
  const [stage, setStage] = useState("all");
  const [goods, setGoods] = useState("all");
  const [query, setQuery] = useState("");
  const [controls, setControls] = useState<"sort" | "filter" | null>(null);
  const visible = useMemo(() => browseOrders(rows, { sort, stage, goods, query }), [rows, sort, stage, goods, query]);
  const filterCount = Number(stage !== "all") + Number(goods !== "all");
  const filtered = filterCount > 0 || query.trim() !== "";
  function clearFilters() { setStage("all"); setGoods("all"); setQuery(""); }

  return (
    <aside id="store-order-browser" className="sm-order-browser" aria-labelledby="order-browser-heading" hidden={!open}>
      <div className="sm-order-browser-head">
        <h2 id="order-browser-heading">{t("Your orders")}</h2>
        <p>{t("Choose an order to view its details.")}</p>
      </div>
      <div className="sm-order-browser-controls">
        <label className="sm-order-search">
          <span>{t("Search orders")}</span>
          <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t("Order number")} />
        </label>
        <div className="sm-order-control-buttons">
          <button type="button" aria-expanded={controls === "sort"} aria-controls="order-sort-controls" onClick={() => setControls(controls === "sort" ? null : "sort")}>
            <span aria-hidden="true">↕</span> {t("Sort by")} <span aria-hidden="true">⌄</span>
          </button>
          <button type="button" aria-expanded={controls === "filter"} aria-controls="order-filter-controls" onClick={() => setControls(controls === "filter" ? null : "filter")}>
            {t("Filter by")}{filterCount > 0 && <span className="sm-order-filter-count">{filterCount}</span>} <span aria-hidden="true">⌄</span>
          </button>
        </div>
        <div id="order-sort-controls" className="sm-order-control-fields" hidden={controls !== "sort"}>
          <label htmlFor="order-sort">{t("Sort by")}</label>
          <select id="order-sort" value={sort} onChange={(event) => setSort(event.target.value as OrderSort)}>
            {orderSortOptions.map(([value, label]) => <option key={value} value={value}>{t(label)}</option>)}
          </select>
        </div>
        <div id="order-filter-controls" className="sm-order-control-fields" hidden={controls !== "filter"}>
          <label htmlFor="order-status-filter">{t("Order status")}</label>
          <select id="order-status-filter" value={stage} onChange={(event) => setStage(event.target.value)}>
            <option value="all">{t("All statuses")}</option>
            {orderStages.map((value) => <option key={value} value={value}>{t(statusLabel(value))}{value === "FAILED" ? ` (${t("Failed")})` : ""}</option>)}
          </select>
          <label htmlFor="order-goods-filter">{t("Goods type")}</label>
          <select id="order-goods-filter" value={goods} onChange={(event) => setGoods(event.target.value)}>
            <option value="all">{t("All goods")}</option>
            <option value="chilled">{t("Chilled")}</option>
            <option value="ambient">{t("Ambient")}</option>
          </select>
        </div>
        <div className="sm-order-results-summary">
          <span role="status">{visible.length} / {rows.length} {t("orders")}</span>
          {filtered && <button type="button" className="linklike" onClick={clearFilters}>{t("Clear filters")}</button>}
        </div>
        <p className="sm-order-sort-caption">{t(orderSortOptions.find(([value]) => value === sort)![1])}</p>
        {filtered && !visible.some((row) => row.order.id === selectedId) && <p className="sm-order-filter-note">{t("The order you are viewing is outside these filters.")}</p>}
      </div>
      <div className="sm-order-results" tabIndex={0} role="region" aria-label={t("Order results")}>
        {visible.length === 0 ? (
          <div className="sm-order-no-results">
            <strong>{t("No matching orders")}</strong>
            <p>{t("Try another order number or clear your filters.")}</p>
            <button type="button" onClick={clearFilters}>{t("Clear filters")}</button>
          </div>
        ) : (
          <ul>
            {visible.map(({ order, stage: current }) => (
              <li key={order.id}>
                <button type="button" className={`sm-order-choice sm-order-choice--${statusTone(current)}`} aria-current={order.id === selectedId ? "true" : undefined} onClick={() => onSelect(order.id)}>
                  <span className="sm-order-choice-title"><strong>{order.orderRef}</strong>{order.id === selectedId && <span className="sm-order-selected">{t("Selected")}</span>}</span>
                  <span className="sm-order-choice-tags">
                    <span className={`sm-badge sm-badge--${statusTone(current)}`}>{t(statusLabel(current))}</span>
                    <span className={`sm-order-goods sm-order-goods--${order.temperatureRequirement === "chilled" ? "chilled" : "ambient"}`}>{t(order.temperatureRequirement === "chilled" ? "Chilled" : "Ambient")}</span>
                  </span>
                  <span className="sm-order-choice-meta">{t("Requested")} {formatDay(order.requestedDeliveryDate)} · {order.orderUnits} {t("units")}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </aside>
  );
}
