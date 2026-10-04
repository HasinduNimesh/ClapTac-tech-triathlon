import { Link } from "react-router-dom";
import { useLocale } from "../i18n";
import { AgentAssistant } from "./AgentAssistant";
import { DpHero, Panel } from "./ui";

export function DispatcherHelpPage() {
  const { t } = useLocale();
  const steps: [string, string, string][] = [
    ["/dispatcher/orders", t("Review the order queue"), t("Confirmed orders after the 4:00 PM cutoff. Chilled and previously deferred orders are marked.")],
    ["/dispatcher/planning", t("Plan and allocate"), t("Generate the plan, fix blocking issues, defer with a reason, then lock and publish.")],
    ["/dispatcher/live", t("Watch live operations"), t("Trips from driver updates. Breakdowns and closing windows come first.")],
    ["/dispatcher/notifications", t("Decide on exceptions"), t("Loader shortfalls, sync conflicts and store receipts that need a decision.")],
    ["/dispatcher/deferrals", t("Check deferral history"), t("Outlets deferred again get priority on the next plan.")],
  ];
  return (
    <>
      <DpHero title={t("Help & Guide")} subtitle={t("How a dispatch day runs in Waypoint, and an assistant for questions.")} />
      <div className="dp-body dp-body--flush">
        <div className="dp-grid-2">
          <Panel title={t("A dispatch day")} flush>
            <ol className="dp-list" style={{ margin: 0, padding: 0, listStyle: "none" }}>
              {steps.map(([to, title, text], index) => (
                <li key={to} className="dp-list-item">
                  <span className="dp-list-icon dp-list-icon--primary" aria-hidden="true">{index + 1}</span>
                  <div className="dp-list-main"><Link to={to} className="dp-list-title" style={{ color: "inherit" }}>{title}</Link><p className="dp-list-text">{text}</p></div>
                </li>
              ))}
            </ol>
          </Panel>
          <div className="dp-panel dp-legacy"><AgentAssistant /></div>
        </div>
      </div>
    </>
  );
}
