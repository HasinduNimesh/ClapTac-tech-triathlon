import { Link, Outlet } from "react-router-dom";
import { roleOf, useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";

export function Layout() {
  const { user, profile, logout } = useAuth();
  const role = roleOf(profile)?.toUpperCase();
  const { locale, setLocale, t } = useLocale();
  return (
    <>
      <a className="skip-link" href="#main-content">{t("Skip to main content")}</a>
      <header>
        <h1>Waypoint</h1>
        <nav>
          <Link to="/">{t("Home")}</Link>
          {!user && <Link to="/login">{t("Login")}</Link>}
          {role === "STORE_MANAGER" && <>
            <Link to="/store-manager/orders">{t("My orders")}</Link>
            <Link to="/store-manager/tracking">{t("Tracking")}</Link>
            <Link to="/store-manager/receipts">{t("Receipts")}</Link>
          </>}
          {role === "DISPATCHER" && <>
            <Link to="/dispatcher/orders">{t("Orders")}</Link>
            <Link to="/dispatcher/planning">{t("Planning")}</Link>
            <Link to="/dispatcher/audit">{t("Audit & KPIs")}</Link>
            <Link to="/dispatcher/master-data">{t("Master data")}</Link>
            <Link to="/dispatcher/forecast">{t("Forecast")}</Link>
          </>}
          {role === "LOADER" && <a href="/loader-app/">{t("Loading")}</a>}
          {role === "DRIVER" && <Link to="/driver/trips">{t("My route")}</Link>}
          {user && <button type="button" onClick={() => void logout()}>{t("Sign out")}</button>}
          <label className="language-picker">{t("Language")}<select aria-label={t("Language")} value={locale} onChange={e => setLocale(e.target.value as "en" | "si" | "ta")}><option value="en">{t("English")}</option><option value="si">{t("Sinhala")}</option><option value="ta">{t("Tamil")}</option></select></label>
        </nav>
      </header>
      <main id="main-content" tabIndex={-1}>
        <Outlet />
      </main>
    </>
  );
}
