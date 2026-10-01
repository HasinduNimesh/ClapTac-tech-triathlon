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

  useEffect(() => {
    if (!user?.access_token) return;
    const q = new URLSearchParams();
    if (brand) q.set("brand", brand);
    if (outlet) q.set("outlet_id", outlet);
    const suffix = q.toString() ? `?${q.toString()}` : "";
    apiJSON<{ items: Order[] }>(`/orders${suffix}`, user.access_token)
      .then((b) => setItems(b.items || []))
      .catch((e) => setError(String(e)));
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
      <h2>{t("Confirmed Orders")}</h2>
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
            <th>{t("Temp")}</th>
            <th>{t("Volume")}</th>
          </tr>
        </thead>
        <tbody>
          {items.map((o) => (
            <tr key={o.id}>
              <td>{o.orderRef}</td>
              <td>{o.outletId}</td>
              <td>{o.brand}</td>
              <td>{o.temperatureRequirement}</td>
              <td>{o.orderVolumeM3} m³</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
