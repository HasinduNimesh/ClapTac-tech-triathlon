import { useLocale } from "../i18n";
import { GuideSections } from "../help/GuideSections";
import { STORE_MANAGER_GUIDE } from "../help/guideContent.mjs";
import { StoreManagerHero } from "./StoreManagerHero";

export function StoreManagerHelpPage() {
  const { t } = useLocale();
  return (
    <>
      <StoreManagerHero compact crumbs={[{ label: t("Store"), to: "/store-manager" }, { label: t("Help & Guide") }]} title={t("Help & Guide")} subtitle={t("How to order, follow a delivery, receive goods and keep up with notices.")} />
      <div className="sm-page-body dp-stack">
        <GuideSections sections={STORE_MANAGER_GUIDE} />
      </div>
    </>
  );
}
