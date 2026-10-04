import { createContext, FormEvent, useContext, useEffect, useState } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { DEPOT_LABELS } from "../api/loading";
import { todayInSriLanka } from "../api/date.mjs";
import { useApi } from "./useApi";
import logo from "../assets/login/logo.png";
import iconOverview from "../assets/dispatcher/icon-overview.svg";
import iconQueue from "../assets/dispatcher/icon-queue.svg";
import iconRoute from "../assets/dispatcher/icon-route.svg";
import iconTruck from "../assets/dispatcher/icon-truck.svg";
import iconFleet from "../assets/dispatcher/icon-fleet.svg";
import iconHistory from "../assets/dispatcher/icon-history.svg";
import iconChart from "../assets/dispatcher/icon-chart.svg";
import iconBell from "../assets/store-manager/icon-bell.svg";
import iconSetting from "../assets/store-manager/icon-setting.svg";
import iconHelp from "../assets/store-manager/icon-help.svg";
import iconUser from "../assets/store-manager/icon-user.svg";
import "./dispatcher.css";

type DepotState = { depot: string; setDepot: (depot: string) => void };
const DepotContext = createContext<DepotState>({ depot: "", setDepot: () => undefined });
export function useDepot() { return useContext(DepotContext); }

const DEPOT_KEY = "waypoint.dispatcher.depot";

