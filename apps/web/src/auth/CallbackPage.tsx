import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { userManager } from "./userManager";
import { useAuth } from "./AuthContext";
import { useLocale } from "../i18n";
import { LOADER_APP_PATH } from "../loader/LoaderAppRedirect";
import { markLoaderHandoff } from "../loader/handoff.mjs";
import { takeReturn } from "./returnTo.mjs";

export function CallbackPage() {
  const navigate = useNavigate();
  const { completeLogin } = useAuth();
  const { t } = useLocale();
  useEffect(() => {
    userManager
      .signinRedirectCallback()
      .then(async (user) => {
        completeLogin(user);
        const res = await fetch("/api/v1/shared/profiles/me", {
          headers: { Authorization: `Bearer ${user.access_token}` },
        });
        const body = res.ok ? await res.json() : { profile: { roles: [] } };
        const role = body.profile?.roles?.[0];
        // After a session ended, signing in again returns to the page the person was on (inside their own area only).
        const back = takeReturn(window.sessionStorage, role);
        if (back) {
          navigate(back, { replace: true });
        } else if (role === "STORE_MANAGER") {
          navigate("/store-manager/orders", { replace: true });
        } else if (role === "DISPATCHER") {
          navigate("/dispatcher/orders", { replace: true });
        } else if (role === "LOADER") {
          // Loaders work in the separate loader app, not in this one. The marker lets it carry on signing in
          // instead of asking for a second sign-in.
          markLoaderHandoff();
          window.location.replace(LOADER_APP_PATH);
        } else if (role === "DRIVER") {
          navigate("/driver", { replace: true });
        } else {
          navigate("/", { replace: true });
        }
      })
      .catch(() => navigate("/login", { replace: true }));
  }, [completeLogin, navigate]);
  return <section className="card">{t("Completing sign-in…")}</section>;
}
