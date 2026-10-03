import { ChangeEvent, useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { apiFetch, Order } from "../api/client";
import { todayInSriLanka } from "../api/date.mjs";
import { DEPOT_LABELS } from "../api/loading";
import { PlanDetail } from "../api/planning";
import { useLocale } from "../i18n";
import { useDepot } from "./DispatcherLayout";
import { Outlet, isVanOnly, outletMap } from "./types";
import { ChipGroup, DpHero, Drawer, Note, Panel, Stat, StatRow, Tag } from "./ui";
import { dateTime, dayLabel, errorText, hhmm, isChilled, kg, m3, useApi, useToken } from "./useApi";

type QueueOrder = Order & { createdAt?: string };
type Filter = "all" | "chilled" | "deferred" | "van";
const PAGE = 10;

export function OrderQueuePage() {
  const { t } = useLocale();
  const token = useToken();
  const { depot: shellDepot } = useDepot();
  const [params] = useSearchParams();
  const [query, setQuery] = useState(params.get("q") || "");
  const [date, setDate] = useState(todayInSriLanka);
  const [brand, setBrand] = useState("");
  const [depot, setDepot] = useState(shellDepot);
  const [status, setStatus] = useState("confirmed");
  const [filter, setFilter] = useState<Filter>("all");
  const [page, setPage] = useState(0);
  const [selected, setSelected] = useState<QueueOrder | null>(null);
  const [error, setError] = useState("");
  const [sourceSystem, setSourceSystem] = useState("erp");
  const [importStatus, setImportStatus] = useState("");
  const [exportUrl, setExportUrl] = useState("");
  const [importing, setImporting] = useState(false);

  useEffect(() => { setDepot(shellDepot); }, [shellDepot]);
  useEffect(() => { setQuery(params.get("q") || ""); }, [params]);
  useEffect(() => { setPage(0); }, [query, date, brand, depot, status, filter]);
  useEffect(() => () => { if (exportUrl) URL.revokeObjectURL(exportUrl); }, [exportUrl]);

  const orderPath = `/orders?${new URLSearchParams({ ...(status ? { status } : {}), ...(brand ? { brand } : {}) })}`;
  const orders = useApi<{ items: QueueOrder[] }>(orderPath);
  const outlets = useApi<{ items: Outlet[] }>("/shared/outlets");
  const plan = useApi<PlanDetail>(`/planning/plans?date=${date}`);
  const outletById = outletMap(outlets.data?.items);
  const history = useMemo(() => new Map((plan.data?.orders || []).map((o) => [o.id, o])), [plan.data]);

  const scoped = (orders.data?.items || []).filter((o) => {
    const outlet = outletById.get(o.outletId);
    if (depot && outlet?.depot !== depot) return false;
    if (date && o.requestedDeliveryDate && o.requestedDeliveryDate > date) return false;
    const q = query.trim().toLowerCase();
    if (q && ![o.orderRef, o.outletId, outlet?.name, outlet?.district].some((v) => v?.toLowerCase().includes(q))) return false;
    return true;
  });
  const deferredCount = (o: QueueOrder) => history.get(o.id)?.outletDeferralCount || 0;
  const counts = {
    all: scoped.length,
    chilled: scoped.filter((o) => isChilled(o.temperatureRequirement)).length,
    deferred: scoped.filter((o) => deferredCount(o) > 0).length,
    van: scoped.filter((o) => isVanOnly(outletById.get(o.outletId))).length,
  };
  const rows = scoped.filter((o) => filter === "all" || (filter === "chilled" && isChilled(o.temperatureRequirement)) || (filter === "deferred" && deferredCount(o) > 0) || (filter === "van" && isVanOnly(outletById.get(o.outletId))))
    .sort((a, b) => (outletById.get(a.outletId)?.windowOpenTime || "").localeCompare(outletById.get(b.outletId)?.windowOpenTime || "") || a.orderRef.localeCompare(b.orderRef));
  const pages = Math.max(1, Math.ceil(rows.length / PAGE));
  const visible = rows.slice(page * PAGE, page * PAGE + PAGE);
  const totalWeight = scoped.reduce((sum, o) => sum + o.orderWeightKg, 0);
  const totalVolume = scoped.reduce((sum, o) => sum + o.orderVolumeM3, 0);

  async function exportCSV() {
    if (!token) return;
    try {
      const response = await apiFetch("/orders/export.csv", token);
      if (!response.ok) throw new Error(await response.text());
      setExportUrl(URL.createObjectURL(await response.blob()));
    } catch (e) { setError(errorText(e)); }
  }

  async function importCSV(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]; if (!file || !token) return;
    setImporting(true); setImportStatus(""); setError("");
    try {
      const query = new URLSearchParams({ version: "1", sourceSystem });
      const response = await apiFetch(`/orders/import.csv?${query}`, token, { method: "POST", headers: { "Content-Type": "text/csv" }, body: await file.text() });
      if (!response.ok) throw new Error(await response.text());
      const result = await response.json() as { created: number; duplicates: number };
      setImportStatus(`${t("Imported")} ${result.created} ${t("new orders;")} ${result.duplicates} ${t("exact replays skipped.")}`);
      await orders.reload();
    } catch (e) { setError(errorText(e)); }
    finally { setImporting(false); event.target.value = ""; }
  }

  const selectedOutlet = selected ? outletById.get(selected.outletId) : undefined;
  const selectedHistory = selected ? history.get(selected.id) : undefined;
  const createdHour = selected?.createdAt ? Number(new Intl.DateTimeFormat("en-GB", { timeZone: "Asia/Colombo", hour: "2-digit", hourCycle: "h23" }).format(new Date(selected.createdAt))) : undefined;

  return (
    <>
      <DpHero title={t("Order queue")} subtitle={t("Confirmed store orders, ready for the next planning run.")} />
      <StatRow cols={4}>
        <Stat label={t("Confirmed orders")} value={scoped.length} sub={`${t("For")} ${dayLabel(date)}`} />
        <Stat label={t("Total weight")} value={kg(totalWeight)} sub={t("Check vehicle weight limits")} />
        <Stat label={t("Total volume")} value={m3(totalVolume)} sub={t("Check usable load space")} />
        <Stat label={t("Previously deferred")} value={counts.deferred} sub={t("Review before planning")} subTone="amber" />
      </StatRow>
      <div className="dp-body">
        <Panel>
          <div className="dp-stack" style={{ paddingTop: 20 }}>
            <div className="dp-filters">
              <label className="dp-field dp-field--wide">{t("Search order or outlet")}<input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("Search by order ID or outlet…")} /></label>
              <label className="dp-field">{t("Needed delivery date")}<input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></label>
              <label className="dp-field">{t("Brand")}<select value={brand} onChange={(e) => setBrand(e.target.value)}><option value="">{t("All brands")}</option><option value="Fresh">{t("Fresh")}</option><option value="Style">{t("Style")}</option><option value="Tech">{t("Tech")}</option></select></label>
              <label className="dp-field">{t("Depot")}<select value={depot} onChange={(e) => setDepot(e.target.value)}><option value="">{t("All depots")}</option>{Object.entries(DEPOT_LABELS).map(([code, label]) => <option key={code} value={code}>{label}</option>)}</select></label>
              <label className="dp-field">{t("Confirmation")}<select value={status} onChange={(e) => setStatus(e.target.value)}><option value="confirmed">{t("Confirmed")}</option><option value="">{t("All statuses")}</option></select></label>
            </div>
            <ChipGroup label={t("Order filters")} value={filter} onChange={setFilter} options={[
              { value: "all", label: `${t("All confirmed")} · ${counts.all}` },
              { value: "chilled", label: `${t("Chilled only")} · ${counts.chilled}` },
              { value: "deferred", label: `${t("Previously deferred")} · ${counts.deferred}` },
              { value: "van", label: `${t("Van-only access")} · ${counts.van}` },
            ]} />
          </div>
        </Panel>
        <Note title={`${t("Next run")}: ${dayLabel(date)} · ${t("Order cutoff")}: 4:00 PM`}>{t("Only confirmed orders submitted by the cutoff enter this run. After-cutoff orders go to the following run.")}</Note>
        {error && <p className="dp-note dp-note--red" role="alert">{error}</p>}
        {orders.error && <p className="dp-note dp-note--red" role="alert">{orders.error}</p>}
        <Panel title={t("Confirmed orders")} sub={t("Select an order to review its items, access and delivery requirements.")} flush actions={<Tag tone="primary">{t("Sort: delivery window")}</Tag>}>
          <div className="dp-table-wrap">
            <table className="dp-table">
              <thead><tr><th>{t("Order")}</th><th>{t("Outlet / brand")}</th><th>{t("Depot")}</th><th>{t("Delivery window")}</th><th>{t("Size")}</th><th>{t("Handling / history")}</th><th>{t("Details")}</th></tr></thead>
              <tbody>
                {orders.loading && <tr><td colSpan={7}>{t("Loading orders…")}</td></tr>}
                {!orders.loading && visible.length === 0 && <tr><td colSpan={7}>{t("No orders match these filters.")}</td></tr>}
                {visible.map((o) => {
                  const outlet = outletById.get(o.outletId);
                  const deferred = deferredCount(o);
                  return (
                    <tr key={o.id} className={selected?.id === o.id ? "is-selected" : deferred > 0 ? "is-alert" : undefined}>
                      <td><span className="dp-cell-main">{o.orderRef}</span><span className="dp-cell-sub">{t(o.status)}</span></td>
                      <td><span className="dp-cell-main">{o.outletId}{outlet?.name ? ` · ${outlet.name}` : ""}</span><span className="dp-cell-sub">{t(o.brand)}{outlet?.district ? ` · ${outlet.district}` : ""}</span></td>
                      <td><span className="dp-cell-main">{DEPOT_LABELS[outlet?.depot || ""] || outlet?.depot || "—"}</span></td>
                      <td><span className="dp-cell-main">{outlet ? `${hhmm(outlet.windowOpenTime)}–${hhmm(outlet.windowCloseTime)}` : "—"}</span><span className="dp-cell-sub">{dayLabel(o.requestedDeliveryDate)}</span></td>
                      <td><span className="dp-cell-main">{kg(o.orderWeightKg)}</span><span className="dp-cell-sub">{m3(o.orderVolumeM3)}</span></td>
                      <td>
                        <Tag tone={isChilled(o.temperatureRequirement) ? "cool" : "primary"}>{isChilled(o.temperatureRequirement) ? `${t("Chilled")} 2–8°C` : t("Ambient")}</Tag>
                        <span className={`dp-cell-sub${deferred > 0 ? " dp-cell-sub--amber" : ""}`}>{deferred > 0 ? `${deferred} ${t("previous deferral(s)")}` : isVanOnly(outlet) ? t("Van-only access") : t("Standard access")}</span>
                      </td>
                      <td><button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" onClick={() => setSelected(o)} aria-label={`${t("View order")} ${o.orderRef}`}>{t("View order")}</button></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          <div className="dp-table-foot">
            <span>{`${t("Showing")} ${visible.length} ${t("of")} ${rows.length} ${t("orders")} · ${t("Orders are assigned in Plan and allocate")}`}</span>
            <div className="dp-row" role="group" aria-label={t("Pages")}>
              <button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" disabled={page === 0} onClick={() => setPage(page - 1)}>{t("Previous")}</button>
              <span>{page + 1} / {pages}</span>
              <button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" disabled={page + 1 >= pages} onClick={() => setPage(page + 1)}>{t("Next")}</button>
            </div>
          </div>
        </Panel>
        <section className="dp-panel" aria-label={t("ERP and WMS data exchange")}>
          <details className="dp-details">
            <summary>{t("ERP and WMS data exchange")}</summary>
            <div className="dp-stack">
              {importStatus && <p role="status" className="dp-note dp-note--green">{importStatus}</p>}
              <div className="dp-row">
                <button type="button" className="dp-btn dp-btn--secondary" onClick={() => void exportCSV()}>{t("Export CSV v1")}</button>
                {exportUrl && <p role="status" className="dp-note dp-note--green">{t("CSV ready.")} <a href={exportUrl} download="waypoint-orders-v1.csv">{t("Download CSV v1")}</a></p>}
              </div>
              <div className="dp-filters">
                <label className="dp-field">{t("Source system")}<input value={sourceSystem} onChange={(e) => setSourceSystem(e.target.value)} maxLength={40} pattern="[A-Za-z0-9_-]+" /></label>
                <label className="dp-field">{t("Import CSV v1")}<input type="file" accept=".csv,text/csv" onChange={(e) => void importCSV(e)} disabled={importing} /></label>
              </div>
              <p className="muted" style={{ margin: 0 }}>{t("CSV v1 requires a source system and external order ID. Imports validate the full file and replaying the same IDs and values is safe.")}</p>
            </div>
          </details>
        </section>
      </div>
      <Drawer open={Boolean(selected)} onClose={() => setSelected(null)} narrow
        eyebrow={selected ? `${selected.orderRef} · ${t(selected.status)}` : undefined}
        title={selected ? `${selected.outletId}${selectedOutlet?.name ? ` · ${selectedOutlet.name}` : ""}` : ""}
        sub={selected ? `${t(selected.brand)}${selectedOutlet ? ` · ${selectedOutlet.district} · ${DEPOT_LABELS[selectedOutlet.depot] || selectedOutlet.depot}` : ""}` : undefined}
        footer={<Link to="/dispatcher/planning" className="dp-btn">{t("Open Plan and allocate")}</Link>}>
        {selected && <>
          <div className="dp-row">
            <Tag tone={isChilled(selected.temperatureRequirement) ? "cool" : "primary"}>{isChilled(selected.temperatureRequirement) ? `${t("Chilled")} 2–8°C` : t("Ambient")}</Tag>
            {(selectedHistory?.outletDeferralCount || 0) > 0 && <Tag tone="amber">{selectedHistory!.outletDeferralCount} {t("previous deferral(s)")}</Tag>}
            {isVanOnly(selectedOutlet) && <Tag>{t("Van-only access")}</Tag>}
          </div>
          {(selectedHistory?.outletDeferralCount || 0) >= 2 && (
            <Note tone="amber" title={`${t("Priority review: deferred on")} ${selectedHistory!.outletDeferralCount} ${t("runs")}`}>
              {selectedHistory?.lastServedAt ? `${t("Last served")} ${dayLabel(selectedHistory.lastServedAt)}. ` : ""}{t("Review this order before allocating fresh demand.")}
            </Note>
          )}
          <dl className="dp-kv dp-kv--plain">
            <div><dt>{t("Delivery date")}</dt><dd>{dayLabel(selected.requestedDeliveryDate)}</dd></div>
            <div><dt>{t("Delivery window")}</dt><dd>{selectedOutlet ? `${hhmm(selectedOutlet.windowOpenTime)}–${hhmm(selectedOutlet.windowCloseTime)}` : "—"}</dd></div>
            <div><dt>{t("Total weight")}</dt><dd>{kg(selected.orderWeightKg)}</dd></div>
            <div><dt>{t("Total volume")}</dt><dd>{m3(selected.orderVolumeM3)}</dd></div>
          </dl>
          <Note tone="primary" title={t("Access & handling")}>
            <span style={{ display: "block", color: "var(--dp-ink)" }}>{selectedOutlet ? `${t(selectedOutlet.dockType || "—")} · ${t(selectedOutlet.parkingConstraint || "—")}` : "—"}</span>
            {isChilled(selected.temperatureRequirement) && <span style={{ display: "block", color: "var(--dp-ink)" }}>{t("Refrigerated vehicle required: maintain 2–8°C")}</span>}
            {selectedOutlet?.mallWindow && <span style={{ display: "block", color: "var(--dp-ink)" }}>{t("Mall receiving window applies")}</span>}
            {selectedOutlet?.accessInstructions && <span style={{ display: "block", color: "var(--dp-ink)" }}>{selectedOutlet.accessInstructions}</span>}
          </Note>
          <h3 className="dp-h3">{t("Order items")}</h3>
          <dl className="dp-kv-rows">
            <div><dt>{t("Units")}</dt><dd>{selected.orderUnits}</dd></div>
            <div><dt>{t("Weight")}</dt><dd>{kg(selected.orderWeightKg)}</dd></div>
            <div><dt>{t("Volume")}</dt><dd>{m3(selected.orderVolumeM3)}</dd></div>
          </dl>
          <h3 className="dp-h3">{t("Confirmation & cutoff")}</h3>
          <p style={{ margin: 0, fontSize: "0.875rem" }}>{selected.createdAt ? `${t("Placed")} ${dateTime(selected.createdAt)} · ${createdHour !== undefined && createdHour < 16 ? t("Before the 4:00 PM cutoff") : t("After the 4:00 PM cutoff · moves to the next run")}` : t("Placement time unavailable")}</p>
          <h3 className="dp-h3">{t("Latest deferral")}</h3>
          <p style={{ margin: 0, fontSize: "0.875rem" }}>{selectedHistory?.lastDeferralDate ? `${dayLabel(selectedHistory.lastDeferralDate)} · ${selectedHistory.deferredLastRun ? t("Deferred on the last run") : t("Earlier deferral")}` : t("No deferral recorded for this outlet.")}</p>
          <Link to={`/dispatcher/deferrals?q=${encodeURIComponent(selected.outletId)}`} className="dp-btn dp-btn--secondary">{t("View deferral history")}</Link>
          <p className="muted" style={{ margin: 0, fontSize: "0.8125rem" }}>{t("Allocation happens in Plan and allocate. Orders remain confirmed until the plan is locked.")}</p>
        </>}
      </Drawer>
    </>
  );
}

