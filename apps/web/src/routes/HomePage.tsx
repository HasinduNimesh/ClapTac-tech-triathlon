import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { roleOf, useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { useDriverApk } from "./useDriverApk";
import { startLoginRedirect } from "./startLoginRedirect.mjs";
import { workspaceFor } from "./workspaceLink.mjs";
import { markLoaderHandoff } from "../loader/handoff.mjs";
import "./landing.css";
import logo from "../assets/login/logo.png";
import hero from "../assets/login/hero.png";
import iconStore from "../assets/login/icon-store.svg";
import iconDispatcher from "../assets/login/icon-dispatcher.svg";
import iconLoader from "../assets/login/icon-loader.svg";
import iconDriver from "../assets/login/icon-driver.svg";
import iconCheck from "../assets/store-manager/icon-check.svg";
import iconTruck from "../assets/store-manager/icon-truck.svg";

type Status = "checking" | "live" | "down";

export function HomePage() {
  const { t, locale, setLocale } = useLocale();
  const { login, user, profile } = useAuth();
  const [status, setStatus] = useState<Status>("checking");
  const apk = useDriverApk();
  const [starting, setStarting] = useState(false);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    fetch("/health/live").then((r) => setStatus(r.ok ? "live" : "down")).catch(() => setStatus("down"));
  }, []);

  const onSignIn = () =>
    void startLoginRedirect(
      login,
      () => { setStarting(false); setFailed(true); },
      () => { setFailed(false); setStarting(true); },
      () => setStarting(false),
    );

  const workspace = workspaceFor(roleOf(profile));
  const signedIn = Boolean(user && profile);

  const primary = signedIn
    ? (workspace.external
      ? <a className="lp-btn lp-btn--primary" href={workspace.href} onClick={() => markLoaderHandoff()}>{t("Open my workspace")}</a>
      : <Link className="lp-btn lp-btn--primary" to={workspace.href}>{t("Open my workspace")}</Link>)
    : <button type="button" className="lp-btn lp-btn--primary" disabled={starting} onClick={onSignIn}>{starting ? t("Connecting to identity provider…") : t("Sign in")}</button>;

  const steps = [
    { icon: iconStore, title: t("Order"), who: t("Store manager"), text: t("A store places its order before the daily cutoff.") },
    { icon: iconDispatcher, title: t("Plan"), who: t("Dispatcher"), text: t("The dispatcher plans and allocates orders to vehicles.") },
    { icon: iconLoader, title: t("Load"), who: t("Loader"), text: t("The loader checks every line onto the truck.") },
    { icon: iconDriver, title: t("Deliver"), who: t("Driver"), text: t("The driver delivers with proof, even without signal.") },
    { icon: iconCheck, title: t("Receipt"), who: t("Store manager"), text: t("The store confirms what arrived.") },
  ];
  const roles = [
    { icon: iconStore, title: t("Store manager"), text: t("Place orders, follow each delivery and confirm receipts.") },
    { icon: iconDispatcher, title: t("Dispatcher"), text: t("Plan runs, watch live operations and handle disruptions.") },
    { icon: iconLoader, title: t("Loader"), text: t("Load by the plan, report shortfalls and mark trucks ready.") },
    { icon: iconDriver, title: t("Driver"), text: t("Follow the route, record stops and send proof.") },
  ];

  return (
    <div className="lp">
      <a className="skip-link" href="#main-content">{t("Skip to main content")}</a>

      <header className="lp-nav">
        <div className="lp-wrap lp-nav-row">
          <a href="/" aria-label="Waypoint"><img className="lp-logo" src={logo} alt="Waypoint Group" /></a>
          <nav className="lp-nav-links" aria-label={t("Sections")}>
            <a href="#how">{t("How it works")}</a>
            <a href="#roles">{t("Roles")}</a>
            <a href="#driver-app">{t("Driver app")}</a>
          </nav>
          <div className="lp-nav-actions">
            <label className="language-picker lp-lang">
              <span className="visually-hidden">{t("Language")}</span>
              <select aria-label={t("Language")} value={locale} onChange={(e) => setLocale(e.target.value as "en" | "si" | "ta")}>
                <option value="en">{t("English")}</option>
                <option value="si">{t("Sinhala")}</option>
                <option value="ta">{t("Tamil")}</option>
              </select>
            </label>
            {primary}
          </div>
        </div>
      </header>

      <main id="main-content" tabIndex={-1} className="lp-main">
        <div className="lp-wrap">
          <section className="lp-hero" aria-labelledby="lp-title">
            <div>
              <p className={`lp-pill lp-pill--${status}`} role="status">
                <span className="lp-dot" aria-hidden="true" />
                {status === "live" ? t("All systems live") : status === "down" ? t("Can't reach the service right now") : t("Checking the service…")}
              </p>
              <h1 id="lp-title">{t("Every delivery, from order to")} <em>{t("doorstep")}</em></h1>
              <p className="lp-lede">{t("Waypoint connects store managers, dispatchers, loaders and drivers in one flow: place the order, plan the run, load the truck, deliver with proof, confirm the receipt.")}</p>
              <div className="lp-cta">
                {primary}
                <a className="lp-btn lp-btn--ghost" href="#driver-app">{t("Get the driver app")}</a>
              </div>
              {failed && <p className="lp-alert" role="alert">{t("Sign-in could not be started. Check your connection and try again.")}</p>}
              <ul className="lp-chips">
                <li>{t("Works offline for drivers")}</li>
                <li>{t("One sign-in, the right workspace")}</li>
                <li>{"English · සිංහල · தமிழ்"}</li>
              </ul>
            </div>
            <div className="lp-art" style={{ backgroundImage: `url(${hero})` }} aria-hidden="true">
              <span className="lp-float lp-float--a"><i />{t("Order placed")}</span>
              <span className="lp-float lp-float--b"><i />{t("Plan locked")}</span>
              <span className="lp-float lp-float--c"><i />{t("Delivered")}</span>
            </div>
          </section>

          <section className="lp-section" id="how" aria-labelledby="lp-how">
            <div className="lp-section-head">
              <p className="lp-eyebrow">{t("How it works")}</p>
              <h2 id="lp-how">{t("How an order travels")}</h2>
              <p>{t("Five steps, one shared record.")}</p>
            </div>
            <div className="lp-steps-wrap">
              <span className="lp-route" aria-hidden="true"><img className="lp-truck" src={iconTruck} alt="" /></span>
            <ol className="lp-steps">
              {steps.map((step, index) => (
                <li className="lp-step" key={step.title}>
                  <span className="lp-step-badge"><img src={step.icon} alt="" aria-hidden="true" /><span className="lp-step-no" aria-hidden="true">{index + 1}</span></span>
                  <h3>{step.title}</h3>
                  <p className="lp-step-role">{step.who}</p>
                  <p>{step.text}</p>
                </li>
              ))}
            </ol>
            </div>
          </section>

          <section className="lp-section" id="roles" aria-labelledby="lp-roles">
            <div className="lp-section-head">
              <p className="lp-eyebrow">{t("Roles")}</p>
              <h2 id="lp-roles">{t("One workspace for every role")}</h2>
              <p>{t("Your account decides which workspace opens. Sign in once and it takes you to the right one.")}</p>
            </div>
            <ul className="lp-roles">
              {roles.map((role) => (
                <li className="lp-role" key={role.title}>
                  <span className="lp-role-icon"><img src={role.icon} alt="" aria-hidden="true" /></span>
                  <h3>{role.title}</h3>
                  <p>{role.text}</p>
                  {signedIn ? null : <a href="/login">{t("Sign in")}</a>}
                </li>
              ))}
            </ul>
          </section>

          <section className="lp-section" id="driver-app" aria-labelledby="lp-app">
            <div className="lp-app">
              <div>
                <p className="lp-eyebrow">{t("Driver app")}</p>
                <h2 id="lp-app">{t("Waypoint Driver for Android")}</h2>
                <p className="lp-app-lede">{t("Your route, your stops and proof of delivery on your phone, kept safe on the phone when there is no signal.")}</p>
                {apk === undefined ? (
                  <p className="lp-app-missing" role="status">{t("Checking for the latest app…")}</p>
                ) : apk ? (
                  <>
                    <a className="lp-btn lp-btn--amber" href="/downloads/waypoint-driver.apk" download>{t("Download the Android app")}</a>
                    <p className="lp-app-meta">
                      <span>{t("Version")} <b>{apk.version}</b></span>
                      <span><b>{apk.sizeLabel}</b></span>
                      <span>{t("Android 8.0 or newer")}</span>
                    </p>
                    <ol className="lp-app-steps">
                      <li>{t("Download the file to your phone.")}</li>
                      <li>{t("Open it, and allow installs from this source if Android asks.")}</li>
                      <li>{t("Sign in with the account your depot supervisor created.")}</li>
                    </ol>
                    <p className="lp-app-note">{t("This app is not on Google Play. Install it only from this page. SHA-256")}: <code>{apk.sha256}</code></p>
                  </>
                ) : (
                  <p className="lp-app-missing" role="status">{t("The Android app has not been published yet. Ask your depot supervisor.")}</p>
                )}
              </div>
              <div className="lp-phone-wrap" aria-hidden="true">
                <div className="lp-phone">
                  <div className="lp-phone-screen">
                    <div className="lp-phone-bar" />
                    <div className="lp-phone-hero">{t("Your route")}</div>
                    <div className="lp-stop"><i>✓</i><span /></div>
                    <div className="lp-stop lp-stop--now"><i>▶</i><span /></div>
                    <div className="lp-stop lp-stop--later"><i>·</i><span /></div>
                    <div className="lp-stop lp-stop--later"><i>·</i><span /></div>
                    <div className="lp-phone-btn" />
                  </div>
                </div>
              </div>
            </div>
          </section>
        </div>
      </main>

      <footer className="lp-footer">
        <div className="lp-wrap lp-footer-row">
          <span>{"Waypoint Group"}</span>
          <span>{t("Questions about your account? Ask your depot supervisor.")}</span>
        </div>
      </footer>
    </div>
  );
}
