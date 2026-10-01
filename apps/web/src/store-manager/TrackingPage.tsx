import { FormEvent, useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiJSON, Order } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { deferralExplanation } from "./deferralMessage.mjs";

type ReceiptIssue = { id:string; issueType:string; affectedUnits:number; note?:string };
type Receipt = { id:string; expectedUnits:number; receivedUnits:number; status:string; confirmedAt:string };
type CustodyEvent = { id:string;stage:string;sealId:string;serialNumbers:string[];condition:string;evidenceRef?:string;receiverName?:string;recordedBy:string;recordedAt:string };
type Tracking = {
  stage:string;
  order:Order;
  planning:{state:string;planRef?:string;stopSequence?:number;reasonCode?:string;reasonComment?:string;plannedArrivalAt?:string;plannedServiceStartAt?:string};
  delivery?:{runStatus:string;outcome?:string;reason?:string;completedAt?:string;proofs:{type:string;mimeType:string;pending:boolean;receiverName?:string}[];loadingShortfallSummary?:unknown[]};
  receipt?:Receipt;
  receiptIssues:ReceiptIssue[];
  custody?:CustodyEvent[];
};
type Pending = {order:Order;tracking:Tracking};

function idempotencyKey(){return typeof crypto!="undefined"&&"randomUUID" in crypto?crypto.randomUUID():`receipt-${Date.now()}-${Math.random().toString(16).slice(2)}`;}

