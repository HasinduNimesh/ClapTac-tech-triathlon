import { useEffect, useRef } from "react";
import { Link, Outlet, useNavigate } from "react-router-dom";
import { roleOf, useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { createLoaderInactivityTimer } from "../loader/inactivityTimer.mjs";

export function Layout() {
  const { user, profile, logout } = useAuth();
  const role = roleOf(profile)?.toUpperCase();
  const { locale, setLocale, t } = useLocale();
  const navigate = useNavigate();
  const signOutRef = useRef(logout);
  const navigateRef = useRef(navigate);
  const loaderTimerRef = useRef<ReturnType<typeof createLoaderInactivityTimer> | null>(null);
  signOutRef.current = logout;
  navigateRef.current = navigate;

  useEffect(() => {
    if (role !== "LOADER" || !user) return;
    const timer = createLoaderInactivityTimer(async () => {
      await signOutRef.current();
      navigateRef.current("/login", { replace: true });
    });
    loaderTimerRef.current = timer;
    const reset = () => timer.reset();
    const activityEvents = ["pointerdown", "pointermove", "keydown", "wheel", "touchstart"];
    for (const event of activityEvents) window.addEventListener(event, reset, { passive: true });
    return () => {
      timer.stop();
      loaderTimerRef.current = null;
      for (const event of activityEvents) window.removeEventListener(event, reset);
    };
  }, [role, user?.profile.sub]);

  const onSignOut = () => {
    if (role === "LOADER") void loaderTimerRef.current?.switchUser();
    else void logout();
  };
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
            <Link to="/dispatcher/automations">{t("My automations")}</Link>
            <Link to="/dispatcher/orders">{t("Orders")}</Link>
            <Link to="/dispatcher/planning">{t("Planning")}</Link>
            <Link to="/dispatcher/audit">{t("Audit & KPIs")}</Link>
            <Link to="/dispatcher/master-data">{t("Master data")}</Link>
            <Link to="/dispatcher/forecast">{t("Forecast")}</Link>
          </>}
          {role === "LOADER" && <Link to="/loader/loading">{t("Loading")}</Link>}
          {role === "DRIVER" && <Link to="/driver/trips">{t("My route")}</Link>}
          {user && <button type="button" onClick={onSignOut}>{t(role === "LOADER" ? "Switch user" : "Sign out")}</button>}
          <label className="language-picker">{t("Language")}<select aria-label={t("Language")} value={locale} onChange={e => setLocale(e.target.value as "en" | "si" | "ta")}><option value="en">{t("English")}</option><option value="si">{t("Sinhala")}</option><option value="ta">{t("Tamil")}</option></select></label>
        </nav>
      </header>
      <main id="main-content" tabIndex={-1}>
        <Outlet />
      </main>
    </>
  );
}

