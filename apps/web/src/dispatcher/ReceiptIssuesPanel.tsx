import { useEffect,useState } from "react";
import { apiJSON } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";

type Item={orderRef:string;outletId:string;receipt:{receivedUnits:number;expectedUnits:number;status:string;confirmedAt:string};issue:{issueType:string;affectedUnits:number;note:string;createdAt:string}};
export function ReceiptIssuesPanel(){
 const{user}=useAuth();const{t}=useLocale();const[items,setItems]=useState<Item[]>([]);const[error,setError]=useState("");
 useEffect(()=>{if(!user?.access_token)return;apiJSON<{items:Item[]}>("/orders/receipt-issues",user.access_token).then(v=>setItems(v.items||[])).catch(e=>setError(String(e)));},[user]);
 return <section className="card"><h2>{t("Receipt issues")}</h2><p className="muted">{t("Read-only receipt discrepancies reported by stores.")}</p>{error&&<p className="status-bad" role="alert">{error}</p>}{!error&&!items.length&&<p>{t("No receipt issues reported.")}</p>}{items.length>0&&<table><thead><tr><th>{t("Order")}</th><th>{t("Outlet")}</th><th>{t("Issue")}</th><th>{t("Units")}</th><th>{t("Note")}</th><th>{t("Reported")}</th></tr></thead><tbody>{items.map((item,i)=><tr key={`${item.orderRef}-${i}`}><td>{item.orderRef}</td><td>{item.outletId}</td><td>{t(item.issue.issueType)}</td><td>{item.issue.affectedUnits}</td><td>{item.issue.note||"—"}</td><td>{new Date(item.issue.createdAt).toLocaleString()}</td></tr>)}</tbody></table>}</section>;
}
