import { ReactNode, useEffect, useRef } from "react";
import { useLocale } from "../i18n";
import { StoreManagerHero } from "../store-manager/StoreManagerHero";

export type Tone = "primary" | "green" | "amber" | "red" | "cool" | "muted" | "purple";

// The dispatcher hero is the same blue banner the store-manager workspace uses,
// so both roles share one visual system (Waypoint UI Kit).
export function DpHero({ title, subtitle, children }: { title: string; subtitle?: string; children?: ReactNode }) {
  return <StoreManagerHero title={title} subtitle={subtitle}>{children}</StoreManagerHero>;
}

export function StatRow({ cols = 4, children }: { cols?: number; children: ReactNode }) {
  return <div className="dp-stats" style={{ ["--dp-stat-cols" as string]: cols }}>{children}</div>;
}

export function Stat({ label, value, sub, subTone, icon, iconTone, variant }: {
  label: string; value: ReactNode; sub?: ReactNode; subTone?: "red" | "green" | "amber"; icon?: string; iconTone?: "red" | "green" | "amber" | "cool"; variant?: "red" | "green";
}) {
  return (
    <article className={`dp-stat${variant ? ` dp-stat--${variant}` : ""}`}>
      {icon && <span className={`dp-stat-icon${iconTone ? ` dp-stat-icon--${iconTone}` : ""}`} aria-hidden="true">{icon}</span>}
      <div>
        <p className="dp-stat-label">{label}</p>
        <p className="dp-stat-value">{value}</p>
        {sub && <p className={`dp-stat-sub${subTone ? ` dp-stat-sub--${subTone}` : ""}`}>{sub}</p>}
      </div>
    </article>
  );
}

export function Panel({ title, sub, actions, children, flush, className, headingLevel = 2, id }: {
  title?: ReactNode; sub?: ReactNode; actions?: ReactNode; children?: ReactNode; flush?: boolean; className?: string; headingLevel?: 2 | 3; id?: string;
}) {
  const Heading = headingLevel === 2 ? "h2" : "h3";
  return (
    <section className={`dp-panel${className ? ` ${className}` : ""}`} aria-labelledby={title && id ? id : undefined}>
      {(title || actions) && (
        <div className="dp-panel-head">
          <div>
            {title && <Heading className="dp-panel-title" id={id}>{title}</Heading>}
            {sub && <p className="dp-panel-sub">{sub}</p>}
          </div>
          {actions && <div className="dp-row">{actions}</div>}
        </div>
      )}
      <div className={flush ? "dp-panel-body--flush" : "dp-panel-body"}>{children}</div>
    </section>
  );
}

export function Tag({ tone = "muted", children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`dp-tag${tone === "muted" ? "" : ` dp-tag--${tone}`}`}>{children}</span>;
}

export function Note({ tone = "primary", title, children, live }: { tone?: "primary" | "green" | "amber" | "red" | "cool"; title?: ReactNode; children?: ReactNode; live?: boolean }) {
  return (
    <div className={`dp-note${tone === "primary" ? "" : ` dp-note--${tone}`}`} role={live ? (tone === "red" ? "alert" : "status") : undefined}>
      {title && <strong>{title}</strong>}
      {children}
    </div>
  );
}

export function Banner({ tone = "red", icon = "!", title, text, children }: { tone?: "red" | "green" | "amber"; icon?: string; title: ReactNode; text?: ReactNode; children?: ReactNode }) {
  return (
    <div className={`dp-banner${tone === "red" ? "" : ` dp-banner--${tone}`}`} role={tone === "red" ? "alert" : "status"}>
      <span className="dp-banner-icon" aria-hidden="true">{icon}</span>
      <div className="dp-spacer">
        <p className="dp-banner-title">{title}</p>
        {text && <p className="dp-banner-text">{text}</p>}
      </div>
      {children}
    </div>
  );
}

export function meterTone(pct: number): "green" | "amber" | "red" | undefined {
  if (pct > 100) return "red";
  if (pct >= 90) return "amber";
  return undefined;
}

export function Meter({ label, valueText, pct, tone, ariaLabel }: { label?: ReactNode; valueText?: ReactNode; pct: number; tone?: "green" | "amber" | "red" | "cool"; ariaLabel: string }) {
  const clamped = Math.max(0, Math.min(100, Number.isFinite(pct) ? pct : 0));
  return (
    <div className={`dp-meter${tone ? ` dp-meter--${tone}` : ""}`}>
      {(label || valueText) && <div className="dp-meter-head"><span>{label}</span><strong>{valueText}</strong></div>}
      <div className="dp-meter-track">
        <span className="dp-meter-fill" style={{ width: `${clamped}%` }} />
        <progress max={100} value={clamped} aria-label={ariaLabel} />
      </div>
    </div>
  );
}

