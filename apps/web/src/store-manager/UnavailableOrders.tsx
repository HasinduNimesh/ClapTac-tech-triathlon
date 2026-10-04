import { Order } from "../api/client";
import { dayLabel } from "../dispatcher/useApi";
import { useLocale } from "../i18n";

/**
 * Orders the store placed whose progress could not be loaded right now. They are listed, not hidden,
 * and nothing is said about their status because it is not known.
 */
export function UnavailableOrders({ orders, onRetry }: { orders: Order[]; onRetry: () => void }) {
  const { t } = useLocale();
  if (orders.length === 0) return null;
  return (
    <div className="sm-load-note muted" role="status">
      <p style={{ margin: 0 }}>{t("These orders are saved, but their progress could not be loaded right now.")}</p>
      <ul style={{ margin: "6px 0" }}>
        {orders.map((order) => <li key={order.id}>{order.orderRef} · {t(order.brand)} · {dayLabel(order.requestedDeliveryDate)}</li>)}
      </ul>
      <button type="button" className="tap" onClick={onRetry}>{t("Retry")}</button>
    </div>
  );
}
