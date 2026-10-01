import { FormEvent, useState } from "react";
import { apiJSON, Order } from "../api/client";
import { todayLocal } from "../api/date";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";

export function NewOrderPage() {
  const { user } = useAuth();
  const { t } = useLocale();
  const [date, setDate] = useState(todayLocal);
  const [units, setUnits] = useState("20");
  const [weight, setWeight] = useState("185.5");
  const [volume, setVolume] = useState("2.4");
  const [temp, setTemp] = useState("chilled");
  const [created, setCreated] = useState<Order | null>(null);
  const [error, setError] = useState("");

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      const body = await apiJSON<{ order: Order }>("/orders", user!.access_token, {
        method: "POST",
        body: JSON.stringify({
          requestedDeliveryDate: date,
          orderUnits: Number(units),
          orderWeightKg: Number(weight),
          orderVolumeM3: Number(volume),
          temperatureRequirement: temp,
        }),
      });
      setCreated(body.order);
    } catch (err) {
      setError(String(err));
    }
  }

  if (created) {
    return (
      <section className="card">
          <h2>{t("Order created successfully")}</h2>
          <p>{t("Order")}: {created.orderRef}</p>
          <p>{t("Status")}: {created.status}</p>
          <p>{t("Requested delivery")}: {created.requestedDeliveryDate}</p>
      </section>
    );
  }

  return (
    <section className="card">
      <h2>{t("New Order")}</h2>
      {error && <p className="status-bad" role="alert">{error}</p>}
      <p>{t("Daily cutoff: 4:00 PM Sri Lanka time. Requests made after cutoff move to the next operating day.")}</p>
      <form onSubmit={onSubmit}>
        <p>
          <label htmlFor="requested-delivery-date">{t("Requested Delivery Date")}</label>
          <br />
          <input id="requested-delivery-date" type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
        </p>
        <p>
          <label htmlFor="order-units">{t("Units")}</label>
          <br />
          <input id="order-units" type="number" min="1" step="1" value={units} onChange={(e) => setUnits(e.target.value)} required />
        </p>
        <p>
          <label htmlFor="order-weight">{t("Weight (kg)")}</label>
          <br />
          <input id="order-weight" type="number" min="0.01" step="0.01" value={weight} onChange={(e) => setWeight(e.target.value)} required />
        </p>
        <p>
          <label htmlFor="order-volume">{t("Volume (m³)")}</label>
          <br />
          <input id="order-volume" type="number" min="0.01" step="0.01" value={volume} onChange={(e) => setVolume(e.target.value)} required />
        </p>
        <div className="form-field">
          <fieldset>
          <legend>{t("Temperature")}</legend>
          <label>
            <input type="radio" name="temp" checked={temp === "chilled"} onChange={() => setTemp("chilled")} /> {t("Chilled")}
          </label>
          <label>
            <input type="radio" name="temp" checked={temp === "ambient"} onChange={() => setTemp("ambient")} /> {t("Ambient")}
          </label>
          </fieldset>
        </div>
        <button type="submit">{t("Submit Order")}</button>
      </form>
    </section>
  );
}
