import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { apiJSON, Order } from "../api/client";
import { dateInTimeZone, todayInSriLanka } from "../api/date.mjs";
import { DEFER_REASONS } from "../api/planning";
import { DEPOT_LABELS, sameDepot } from "../api/loading";
import { useLocale } from "../i18n";
import { useDepot } from "./DispatcherLayout";
import { AuditEvent, Outlet, outletMap } from "./types";
import { ChipGroup, DpHero, Note, Panel, Stat, StatRow, Tag } from "./ui";
import { clock, dayLabel, errorText, hhmm, isChilled, m3, pct, useApi, useToken } from "./useApi";

type Row = { id: string; at: string; orderId: string; order?: Order; outlet?: Outlet; outletId: string; reason: string; comment?: string; nextRun?: string; actor: string; inRow: number };
type Quick = "all" | "repeat" | "open" | "chilled";
const DELIVERED = /deliver|complete|received/i;
const PAGE = 10;

function daysBefore(date: string, days: number) {
  const d = new Date(`${date}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() - days);
  return d.toISOString().slice(0, 10);
}

export function DeferralHistoryPage() {
  const { t } = useLocale();
  const token = useToken();
  const { depot: shellDepot } = useDepot();
  const [params] = useSearchParams();
  const today = todayInSriLanka();
  const [query, setQuery] = useState(params.get("q") || "");
  const [from, setFrom] = useState(daysBefore(today, 27));
  const [to, setTo] = useState(today);
  const [brand, setBrand] = useState("");
  const [depot, setDepot] = useState(shellDepot);
  const [reasonFilter, setReasonFilter] = useState("");
  const [actor, setActor] = useState("");
  const [quick, setQuick] = useState<Quick>("all");
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [page, setPage] = useState(0);
  const [selectedOutlet, setSelectedOutlet] = useState("");
  const orders = useApi<{ items: Order[] }>("/orders");
  const outlets = useApi<{ items: Outlet[] }>("/shared/outlets");
  useEffect(() => { setDepot(shellDepot); }, [shellDepot]);
  useEffect(() => { setPage(0); }, [query, from, to, brand, depot, reasonFilter, actor, quick]);

  useEffect(() => {
    if (!token) return;
    let active = true;
    setLoading(true);
    (async () => {
      const all: AuditEvent[] = [];
      try {
        for (let offset = 0; offset < 300; offset += 100) {
          const q = new URLSearchParams({ action: "ORDER_DEFERRED", limit: "100", offset: String(offset), from: new Date(`${from}T00:00:00+05:30`).toISOString(), to: new Date(`${to}T23:59:59.999+05:30`).toISOString() });
          const result = await apiJSON<{ items: AuditEvent[]; total: number }>(`/shared/audit/events?${q}`, token);
          all.push(...(result.items || []));
          if (all.length >= result.total || !result.items?.length) break;
        }
        if (active) { setEvents(all); setError(""); }
      } catch (e) { if (active) setError(errorText(e)); }
      finally { if (active) setLoading(false); }
    })();
    return () => { active = false; };
  }, [token, from, to]);

  const ordersById = new Map((orders.data?.items || []).map((o) => [o.id, o]));
  const outletById = outletMap(outlets.data?.items);
  const byOutlet = new Map<string, AuditEvent[]>();
  const rowsAll: Row[] = [...events].sort((a, b) => a.timestamp.localeCompare(b.timestamp)).map((ev) => {
    const order = ordersById.get(ev.resource_id);
    const outletId = order?.outletId || String(ev.new_state?.outletId || "");
    const list = byOutlet.get(outletId) || [];
    list.push(ev); byOutlet.set(outletId, list);
    return { id: ev.event_id, at: ev.timestamp, orderId: ev.resource_id, order, outletId, outlet: outletById.get(outletId), reason: String(ev.new_state?.reasonCode || ev.reason || "MANUAL_DISPATCHER_DEFERRAL"), comment: typeof ev.new_state?.comment === "string" ? ev.new_state.comment : undefined, nextRun: typeof ev.new_state?.nextRunTarget === "string" ? ev.new_state.nextRunTarget : undefined, actor: ev.actor_id || t("System"), inRow: list.length };
  }).reverse();
  const actors = [...new Set(rowsAll.map((r) => r.actor))];
  const rows = rowsAll.filter((r) => {
    const q = query.trim().toLowerCase();
    if (q && ![r.order?.orderRef, r.outletId, r.outlet?.name, r.outlet?.district].some((v) => v?.toLowerCase().includes(q))) return false;
    if (brand && r.order?.brand.toLowerCase() !== brand.toLowerCase()) return false;
    if (depot && !sameDepot(r.outlet?.depot, depot)) return false;
    if (reasonFilter && r.reason !== reasonFilter) return false;
    if (actor && r.actor !== actor) return false;
    if (quick === "repeat" && r.inRow < 2) return false;
    if (quick === "chilled" && !isChilled(r.order?.temperatureRequirement)) return false;
    // Not yet rescheduled: still undelivered and no next run chosen.
    if (quick === "open" && (r.nextRun || DELIVERED.test(r.order?.status || ""))) return false;
    return true;
  });
  const pages = Math.max(1, Math.ceil(rows.length / PAGE));
  const visible = rows.slice(page * PAGE, page * PAGE + PAGE);
  const half = daysBefore(to, 13);
  const recent = rowsAll.filter((r) => dateInTimeZone(r.at, "Asia/Colombo") >= half);
  const earlier = rowsAll.length - recent.length;
  const repeatOutlets = [...byOutlet.entries()].filter(([, list]) => list.length >= 2);
  const reasonCounts = new Map<string, number>();
  for (const r of rowsAll) reasonCounts.set(r.reason, (reasonCounts.get(r.reason) || 0) + 1);
  const topReason = [...reasonCounts.entries()].sort((a, b) => b[1] - a[1])[0];
  const served = rowsAll.filter((r) => /deliver|complete|received/i.test(r.order?.status || "")).length;

  const focusId = selectedOutlet || visible[0]?.outletId || "";
  const focus = outletById.get(focusId);
  const focusRows = rowsAll.filter((r) => r.outletId === focusId);
  const openOrders = (orders.data?.items || []).filter((o) => o.outletId === focusId && /confirm|placed|pending|deferred/i.test(o.status));
  const lastServed = (orders.data?.items || []).filter((o) => o.outletId === focusId && DELIVERED.test(o.status)).map((o) => o.requestedDeliveryDate).sort().slice(-1)[0];
  const daysSinceServed = lastServed ? Math.max(0, Math.round((Date.parse(`${today}T00:00:00Z`) - Date.parse(`${lastServed.slice(0, 10)}T00:00:00Z`)) / 86400000)) : undefined;

  function exportCSV() {
    const header = ["deferred_at", "order", "outlet", "brand", "reason", "decided_by", "in_a_row", "next_run"];
    const lines = rows.map((r) => [r.at, r.order?.orderRef || r.orderId, r.outletId, r.order?.brand || "", r.reason, r.actor, String(r.inRow), r.nextRun || ""].map((v) => `"${v.replace(/"/g, '""')}"`).join(","));
    const url = URL.createObjectURL(new Blob([[header.join(","), ...lines].join("\n")], { type: "text/csv" }));
    const a = document.createElement("a"); a.href = url; a.download = `waypoint-deferrals-${from}-${to}.csv`; a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <>
      <DpHero title={t("Deferral history")} subtitle={t("Every order moved to a later run, with the reason, who decided and when it runs next.")}>
        <button type="button" className="dp-btn dp-btn--ghost-light" onClick={exportCSV}>⤓ {t("Export CSV")}</button>
      </DpHero>
      <StatRow cols={4}>
        <Stat icon="◷" label={t("Deferred orders, last 14 days")} value={recent.length} sub={`${recent.length - earlier >= 0 ? "+" : ""}${recent.length - earlier} ${t("vs. the 14 days before")}`} subTone={recent.length > earlier ? "red" : "green"} />
        <Stat icon="⇄" iconTone="red" label={t("Outlets deferred 2+ times")} value={repeatOutlets.length} sub={t("Give these priority on the next plan")} subTone="red" />
        <Stat icon="▣" iconTone="cool" label={`${t("Top reason")}: ${topReason ? t(topReason[0]) : "—"}`} value={`${topReason ? pct(topReason[1], rowsAll.length) : 0}%`} sub={`${topReason?.[1] || 0} ${t("of")} ${rowsAll.length} ${t("deferrals")}`} />
        <Stat icon="✓" iconTone="green" label={t("Delivered since deferral")} value={`${pct(served, rowsAll.length)}%`} sub={`${served} ${t("of")} ${rowsAll.length} ${t("orders")}`} subTone="green" />
      </StatRow>
      <div className="dp-body">
        <Panel>
          <div className="dp-stack" style={{ paddingTop: 20 }}>
            <div className="dp-filters">
              <label className="dp-field dp-field--wide">{t("Search")}<input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("Outlet ID, name, order or district")} /></label>
              <label className="dp-field">{t("Deferred from")}<input type="date" value={from} max={to} onChange={(e) => setFrom(e.target.value)} /></label>
              <label className="dp-field">{t("Deferred to")}<input type="date" value={to} min={from} max={today} onChange={(e) => setTo(e.target.value)} /></label>
              <label className="dp-field">{t("Brand")}<select value={brand} onChange={(e) => setBrand(e.target.value)}><option value="">{t("All brands")}</option><option value="Fresh">{t("Fresh")}</option><option value="Style">{t("Style")}</option><option value="Tech">{t("Tech")}</option></select></label>
              <label className="dp-field">{t("Depot")}<select value={depot} onChange={(e) => setDepot(e.target.value)}><option value="">{t("All depots")}</option>{Object.entries(DEPOT_LABELS).map(([code, label]) => <option key={code} value={code}>{label}</option>)}</select></label>
              <label className="dp-field">{t("Reason")}<select value={reasonFilter} onChange={(e) => setReasonFilter(e.target.value)}><option value="">{t("All reasons")}</option>{DEFER_REASONS.map((r) => <option key={r} value={r}>{t(r)}</option>)}</select></label>
              <label className="dp-field">{t("Decided by")}<select value={actor} onChange={(e) => setActor(e.target.value)}><option value="">{t("Anyone")}</option>{actors.map((a) => <option key={a} value={a}>{a}</option>)}</select></label>
            </div>
            <div className="dp-row dp-row--between">
              <ChipGroup label={t("Deferral filters")} value={quick} onChange={setQuick} options={[{ value: "all", label: t("All deferrals") }, { value: "repeat", label: t("Repeat deferrals (2+ in a row)") }, { value: "open", label: t("Not yet rescheduled") }, { value: "chilled", label: t("Chilled only") }]} />
              <span className="muted" style={{ fontSize: "0.8125rem" }}>{`${t("Showing")} ${rows.length} ${t("deferrals")}`}</span>
            </div>
          </div>
        </Panel>
        {error && <p className="dp-note dp-note--red" role="alert">{error}</p>}
        <div className="dp-grid-2">
          <Panel title={t("Deferred orders")} sub={t("Newest first. Select a row to see that outlet's deferral history.")} flush actions={<Link to="/dispatcher/audit" className="dp-link">{t("Full audit log")} →</Link>}>
            <div className="dp-table-wrap">
              <table className="dp-table">
                <thead><tr><th>{t("Deferred on")}</th><th>{t("Order")}</th><th>{t("Outlet")}</th><th>{t("Reason")}</th><th>{t("Decided by")}</th><th>{t("In a row")}</th><th>{t("Next run")}</th></tr></thead>
                <tbody>
                  {loading && <tr><td colSpan={7}>{t("Loading deferrals…")}</td></tr>}
                  {!loading && visible.length === 0 && <tr><td colSpan={7}>{t("No deferrals match these filters.")}</td></tr>}
                  {visible.map((r) => (
                    <tr key={r.id} className={`is-clickable${r.outletId === focusId ? " is-selected" : ""}`} onClick={() => setSelectedOutlet(r.outletId)}>
                      <td>{dayLabel(r.at)}</td>
                      <td><span className="dp-cell-main">{r.order?.orderRef || r.orderId}</span><span className="dp-cell-sub">{r.order ? `${isChilled(r.order.temperatureRequirement) ? t("Chilled") : t("Ambient")} · ${m3(r.order.orderVolumeM3)}` : ""}</span></td>
                      <td><button type="button" className="dp-link" onClick={(e) => { e.stopPropagation(); setSelectedOutlet(r.outletId); }}>{r.outletId}{r.outlet?.name ? ` ${r.outlet.name}` : ""}</button><span className="dp-cell-sub">{r.order ? t(r.order.brand) : ""}{r.outlet?.district ? ` · ${r.outlet.district}` : ""}</span></td>
                      <td>{t(r.reason)}{r.comment && <span className="dp-cell-sub">“{r.comment}”</span>}</td>
                      <td><span className="dp-cell-main">{r.actor}</span><span className="dp-cell-sub">{t("at")} {clock(r.at)}</span></td>
                      <td><Tag tone={r.inRow >= 3 ? "red" : r.inRow === 2 ? "amber" : "muted"}>{r.inRow >= 2 ? `${r.inRow} ${t("in a row")}` : t("1st")}</Tag></td>
                      <td><span className="dp-cell-main">{r.nextRun ? dayLabel(r.nextRun) : t("Next available run")}</span><span className={`dp-cell-sub${/deliver|complete/i.test(r.order?.status || "") ? " dp-cell-sub--green" : ""}`}>{r.order ? t(r.order.status) : ""}</span></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="dp-table-foot">
              <span>{`${t("Showing")} ${visible.length} ${t("of")} ${rows.length}`}</span>
              <div className="dp-row" role="group" aria-label={t("Pages")}>
                <button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" disabled={page === 0} onClick={() => setPage(page - 1)}>{t("Previous")}</button>
                <span>{page + 1} / {pages}</span>
                <button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" disabled={page + 1 >= pages} onClick={() => setPage(page + 1)}>{t("Next")}</button>
              </div>
            </div>
          </Panel>
          <Panel title={focusId ? `${focusId}${focus?.name ? ` · ${focus.name}` : ""}` : t("Outlet history")} sub={focus ? `${t(focus.brand)} · ${focus.district} · ${DEPOT_LABELS[focus.depot] || focus.depot}` : t("Select a deferral to see the outlet.")}>
            {focusId && <div className="dp-stack">
              {focusRows.length >= 2 && <Note tone="red" title={`${t("Deferred")} ${focusRows.length} ${t("times in this period")}`}>{t("Repeat deferrals raise this outlet's priority on the next plan. Review it before allocating fresh demand.")}</Note>}
              <dl className="dp-kv">
                <div><dt>{t("Days since last served")}</dt><dd>{daysSinceServed == null ? t("No delivery on record") : `${daysSinceServed} ${t("days")} · ${dayLabel(lastServed!)}`}</dd></div>
                <div><dt>{t("Open orders")}</dt><dd>{openOrders.length} ({openOrders.filter((o) => isChilled(o.temperatureRequirement)).length} {t("chilled")})</dd></div>
                <div><dt>{t("Deferrals in period")}</dt><dd>{focusRows.length}</dd></div>
                <div><dt>{t("Delivery window")}</dt><dd>{focus ? `${hhmm(focus.windowOpenTime)} – ${hhmm(focus.windowCloseTime)}` : "—"}</dd></div>
                <div><dt>{t("Access")}</dt><dd>{focus ? `${t(focus.dockType)} · ${t(focus.parkingConstraint)}` : "—"}</dd></div>
              </dl>
              <h3 className="dp-h3">{t("Deferral timeline")}</h3>
              <ul className="dp-checks">
                {focusRows.map((r) => <li key={r.id}><span className="dp-check dp-check--warn" aria-hidden="true">!</span><span><strong>{dayLabel(r.at)} · {t(r.reason)}</strong><span className="dp-cell-sub">{r.actor} {t("at")} {clock(r.at)} · {r.order?.orderRef || r.orderId}{r.nextRun ? ` · ${t("next run")} ${dayLabel(r.nextRun)}` : ""}</span>{r.comment && <span className="dp-cell-sub">“{r.comment}”</span>}</span></li>)}
              </ul>
              <Link to="/dispatcher/planning" className="dp-btn dp-btn--block">⚑ {t("Review on the next plan")}</Link>
              <Link to={`/dispatcher/orders?q=${encodeURIComponent(focusId)}`} className="dp-btn dp-btn--secondary dp-btn--block">{t("View open orders")}</Link>
            </div>}
          </Panel>
        </div>
      </div>
    </>
  );
}
