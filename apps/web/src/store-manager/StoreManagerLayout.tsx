import { useState } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import logo from "../assets/store-manager/sidebar-logo.png";
import iconCategory from "../assets/store-manager/icon-category.svg";
import iconVector from "../assets/store-manager/icon-vector.svg";
import iconBox from "../assets/store-manager/icon-box.svg";
import iconBell from "../assets/store-manager/icon-bell.svg";
import iconHelp from "../assets/store-manager/icon-help.svg";
import iconUser from "../assets/store-manager/icon-user.svg";

export function StoreManagerLayout() {
  const { profile, logout } = useAuth();
  const { t } = useLocale();
  const [navOpen, setNavOpen] = useState(false);

  const outletId = profile?.outletIds?.[0];

  const navContent = (
    <>
      <img className="sm-sidebar-logo" src={logo} alt="Waypoint Group" />
      <hr className="sm-sidebar-hr" />

      <nav className="sm-nav" aria-label={t("Store Manager")}>
        <span className="sm-nav-label">{t("Store Manager Workspace")}</span>
        <NavLink to="/store-manager" end className={({ isActive }) => `sm-nav-item${isActive ? " active" : ""}`} onClick={() => setNavOpen(false)}>
          <img src={iconCategory} alt="" aria-hidden="true" width={20} height={20} />
          {t("Dashboard")}
        </NavLink>
        <NavLink to="/store-manager/orders/new" className={({ isActive }) => `sm-nav-item${isActive ? " active" : ""}`} onClick={() => setNavOpen(false)}>
          <img src={iconVector} alt="" aria-hidden="true" width={16} height={16} />
          {t("Place Orders")}
        </NavLink>
        <NavLink to="/store-manager/orders" className={({ isActive }) => `sm-nav-item${isActive ? " active" : ""}`} onClick={() => setNavOpen(false)}>
          <img src={iconBox} alt="" aria-hidden="true" width={20} height={20} />
          {t("Orders")}
        </NavLink>
      </nav>

      <hr className="sm-sidebar-hr sm-sidebar-hr--mid" />

      <nav className="sm-nav sm-nav--secondary" aria-label={t("Secondary")}>
        <NavLink to="/store-manager/notifications" className={({ isActive }) => `sm-nav-item${isActive ? " active" : ""}`} onClick={() => setNavOpen(false)}>
          <img src={iconBell} alt="" aria-hidden="true" width={20} height={20} />
          {t("Notifications")}
        </NavLink>
        <span className="sm-nav-item sm-nav-item--static">
          <img src={iconHelp} alt="" aria-hidden="true" width={20} height={20} />
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
