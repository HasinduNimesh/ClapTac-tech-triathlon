import { useEffect, useState } from "react";
import { Link, NavLink, Outlet, useLocation } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import logo from "../assets/login/logo.png";
import iconCategory from "../assets/store-manager/icon-category.svg";
import iconVector from "../assets/store-manager/icon-vector.svg";
import iconBox from "../assets/store-manager/icon-box.svg";
import iconBell from "../assets/store-manager/icon-bell.svg";
import iconHelp from "../assets/store-manager/icon-help.svg";
import iconSetting from "../assets/store-manager/icon-setting.svg";
import "../dispatcher/dispatcher.css";
import "./storeManager.css";
import iconUser from "../assets/store-manager/icon-user.svg";

export function StoreManagerLayout() {
  const { profile, logout } = useAuth();
  const { t } = useLocale();
  const [navOpen, setNavOpen] = useState(false);
  const { pathname } = useLocation();
  const ordersActive = pathname === "/store-manager/orders" || pathname.startsWith("/store-manager/tracking") || pathname.startsWith("/store-manager/receipts");

  const outletId = profile?.outletIds?.[0];

  useEffect(() => {
    if (!navOpen) return;
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") setNavOpen(false); };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [navOpen]);

  const navContent = (
    <>
      <img className="sm-sidebar-logo" src={logo} alt="Waypoint Group" />
      <hr className="sm-sidebar-hr" />

      <nav className="sm-nav" aria-label={t("Store Manager")}>
        <span className="sm-nav-label">{t("Store Manager Workspace")}</span>
        <NavLink to="/store-manager" end className={({ isActive }) => `sm-nav-item${isActive ? " active" : ""}`} onClick={() => setNavOpen(false)}>
          <span className="sm-nav-icon sm-nav-icon--light" aria-hidden="true"><img src={iconCategory} alt="" width={20} height={20} /></span>
          {t("Dashboard")}
        </NavLink>
        <NavLink to="/store-manager/orders/new" className={({ isActive }) => `sm-nav-item${isActive ? " active" : ""}`} onClick={() => setNavOpen(false)}>
          <span className="sm-nav-icon" aria-hidden="true"><img src={iconVector} alt="" width={15} height={11} /></span>
          {t("Place Orders")}
        </NavLink>
        <Link to="/store-manager/orders" className={`sm-nav-item${ordersActive ? " active" : ""}`} aria-current={ordersActive ? "page" : undefined} onClick={() => setNavOpen(false)}>
          <span className="sm-nav-icon" aria-hidden="true"><img src={iconBox} alt="" width={20} height={20} /></span>
          {t("Orders")}
        </Link>
      </nav>

      <hr className="sm-sidebar-hr sm-sidebar-hr--mid" />

      <nav className="sm-nav sm-nav--secondary" aria-label={t("Secondary")}>
        <NavLink to="/store-manager/notifications" className={({ isActive }) => `sm-nav-item${isActive ? " active" : ""}`} onClick={() => setNavOpen(false)}>
          <span className="sm-nav-icon" aria-hidden="true"><img src={iconBell} alt="" width={15} height={16} /></span>
          {t("Notifications")}
        </NavLink>
        <NavLink to="/store-manager/settings" className={({ isActive }) => `sm-nav-item${isActive ? " active" : ""}`} onClick={() => setNavOpen(false)}>
          <span className="sm-nav-icon" aria-hidden="true"><img src={iconSetting} alt="" width={20} height={20} /></span>
          {t("Settings")}
        </NavLink>
        <span className="sm-nav-item sm-nav-item--static">
          <span className="sm-nav-icon" aria-hidden="true"><img src={iconHelp} alt="" width={6} height={18} /></span>
          {t("Help & Guide")}
        </span>
      </nav>

      <hr className="sm-sidebar-hr sm-sidebar-hr--bottom" />

      <div className="sm-sidebar-user">
        <div className="sm-sidebar-avatar" aria-hidden="true">
          <img src={iconUser} alt="" width={20} height={20} />
        </div>
        <div>
          <p className="sm-sidebar-user-name">{profile?.userId || t("Store Manager")}</p>
          <p className="sm-sidebar-user-role muted">{t("Store Manager")}</p>
        </div>
      </div>
    </>
  );

  return (
    <div className="sm-shell">
      <a className="skip-link" href="#sm-main">{t("Skip to main content")}</a>

      <aside className="sm-sidebar" aria-label={t("Store Manager navigation")}>
        {navContent}
      </aside>

      {navOpen && (
        <div className="sm-nav-overlay" onClick={() => setNavOpen(false)} aria-hidden="true" />
      )}
      <aside
        className={`sm-sidebar sm-sidebar--drawer${navOpen ? " open" : ""}`}
        aria-label={t("Store Manager navigation")}
        aria-hidden={!navOpen}
      >
        {navContent}
      </aside>

      <div className="sm-content">
        <header className="sm-topbar">
          <button
            type="button"
            className="sm-menu-btn"
            aria-label={t("Open navigation")}
            aria-expanded={navOpen}
            onClick={() => setNavOpen(true)}
          >
            <span className="sm-hamburger" aria-hidden="true">☰</span>
          </button>
          <div className="sm-topbar-outlet">
            {outletId
              ? <p className="sm-topbar-outlet-name">{outletId}</p>
              : <p className="sm-topbar-outlet-name muted">{t("No outlet assigned")}</p>
            }
            <p className="sm-topbar-outlet-role muted">{`Waypoint Delivery · ${t("Store Manager")}`}</p>
          </div>
          <div className="sm-topbar-right">
            {outletId && (
              <div className="sm-outlet-badge">
                <span>{outletId}</span>
              </div>
            )}
            <button type="button" className="sm-topbar-signout tap" onClick={() => void logout()}>
              {t("Sign out")}
            </button>
          </div>
        </header>

        <main id="sm-main" tabIndex={-1} className="sm-page-content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
