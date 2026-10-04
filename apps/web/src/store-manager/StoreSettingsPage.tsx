import { useState } from "react";
import { Locale, useLocale } from "../i18n";
import { CONTRAST_KEY, SIZE_KEY, TextSize, applyDisplayPreferences, readPreference, writePreference } from "../displayPreferences";
import { StoreManagerHero } from "./StoreManagerHero";
import { DisplayNameForm } from "../auth/DisplayNameForm";

export function StoreSettingsPage() {
  const { t, locale, setLocale } = useLocale();
  const [lang, setLang] = useState<Locale>(locale);
  const [size, setSize] = useState<TextSize>(() => (readPreference(SIZE_KEY) as TextSize) || "normal");
  const [contrast, setContrast] = useState(() => readPreference(CONTRAST_KEY) === "true");
  const [saved, setSaved] = useState(false);

  function save() {
    writePreference(SIZE_KEY, size);
    writePreference(CONTRAST_KEY, String(contrast));
    applyDisplayPreferences();
    if (lang !== locale) setLocale(lang);
    setSaved(true);
  }
  const languages: [Locale, string][] = [["en", "English"], ["si", "සිංහල"], ["ta", "தமிழ்"]];

  return (
    <>
      <StoreManagerHero compact title={t("Language & display")} subtitle={t("Choose how Waypoint labels and notices appear for this account.")} />
      <div className="sm-page-body dp-stack">
        <section className="dp-panel">
          <div className="dp-panel-head"><h2 className="dp-panel-title">{t("Account name")}</h2></div>
          <div className="dp-panel-body"><DisplayNameForm /></div>
        </section>
        <div className="dp-grid-2 dp-grid-2--even">
          <section className="dp-panel" aria-labelledby="lang-heading">
            <div className="dp-panel-head"><div><h2 className="dp-panel-title" id="lang-heading">{t("Interface language")}</h2><p className="dp-panel-sub">{t("This changes labels, dates and system notices for your account.")}</p></div></div>
            <div className="dp-panel-body">
              <div className="sm-lang-options" role="group" aria-label={t("Interface language")}>
                {languages.map(([code, name]) => (
                  <button key={code} type="button" className="sm-lang-option" aria-pressed={lang === code} lang={code} onClick={() => { setLang(code); setSaved(false); }}>
                    <strong>{name}</strong>
                    <span className="muted">{lang === code ? t("Selected") : t("Preview")}</span>
                  </button>
                ))}
              </div>
              <p className="muted" style={{ fontSize: "0.8125rem" }}>{t("Changing the language keeps your current task and unsaved work.")}</p>
            </div>
          </section>
          <section className="dp-panel" aria-labelledby="display-heading">
            <div className="dp-panel-head"><div><h2 className="dp-panel-title" id="display-heading">{t("Reading & interaction")}</h2><p className="dp-panel-sub">{t("Display settings apply across desktop, tablet and phone.")}</p></div></div>
            <div className="dp-panel-body">
              <dl className="dp-kv-rows">
                <div><dt>{t("Text size")}</dt><dd><div className="dp-chips" role="group" aria-label={t("Text size")}>{([["normal", t("Standard")], ["large", t("Large")]] as [TextSize, string][]).map(([v, l]) => <button key={v} type="button" className="dp-chip" aria-pressed={size === v} onClick={() => { setSize(v); setSaved(false); }}>{l}</button>)}</div></dd></div>
                <div><dt>{t("Contrast")}</dt><dd><div className="dp-chips" role="group" aria-label={t("Contrast")}><button type="button" className="dp-chip" aria-pressed={!contrast} onClick={() => { setContrast(false); setSaved(false); }}>{t("Default")}</button><button type="button" className="dp-chip" aria-pressed={contrast} onClick={() => { setContrast(true); setSaved(false); }}>{t("High contrast")}</button></div></dd></div>
                <div><dt>{t("Keyboard focus")}</dt><dd>{t("Visible focus outline on interactive controls")}</dd></div>
                <div><dt>{t("Touch target")}</dt><dd>{t("Minimum 44 × 44 px on phone and tablet")}</dd></div>
              </dl>
            </div>
          </section>
        </div>
        <div className="dp-row" style={{ justifyContent: "flex-end" }}>
          {saved && <span className="dp-tag dp-tag--green" role="status">{t("Preferences saved on this device")}</span>}
          <button type="button" className="dp-btn" onClick={save}>{t("Save preferences")}</button>
        </div>
      </div>
    </>
  );
}
