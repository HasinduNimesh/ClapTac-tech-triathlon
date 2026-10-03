import { Link } from "react-router-dom";
import { Order } from "../api/client";
import { todayInSriLanka } from "../api/date.mjs";
import { DEPOT_LABELS, LoadingTripSummary } from "../api/loading";
import { PlanDetail } from "../api/planning";
import { useLocale } from "../i18n";
import { useDepot } from "./DispatcherLayout";
import { Availability, Incident, Outlet, ReceiptIssue, Vehicle, availabilityOf, isRefrigerated, outletMap } from "./types";
import { BRAND_COLORS, Donut, DpHero, Meter, Panel, Stat, StatRow, Tag } from "./ui";
import { clock, dayLabel, isChilled, minutesAgo, pct, useApi } from "./useApi";

type Exception = { key: string; tone: "red" | "amber" | "primary"; icon: string; title: string; text: string; when?: string; to: string };

export function DispatcherPage() {
  const { t } = useLocale();
  const { depot } = useDepot();
  const date = todayInSriLanka();
  const plan = useApi<PlanDetail>(`/planning/plans?date=${date}`);
  const orders = useApi<{ items: Order[] }>("/orders?status=confirmed");
  const outlets = useApi<{ items: Outlet[] }>("/shared/outlets");
  const vehicles = useApi<{ items: Vehicle[] }>("/fleet/vehicles");
  const availability = useApi<{ items: Availability[] }>(`/fleet/availability?date=${date}`);
  const incidents = useApi<{ items: Incident[] }>("/fleet/incidents?openOnly=true");
  const loading = useApi<{ items: LoadingTripSummary[] }>(`/loading/trips?date=${date}`);
  const receipts = useApi<{ items: ReceiptIssue[] }>("/orders/receipt-issues");

  const outletById = outletMap(outlets.data?.items);
  const inDepot = (outletId: string) => !depot || outletById.get(outletId)?.depot === depot;
  const queue = (orders.data?.items || []).filter((o) => inDepot(o.outletId));
  const fleet = (vehicles.data?.items || []).filter((v) => !depot || v.homeDepot === depot);
  const available = fleet.filter((v) => availabilityOf(v.id, availability.data?.items) === "available");
  const trips = (loading.data?.items || []).filter((trip) => !depot || trip.depot === depot);
  const readyTrips = trips.filter((trip) => /ready/i.test(trip.loadingStatus || ""));
  const unallocated = plan.data?.unallocated || [];
  const brandCounts = ["Fresh", "Style", "Tech"].map((brand) => ({ brand, count: queue.filter((o) => o.brand.toLowerCase() === brand.toLowerCase()).length }));
  const chilled = queue.filter((o) => isChilled(o.temperatureRequirement)).length;
  const depotCounts = Object.entries(DEPOT_LABELS).map(([code, label]) => ({ code, label, count: (orders.data?.items || []).filter((o) => outletById.get(o.outletId)?.depot === code).length }));
  const maxDepot = Math.max(1, ...depotCounts.map((d) => d.count));
  const firstArrival = (plan.data?.allocations || []).map((a) => a.plannedArrivalAt).filter(Boolean).sort()[0];

  const exceptions: Exception[] = [];
  const refrigerationShort = unallocated.filter((u) => u.reasonCode === "REFRIGERATION_REQUIRED");
  if (refrigerationShort.length) exceptions.push({ key: "reefer", tone: "red", icon: "!", title: t("Refrigerated capacity at risk"), text: `${refrigerationShort.length} ${t("cooling orders exceed available refrigerated capacity for the next run.")}`, to: "/dispatcher/planning" });
  for (const incident of incidents.data?.items || []) exceptions.push({ key: incident.id, tone: "red", icon: "!", title: `${t("Vehicle in workshop")} · ${incident.vehicleId}`, text: `${t(incident.type)} · ${incident.description}`, when: incident.reportedAt, to: "/dispatcher/live" });
  const windowRisk = unallocated.filter((u) => u.reasonCode === "DELIVERY_WINDOW_CONFLICT");
  if (windowRisk.length) exceptions.push({ key: "window", tone: "amber", icon: "!", title: t("Missed delivery window risk"), text: `${windowRisk.length} ${t("orders may miss their delivery window.")}`, to: "/dispatcher/planning" });
  for (const trip of trips.filter((item) => (item.shortfallCount || 0) > 0)) exceptions.push({ key: `short-${trip.tripId}`, tone: "amber", icon: "!", title: `${t("Loading shortfall")} · ${trip.vehicleId || trip.planRef}`, text: `${trip.shortfallCount} ${t("order(s) short before departure")}`, to: "/dispatcher/notifications" });
  for (const [index, item] of (receipts.data?.items || []).slice(0, 3).entries()) exceptions.push({ key: `receipt-${index}`, tone: "primary", icon: "i", title: `${t("Receipt issue")} · ${item.outletId}`, text: `${item.orderRef} · ${t(item.issue.issueType)} · ${item.issue.affectedUnits} ${t("units")}`, when: item.issue.createdAt, to: "/dispatcher/notifications" });
  if (!plan.data && !plan.loading) exceptions.push({ key: "noplan", tone: "amber", icon: "!", title: t("No plan for today yet"), text: t("Create a plan to allocate confirmed orders to vehicles."), to: "/dispatcher/planning" });

  const groups = new Map<string, { label: string; total: number; free: number; reefer: boolean }>();
  for (const vehicle of fleet) {
    const key = `${vehicle.type}|${vehicle.temp}`;
    const group = groups.get(key) || { label: `${vehicle.type} (${vehicle.temp})`, total: 0, free: 0, reefer: isRefrigerated(vehicle) };
    group.total += 1;
    if (availabilityOf(vehicle.id, availability.data?.items) === "available") group.free += 1;
    groups.set(key, group);
  }

  return (
    <>
      <DpHero title={t("Dispatcher Overview")} subtitle={t("Your daily planning dashboard for a clear view of what’s next, what needs attention, and fleet readiness")}>
        <Link to="/dispatcher/planning" className="dp-btn dp-btn--ghost-light">{t("Review plan")} →</Link>
      </DpHero>
      <StatRow cols={5}>
        <Stat icon="▣" label={t("Next run")} value={`${dayLabel(plan.data?.plan.deliveryDate || date)}${firstArrival ? `, ${clock(firstArrival)}` : ""}`} sub={depot ? `${DEPOT_LABELS[depot]} ${t("depot")}` : t("All depots")} />
        <Stat icon="▤" iconTone="red" label={t("Orders awaiting planning")} value={plan.data ? unallocated.length : queue.length} sub={`${chilled} ${t("chilled")} · ${queue.length - chilled} ${t("ambient")}`} subTone="red" />
        <Stat icon="▰" iconTone="green" label={t("Trips ready")} value={readyTrips.length} sub={`${t("of")} ${trips.length} ${t("trips today")}`} />
        <Stat icon="▱" label={t("Available vehicles")} value={`${available.length} / ${fleet.length}`} sub={`${fleet.length - available.length} ${t("in workshop/other")}`} />
        <Stat icon="!" iconTone="red" variant="red" label={t("Urgent exceptions")} value={exceptions.length} sub={<Link to="/dispatcher/notifications" className="dp-link">{t("View all exceptions")} →</Link>} />
      </StatRow>
      <div className="dp-body">
        <div className="dp-grid-2">
          <Panel title={t("Today's planning snapshot")} sub={t("Key volumes for today's plan across brands, product types and depots.")} actions={<Tag tone="primary">{dayLabel(date)}</Tag>}>
            <div className="dp-grid-3">
              <div className="dp-subcard">
                <h3 className="dp-subcard-title">{t("Orders by brand")}</h3>
                <div className="dp-row">
                  <Donut total={queue.length} label={t("orders")} segments={brandCounts.map((b) => ({ value: b.count, color: BRAND_COLORS[b.brand.toLowerCase()] }))} />
                  <ul className="dp-legend">
                    {brandCounts.map((b) => <li key={b.brand}><span className="dp-swatch" style={{ background: BRAND_COLORS[b.brand.toLowerCase()] }} />{t(b.brand)} <strong>{b.count}</strong> <span className="muted">{pct(b.count, queue.length)}%</span></li>)}
                  </ul>
                </div>
              </div>
              <div className="dp-subcard">
                <h3 className="dp-subcard-title">{t("Cooling-sensitive orders")}</h3>
                <p className="dp-stat-value">❄ {chilled}</p>
                <p className="dp-stat-sub">{pct(chilled, queue.length)}% {t("of all orders")}</p>
                <Meter pct={pct(chilled, queue.length)} tone="cool" ariaLabel={t("Cooling-sensitive orders")} />
              </div>
              <div className="dp-subcard">
                <h3 className="dp-subcard-title">{t("Orders by depot")}</h3>
                <div className="dp-stack">
                  {depotCounts.map((d) => (
                    <div className="dp-hbar" key={d.code}>
                      <span>{d.label}</span>
                      <Meter pct={(d.count / maxDepot) * 100} ariaLabel={`${t("Orders by depot")} ${d.label}`} />
                      <strong>{d.count}</strong>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </Panel>
          <Panel title={t("Urgent exceptions")} className="dp-panel--tinted-red" flush actions={<Link to="/dispatcher/notifications" className="dp-link">{t("View all")} ({exceptions.length}) →</Link>}>
            {exceptions.length === 0 ? <p className="dp-empty">{t("No urgent exceptions right now.")}</p> : (
              <div className="dp-list">
                {exceptions.slice(0, 5).map((ex) => (
                  <Link key={ex.key} to={ex.to} className="dp-list-item" style={{ color: "inherit", textDecoration: "none" }}>
                    <span className={`dp-list-icon${ex.tone === "red" ? "" : ` dp-list-icon--${ex.tone}`}`} aria-hidden="true">{ex.icon}</span>
                    <span className="dp-list-main"><span className="dp-list-title">{ex.title}</span><span className="dp-list-text" style={{ display: "block" }}>{ex.text}</span></span>
                    {ex.when && <span className="dp-list-meta">{minutesAgo(ex.when) ?? "—"} {t("min ago")}</span>}
                  </Link>
                ))}
              </div>
            )}
          </Panel>
        </div>
        <div className="dp-grid-2">
          <Panel title={t("Trips ready to depart")} sub={t("Trips that are planned and ready for dispatch.")} flush actions={<Link to="/dispatcher/live" className="dp-link">{t("View all trips")} →</Link>}>
            {trips.length === 0 ? <p className="dp-empty">{loading.loading ? t("Loading trips…") : t("No trips are planned for today yet.")}</p> : (
              <div className="dp-table-wrap">
                <table className="dp-table">
                  <thead><tr><th>{t("Trip no.")}</th><th>{t("Depot")}</th><th>{t("Vehicle")}</th><th>{t("Stops")}</th><th>{t("Loaded / short / pending")}</th><th>{t("Status")}</th></tr></thead>
                  <tbody>
                    {(readyTrips.length ? readyTrips : trips).slice(0, 6).map((trip) => (
                      <tr key={trip.tripId}>
                        <td><span className="dp-cell-main">{trip.planRef || trip.tripId}</span><span className="dp-cell-sub">{t("Trip")} {trip.tripNumber ?? 1}</span></td>
                        <td>{DEPOT_LABELS[trip.depot || ""] || trip.depot || "—"}</td>
                        <td><span className="dp-cell-main">{trip.vehicleId}</span><span className="dp-cell-sub">{[trip.vehicleType, trip.vehicleTemperatureCapability].filter(Boolean).join(" · ")}</span></td>
                        <td>{trip.stopCount ?? "—"}</td>
                        <td>{trip.loadedCount ?? 0} / {trip.shortfallCount ?? 0} / {trip.pendingCount ?? 0}</td>
                        <td><Tag tone={/ready/i.test(trip.loadingStatus || "") ? "green" : (trip.shortfallCount || 0) > 0 ? "red" : "amber"}>{t(trip.loadingStatus || "pending")}</Tag></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Panel>
          <Panel title={t("Vehicle availability")} sub={t("Current fleet status across vehicle types.")} actions={<Link to="/dispatcher/fleet" className="dp-link">{t("View fleet")} →</Link>}>
            {groups.size === 0 ? <p className="muted">{t("Fleet data is unavailable.")}</p> : (
              <div className="dp-stack">
                {[...groups.values()].map((group) => (
                  <div className="dp-row" key={group.label}>
                    <span className="dp-stat-icon" aria-hidden="true">{group.reefer ? "❄" : "▣"}</span>
                    <div className="dp-spacer">
                      <Meter label={group.label} valueText={`${group.free} / ${group.total} ${t("available")}`} pct={pct(group.free, group.total)} ariaLabel={`${group.label} ${t("available")}`} />
                    </div>
                    <strong>{pct(group.free, group.total)}%</strong>
                  </div>
                ))}
              </div>
            )}
          </Panel>
        </div>
        {(orders.error || vehicles.error) && <p className="dp-note dp-note--amber" role="status">{t("Some overview data could not be loaded. Figures shown may be incomplete.")}</p>}
      </div>
    </>
  );
}
