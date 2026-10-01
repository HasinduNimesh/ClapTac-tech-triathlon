import { FormEvent, useState } from "react";
import { ApiError, apiJSON } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";

type Approval = { id:string; tool:string; args:Record<string,unknown>; args_hash:string; status:string; expires_at:string };
type ChatResponse = { reply?:string; sources?:string[]; pendingAction?:Approval; verifiedResult?:unknown };

export function AgentAssistant(){
  const {user,profile}=useAuth();
  const {t}=useLocale();
  const [question,setQuestion]=useState("");
  const [reply,setReply]=useState("");
  const [sources,setSources]=useState<string[]>([]);
  const [approval,setApproval]=useState<Approval|null>(null);
  const [verified,setVerified]=useState<unknown>();
  const [error,setError]=useState("");
  const [busy,setBusy]=useState(false);
  const token=user?.access_token||"";
  const isDispatcher=profile?.roles?.includes("DISPATCHER")||false;
  async function send(e:FormEvent){
    e.preventDefault();setBusy(true);setError("");setReply("");setApproval(null);setVerified(undefined);setSources([]);
    try{const result=await apiJSON<ChatResponse>("/agent/chat",token,{method:"POST",body:JSON.stringify({message:question})});setReply(result.reply||"");setSources(result.sources||[]);setApproval(result.pendingAction||null);setVerified(result.verifiedResult);}
    catch(err){const status=err instanceof ApiError?err.status:0;setError(status===503?t("The assistant is unavailable right now. Orders, planning, loading and delivery remain available."):String(err));}
    finally{setBusy(false);}
  }
  async function decide(approved:boolean){if(!approval)return;setBusy(true);setError("");
    try{const result=await apiJSON<{executed:boolean;result?:unknown;approval:Approval}>(`/agent/approvals/${approval.id}/decide`,token,{method:"POST",body:JSON.stringify({approved,reason:approved?"Approved by the signed-in user":"Rejected by the signed-in user"})});setApproval(null);setReply(approved?t("Approved action sent to the business service for authorization and validation."):t("Action rejected; no business service was called."));setVerified(result.result);}
    catch(err){setError(String(err));}
    finally{setBusy(false);}
  }
  if(!token||!profile)return <section className="card"><h2>{t("Waypoint assistant")}</h2><p>{t("Sign in with a dispatcher or store-manager account to use the assistant.")}</p></section>;
  return <section className="card agent-panel" aria-label={t("Waypoint assistant")}>
    <h2>{t("Waypoint assistant")}</h2>
    <p>{t(isDispatcher?"Ask about orders, plans, fleet, loading, delivery or receipts.":"Ask about your outlet orders, tracking, receipts or draft a new order.")} {t("Verified business data comes from Waypoint services.")}</p>
    <form onSubmit={send}>
      <label htmlFor="agent-question">{t("Question")}</label>
      <textarea id="agent-question" value={question} onChange={e=>setQuestion(e.target.value)} maxLength={4000} required placeholder={t(isDispatcher?"Why was this order deferred?":"Summarize my next delivery.")} />
      <button type="submit" disabled={busy||!question.trim()}>{busy?t("Working…"):t("Ask assistant")}</button>
    </form>
    {error&&<p className="status-bad" role="alert">{error}</p>}
    {reply&&<div role="status"><h3>{t("Assistant")}</h3><p>{reply}</p></div>}
    {sources.length>0&&<p>{t("Sources:")} {sources.join(", ")}</p>}
    {verified!==undefined&&<details><summary>{t("Verified service result")}</summary><pre>{JSON.stringify(verified,null,2)}</pre></details>}
    {approval&&<div className="agent-approval" role="group" aria-label={t("Pending action approval")}>
      <h3>{t("Review proposed action")}</h3><p><strong>{approval.tool}</strong> {t("will be submitted using your account. The target service will check your permission and revalidate its business rules.")}</p>
      <p>{t("Approval expires")} {new Date(approval.expires_at).toLocaleTimeString()}.</p>
      <pre>{JSON.stringify(approval.args,null,2)}</pre>
      <button type="button" disabled={busy} onClick={()=>void decide(true)}>{t("Approve and execute")}</button>{" "}
      <button type="button" disabled={busy} onClick={()=>void decide(false)}>{t("Reject")}</button>
    </div>}
  </section>;
}