export function ChipGroup<T extends string>({ value, options, onChange, label }: { value: T; options: { value: T; label: string }[]; onChange: (value: T) => void; label: string }) {
  return (
    <div className="dp-chips" role="group" aria-label={label}>
      {options.map((option) => (
        <button key={option.value} type="button" className="dp-chip" aria-pressed={option.value === value} onClick={() => onChange(option.value)}>
          {option.label}
        </button>
      ))}
    </div>
  );
}

export function Check({ state, children }: { state: "ok" | "bad" | "warn" | "todo"; children: ReactNode }) {
  const symbol = state === "ok" ? "✓" : state === "todo" ? "" : "!";
  return <li><span className={`dp-check${state === "ok" ? "" : ` dp-check--${state}`}`} aria-hidden="true">{symbol}</span><span>{children}</span></li>;
}

// Side drawer on desktop: sits on a scrim, holds one decision, closes on Esc.
export function Drawer({ open, title, eyebrow, sub, onClose, children, footer, narrow, steps }: {
  open: boolean; title: ReactNode; eyebrow?: ReactNode; sub?: ReactNode; onClose: () => void; children: ReactNode; footer?: ReactNode; narrow?: boolean; steps?: ReactNode;
}) {
  const { t } = useLocale();
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement as HTMLElement | null;
    closeRef.current?.focus();
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("keydown", onKey); previous?.focus?.(); };
  }, [open]);
  if (!open) return null;
  return (
    <>
      <div className="dp-scrim" onClick={onClose} aria-hidden="true" />
      <aside className={`dp-drawer${narrow ? " dp-drawer--narrow" : ""}`} role="dialog" aria-modal="true" aria-labelledby="dp-drawer-title">
        <div className="dp-drawer-head">
          <div>
            {eyebrow && <p className="dp-drawer-eyebrow">{eyebrow}</p>}
            <h2 className="dp-drawer-title" id="dp-drawer-title">{title}</h2>
            {sub && <p className="dp-drawer-sub">{sub}</p>}
          </div>
          <button ref={closeRef} type="button" className="dp-drawer-close" onClick={onClose} aria-label={t("Close")}>✕</button>
        </div>
        {steps}
        <div className="dp-drawer-body">{children}</div>
        {footer && <div className="dp-drawer-foot">{footer}</div>}
      </aside>
    </>
  );
}

export function Toast({ children, onClose }: { children: ReactNode; onClose: () => void }) {
  const { t } = useLocale();
  useEffect(() => { const id = window.setTimeout(onClose, 8000); return () => window.clearTimeout(id); }, []);
  return <div className="dp-toast" role="status"><span className="dp-spacer">{children}</span><button type="button" className="dp-link" style={{ color: "#fff" }} onClick={onClose}>{t("Dismiss")}</button></div>;
}

export function Donut({ segments, total, label }: { segments: { value: number; color: string }[]; total: number; label: string }) {
  const r = 15.9155;
  let offset = 0;
  return (
    <div className="dp-donut">
      <svg viewBox="0 0 36 36" aria-hidden="true">
        <circle cx="18" cy="18" r={r} fill="none" stroke="#eef1ff" strokeWidth="4.5" />
        {total > 0 && segments.map((segment, index) => {
          const share = (segment.value / total) * 100;
          const el = <circle key={index} cx="18" cy="18" r={r} fill="none" stroke={segment.color} strokeWidth="4.5" strokeDasharray={`${share} ${100 - share}`} strokeDashoffset={-offset} />;
          offset += share;
          return el;
        })}
      </svg>
      <div className="dp-donut-center"><span className="dp-donut-value">{total}</span><span className="muted">{label}</span></div>
    </div>
  );
}

export const BRAND_COLORS: Record<string, string> = { fresh: "#3a57e8", style: "#8b5cf6", tech: "#08b1ba" };
export function brandColor(brand: string) { return BRAND_COLORS[brand.toLowerCase()] || "#8a92a6"; }
export function brandTone(brand: string): Tone { const b = brand.toLowerCase(); return b === "fresh" ? "primary" : b === "style" ? "purple" : b === "tech" ? "cool" : "muted"; }
