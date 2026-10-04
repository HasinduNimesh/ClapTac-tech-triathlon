import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { userManager } from "./userManager";
import { useLocale } from "../i18n";
import { LOADER_APP_PATH } from "../loader/LoaderAppRedirect";

export function CallbackPage() {
  const navigate = useNavigate();
  const { t } = useLocale();
  useEffect(() => {
    userManager
      .signinRedirectCallback()
      .then(async (user) => {
        const res = await fetch("/api/v1/shared/profiles/me", {
          headers: { Authorization: `Bearer ${user.access_token}` },
        });
        const body = res.ok ? await res.json() : { profile: { roles: [] } };
        const role = body.profile?.roles?.[0];
        if (role === "STORE_MANAGER") {
          navigate("/store-manager/orders", { replace: true });
        } else if (role === "DISPATCHER") {
          navigate("/dispatcher/orders", { replace: true });
        } else if (role === "LOADER") {
          // Loaders work in the separate loader app, not in this one.
          window.location.replace(LOADER_APP_PATH);
        } else if (role === "DRIVER") {
          navigate("/driver", { replace: true });
        } else {
          navigate("/", { replace: true });
        }
      })
      .catch(() => navigate("/login", { replace: true }));
  }, [navigate]);
  return <section className="card">{t("Completing sign-in…")}</section>;
}
