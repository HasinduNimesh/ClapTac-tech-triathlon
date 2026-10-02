import { useState } from "react";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { startLoginRedirect } from "./startLoginRedirect.mjs";
import logo from "../assets/login/logo.png";
import hero from "../assets/login/hero.png";
import iconDispatcher from "../assets/login/icon-dispatcher.svg";
import iconStore from "../assets/login/icon-store.svg";
import iconLoader from "../assets/login/icon-loader.svg";
import iconDriver from "../assets/login/icon-driver.svg";

export function LoginPage() {
  const { login, user, profile } = useAuth();
  const { t, locale, setLocale } = useLocale();
  const [loginFailed, setLoginFailed] = useState(false);
  const [loginStarting, setLoginStarting] = useState(false);

  const onSignIn = () =>
    void startLoginRedirect(
      login,
      () => { setLoginStarting(false); setLoginFailed(true); },
      () => { setLoginFailed(false); setLoginStarting(true); },
      () => setLoginStarting(false),
    );

  return (
    <div className="login-screen">
      <a className="skip-link" href="#main-content">{t("Skip to main content")}</a>
      <div className="login-pane">
        <label className="language-picker login-language-picker">
          {t("Language")}
          <select aria-label={t("Language")} value={locale} onChange={e => setLocale(e.target.value as "en" | "si" | "ta")}>
            <option value="en">{t("English")}</option>
            <option value="si">{t("Sinhala")}</option>
            <option value="ta">{t("Tamil")}</option>
          </select>
        </label>
        <main id="main-content" tabIndex={-1} className="login-form">
          <img className="login-logo" src={logo} alt="Waypoint Group" />
          <div className="login-heading">
            <h1>{t("Sign in")}</h1>
            <p className="muted">{t("One sign-in for every Waypoint role. We open the right workspace for you.")}</p>
          </div>

          {user && <p className="status-ok">{t("Signed in as")} {profile?.userId || user.profile.sub}</p>}
          {loginStarting && <p role="status" aria-live="polite">{t("Connecting to identity provider…")}</p>}
          {loginFailed && <p role="alert">{t("Sign-in could not be started. Check your connection and try again.")}</p>}

          <button type="button" className="tap primary login-submit" disabled={loginStarting} onClick={onSignIn}>
            {t("Sign in")}
          </button>

          <div className="login-role-note">
            <p>{t("Your workspace opens by role")}</p>
            <div className="login-role-badges">
              <span className="login-badge"><img src={iconDispatcher} alt="" aria-hidden="true" />{t("Dispatcher")}</span>
              <span className="login-badge"><img src={iconStore} alt="" aria-hidden="true" />{t("Store manager")}</span>
              <span className="login-badge"><img src={iconLoader} alt="" aria-hidden="true" />{t("Loader")}</span>
              <span className="login-badge"><img src={iconDriver} alt="" aria-hidden="true" />{t("Driver")}</span>
            </div>
          </div>

          <p className="login-footnote muted">
            {t("No account yet?")} <span className="login-footnote-link">{t("Ask your depot supervisor to add you.")}</span>
          </p>
        </main>
      </div>
      <div className="login-hero" style={{ backgroundImage: `url(${hero})` }} aria-hidden="true" />
    </div>
  );
}
