import { ReactNode } from "react";
import { Link } from "react-router-dom";
import { useLocale } from "../i18n";
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
  const { t } = useLocale();
  const hour = Number(new Intl.DateTimeFormat("en-GB", { timeZone: "Asia/Colombo", hour: "2-digit", hourCycle: "h23" }).format(new Date()));
  const open = hour < 16;
  return (
    <div className="sm-cutoff-card" role="note">
      <span className="sm-cutoff-icon" aria-hidden="true"><img src={iconHistory} alt="" width={24} height={24} /></span>
      <div>
        <p className="sm-cutoff-title">{open ? t("Order by 4:00 PM today for the next run.") : t("Today's 4:00 PM cutoff has passed.")}</p>
        <p className="sm-cutoff-body muted">{t("Orders placed after 4:00 PM move to the next operating day. Individual delivery windows still apply.")}</p>
      </div>
    </div>
  );
}
