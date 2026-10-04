import { ReactNode, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Locale, useLocale } from "../i18n";
import { DpHero, Note, Panel } from "./ui";
import { CONTRAST_KEY, SIZE_KEY, TextSize, applyDisplayPreferences, readPreference as read, writePreference as write } from "../displayPreferences";
import { DisplayNameForm } from "../auth/DisplayNameForm";

export function DispatcherSettingsPage() {
  const { t, locale, setLocale } = useLocale();
  const [size, setSize] = useState<TextSize>(() => (read(SIZE_KEY) as TextSize) || "normal");
  const [contrast, setContrast] = useState(() => read(CONTRAST_KEY) === "true");
  useEffect(() => { write(SIZE_KEY, size); write(CONTRAST_KEY, String(contrast)); applyDisplayPreferences(); }, [size, contrast]);

  return (
    <>
      <DpHero title={t("Settings")} subtitle={t("Language, display and the reference data every plan depends on.")} />
      <div className="dp-body dp-body--flush">
        <div className="dp-grid-2 dp-grid-2--even">
          <Panel title={t("Account name")} sub={t("Your name appears in the sidebar across devices.")}><DisplayNameForm /></Panel>
          <Panel title={t("Language and accessibility")} sub={t("Larger text and high contrast help when reading a tablet in a busy depot.")}>
            <div className="dp-stack">
              <label className="dp-field">{t("Language")}
                <select value={locale} onChange={(e) => setLocale(e.target.value as Locale)}>
                  <option value="en">{t("English")}</option><option value="si">{t("Sinhala")}</option><option value="ta">{t("Tamil")}</option>
                </select>
              </label>
              <fieldset className="dp-stack" style={{ border: 0, padding: 0, margin: 0 }}>
                <legend className="dp-section-label">{t("Text size")}</legend>
                {([["normal", t("Standard")], ["large", t("Large")], ["xlarge", t("Extra large")]] as [TextSize, string][]).map(([value, label]) => (
                  <label key={value} className="dp-option"><input type="radio" name="text-size" value={value} checked={size === value} onChange={() => setSize(value)} /><div><p className="dp-option-title">{label}</p></div></label>
                ))}
              </fieldset>
              <label className="dp-option"><input type="checkbox" checked={contrast} onChange={(e) => setContrast(e.target.checked)} /><div><p className="dp-option-title">{t("High contrast")}</p><p className="dp-option-text">{t("Darker text and stronger borders on every screen.")}</p></div></label>
              <Note tone="green">{t("Saved on this device. Other devices keep their own settings.")}</Note>
            </div>
          </Panel>
          <Panel title={t("Reference data")} sub={t("Outlets, operating calendar, planning policy, vehicles and incidents.")}>
            <div className="dp-stack">
              <Link to="/dispatcher/master-data" className="dp-btn dp-btn--secondary dp-btn--block">{t("Master data")}</Link>
              <Link to="/dispatcher/audit" className="dp-btn dp-btn--secondary dp-btn--block">{t("Audit & KPIs")}</Link>
              <p className="muted" style={{ margin: 0, fontSize: "0.8125rem" }}>{t("Changes to outlets, vehicles and policy are versioned and recorded in the audit log.")}</p>
            </div>
          </Panel>
        </div>
      </div>
    </>
  );
}

// Wraps the existing form-heavy dispatcher screens in the workspace chrome.
export function LegacyFrame({ title, subtitle, children }: { title: string; subtitle?: string; children: ReactNode }) {
  return (
    <>
      <DpHero title={title} subtitle={subtitle} />
      <div className="dp-body dp-body--flush"><div className="dp-panel dp-legacy">{children}</div></div>
    </>
  );
}

export function MasterDataFrame({ children }: { children: ReactNode }) {
  const { t } = useLocale();
  return <LegacyFrame title={t("Master data")} subtitle={t("Outlets, operating calendar, planning policy, vehicles and incidents.")}>{children}</LegacyFrame>;
}

export function AuditFrame({ children }: { children: ReactNode }) {
  const { t } = useLocale();
  return <LegacyFrame title={t("Audit & KPIs")} subtitle={t("Dispatcher access · audit records are read-only. KPI totals cover the most recent seven days.")}>{children}</LegacyFrame>;
}
