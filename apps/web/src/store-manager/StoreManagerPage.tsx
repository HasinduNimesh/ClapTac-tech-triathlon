import { Link } from "react-router-dom";
import { AgentAssistant } from "../dispatcher/AgentAssistant";
import { useEffect, useState } from "react";
import { apiJSON, ApiError } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";

type Outlet = { id:string; brand:string; name:string; accessInstructions:string; accessInstructionsUpdatedBy?:string; accessInstructionsUpdatedAt?:string; accessInstructionsConfirmedBy?:string; accessInstructionsConfirmedAt?:string; version:number };

export function StoreManagerPage() {
  const { t } = useLocale(); const {user,profile}=useAuth(); const token=user?.access_token||"";
  const [outlets,setOutlets]=useState<Outlet[]>([]);const [busy,setBusy]=useState("");const [error,setError]=useState("");const [notice,setNotice]=useState("");
  useEffect(()=>{let active=true;if(!token||!profile)return;void apiJSON<{items:Outlet[]}>("/shared/outlets",token).then(data=>{if(active){const allowed=new Set(profile.outletIds||[]);setOutlets((data.items||[]).filter(o=>allowed.has(o.id)));}}).catch(e=>{if(active)setError(e instanceof Error?e.message:t("Outlet access notes could not be loaded"));});return()=>{active=false;};},[token,profile,t]);
  async function confirm(outlet:Outlet){setBusy(outlet.id);setError("");setNotice("");try{const data=await apiJSON<{outlet:Outlet}>(`/shared/outlets/${encodeURIComponent(outlet.id)}/access-instructions/confirm`,token,{method:"POST",body:JSON.stringify({expectedVersion:outlet.version})});setOutlets(items=>items.map(item=>item.id===outlet.id?data.outlet:item));setNotice(`${t("Access note confirmed for")} ${outlet.name}.`);}catch(e){setError(e instanceof ApiError&&e.status===409?t("This outlet note changed. Reload before confirming."):e instanceof Error?e.message:t("Access note confirmation failed"));}finally{setBusy("");}}
  return (
    <>
    <section className="card">
      <h2>{t("Store Manager")}</h2>
      <p>
        <Link to="/store-manager/orders/new">{t("Create an order")}</Link>,{" "}
        <Link to="/store-manager/orders">{t("view your outlet orders")}</Link>,{" "}
        <Link to="/store-manager/tracking">{t("track delivery")}</Link>, {t("or")}{" "}
        <Link to="/store-manager/receipts">{t("confirm a receipt")}</Link>.
      </p>
    </section>
    {outlets.some(o=>o.accessInstructions)&&<section className="card"><h3>{t("Confirm outlet access instructions")}</h3><p>{t("Check the landmark, gate and last 200 metres approach. Confirm the note is still correct; it will be reviewed again in 90 days.")}</p>
      {error&&<p className="status-bad" role="alert">{error}</p>}{notice&&<p role="status">{notice}</p>}
      {outlets.filter(o=>o.accessInstructions).map(outlet=><article className="card" key={outlet.id}><h4>{outlet.name} · {outlet.id}</h4><p>{outlet.accessInstructions}</p><p className="muted">{outlet.accessInstructionsUpdatedBy&&`${t("Updated by")} ${outlet.accessInstructionsUpdatedBy} · `}{outlet.accessInstructionsUpdatedAt?new Date(outlet.accessInstructionsUpdatedAt).toLocaleDateString("en-LK",{timeZone:"Asia/Colombo"}):t("Update date unavailable")}{outlet.accessInstructionsConfirmedAt?` · ${t("Last confirmed")} ${new Date(outlet.accessInstructionsConfirmedAt).toLocaleDateString("en-LK",{timeZone:"Asia/Colombo"})}`:` · ${t("Not yet confirmed")}`}</p><button type="button" disabled={!!busy} onClick={()=>void confirm(outlet)}>{busy===outlet.id?t("Confirming…"):t("Confirm this note is accurate")}</button></article>)}
    </section>}
    <AgentAssistant />
    </>
  );
}