export function DispatcherLayout() {
  const { profile, logout } = useAuth();
  const { t } = useLocale();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const [navOpen, setNavOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [depot, setDepotState] = useState(() => { try { return localStorage.getItem(DEPOT_KEY) ?? "DEPOT_NORTH"; } catch { return "DEPOT_NORTH"; } });
  const setDepot = (next: string) => { setDepotState(next); try { localStorage.setItem(DEPOT_KEY, next); } catch { /* storage is optional */ } };
  const today = todayInSriLanka();
  const orders = useApi<{ items: { status: string }[] }>("/orders?status=confirmed");
  const incidents = useApi<{ items: unknown[] }>("/fleet/incidents?openOnly=true");
  const receiptIssues = useApi<{ items: unknown[] }>("/orders/receipt-issues");
  const queueCount = orders.data?.items?.length ?? 0;
  const alertCount = (incidents.data?.items?.length ?? 0) + (receiptIssues.data?.items?.length ?? 0);

  useEffect(() => { setNavOpen(false); }, [pathname]);
  useEffect(() => {
    if (!navOpen) return;
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") setNavOpen(false); };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [navOpen]);

  function search(event: FormEvent) {
    event.preventDefault();
    const q = query.trim();
    if (q) navigate(`/dispatcher/orders?q=${encodeURIComponent(q)}`);
  }

  const name = profile?.displayName?.trim() || profile?.userId || t("Dispatcher");
  const initials = name.split(/[\s._-]+/).filter(Boolean).slice(0, 2).map((part) => part[0]?.toUpperCase()).join("") || "D";
  const item = (to: string, label: string, icon: string, width: number, height: number, count?: number, end?: boolean) => (
    <NavLink to={to} end={end} className={({ isActive }) => `sm-nav-item${isActive ? " active" : ""}`}>
      <span className="sm-nav-icon" aria-hidden="true"><img src={icon} alt="" width={width} height={height} /></span>
      {label}
      {count ? <span className="dp-nav-count" aria-label={`${count} ${t("open")}`}>{count}</span> : null}
    </NavLink>
  );

  const navContent = (
    <>
      <img className="sm-sidebar-logo" src={logo} alt="Waypoint Group" />
      <hr className="sm-sidebar-hr" />
      <nav className="sm-nav" aria-label={t("Dispatch")}>
        <span className="sm-nav-label">{t("Dispatch")}</span>
        {item("/dispatcher", t("Overview"), iconOverview, 20, 20, undefined, true)}
        {item("/dispatcher/orders", t("Order queue"), iconQueue, 15, 12, queueCount)}
        {item("/dispatcher/planning", t("Plan and allocate"), iconRoute, 20, 20)}
        {item("/dispatcher/live", t("Live operations"), iconTruck, 20, 20, incidents.data?.items?.length)}
        {item("/dispatcher/fleet", t("Fleet"), iconFleet, 20, 20)}
      </nav>
      <hr className="sm-sidebar-hr sm-sidebar-hr--mid" />
      <nav className="sm-nav" aria-label={t("Records and planning")}>
        <span className="sm-nav-label">{t("Records and planning")}</span>
        {item("/dispatcher/deferrals", t("Deferral history"), iconHistory, 20, 20)}
        {item("/dispatcher/forecast", t("Demand forecast"), iconChart, 20, 20)}
      </nav>
      <hr className="sm-sidebar-hr sm-sidebar-hr--mid" />
      <nav className="sm-nav sm-nav--secondary" aria-label={t("Secondary")}>
        {item("/dispatcher/notifications", t("Notifications"), iconBell, 15, 16, alertCount)}
        {item("/dispatcher/settings", t("Settings"), iconSetting, 20, 20)}
        {item("/dispatcher/help", t("Help & Guide"), iconHelp, 6, 18)}
      </nav>
      <hr className="sm-sidebar-hr sm-sidebar-hr--bottom" />
      <div className="sm-sidebar-user">
        <div className="sm-sidebar-avatar" aria-hidden="true"><img src={iconUser} alt="" width={20} height={20} /></div>
        <div>
          <p className="sm-sidebar-user-name">{name}</p>
          <p className="sm-sidebar-user-role muted">{t("Dispatcher")}{depot && DEPOT_LABELS[depot] ? ` · ${DEPOT_LABELS[depot]}` : ""}</p>
        </div>
      </div>
    </>
  );

  return (
    <DepotContext.Provider value={{ depot, setDepot }}>
      <div className="sm-shell dp-shell">
        <a className="skip-link" href="#dp-main">{t("Skip to main content")}</a>
        <aside className="sm-sidebar" aria-label={t("Dispatcher navigation")}>{navContent}</aside>
        {navOpen && <div className="sm-nav-overlay" onClick={() => setNavOpen(false)} aria-hidden="true" />}
        <aside className={`sm-sidebar sm-sidebar--drawer${navOpen ? " open" : ""}`} aria-label={t("Dispatcher navigation")} aria-hidden={!navOpen}>{navContent}</aside>
        <div className="sm-content">
          <header className="sm-topbar">
            <button type="button" className="sm-menu-btn" aria-label={t("Open navigation")} aria-expanded={navOpen} onClick={() => setNavOpen(true)}>
              <span className="sm-hamburger" aria-hidden="true">☰</span>
            </button>
            <form className="dp-topbar-search" role="search" onSubmit={search}>
              <span aria-hidden="true">⌕</span>
              <input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("Search outlet, order or vehicle…")} aria-label={t("Search outlet, order or vehicle…")} />
            </form>
            <div className="sm-topbar-right">
              <select className="dp-topbar-select" value={depot} onChange={(e) => setDepot(e.target.value)} aria-label={t("Depot")}>
                <option value="">{t("All depots")}</option>
                {Object.entries(DEPOT_LABELS).map(([code, label]) => <option key={code} value={code}>{`${label} ${t("depot")}`}</option>)}
              </select>
              <NavLink to="/dispatcher/notifications" className="dp-bell" aria-label={`${t("Notifications")} · ${alertCount} ${t("open")}`}>
                <img src={iconBell} alt="" width={16} height={17} />
                {alertCount > 0 && <span className="dp-bell-dot" aria-hidden="true">{alertCount}</span>}
              </NavLink>
              <div className="dp-user">
                <span className="dp-avatar" aria-hidden="true">{initials}</span>
                <div className="dp-user-name">
                  <p className="sm-topbar-outlet-name">{name}</p>
                  <p className="sm-topbar-outlet-role muted">{t("Dispatcher")} · {today}</p>
                </div>
              </div>
              <button type="button" className="sm-topbar-signout tap" onClick={() => void logout()}>{t("Sign out")}</button>
            </div>
          </header>
          <main id="dp-main" tabIndex={-1} className="sm-page-content">
            <Outlet />
          </main>
        </div>
      </div>
    </DepotContext.Provider>
  );
}
