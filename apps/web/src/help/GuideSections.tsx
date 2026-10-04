import { useLocale } from "../i18n";
import { Panel } from "../dispatcher/ui";
import type { GuideSection } from "./guideContent.mjs";

/** One panel per guide section: a heading, then each topic as a short title and explanation. */
export function GuideSections({ sections }: { sections: GuideSection[] }) {
  const { t } = useLocale();
  return (
    <>
      {sections.map((section) => (
        <Panel key={section.title} title={t(section.title)}>
          <dl style={{ margin: 0, display: "grid", gap: 14 }}>
            {section.items.map((item) => (
              <div key={item.title}>
                <dt style={{ fontWeight: 600, marginBottom: 4 }}>{t(item.title)}</dt>
                <dd style={{ margin: 0, lineHeight: 1.5 }}>{t(item.text)}</dd>
              </div>
            ))}
          </dl>
        </Panel>
      ))}
    </>
  );
}
