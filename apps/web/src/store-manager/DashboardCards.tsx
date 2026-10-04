import { Link } from "react-router-dom";
import { useLocale } from "../i18n";
import { deferralExplanation } from "./deferralMessage.mjs";
import { CardId, Dashboard } from "./dashboards";
import { colomboDate, colomboTime, formatDay, isDeferred, needsReceipt } from "./orderStage.mjs";
import { Tracking } from "./useOrderTrackings";

// Shortages must be reported within this many hours of delivery so someone
// can still act on them (Figma "report-by deadline").
export const REPORT_WINDOW_HOURS = 48;

const DAY = 86_400_000;
const shortUnits = (row: Tracking) => (row.receiptIssues || []).reduce((s, i) => s + i.affectedUnits, 0)
  || (row.receipt ? Math.max(0, row.receipt.expectedUnits - row.receipt.receivedUnits) : 0);
const deliveredAt = (row: Tracking) => row.delivery?.completedAt || row.delivery?.occurredAt;
export function reportBy(row: Tracking) {
  const at = deliveredAt(row);
  return at ? new Date(new Date(at).getTime() + REPORT_WINDOW_HOURS * 3_600_000) : undefined;
}
function weekStart(d: Date) { const x = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate())); x.setUTCDate(x.getUTCDate() - ((x.getUTCDay() + 6) % 7)); return x; }

export function filterRows(rows: Tracking[], filter: Dashboard["filter"]) {
  if (filter === "chilled") return rows.filter((r) => r.order.temperatureRequirement === "chilled");
  if (filter === "ambient") return rows.filter((r) => r.order.temperatureRequirement !== "chilled");
  return rows;
}

