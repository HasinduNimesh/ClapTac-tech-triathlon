import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { todayInSriLanka } from "../api/date.mjs";
import { DEPOT_LABELS, sameDepot } from "../api/loading";
import { FuelLedger, PlanDetail } from "../api/planning";
import { useLocale } from "../i18n";
import { useDepot } from "./DispatcherLayout";
import { tripLoads } from "./planModel";
import { Availability, Incident, Vehicle, availabilityOf, isRefrigerated } from "./types";
import { DpHero, Drawer, Meter, Note, Panel, Stat, StatRow, Tag } from "./ui";
import { dateTime, dayLabel, kg, m3, pct, useApi } from "./useApi";

const STEP = 24;

export function FleetPage() {
  const { t } = useLocale();
  const { depot: shellDepot } = useDepot();
  const date = todayInSriLanka();
  const [query, setQuery] = useState("");
  const [depot, setDepot] = useState(shellDepot);
  const [type, setType] = useState("");
  const [state, setState] = useState("");
  const [cooling, setCooling] = useState("");
  const [limit, setLimit] = useState(STEP);
  const [selected, setSelected] = useState<Vehicle | null>(null);
  useEffect(() => { setDepot(shellDepot); }, [shellDepot]);
  const vehicles = useApi<{ items: Vehicle[] }>("/fleet/vehicles");
  const availability = useApi<{ items: Availability[] }>(`/fleet/availability?date=${date}`);
  const incidents = useApi<{ items: Incident[] }>("/fleet/incidents?openOnly=true");
  const ledger = useApi<FuelLedger>(`/fleet/fuel/ledger?weekOf=${date}`);
  const plan = useApi<PlanDetail>(`/planning/plans?date=${date}`);

  const loads = plan.data ? tripLoads(plan.data) : [];
  const tripsFor = (id: string) => loads.filter((l) => l.vehicle?.id === id && l.orderCount > 0);
  const fuelUsed = (id: string) => ledger.data?.items?.find((i) => i.vehicleId === id)?.actualLitersL ?? plan.data?.vehicles?.find((v) => v.id === id)?.weekFuelActualL ?? 0;
  const status = (v: Vehicle) => {
    const raw = availabilityOf(v.id, availability.data?.items);
    if (raw !== "available" || (incidents.data?.items || []).some((i) => i.vehicleId === v.id)) return "workshop";
    return tripsFor(v.id).length ? "assigned" : "available";
  };
  const all = vehicles.data?.items || [];
  const scoped = all.filter((v) => !depot || sameDepot(v.homeDepot, depot));
  const types = [...new Set(all.map((v) => v.type))].sort();
  const rows = scoped.filter((v) => {
    const q = query.trim().toLowerCase();
    if (q && !v.id.toLowerCase().includes(q)) return false;
    if (type && v.type !== type) return false;
    if (state && status(v) !== state) return false;
    if (cooling === "reefer" && !isRefrigerated(v)) return false;
    if (cooling === "ambient" && isRefrigerated(v)) return false;
    return true;
  });
  const lowFuel = scoped.filter((v) => v.weeklyFuelQuotaL > 0 && (v.weeklyFuelQuotaL - fuelUsed(v.id)) / v.weeklyFuelQuotaL < 0.2);
  const available = scoped.filter((v) => status(v) === "available");
  const workshop = scoped.filter((v) => status(v) === "workshop");
  const assigned = scoped.filter((v) => status(v) === "assigned");
  const tag = (s: string) => s === "available" ? <Tag tone="green">{t("Available")}</Tag> : s === "assigned" ? <Tag tone="primary">{t("Assigned")}</Tag> : <Tag tone="red">{t("Workshop")}</Tag>;
  const selectedIncidents = (incidents.data?.items || []).filter((i) => i.vehicleId === selected?.id);
  const selectedEntries = (ledger.data?.entries || []).filter((e) => e.vehicleId === selected?.id);

  return (
    <>
      <DpHero title={t("Fleet")} subtitle={t("Vehicle capability, depot readiness and weekly fuel in one place.")} />
      <StatRow cols={4}>
        <Stat label={t("Fleet vehicles")} value={scoped.length} sub={depot ? DEPOT_LABELS[depot] : t("Across Peliyagoda and Kandy")} />
        <Stat label={t("Available")} value={available.length} sub={`${t("Includes")} ${available.filter(isRefrigerated).length} ${t("refrigerated")}`} subTone="green" />
        <Stat label={t("Assigned / workshop")} value={`${assigned.length} / ${workshop.length}`} sub={t("Workshop vehicles cannot be allocated")} subTone="red" />
        <Stat label={t("Low fuel remaining")} value={lowFuel.length} sub={t("Below 20% of weekly quota")} subTone="amber" />
      </StatRow>
      <div className="dp-body">
        <Panel>
          <div className="dp-filters" style={{ paddingTop: 20 }}>
            <label className="dp-field dp-field--wide">{t("Search vehicle")}<input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("Vehicle ID…")} /></label>
            <label className="dp-field">{t("Depot")}<select value={depot} onChange={(e) => setDepot(e.target.value)}><option value="">{t("All depots")}</option>{Object.entries(DEPOT_LABELS).map(([code, label]) => <option key={code} value={code}>{label}</option>)}</select></label>
            <label className="dp-field">{t("Vehicle type")}<select value={type} onChange={(e) => setType(e.target.value)}><option value="">{t("All types")}</option>{types.map((v) => <option key={v} value={v}>{t(v)}</option>)}</select></label>
            <label className="dp-field">{t("Availability")}<select value={state} onChange={(e) => setState(e.target.value)}><option value="">{t("All states")}</option><option value="available">{t("Available")}</option><option value="assigned">{t("Assigned")}</option><option value="workshop">{t("Workshop")}</option></select></label>
            <label className="dp-field">{t("Cooling")}<select value={cooling} onChange={(e) => setCooling(e.target.value)}><option value="">{t("All capabilities")}</option><option value="reefer">{t("Refrigerated")}</option><option value="ambient">{t("Ambient")}</option></select></label>
          </div>
        </Panel>
        <Note title={t("Allocation checks apply to every vehicle")}>{t("Match depot, cooling, weight, volume and delivery windows. Check weekly fuel and the maximum trips per vehicle set in Master data.")}</Note>
        {vehicles.error && <p className="dp-note dp-note--red" role="alert">{vehicles.error}</p>}
        <div className="dp-row dp-row--between"><h2 className="dp-panel-title">{t("Vehicle inventory")}</h2><span className="muted" style={{ fontSize: "0.8125rem" }}>{t("Week")}: {ledger.data ? `${dayLabel(ledger.data.weekStart)} – ${dayLabel(ledger.data.weekEnd)}` : dayLabel(date)}</span></div>
        {vehicles.loading && <p role="status">{t("Loading fleet…")}</p>}
        <div className="dp-vehicle-grid">
          {rows.slice(0, limit).map((v) => {
            const used = fuelUsed(v.id);
            const remaining = Math.max(0, v.weeklyFuelQuotaL - used);
            const fuelPct = pct(remaining, v.weeklyFuelQuotaL);
            const trips = tripsFor(v.id);
            const s = status(v);
            return (
              <article key={v.id} className="dp-vehicle">
                <div className="dp-row dp-row--between" style={{ alignItems: "flex-start" }}>
                  <div><p className="dp-vehicle-id">{v.id}</p><p className="muted" style={{ margin: 0, fontSize: "0.8125rem" }}>{`${t(v.fuelType)} · ${v.kmPerL} km/L`}</p></div>
                  {tag(s)}
                </div>
                <div className="dp-row dp-row--between" style={{ alignItems: "flex-start" }}>
                  <div><p style={{ margin: 0, fontWeight: 600 }}>{t(v.type)}</p><p className="muted" style={{ margin: 0, fontSize: "0.8125rem" }}>{DEPOT_LABELS[v.homeDepot] || v.homeDepot} {t("depot")}</p></div>
                  <Tag tone={isRefrigerated(v) ? "cool" : "primary"}>{isRefrigerated(v) ? t("Refrigerated") : t("Ambient")}</Tag>
                </div>
                <dl className="dp-kv dp-kv--plain"><div><dt>{t("Weight limit")}</dt><dd>{kg(v.weightCapacityKg)}</dd></div><div><dt>{t("Volume limit")}</dt><dd>{m3(v.volumeCapacityM3)}</dd></div></dl>
                <Meter label={t("Weekly fuel remaining")} valueText={`${Math.round(remaining)} / ${Math.round(v.weeklyFuelQuotaL)} L`} pct={fuelPct} tone={fuelPct < 20 ? "amber" : undefined} ariaLabel={`${t("Weekly fuel remaining")} ${v.id}`} />
                <div className="dp-row dp-row--between">
                  <div><p style={{ margin: 0, fontWeight: 600 }}>{trips.length} {t("of")} 2 {t("trips")}</p><p className="muted" style={{ margin: 0, fontSize: "0.75rem" }}>{s === "workshop" ? t("Unavailable for allocation") : trips.length ? trips.map((l) => `${t("Trip")} ${l.tripNumber}`).join(" + ") : t("No trips assigned")}</p></div>
                  <button type="button" className="dp-btn dp-btn--secondary dp-btn--sm" onClick={() => setSelected(v)} aria-label={`${t("View vehicle")} ${v.id}`}>{t("View vehicle")}</button>
                </div>
              </article>
            );
          })}
        </div>
        {rows.length > limit && <div className="dp-row" style={{ justifyContent: "center" }}><button type="button" className="dp-btn dp-btn--secondary" onClick={() => setLimit(limit + STEP)}>{t("Show more vehicles")} ({rows.length - limit})</button></div>}
        {!vehicles.loading && rows.length === 0 && <p className="dp-empty">{t("No vehicles match these filters.")}</p>}
      </div>
      <Drawer open={Boolean(selected)} onClose={() => setSelected(null)} narrow title={selected?.id || ""} sub={selected ? `${t(selected.type)} · ${DEPOT_LABELS[selected.homeDepot] || selected.homeDepot} ${t("depot")}` : undefined}
        footer={<Link to="/dispatcher/master-data" className="dp-btn dp-btn--secondary">{t("Edit in master data")}</Link>}>
        {selected && <>
          <div className="dp-row">{tag(status(selected))}<Tag tone={isRefrigerated(selected) ? "cool" : "primary"}>{isRefrigerated(selected) ? t("Refrigerated") : t("Ambient")}</Tag></div>
          {status(selected) === "workshop" && <Note tone="red">{t("This vehicle is in the workshop and cannot be allocated until it is released.")}{availability.data?.items.find((a) => a.vehicleId === selected.id)?.reason ? ` ${availability.data.items.find((a) => a.vehicleId === selected.id)!.reason}` : ""}</Note>}
          <dl className="dp-kv">
            <div><dt>{t("Weight limit")}</dt><dd>{kg(selected.weightCapacityKg)}</dd></div>
            <div><dt>{t("Volume limit")}</dt><dd>{m3(selected.volumeCapacityM3)}</dd></div>
            <div><dt>{t("Weekly quota")}</dt><dd>{Math.round(selected.weeklyFuelQuotaL)} L</dd></div>
            <div><dt>{t("Actual this week")}</dt><dd>{fuelUsed(selected.id).toFixed(1)} L</dd></div>
          </dl>
          <h3 className="dp-h3">{t("Trips today")}</h3>
          {tripsFor(selected.id).length === 0 ? <p className="muted" style={{ margin: 0 }}>{t("No trips assigned")}</p> : tripsFor(selected.id).map((l) => <Meter key={l.tripId} label={`${t("Trip")} ${l.tripNumber} · ${l.orderCount} ${t("orders")}`} valueText={`${l.weightPct}% / ${l.volumePct}%`} pct={Math.max(l.weightPct, l.volumePct)} ariaLabel={`${t("Trip")} ${l.tripNumber}`} />)}
          <h3 className="dp-h3">{t("Open vehicle incidents")}</h3>
          {selectedIncidents.length === 0 ? <p className="muted" style={{ margin: 0 }}>{t("No open incidents.")}</p> : <ul className="dp-checks">{selectedIncidents.map((i) => <li key={i.id}><span className="dp-check dp-check--bad" aria-hidden="true">!</span><span>{t(i.type)} · {i.date} · {i.description}</span></li>)}</ul>}
          <h3 className="dp-h3">{t("Fuel entries this week")}</h3>
          {selectedEntries.length === 0 ? <p className="muted" style={{ margin: 0 }}>{t("No fuel entries recorded this week.")}</p> : <dl className="dp-kv-rows">{selectedEntries.map((e) => <div key={e.id}><dt>{e.date} · {e.receiptRef || e.note || "—"}</dt><dd>{e.liters.toFixed(1)} L</dd></div>)}</dl>}
          <p className="muted" style={{ margin: 0, fontSize: "0.75rem" }}>{t("Version")} {selected.version} · {dateTime(new Date().toISOString())}</p>
        </>}
      </Drawer>
    </>
  );
}
