import { useEffect, useState } from "react";
import { useLocale } from "../i18n";

export function HomePage() {
  const [live, setLive] = useState<string>("checking");
  const { t } = useLocale();

  useEffect(() => {
    fetch("/health/live")
      .then((r) => setLive(r.ok ? "edge live" : "edge not ready"))
      .catch(() => setLive("edge unreachable (start Compose)"));
  }, []);

  return (
    <section className="card">
      <div className="banner">
        {t("Waypoint connects Store Managers, Dispatchers, Loaders, and Drivers from order placement through planning, delivery, and receipt confirmation. Your account profile determines which workspace is available; APIs enforce authorization for every action.")}
      </div>
      <p className={live.startsWith("edge live") ? "status-ok" : "status-bad"}>{t(live)}</p>
      <p>{t("Store Manager places an order → Dispatcher plans → Loader prepares → Driver delivers → receipt confirmed.")}</p>
    </section>
  );
}