export function TrackingPage({receiptsOnly=false}:{receiptsOnly?:boolean}){
  const {user}=useAuth();const {t}=useLocale();const token=user?.access_token||"";
  const [items,setItems]=useState<Tracking[]>([]);const [pending,setPending]=useState<Pending[]>([]);const [received,setReceived]=useState<Record<string,string>>({});const [issueType,setIssueType]=useState<Record<string,string>>({});const [note,setNote]=useState<Record<string,string>>({});const [issueUnits,setIssueUnits]=useState<Record<string,string>>({});const [reportFor,setReportFor]=useState("");const [error,setError]=useState("");const [message,setMessage]=useState("");const [techReceipt,setTechReceipt]=useState<Record<string,{sealId:string;serials:string;condition:string;receiver:string}>>({});const [custodyKeys,setCustodyKeys]=useState<Record<string,string>>({});
  const load=useCallback(async()=>{
    if(!token)return;setError("");
    try{
      if(receiptsOnly){const body=await apiJSON<{items:Pending[]}>("/orders/receipts/pending",token);setPending(body.items||[]);return;}
      const body=await apiJSON<{items:Order[]}>("/orders",token);
      const rows=await Promise.all((body.items||[]).map(async order=>{
        const result=await apiJSON<{tracking:Tracking}>(`/orders/${order.id}/tracking`,token);return result.tracking;
      }));setItems(rows);
    }catch(e){setError(String(e));}
  },[token,receiptsOnly]);
  useEffect(()=>{void load();},[load]);

  async function confirm(e:FormEvent,order:Order,tracking:Tracking){
    e.preventDefault();setError("");setMessage("");const units=Number(received[order.id]??order.orderUnits);const issue=units<order.orderUnits;
    const tech=order.brand.toLowerCase()==="tech";const loaded=tracking.custody?.[0];const custody=techReceipt[order.id]||{sealId:loaded?.sealId||"",serials:(loaded?.serialNumbers||[]).join(", "),condition:"",receiver:""};
    if(tech&&(!custody.sealId.trim()||!custody.serials.trim()||!custody.condition.trim()||!custody.receiver.trim())){setError(t("Tech receipt requires the seal, serials, received condition, and receiver name."));return;}
    const payload:{receivedUnits:number;issue?:{issueType:string;affectedUnits:number;note:string;idempotencyKey:string}}={receivedUnits:units};
    if(issue)payload.issue={issueType:issueType[order.id]||"MISSING",affectedUnits:order.orderUnits-units,note:note[order.id]||"",idempotencyKey:idempotencyKey()};
    try{await apiJSON(`/orders/${order.id}/receipt/confirm`,token,{method:"POST",headers:{"Idempotency-Key":payload.issue?.idempotencyKey||idempotencyKey()},body:JSON.stringify(payload)});
      if(tech){const key=custodyKeys[order.id]||idempotencyKey();setCustodyKeys(v=>({...v,[order.id]:key}));await apiJSON(`/orders/${order.id}/custody`,token,{method:"POST",headers:{"Idempotency-Key":key},body:JSON.stringify({stage:"RECEIVED",sealId:custody.sealId.trim(),serialNumbers:custody.serials.split(/[\n,;]/).map(v=>v.trim()).filter(Boolean),condition:custody.condition.trim(),receiverName:custody.receiver.trim(),idempotencyKey:key})});}
      setMessage(`${t("Receipt saved for")} ${order.orderRef}.`);await load();}
    catch(err){setError(String(err));}
    void tracking;
  }

  async function reportIssue(e:FormEvent,order:Order){
    e.preventDefault();setError("");setMessage("");const key=idempotencyKey();
    try{await apiJSON(`/orders/${order.id}/receipt/issues`,token,{method:"POST",headers:{"Idempotency-Key":key},body:JSON.stringify({issueType:issueType[order.id]||"DAMAGED",affectedUnits:Number(issueUnits[order.id]||0),note:note[order.id]||"",idempotencyKey:key})});setMessage(`${t("Issue recorded for")} ${order.orderRef}.`);setReportFor("");await load();}
    catch(err){setError(String(err));}
  }

  return <section className="card">
    <h2>{receiptsOnly?t("Receipt confirmation"):t("Order tracking")}</h2>
    <p>{receiptsOnly?t("Confirm the quantity received after delivery. Report a discrepancy if anything is missing or damaged."):t("Follow each order from confirmation through planning, delivery and store receipt.")}</p>
    {!receiptsOnly&&<p><Link to="/store-manager/receipts">{t("Open pending receipts")}</Link></p>}
    {error&&<p className="status-bad" role="alert">{error}</p>}{message&&<p role="status">{message}</p>}
    {receiptsOnly?pending.map(({order,tracking})=><article className="card" key={order.id}>
      <h3>{order.orderRef} · {order.brand}</h3><p>{order.orderUnits} {t("expected units")} · {t("Driver outcome")}: {t(tracking.delivery?.outcome||"")}</p>
      {(tracking.delivery?.loadingShortfallSummary?.length||0)>0&&<p className="status-bad">{t("Loading shortfall reported")} {JSON.stringify(tracking.delivery?.loadingShortfallSummary)}</p>}
      {order.brand.toLowerCase()==="tech"&&<fieldset><legend>{t("High-value Tech custody")}</legend><label>{t("Seal ID")}<input value={techReceipt[order.id]?.sealId??tracking.custody?.[0]?.sealId??""} onChange={e=>setTechReceipt(v=>({...v,[order.id]:{sealId:e.target.value,serials:v[order.id]?.serials??(tracking.custody?.[0]?.serialNumbers||[]).join(", "),condition:v[order.id]?.condition||"",receiver:v[order.id]?.receiver||""}}))}/></label><label>{t("Serial number(s), comma separated")}<textarea value={techReceipt[order.id]?.serials??(tracking.custody?.[0]?.serialNumbers||[]).join(", ")} onChange={e=>setTechReceipt(v=>({...v,[order.id]:{sealId:v[order.id]?.sealId??tracking.custody?.[0]?.sealId??"",serials:e.target.value,condition:v[order.id]?.condition||"",receiver:v[order.id]?.receiver||""}}))}/></label><label>{t("Received condition")}<input value={techReceipt[order.id]?.condition||""} onChange={e=>setTechReceipt(v=>({...v,[order.id]:{sealId:v[order.id]?.sealId??tracking.custody?.[0]?.sealId??"",serials:v[order.id]?.serials??(tracking.custody?.[0]?.serialNumbers||[]).join(", "),condition:e.target.value,receiver:v[order.id]?.receiver||""}}))}/></label><label>{t("Receiver name")}<input value={techReceipt[order.id]?.receiver||""} onChange={e=>setTechReceipt(v=>({...v,[order.id]:{sealId:v[order.id]?.sealId??tracking.custody?.[0]?.sealId??"",serials:v[order.id]?.serials??(tracking.custody?.[0]?.serialNumbers||[]).join(", "),condition:v[order.id]?.condition||"",receiver:e.target.value}}))}/></label></fieldset>}
      <form onSubmit={e=>confirm(e,order,tracking)}>
        <label>{t("Received units")} <input type="number" min="1" max={order.orderUnits} value={received[order.id]??String(order.orderUnits)} onChange={e=>setReceived({...received,[order.id]:e.target.value})} required /></label>
        {Number(received[order.id]??order.orderUnits)<order.orderUnits&&<fieldset><legend>{t("Discrepancy required")}</legend>
          <label>{t("Issue type")} <select value={issueType[order.id]||"MISSING"} onChange={e=>setIssueType({...issueType,[order.id]:e.target.value})}><option value="MISSING">{t("MISSING")}</option><option value="DAMAGED">{t("DAMAGED")}</option><option value="QUANTITY_MISMATCH">{t("QUANTITY_MISMATCH")}</option><option value="OTHER">{t("OTHER")}</option></select></label>
          <label>{t("Note")} <textarea value={note[order.id]||""} onChange={e=>setNote({...note,[order.id]:e.target.value})} maxLength={1000}/></label>
        </fieldset>}
        <button type="submit">{Number(received[order.id]??order.orderUnits)<order.orderUnits?t("Report Issue & Confirm Receipt"):t("Confirm Receipt")}</button>
      </form>
    </article>):items.map(row=><article className="card" key={row.order.id}>
      <h3>{row.order.orderRef} · {row.order.brand}</h3><p>{t("Requested")} {row.order.requestedDeliveryDate} · {row.order.orderUnits} {t("units")} · {t(row.stage.replace(/_/g," "))}</p>
      {row.planning.plannedArrivalAt&&<p>{t("Planned arrival")}: {new Date(row.planning.plannedArrivalAt).toLocaleString()}</p>}
      {row.planning.reasonCode&&(()=>{const why=deferralExplanation(row.planning.reasonCode);return <div className="status-bad" role="status"><p><strong>{t("Deferred")}</strong>: {t(why.message)}{row.planning.reasonComment?` · ${row.planning.reasonComment}`:""}</p><p>{t("What happens next")}: {t(why.nextAction)}</p></div>;})()}
      {(row.custody||[]).length>0&&<section className="card"><h4>{t("Tech chain of custody")}</h4><ol>{row.custody!.map(event=><li key={event.id}>{t(event.stage)} · {t("Seal ID")} {event.sealId} · {event.serialNumbers.join(", ")} · {event.condition} · {event.recordedBy} · {new Date(event.recordedAt).toLocaleString()}{event.receiverName?` · ${event.receiverName}`:""}{event.evidenceRef?` · ${t("Evidence reference")} ${event.evidenceRef}`:""}</li>)}</ol></section>}
      {row.delivery&&<p>{t("Driver outcome")}: {t(row.delivery.outcome||row.delivery.runStatus)}{row.delivery.reason?` · ${t(row.delivery.reason)}`:""} · {row.delivery.proofs?.length||0} {t("proof item(s)")}{row.delivery.proofs?.find((p)=>p.receiverName)&&` · ${t("Received by")} ${row.delivery.proofs.find((p)=>p.receiverName)!.receiverName}`}</p>}
      {(row.delivery?.loadingShortfallSummary?.length||0)>0&&<p className="status-bad">{t("Loading shortfall")}: {JSON.stringify(row.delivery?.loadingShortfallSummary)}</p>}
      {row.receipt?<><p>{t("Receipt")} {t(row.receipt.status.replace(/_/g," "))} · {row.receipt.receivedUnits}/{row.receipt.expectedUnits} {t("units")} · {row.receiptIssues.length} {t("issue(s)")}</p>{row.receiptIssues.map(i=><p key={i.id}>{t(i.issueType)} · {i.affectedUnits} {t("affected units")}{i.note?` · ${i.note}`:""}</p>)}<button type="button" onClick={()=>setReportFor(reportFor===row.order.id?"":row.order.id)}>{t("Report Issue")}</button>{reportFor===row.order.id&&<form onSubmit={e=>reportIssue(e,row.order)}><label>{t("Issue type")} <select value={issueType[row.order.id]||"DAMAGED"} onChange={e=>setIssueType({...issueType,[row.order.id]:e.target.value})}><option value="MISSING">{t("MISSING")}</option><option value="DAMAGED">{t("DAMAGED")}</option><option value="QUANTITY_MISMATCH">{t("QUANTITY_MISMATCH")}</option><option value="OTHER">{t("OTHER")}</option></select></label><label>{t("Affected units")} <input type="number" min="0" max={row.order.orderUnits} value={issueUnits[row.order.id]||"0"} onChange={e=>setIssueUnits({...issueUnits,[row.order.id]:e.target.value})}/></label><label>{t("Note")} <textarea value={note[row.order.id]||""} onChange={e=>setNote({...note,[row.order.id]:e.target.value})} maxLength={1000}/></label><button type="submit">{t("Submit Issue")}</button></form>}</>:row.delivery&&(row.delivery.outcome==="DELIVERED"||row.delivery.outcome==="PARTIAL")&&<p><Link to="/store-manager/receipts">{t("Confirm this receipt")}</Link></p>}
    </article>)}
    {receiptsOnly&&!pending.length&&!error&&<p>{t("No receipts are waiting for confirmation.")}</p>}
    {!receiptsOnly&&!items.length&&!error&&<p>{t("No orders found for this outlet.")}</p>}
  </section>;
}