export function DashboardCard({ id, rows }: { id: CardId; rows: Tracking[] }) {
  const { t } = useLocale();
  const now = Date.now();
  switch (id) {
    case "deadlines": {
      const items = rows.filter((r) => r.delivery && (needsReceipt(r.stage) || r.receipt)).slice(0, 5);
      return (
        <section className="dp-panel" aria-label={t("Report-by deadlines")}>
          <div className="dp-panel-head"><h3 className="dp-panel-title" style={{ fontSize: "1rem" }}>{t("Report-by deadlines")}</h3></div>
          <div className="dp-panel-body--flush">
            {items.length === 0 ? <p className="dp-empty">{t("No deliveries waiting for a receipt.")}</p> : items.map((r) => {
              const due = reportBy(r);
              const short = shortUnits(r);
              const open = needsReceipt(r.stage);
              return (
                <div key={r.order.id} className="dp-list-item">
                  <div className="dp-list-main"><p className="dp-list-title" style={{ fontWeight: 500 }}>{r.order.orderRef} · {r.order.temperatureRequirement === "chilled" ? t("Chilled") : t("Ambient")}{short ? ` · ${short} ${t("short of")} ${r.order.orderUnits}` : ""}</p></div>
                  {open ? <><span className="dp-cell-sub--amber" style={{ fontWeight: 600, fontSize: "0.875rem" }}>{due ? `${t("Report by")} ${formatDay(colomboDate(due))}, ${colomboTime(due)}` : t("Opens after delivery")}</span><Link to={`/store-manager/receipts?order=${encodeURIComponent(r.order.id)}`} className="dp-btn dp-btn--sm">{t("Confirm receipt")}</Link></>
                    : <span className="dp-cell-sub--green" style={{ fontWeight: 600, fontSize: "0.875rem" }}>{short ? `${t("Reported")} · ${short} ${t("short")}` : t("Closed · no issues")}</span>}
                </div>
              );
            })}
          </div>
        </section>
      );
    }
    case "receipts": {
      const waiting = rows.filter((r) => needsReceipt(r.stage));
      return <Stat title={t("Receipts to confirm")} value={String(waiting.length)} sub={waiting[0] ? `${waiting[0].order.orderRef}${deliveredAt(waiting[0]) ? `, ${t("delivered")} ${colomboTime(deliveredAt(waiting[0])!)}` : ""}` : t("All receipts confirmed")} />;
    }
    case "short": {
      const week = rows.filter((r) => { const at = deliveredAt(r); return at && now - new Date(at).getTime() < 7 * DAY && shortUnits(r) > 0; });
      const total = week.reduce((s, r) => s + shortUnits(r), 0);
      return <Stat title={t("Short this week")} value={`${total} ${t("items")}`} tone="amber" sub={`${week.length} ${week.length === 1 ? t("order") : t("orders")}`} />;
    }
    case "ontime": {
      const done = rows.filter((r) => r.delivery?.completedAt && r.planning.plannedArrivalAt && now - new Date(r.delivery.completedAt).getTime() < 14 * DAY);
      const onTime = done.filter((r) => new Date(r.delivery!.completedAt!).getTime() <= new Date(r.planning.plannedArrivalAt!).getTime() + 30 * 60_000);
      return <Stat title={t("On-time arrivals")} value={`${onTime.length} ${t("of")} ${done.length}`} tone="green" sub={t("Last 2 weeks · within 30 min of plan")} />;
    }
    case "shortByWeek": {
      const weeks = Array.from({ length: 4 }, (_, i) => weekStart(new Date(now - (3 - i) * 7 * DAY)));
      const values = weeks.map((w) => rows.filter((r) => { const at = deliveredAt(r); if (!at) return false; const d = new Date(at).getTime(); return d >= w.getTime() && d < w.getTime() + 7 * DAY; }).reduce((s, r) => s + shortUnits(r), 0));
      const max = Math.max(1, ...values);
      return (
        <section className="dp-panel" aria-label={t("Items short by week")}>
          <div className="dp-panel-head"><h3 className="dp-panel-title" style={{ fontSize: "1rem" }}>{t("Items short by week")}</h3></div>
          <div className="dp-panel-body">
            <div className="dp-chart" style={{ height: 160, justifyContent: "flex-start" }} role="img" aria-label={values.map((v, i) => `${weeks[i].toISOString().slice(5, 10)}: ${v}`).join(", ")}>
              {values.map((v, i) => (
                <div key={i} className="dp-chart-col" style={{ flex: "0 0 70px" }}>
                  <span className="dp-chart-value">{v}</span>
                  <div className="dp-chart-bar" style={{ height: `${(v / max) * 80 + 2}%`, width: "100%", maxWidth: 56, background: i === values.length - 1 ? "#a45c00" : "#b9c4f6" }} />
                  <span className="dp-chart-label">{formatDay(weeks[i].toISOString().slice(0, 10))}</span>
                </div>
              ))}
            </div>
          </div>
        </section>
      );
    }
    case "deferrals": {
      const deferred = rows.filter((r) => isDeferred(r.stage));
      const reasons = new Map<string, number>();
      for (const r of deferred) { const m = deferralExplanation(r.planning.reasonCode).message; reasons.set(m, (reasons.get(m) || 0) + 1); }
      return (
        <section className="dp-panel" aria-label={t("Deferrals by reason")}>
          <div className="dp-panel-head"><h3 className="dp-panel-title" style={{ fontSize: "1rem" }}>{t("Deferrals by reason")}</h3></div>
          <div className="dp-panel-body">{reasons.size === 0 ? <p className="muted" style={{ margin: 0 }}>{t("No deferred orders.")}</p> : <dl className="dp-kv-rows">{[...reasons.entries()].map(([m, n]) => <div key={m}><dt>{t(m)}</dt><dd>{n}</dd></div>)}</dl>}</div>
        </section>
      );
    }
    case "chilled": {
      const chilled = rows.filter((r) => r.order.temperatureRequirement === "chilled" && r.planning.plannedArrivalAt && colomboDate(r.planning.plannedArrivalAt) === colomboDate(now));
      return <Stat title={t("Chilled deliveries today")} value={String(chilled.length)} tone="cool" sub={chilled[0] ? `${chilled[0].order.orderRef} · ${t("arriving")} ${colomboTime(chilled[0].planning.plannedArrivalAt!)}` : t("None today")} />;
    }
    case "arrivals": {
      const today = rows.filter((r) => r.planning.plannedArrivalAt && colomboDate(r.planning.plannedArrivalAt) === colomboDate(now));
      return (
        <section className="dp-panel" aria-label={t("Arrivals vs my window")}>
          <div className="dp-panel-head"><h3 className="dp-panel-title" style={{ fontSize: "1rem" }}>{t("Arrivals vs my window")}</h3></div>
          <div className="dp-panel-body">{today.length === 0 ? <p className="muted" style={{ margin: 0 }}>{t("No arrivals planned today.")}</p> : <dl className="dp-kv-rows">{today.map((r) => <div key={r.order.id}><dt>{r.order.orderRef}</dt><dd>{colomboTime(r.planning.plannedArrivalAt!)}</dd></div>)}</dl>}</div>
        </section>
      );
    }
  }
}

function Stat({ title, value, sub, tone }: { title: string; value: string; sub: string; tone?: "amber" | "green" | "cool" }) {
  const color = tone === "amber" ? "#a45c00" : tone === "green" ? "#008b52" : tone === "cool" ? "#007f91" : undefined;
  return (
    <section className="dp-panel" aria-label={title}>
      <div className="dp-panel-body" style={{ paddingTop: 20 }}>
        <h3 className="dp-h3">{title}</h3>
        <p className="dp-stat-value" style={{ color, fontSize: "1.75rem", margin: "6px 0" }}>{value}</p>
        <p className="dp-stat-sub">{sub}</p>
      </div>
    </section>
  );
}

const WIDE: CardId[] = ["deadlines", "shortByWeek", "deferrals", "arrivals"];

export function DashboardGrid({ dashboard, rows }: { dashboard: Dashboard; rows: Tracking[] }) {
  const scoped = filterRows(rows, dashboard.filter);
  const groups: CardId[][] = [];
  for (const id of dashboard.cards) {
    const last = groups[groups.length - 1];
    if (!WIDE.includes(id) && last && !WIDE.includes(last[0]) && last.length < 3) last.push(id); else groups.push([id]);
  }
  return (
    <div className="dp-stack">
      {groups.map((g, i) => g.length === 1 && WIDE.includes(g[0])
        ? <DashboardCard key={i} id={g[0]} rows={scoped} />
        : <div key={i} className="dp-grid-3">{g.map((id) => <DashboardCard key={id} id={id} rows={scoped} />)}</div>)}
    </div>
  );
}
