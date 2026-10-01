import { useState } from "react";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { startLoginRedirect } from "./startLoginRedirect.mjs";

export function LoginPage() {
  const { login, user, profile } = useAuth();
  const { t } = useLocale();
  const [loginFailed, setLoginFailed] = useState(false);
  const [loginStarting, setLoginStarting] = useState(false);
  return (
    <section className="card">
      <h2>{t("Sign in")}</h2>
      <p>{t("OIDC Authorization Code + PKCE. Role comes from the application profile, not this screen.")}</p>
      {user && <p>{t("Signed in as")} {profile?.userId || user.profile.sub}</p>}
      {loginStarting && <p role="status" aria-live="polite">{t("Connecting to identity provider…")}</p>}
      {loginFailed && <p role="alert">{t("Sign-in could not be started. Check your connection and try again.")}</p>}
      <button type="button" disabled={loginStarting} onClick={() => void startLoginRedirect(login, () => { setLoginStarting(false); setLoginFailed(true); }, () => { setLoginFailed(false); setLoginStarting(true); }, () => setLoginStarting(false))}>
        {t("Continue to identity provider")}
      </button>
    </section>
  );
}
