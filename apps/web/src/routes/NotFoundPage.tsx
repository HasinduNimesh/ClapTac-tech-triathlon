import { useEffect } from "react";
import { Link } from "react-router-dom";
import { useLocale } from "../i18n";
import { StoreManagerHero } from "../store-manager/StoreManagerHero";

export function NotFoundPage({ inWorkspace = false }: { inWorkspace?: boolean }) {
  const { t } = useLocale();

  useEffect(() => {
    const previous = document.title;
    document.title = `${t("Page not found")} · Waypoint`;
    return () => { document.title = previous; };
  }, [t]);

  const body = (
    <section className={inWorkspace ? "sm-form-card not-found-card" : "card not-found-card"} aria-labelledby="not-found-heading">
      <p className="not-found-code" aria-hidden="true">404</p>
      {inWorkspace
        ? <h2 id="not-found-heading" className="sm-form-card-title">{t("Page not found")}</h2>
        : <h2 id="not-found-heading">{t("Page not found")}</h2>}
      <p className="muted">{t("The page you are looking for doesn't exist or has moved.")}</p>
      {inWorkspace
        ? <Link to="/store-manager" className="sm-btn-secondary">{t("Back to dashboard")}</Link>
        : <Link to="/" className="not-found-action">{t("Go to the home page")}</Link>}
    </section>
  );

  if (!inWorkspace) return body;

  return (
    <>
      <StoreManagerHero compact crumbs={[{ label: t("Store"), to: "/store-manager" }, { label: t("Page not found") }]} title={t("Page not found")} />
      <div className="sm-page-body">{body}</div>
    </>
  );
}
