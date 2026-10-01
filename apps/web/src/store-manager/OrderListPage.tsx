import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiJSON, Order } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";

export function OrderListPage() {
  const { user } = useAuth();
  const { t } = useLocale();
  const [items, setItems] = useState<Order[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!user?.access_token) return;
    apiJSON<{ items: Order[] }>("/orders", user.access_token)
      .then((b) => setItems(b.items || []))
      .catch((e) => setError(String(e)));
  }, [user]);

  return (
    <section className="card">
      <h2>{t("My Orders")}</h2>
      <p>
        <Link to="/store-manager/orders/new">{t("New order")}</Link>
        {" · "}<Link to="/store-manager/tracking">{t("Track orders")}</Link>{" · "}<Link to="/store-manager/receipts">{t("Pending receipts")}</Link>
      </p>
      {error && <p className="status-bad" role="alert">{error}</p>}
      <ul>
        {items.map((o) => (
          <li key={o.id}>
            <strong>{o.orderRef}</strong> {o.requestedDeliveryDate} {o.temperatureRequirement} {o.orderVolumeM3} m³{" "}
            {o.status}
          </li>
        ))}
      </ul>
    </section>
  );
}
