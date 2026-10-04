import { ChangeEvent, useEffect, useState } from "react";
import { apiFetch, apiJSON, Order } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";

export function OrderQueuePage() {
  const { user } = useAuth();
  const { t } = useLocale();
  const [items, setItems] = useState<Order[]>([]);
  const [brand, setBrand] = useState("");
  const [outlet, setOutlet] = useState("");
  const [error, setError] = useState("");
  const [sourceSystem, setSourceSystem] = useState("erp");
  const [importStatus, setImportStatus] = useState("");
  const [exportUrl, setExportUrl] = useState("");
  const [importing, setImporting] = useState(false);
  const [outletMap, setOutletMap] = useState<Map<string, string>>(new Map());

  useEffect(() => {
    if (!user?.access_token) return;
    apiJSON<{ items: Array<{ id: string; parkingConstraint: string }> }>("/shared/outlets", user.access_token)
      .then((data) => {
        const m = new Map<string, string>();
        for (const item of data.items || []) {
          m.set(item.id, item.parkingConstraint);
        }
        setOutletMap(m);
      })
      .catch(() => {});
  }, [user]);

  useEffect(() => {
    if (!user?.access_token) return;
    const q = new URLSearchParams();
    if (brand) q.set("brand", brand);
    if (outlet) q.set("outlet_id", outlet);
    const suffix = q.toString() ? `?${q.toString()}` : "";

    function fetchOrders() {
      apiJSON<{ items: Order[] }>(`/orders${suffix}`, user!.access_token)
        .then((b) => setItems(b.items || []))
        .catch((e) => setError(String(e)));
    }

    fetchOrders();
    const interval = setInterval(fetchOrders, 4000);
    return () => clearInterval(interval);
  }, [user, brand, outlet]);

  useEffect(() => () => {
    if (exportUrl) URL.revokeObjectURL(exportUrl);
  }, [exportUrl]);

  async function exportCSV() {
    if (!user?.access_token) return;
    try {
      const response = await apiFetch("/orders/export.csv", user.access_token);
      if (!response.ok) throw new Error(await response.text());
      const blob = await response.blob();
      setExportUrl(URL.createObjectURL(blob));
    }
    catch(e){setError(e instanceof Error?e.message:String(e));}
  }

  async function importCSV(event: ChangeEvent<HTMLInputElement>) {
    const file=event.target.files?.[0]; if(!file||!user?.access_token)return;
    setImporting(true);setImportStatus("");setError("");
    try { const query=new URLSearchParams({version:"1",sourceSystem}); const response=await apiFetch(`/orders/import.csv?${query}`,user.access_token,{method:"POST",headers:{"Content-Type":"text/csv"},body:await file.text()}); if(!response.ok)throw new Error(await response.text()); const result=await response.json() as {created:number;duplicates:number}; setImportStatus(`${t("Imported")} ${result.created} ${t("new orders;")} ${result.duplicates} ${t("exact replays skipped.")}`); const b=await apiJSON<{items:Order[]}>("/orders",user.access_token);setItems(b.items||[]); }
    catch(e){setError(e instanceof Error?e.message:String(e));}
    finally {setImporting(false);event.target.value="";}
  }

  return (
    <section className="card">
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center" }}>
        <h2>{t("Confirmed Orders")}</h2>
        <span className="chip status-badge" style={{ background: "#ecfdf5", color: "#065f46", fontSize: "0.85rem" }}>
          ● {t("Live queue active")}
        </span>
      </div>
      {error && <p className="status-bad" role="alert">{error}</p>}
      {importStatus && <p role="status" className="status-ok">{importStatus}</p>}
      <section className="row" aria-label={t("ERP and WMS data exchange")}>
        <button type="button" onClick={()=>void exportCSV()}>{t("Export CSV v1")}</button>
        {exportUrl && <p role="status" className="status-ok">{t("CSV ready.")} <a href={exportUrl} download="waypoint-orders-v1.csv">{t("Download CSV v1")}</a></p>}
        <label>{t("Source system")}<input value={sourceSystem} onChange={e=>setSourceSystem(e.target.value)} maxLength={40} pattern="[A-Za-z0-9_-]+" /></label>
        <label>{t("Import CSV v1")}<input type="file" accept=".csv,text/csv" onChange={e=>void importCSV(e)} disabled={importing} /></label>
      </section>
      <p className="muted">{t("CSV v1 requires a source system and external order ID. Imports validate the full file and replaying the same IDs and values is safe.")}</p>
      <p>
        <label>{t("Brand")}<input aria-label={t("Filter by brand")} value={brand} onChange={(e) => setBrand(e.target.value)} /></label>
        <label>{t("Outlet")}<input aria-label={t("Filter by outlet")} value={outlet} onChange={(e) => setOutlet(e.target.value)} /></label>
      </p>
      <table>
        <thead>
          <tr>
            <th>{t("Order")}</th>
            <th>{t("Outlet")}</th>
            <th>{t("Brand")}</th>
            <th>{t("Cooling")}</th>
            <th>{t("Access")}</th>
            <th>{t("Delivery date")}</th>
            <th>{t("Volume")}</th>
          </tr>
        </thead>
        <tbody>
          {items.map((o) => {
            const isChilled = o.temperatureRequirement === "chilled";
            const isVanOnly = outletMap.get(o.outletId) === "van_only";
            return (
              <tr key={o.id}>
                <td><strong>{o.orderRef}</strong></td>
                <td>{o.outletId}</td>
                <td>{o.brand}</td>
                <td>
                  {isChilled ? (
                    <span className="chip" style={{ background: "#e0f2fe", color: "#0369a1", fontWeight: 600 }}>
                      ❄️ {t("Chilled")}
                    </span>
                  ) : (
                    <span className="muted">{t("Ambient")}</span>
                  )}
                </td>
                <td>
                  {isVanOnly ? (
                    <span className="chip" style={{ background: "#fef3c7", color: "#92400e", fontWeight: 600 }}>
                      🚐 {t("Van only")}
                    </span>
                  ) : (
                    <span className="muted">{t("Standard")}</span>
                  )}
                </td>
                <td>{o.requestedDeliveryDate}</td>
                <td>{o.orderUnits} {t("units")} · {o.orderVolumeM3} m³</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </section>
  );
}
