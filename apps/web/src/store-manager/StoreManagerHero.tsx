import { ReactNode } from "react";
import { Link } from "react-router-dom";
import { useLocale } from "../i18n";
import { cutoffHasPassed, formatCutoff, withTime } from "./cutoff.mjs";
import { useOrderCutoff } from "./useOrderCutoff";
import heroBg from "../assets/store-manager/hero-bg.png";
import iconHistory from "../assets/store-manager/icon-history.svg";

type Crumb = { label: string; to?: string };

export function StoreManagerHero({
  crumbs, title, subtitle, compact = false, children,
}: { crumbs?: Crumb[]; title: string; subtitle?: string; compact?: boolean; children?: ReactNode }) {
  const { t } = useLocale();
  return (
    <div className={`sm-hero${compact ? " sm-hero--compact" : ""}`} style={{ backgroundImage: `url(${heroBg})` }}>
      <div className="sm-hero-content">
        {crumbs && crumbs.length > 0 && (
          <nav aria-label={t("Breadcrumb")}>
            <ol className="sm-breadcrumb">
              {crumbs.map((crumb, index) => (
                <li key={crumb.label}>
                  {crumb.to ? <Link to={crumb.to} className="sm-breadcrumb-link">{crumb.label}</Link> : <span aria-current="page">{crumb.label}</span>}
                  {index < crumbs.length - 1 && <span aria-hidden="true"> / </span>}
                </li>
              ))}
            </ol>
          </nav>
        )}
        <h1 className="sm-hero-title">{title}</h1>
        {subtitle && <p className="sm-hero-sub">{subtitle}</p>}
      </div>
      {children && <div className="sm-hero-actions">{children}</div>}
    </div>
  );
}

export function CutoffNotice() {
  const { t, locale } = useLocale();
  // The cutoff is a policy a dispatcher can change, so it is read from the order service. Until it is
  // known (or if it cannot be read) no time is stated.
  const local = useOrderCutoff();
  const time = formatCutoff(local, locale);
  const passed = cutoffHasPassed(local);
  const title = time === undefined || passed === undefined
    ? t("Orders placed after the daily cutoff move to the next operating day.")
    : passed ? withTime(t("Today's {time} cutoff has passed."), time) : withTime(t("Order by {time} today for the next run."), time);
  return (
    <div className="sm-cutoff-card" role="note">
      <span className="sm-cutoff-icon" aria-hidden="true"><img src={iconHistory} alt="" width={24} height={24} /></span>
      <div>
        <p className="sm-cutoff-title">{title}</p>
        <p className="sm-cutoff-body muted">{time === undefined ? t("Individual delivery windows still apply.") : withTime(t("Orders placed after {time} move to the next operating day. Individual delivery windows still apply."), time)}</p>
      </div>
    </div>
  );
}
