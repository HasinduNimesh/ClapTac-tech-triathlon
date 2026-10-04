import { useLocale } from "../i18n";
import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { apiJSON } from "../api/client";
import { AutomationInbox, ResultView } from "./AutomationInbox";
import { PriorityReviews } from "./PriorityReviews";
import { apiBase, days, Definition, Result } from "./types";
import "./automations.css";
const initial: Definition = { version: 1, name: "Weekly deferred orders", weekday: 5, time: "15:00", timezone: "Asia/Colombo", action: "notify_deferrals" };
export function AutomationsPage() {
  const { t } = useLocale();
  const { user, profile } = useAuth(); const location = useLocation(); const navigate = useNavigate(); const token = user?.access_token || ""; const dispatcher = profile?.roles.includes("DISPATCHER");
  const incoming = location.state?.draftOwner === profile?.userId ? location.state?.draft as Definition | undefined : undefined;
  const [draft, setDraft] = useState<Definition>(incoming || initial); const [building, setBuilding] = useState(!!incoming);
  const [message, setMessage] = useState("Every Friday at 3 PM, if any of my orders are still deferred, send me a list.");
  const [items, setItems] = useState<{ id: string; definition: Definition; status: string; nextRunAt: string }[]>([]);
  const [demo, setDemo] = useState(false);
  const [enabled, setEnabled] = useState(true); const [preview, setPreview] = useState<{ id: string; result: Result } | null>(null);
  const [testAt, setTestAt] = useState("previous"); const [busy, setBusy] = useState(false); const [error, setError] = useState(""); const [notice, setNotice] = useState(""); const [refresh, setRefresh] = useState(0);
  async function load() { const data = await apiJSON<{ items: typeof items; habitEnabled: boolean; demoHistory: boolean }>(`${apiBase}/`, token); setItems(data.items); setEnabled(data.habitEnabled); setDemo(data.demoHistory); }
  useEffect(() => { if (token) void load().catch(e => setError(String(e))); }, [token]);
  useEffect(() => {if(incoming){setDraft(incoming);setBuilding(true);setPreview(null);navigate(location.pathname,{replace:true,state:null});}},[location.key]);
  function change(next: Definition) { setDraft(next); setPreview(null); setNotice(""); }
  async function perform(action: () => Promise<void>) { if (busy) return; setBusy(true); setError(""); try { await action(); } catch (e) { setError(String(e)); } finally { setBusy(false); } }
  async function interpret() { const data = await apiJSON<{ definition?: Definition; clarification?: string; source?: string }>("/agent/workflows/draft", token, { method: "POST", body: JSON.stringify({ message }) }); if (data.definition) { change(data.definition);setNotice(`Draft prepared using ${data.source}. Review the schedule below.`); } else {setPreview(null);setNotice(data.clarification || "Please clarify your request.");} }
  async function test() {setPreview(null);setPreview(await apiJSON(`${apiBase}/preview`, token, { method: "POST", body: JSON.stringify({ definition: draft, at: testAt }) }));}
  async function activate() { if (!preview) return; await apiJSON(`${apiBase}/`, token, { method: "POST", body: JSON.stringify({ previewId: preview.id }) });setBuilding(false);setPreview(null);setNotice("Automation is on. It will run at the next scheduled time.");await load(); }
  async function status(id: string, next: string) {await apiJSON(`${apiBase}/${id}/status`,token,{method:"POST",body:JSON.stringify({status:next})});await load();setRefresh(n=>n+1);}
  return <div className="automations-page">
    <div className="auto-page-heading"><div><div className="auto-eyebrow">{t("SETTINGS · PERSONAL WORKFLOWS")}</div><h1>{t("My automations")}</h1><p>{t("Small routines, on your terms. Review once, then let a fixed workflow help.")}</p></div><button disabled={busy} onClick={() => {setBuilding(true);change(initial);}}>{t("+ New automation")}</button></div>
    {demo && <p className="auto-notice">{t("Demo history is loaded. Records labelled DEMO are synthetic fixtures for testing and screen recording.")}</p>}
    <div className="auto-preference"><div><strong>{t("A3 · Habit helper")}</strong><p>{t("Notices your repeated actions. Every suggestion needs your Yes.")}</p></div><label><input type="checkbox" checked={enabled} disabled={busy} onChange={e => { const next=e.target.checked;void perform(async()=>{await apiJSON(`${apiBase}/preferences`,token,{method:"PUT",body:JSON.stringify({enabled:next})});setEnabled(next);window.dispatchEvent(new Event("waypoint-habits-changed"));});}}/>{" "}{t("Enabled")}</label></div>
    {error && <p className="auto-error" role="alert">{error}</p>}{notice && <p className="auto-notice" role="status">{notice}</p>}
    {building && <section className="auto-builder"><div className="auto-panel"><div className="auto-eyebrow">{t("A4 · WORKFLOW BUILDER")}</div><h2>{t("Describe your routine")}</h2><label htmlFor="routine">{t("What would you like help with?")}</label><textarea id="routine" rows={5} value={message} maxLength={3500} onChange={e=>{setMessage(e.target.value);setPreview(null);}}/><button disabled={busy || !message.trim()} onClick={()=>void perform(interpret)}>{t("Build draft")}</button><p className="muted">{t("The sentence builder supports weekly deferred-order lists. You can also set the schedule directly.")}</p>
      <label>{t("Name")}<input value={draft.name} maxLength={120} onChange={e=>change({...draft,name:e.target.value})}/></label>
      <div className="auto-fields"><label>{t("Day")}<select value={draft.weekday} onChange={e=>change({...draft,weekday:Number(e.target.value)})}>{days.map((day,i)=><option value={i} key={day}>{day}</option>)}</select></label><label>{t("Time")}<input type="time" value={draft.time} onChange={e=>change({...draft,time:e.target.value})}/></label></div>
      <p className="muted">{t("Asia/Colombo · UTC+05:30")}</p>
      <label>{t("Action")}<select value={draft.action} onChange={e=>change({...draft,action:e.target.value as Definition["action"],prefill:undefined})}><option value="notify_deferrals">{t("Send my deferred-order list")}</option>{dispatcher && <option value="priority_review">{t("Flag repeat-deferred outlets for my review")}</option>}{draft.prefill && <option value="prefill_order">{t("Prepare my regular order draft")}</option>}</select></label>
    </div><div className="auto-panel auto-preview"><div className="auto-eyebrow">{t("REVIEW BEFORE TURNING ON")}</div><h2>{t("Your workflow")}</h2><ol className="auto-steps"><li><span>{t("WHEN")}</span><strong>{t("Every")}{" "}{days[draft.weekday]}, {draft.time}</strong><small>{t("Asia/Colombo")}</small></li><li><span>{t("CHECK")}</span><strong>{draft.action==="prefill_order"?"My order template is still authorized":draft.action==="priority_review"?"Outlets have repeated recorded deferrals":"Any of my visible orders are still deferred"}</strong></li><li><span>{t("DO")}</span><strong>{draft.action==="prefill_order"?"Prepare a draft in Notifications for me to review":draft.action==="priority_review"?"Add personal review flags and an in-app list":"Send me a list in Notifications"}</strong></li></ol>
      <label>{t("Test against")}<select value={testAt} onChange={e=>{setTestAt(e.target.value);setPreview(null);}}><option value="previous">{t("Previous scheduled occurrence")}</option><option value="now">{t("Current data")}</option></select></label><button disabled={busy} onClick={()=>void perform(test)}>{t("Test workflow")}</button>
      {preview && <div className="auto-test-result"><strong>{t("Test complete · no actions taken")}</strong><p>{new Date(preview.result.at).toLocaleString("en-GB",{timeZone:"Asia/Colombo"})}{" "}{t("· Sri Lanka")}</p><ResultView result={preview.result}/></div>}
      <div className="auto-actions"><button className="primary" disabled={busy||!preview} onClick={()=>void perform(activate)}>{t("Turn on")}</button><button disabled={busy} onClick={()=>{setBuilding(false);setPreview(null);}}>{t("Cancel")}</button></div><small>{t("Orders are never submitted and plans are never published by these workflows.")}</small>
    </div></section>}
    <section className="auto-panel"><div className="auto-eyebrow">{t("SAVED ROUTINES")}</div><h2>{t("Your workflows")}</h2>{items.length===0 && <p className="muted">{t("No automations yet. Start with a weekly deferred-order list.")}</p>}{items.map(item=><article className="auto-workflow" key={item.id}><div><h3>{item.definition.name}</h3><p>{t("Every")}{" "}{days[item.definition.weekday]}{" "}{t("at")}{" "}{item.definition.time}{" "}{t("· Asia/Colombo")}</p><small>{item.status==="active"?`Next run: ${new Date(item.nextRunAt).toLocaleString("en-GB",{timeZone:"Asia/Colombo"})}`:"Paused — no scheduled actions"}</small></div><span className={`auto-status ${item.status}`}>{item.status}</span><button disabled={busy} onClick={()=>void perform(()=>status(item.id,item.status==="active"?"paused":"active"))}>{item.status==="active"?"Pause":"Resume"}</button><button disabled={busy} onClick={()=>void perform(()=>status(item.id,"deleted"))}>{t("Delete")}</button></article>)}</section>
    {dispatcher && <PriorityReviews/>}<div className="auto-builder"><AutomationInbox refresh={refresh}/><AutomationInbox refresh={refresh} runs/></div>
  </div>;
}
