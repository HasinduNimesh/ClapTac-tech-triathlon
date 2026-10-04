import { useLocale } from "../i18n";

/** The one calm line shown when a language helper is off. The form or screen next to it keeps working by hand. */
export function HelperUnavailableNote() {
  const { t } = useLocale();
  return <p className="sm-helper-unavailable muted" role="status">{t("The helper isn't available right now — you can fill this in by hand.")}</p>;
}
