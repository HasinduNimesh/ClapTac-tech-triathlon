import { useEffect } from "react";
import { useLocale } from "../i18n";

// The loader workspace is the separate Flutter app served at /loader-app/.
export const LOADER_APP_PATH = "/loader-app/";

export function LoaderAppRedirect() {
  const { t } = useLocale();
  useEffect(() => {
    window.location.replace(LOADER_APP_PATH);
  }, []);
  return <section className="card">{t("Opening the loader app…")}</section>;
}
