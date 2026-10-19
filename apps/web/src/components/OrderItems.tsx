import { OrderLine } from "../api/client";
import { useLocale } from "../i18n";
import "./orderItems.css";

const kg = (v: number) => `${v.toLocaleString(undefined, { maximumFractionDigits: 2 })} kg`;
const m3 = (v: number) => `${v.toLocaleString(undefined, { maximumFractionDigits: 3 })} m³`;

/** The item list of one order, read-only. Orders placed before item lines existed show a plain note instead. */
export function OrderItems({ lines, units }: { lines?: OrderLine[]; units: number }) {
  const { t } = useLocale();
  if (!lines || lines.length === 0) {
    return <p className="order-items-none muted">{t("No item details were recorded for this order. It only has totals:")} {units} {t("boxes")}.</p>;
  }
  const weight = lines.reduce((sum, l) => sum + l.weightKg, 0);
  const volume = lines.reduce((sum, l) => sum + l.volumeM3, 0);
  const packs = lines.reduce((sum, l) => sum + l.packQty, 0);
  return (
    <div className="order-items-wrap">
      <table className="order-items">
        <caption className="visually-hidden">{t("Items on this order")}</caption>
        <thead>
          <tr>
            <th scope="col">{t("Item")}</th>
            <th scope="col" className="num">{t("Boxes")}</th>
            <th scope="col" className="num">{t("Weight")}</th>
            <th scope="col" className="num">{t("Volume")}</th>
          </tr>
        </thead>
        <tbody>
          {lines.map((l) => (
            <tr key={l.lineNo}>
              <th scope="row">
                {l.productName}
                <span className="order-items-sub">{l.packQty} × {l.pack}{l.unitsPerPack > 1 ? ` · ${l.unitsPerPack} ${t("per")} ${l.pack}` : ""}</span>
              </th>
              <td className="num">{l.packQty}</td>
              <td className="num">{kg(l.weightKg)}</td>
              <td className="num">{m3(l.volumeM3)}</td>
            </tr>
          ))}
        </tbody>
        <tfoot>
          <tr>
            <th scope="row">{t("Total")}</th>
            <td className="num">{packs}</td>
            <td className="num">{kg(weight)}</td>
            <td className="num">{m3(volume)}</td>
          </tr>
        </tfoot>
      </table>
    </div>
  );
}
