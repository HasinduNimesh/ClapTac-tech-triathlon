import { useEffect } from "react";
import { useLocale } from "../i18n";

// The loader workspace is the separate Flutter app served at /loader-app/.
export const LOADER_APP_PATH = "/loader-app/";
export const LOADER_HANDOFF_KEY = "waypoint.loader.handoff";

export function LoaderAppRedirect() {
  const { t } = useLocale();
  useEffect(() => {
    // The loader app reads this once, in this tab, to carry on signing in without asking again.
    try { sessionStorage.setItem(LOADER_HANDOFF_KEY, String(Date.now())); } catch { /* the loader app then shows its own Sign in */ }
    window.location.replace(LOADER_APP_PATH);
  }, []);
  return <section className="card">{t("Opening the loader app…")}</section>;
}
