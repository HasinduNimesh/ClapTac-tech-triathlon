import { Link } from "react-router-dom";
import { DeliveryPanel } from "./DeliveryPanel";
import { LoadingPanel } from "./LoadingPanel";
import { ReceiptIssuesPanel } from "./ReceiptIssuesPanel";
import { AgentAssistant } from "./AgentAssistant";
import { useLocale } from "../i18n";

export function DispatcherPage() {
  const { t } = useLocale();
  return (
    <>
      <section className="card">
        <h2>{t("Dispatcher")}</h2>
        <p>
          <Link to="/dispatcher/orders">{t("Open the confirmed order queue")}</Link> {t("or")} {" "}
          <Link to="/dispatcher/planning">{t("build the daily plan")}</Link>.
        </p>
      </section>
      <LoadingPanel />
      <DeliveryPanel />
      <ReceiptIssuesPanel />
      <AgentAssistant />
    </>
  );
}
