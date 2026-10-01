import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { userManager } from "./userManager";
import { useLocale } from "../i18n";

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
          navigate("/loader/loading", { replace: true });
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
