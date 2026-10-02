import { Link } from "react-router-dom";
import { useLocale } from "../i18n";
import { StoreManagerHero } from "./StoreManagerHero";

export function StoreManagerHelpPage() {
  const { t } = useLocale();
  return (
    <>
      <StoreManagerHero
        compact
        crumbs={[{ label: t("Store"), to: "/store-manager" }, { label: t("Help & Guide") }]}
        title={t("Help & Guide")}
        subtitle={t("How ordering, delivery tracking and receipts work.")}
      />
      <div className="sm-page-body">
        <section className="sm-form-card sm-help-actions" aria-labelledby="help-actions-heading">
          <h2 id="help-actions-heading" className="sm-form-card-title">{t("Quick actions")}</h2>
          <div className="sm-help-action-row">
            <Link to="/store-manager/orders/new" className="sm-btn-secondary">{t("Place an order")}</Link>
            <Link to="/store-manager/orders" className="sm-btn-secondary">{t("View Your Orders")}</Link>
            <Link to="/store-manager/receipts" className="sm-btn-secondary">{t("Receipt confirmation")}</Link>
            <Link to="/store-manager/notifications" className="sm-btn-secondary">{t("Notifications")}</Link>
          </div>
        </section>

        <div className="sm-help-grid">
          <section className="sm-form-card" aria-labelledby="help-ordering-heading">
            <h2 id="help-ordering-heading" className="sm-form-card-title">{t("Placing an order")}</h2>
            <ol className="sm-help-steps">
              <li>{t("Open Place Orders from the menu.")}</li>
              <li>{t("Choose the delivery date and whether the goods are ambient or chilled. Each type needs its own order.")}</li>
              <li>{t("Enter the number of units, the total weight and the total volume.")}</li>
              <li>{t("Check the review panel, then submit. You will get a request reference.")}</li>
            </ol>
          </section>

          <section className="sm-form-card" aria-labelledby="help-cutoff-heading">
            <h2 id="help-cutoff-heading" className="sm-form-card-title">{t("Ordering cutoff")}</h2>
            <p className="muted sm-help-text">{t("Daily cutoff: 4:00 PM Sri Lanka time. Requests made after cutoff move to the next operating day.")}</p>
            <p className="muted sm-help-text">{t("Individual delivery windows still apply.")}</p>
          </section>

          <section className="sm-form-card" aria-labelledby="help-tracking-heading">
            <h2 id="help-tracking-heading" className="sm-form-card-title">{t("Following a delivery")}</h2>
            <p className="muted sm-help-text">{t("The Orders page shows each order's progress from placed to planned, on route, delivered and receipt confirmed.")}</p>
            <p className="muted sm-help-text">{t("If an order is deferred, the order page explains why and what happens next.")}</p>
            <p className="muted sm-help-text">{t("ETA reflects reported events; this is not continuous GPS tracking.")}</p>
          </section>

          <section className="sm-form-card" aria-labelledby="help-receipt-heading">
            <h2 id="help-receipt-heading" className="sm-form-card-title">{t("Confirming what you received")}</h2>
            <p className="muted sm-help-text">{t("After the driver records a delivery, open Receipt confirmation and enter the quantity you received.")}</p>
            <p className="muted sm-help-text">{t("Confirm the quantity received after delivery. Report a discrepancy if anything is missing or damaged.")}</p>
            <p className="muted sm-help-text">{t("Any shortage or damage you report is shared with dispatch.")}</p>
          </section>

          <section className="sm-form-card" aria-labelledby="help-notices-heading">
            <h2 id="help-notices-heading" className="sm-form-card-title">{t("Notifications")}</h2>
            <p className="muted sm-help-text">{t("Notifications lists deferred orders and deliveries waiting for your receipt confirmation.")}</p>
          </section>

          <section className="sm-form-card" aria-labelledby="help-contact-heading">
            <h2 id="help-contact-heading" className="sm-form-card-title">{t("Need more help?")}</h2>
            <p className="muted sm-help-text">{t("Contact your dispatcher if your outlet details are wrong or an order is urgent.")}</p>
          </section>
        </div>
      </div>
    </>
  );
}
