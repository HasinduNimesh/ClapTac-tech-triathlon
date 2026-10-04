import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useLocale } from "../i18n";
import { DEFAULT_ID, Dashboard } from "./dashboards";

/// Dashboard switcher: the default store overview first, saved dashboards
/// tagged when new, and "Create new dashboard" always last.
export function DashboardMenu({ dashboards, current }: { dashboards: Dashboard[]; current: string }) {
  const { t } = useLocale();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const active = dashboards.find((d) => d.id === current);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent) {
        // Escape closes the menu and hands focus back to the button that opened it.
        if (e.key === "Escape") { setOpen(false); trigger.current?.focus(); }
      } else if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close); document.addEventListener("keydown", close);
    return () => { document.removeEventListener("mousedown", close); document.removeEventListener("keydown", close); };
  }, [open]);
  const go = (id: string) => { setOpen(false); navigate(id === DEFAULT_ID ? "/store-manager" : `/store-manager?dashboard=${encodeURIComponent(id)}`); };
  const fresh = (d: Dashboard) => Date.now() - new Date(d.createdAt).getTime() < 3 * 86_400_000;
  return (
    <div className="sm-dash-menu" ref={ref}>
      <button ref={trigger} type="button" className="sm-dash-trigger" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(!open)}>
        <span className="sm-dash-label">{t("Dashboard")}</span>
        <span className="sm-dash-name">{active ? active.name : t("Store overview")}</span>
        <span aria-hidden="true">▾</span>
      </button>
      {open && (
        <div className="sm-dash-list" role="menu" aria-label={t("Your dashboards")}>
          <p className="sm-dash-heading">{t("Your dashboards")}</p>
          <button type="button" role="menuitemradio" aria-checked={!active} className="sm-dash-item" onClick={() => go(DEFAULT_ID)}>
            <span><strong>{t("Store overview")}</strong><span className="sm-dash-desc">{t("Default · arrivals, attention, recent orders")}</span></span>
            {!active && <span aria-hidden="true">✓</span>}
          </button>
          {dashboards.map((d) => (
            <button key={d.id} type="button" role="menuitemradio" aria-checked={active?.id === d.id} className="sm-dash-item" onClick={() => go(d.id)}>
              <span><strong>{d.name}</strong><span className="sm-dash-desc">{t("Made with chat")} · {d.cards.length} {t("cards")}</span></span>
              {active?.id === d.id ? <span aria-hidden="true">✓</span> : fresh(d) ? <span className="dp-tag dp-tag--green">{t("New")}</span> : null}
            </button>
          ))}
          <Link to="/store-manager/dashboards/new" role="menuitem" className="sm-dash-create" onClick={() => setOpen(false)}>+ {t("Create new dashboard")}</Link>
        </div>
      )}
    </div>
  );
}
