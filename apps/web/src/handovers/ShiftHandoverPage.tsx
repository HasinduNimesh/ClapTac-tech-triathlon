import {useEffect,useState} from 'react';
import {Link} from 'react-router-dom';
import {useAuth} from '../auth/AuthContext';
import {ApiError,apiJSON} from '../api/client';
import {useLocale} from '../i18n';
import {describeError} from '../api/friendlyError.mjs';
import {usePolledResource} from '../notifications/usePolledResource';
import {useOrderCutoffStatus} from '../store-manager/useOrderCutoff';
import {loadTasks} from '../tasks/loadTasks';
import {prepareHandover,handoverCounts,snapshotFresh,type Handover,type Report,type HandoverTask} from './model.mjs';
import './handovers.css';

const time=(s:string)=>new Intl.DateTimeFormat('en-GB',{timeZone:'Asia/Colombo',day:'2-digit',month:'short',hour:'2-digit',minute:'2-digit'}).format(new Date(s));
export function ShiftHandoverPage({dispatcher=false}:{dispatcher?:boolean}){
 const {profile,user}=useAuth();
 return <ScopePicker key={`${profile?.userId}:${user?.access_token}:${dispatcher}`} dispatcher={dispatcher}/>;
}
function ScopePicker({dispatcher}:{dispatcher:boolean}){
 const {profile}=useAuth();const {t}=useLocale();
 const [scope,setScope]=useState(dispatcher?'fleet':profile?.outletIds?.[0]||'');
 return <div className="ho-page"><header className="ho-heading"><div><p className="ho-eyebrow">{t('AGENT 04 · SHIFT HANDOVER')}</p><h1>{t('Leave the next shift ready.')}</h1><p>{t('A clear brief, the work still open, and a confirmed handoff.')}</p></div><div className="ho-scope">{dispatcher?<span>{t('Dispatcher · fleet-wide')}</span>:<label>{t('Store')}<select aria-label={t('Handover store')} value={scope} onChange={e=>setScope(e.target.value)}>{(profile?.outletIds||[]).map(id=><option key={id}>{id}</option>)}</select></label>}</div></header>{scope?<Workspace key={scope} dispatcher={dispatcher} scope={scope}/>:<p role="alert">{t('No authorized store is available.')}</p>}</div>;
}
function Workspace({dispatcher,scope}:{dispatcher:boolean;scope:string}){
 const {user,profile}=useAuth();const token=user?.access_token||'';const {t}=useLocale();
 const {now}=useOrderCutoffStatus();
 const [tab,setTab]=useState<'prepare'|'received'>('prepare');
 const [draft,setDraft]=useState<Pick<Report,'asOf'|'tasks'>>();
 const [title,setTitle]=useState('');const [notes,setNotes]=useState('');
 const [pending,setPending]=useState<{id:string;snapshot:Report}>();
 const [busy,setBusy]=useState(false);const [error,setError]=useState('');const [notice,setNotice]=useState('');
 const [selected,setSelected]=useState<Handover>();const [reviewed,setReviewed]=useState(false);const [offset,setOffset]=useState(0);
 const query=`role=${dispatcher?'dispatcher':'store'}&scope=${encodeURIComponent(scope)}`;
 const history=usePolledResource(()=>apiJSON<{items:Handover[];hasMore:boolean}>(`/shared/handovers?${query}&offset=${offset}`,token,{cache:'no-store'}),Boolean(token),`${query}:${offset}`,30_000);
 const fresh=Boolean(draft && snapshotFresh(draft.asOf,now?.getTime()??NaN));
 const records=history.data?.items||[];
 const listed=selected ? records.find(h=>h.id===selected.id) : undefined;
 const current=selected?.acknowledgedBy ? selected : listed||selected;
 useEffect(()=>{setReviewed(false);},[selected?.id]);
 // If a response was lost after commit, history reconciles the exact publication ID.
 useEffect(()=>{
  if(!pending)return;
  const found=records.find(h=>h.id===pending.id);
  if(found){setPending(undefined);setDraft(undefined);setNotes('');setTitle('');setSelected(found);setTab('received');setError('');setNotice(t('Handover published. The next shift can now review it.'));}
 },[history.data,pending,t]);
 async function generate(){
  setBusy(true);setError('');setNotice('');
  try{const data=await loadTasks(token,dispatcher);setDraft(prepareHandover(data,dispatcher,scope));}
  catch(e){setDraft(undefined);setError(describeError(e));}finally{setBusy(false);}
 }
 async function publish(){
  if(!draft || (!pending && (!fresh || !title.trim())))return;
  const payload=pending||{id:crypto.randomUUID(),snapshot:{...draft,title:title.trim(),notes:notes.trim()}};
  setPending(payload);setBusy(true);setError('');
  try{
   const result=await apiJSON<{handover:Handover}>(`/shared/handovers?${query}`,token,{method:'POST',body:JSON.stringify(payload)});
   setPending(undefined);setDraft(undefined);setNotes('');setTitle('');setSelected(result.handover);setOffset(0);setTab('received');history.reload();setNotice(t('Handover published. The next shift can now review it.'));
  }catch(e){setError(describeError(e));if(e instanceof ApiError && [400,403,409].includes(e.status))setPending(undefined);}
  finally{setBusy(false);}
 }
 async function acknowledge(){
  if(!current || !reviewed)return;
  setBusy(true);setError('');
  try{const result=await apiJSON<{handover:Handover}>(`/shared/handovers/${current.id}/acknowledge?${query}`,token,{method:'POST'});setSelected(result.handover);history.reload();setReviewed(false);setNotice(t('Handover acknowledged. Outstanding tasks still need their usual actions.'));}
  catch(e){setError(describeError(e));history.reload();}finally{setBusy(false);}
 }
 return <>
  <nav className="ho-tabs" aria-label={t('Handover views')}><button aria-pressed={tab==='prepare'} className={tab==='prepare'?'active':''} onClick={()=>setTab('prepare')}>{t('Prepare handover')}</button><button aria-pressed={tab==='received'} className={tab==='received'?'active':''} onClick={()=>setTab('received')}>{t('Team handovers')} <span>{records.filter(h=>!h.acknowledgedBy).length}</span></button></nav>
  {error&&<div className="ho-alert" role="alert">{t(error)}</div>}{notice&&<div className="ho-notice" role="status">{notice}</div>}
  {tab==='prepare'?<div className="ho-compose"><section className="ho-editor"><div className="ho-step">01 <span>{t('PREPARE THE BRIEF')}</span></div><h2>{t('What does the next shift need to know?')}</h2><p>{t('The agent collects open work. Add the context only your team knows, then review before publishing.')}</p><label>{t('Shift title')}<input maxLength={100} disabled={busy||Boolean(pending)} placeholder={t('e.g. Afternoon to evening shift')} value={title} onChange={e=>setTitle(e.target.value)}/></label><label>{t('Notes for the next shift')}<textarea maxLength={4000} rows={7} disabled={busy||Boolean(pending)} placeholder={t('Key decisions, people to contact, and anything to watch…')} value={notes} onChange={e=>setNotes(e.target.value)}/></label><small>{notes.length} / 4000 · {t('Notes are shared with this handover audience.')}</small><div className="ho-audience"><span aria-hidden="true">↗</span><div><strong>{t('Who can read this?')}</strong><p>{dispatcher?t('All dispatchers. This brief covers the fleet across depots.'):t('Store managers authorized for the selected outlet only.')}</p></div></div><button className="ho-secondary" disabled={busy||Boolean(pending)} onClick={()=>void generate()}>{busy?t('Working…'):draft?t('Refresh open work'):t('Generate handover brief')}</button><p className="ho-footnote">{t('Draft notes stay on this page until published. Changing store or leaving discards an unpublished draft.')}</p></section>
   <section className="ho-preview"><div className="ho-preview-heading"><div className="ho-step">02 <span>{t('REVIEW & PUBLISH')}</span></div><span className="ho-status">{t('Draft')}</span></div><h2>{title||t('Your next-shift brief')}</h2>{draft?<><p className="ho-asof">{t('Snapshot')} · {time(draft.asOf)} · {t('Sri Lanka time')}</p><Counts tasks={draft.tasks}/><div className="ho-preview-notes"><h3>{t('Shift notes')}</h3><p>{notes||t('No additional notes.')}</p></div><TaskList tasks={draft.tasks}/>{!fresh&&!pending&&<p className="ho-alert" role="status">{t('Refresh open work before publishing. Snapshots are valid for five minutes and require server time.')}</p>}{pending&&<p className="ho-alert">{t('Publication is unconfirmed. Retry sends the same brief safely; refresh team history to check whether it was saved.')}</p>}<div className="ho-publish"><p>{t('Publishing saves an immutable snapshot. It does not complete tasks or change the delivery plan.')}</p><button className="ho-primary" disabled={busy||(!pending&&(!fresh||!title.trim()))} onClick={()=>void publish()}>{busy?t('Working…'):pending?t('Retry publication'):t('Publish to next shift')}</button></div></>:<div className="ho-empty"><span aria-hidden="true">✦</span><h3>{t('Start with the work already in motion')}</h3><p>{dispatcher?t('Open incidents, recovery decisions, loading blockers and unallocated orders will appear here.'):t('Pending receipts, recovery preferences, deferrals and upcoming arrivals will appear here.')}</p><p>{t('Only complete source checks can become a handover brief.')}</p></div>}</section></div>:
   <div className="ho-inbox"><aside className="ho-history"><div className="ho-history-head"><h2>{t('Published handovers')}</h2><button className="ho-secondary" onClick={history.reload}>{t('Refresh')}</button></div>{history.error&&<p className="ho-alert" role="alert">{t('History could not be refreshed. Previously loaded reports may be out of date.')}</p>}{history.loading?<p role="status">{t('Loading handovers…')}</p>:!records.length?<p>{t('No published handovers for this audience yet.')}</p>:records.map(h=><button key={h.id} className={`ho-history-item ${current?.id===h.id?'active':''}`} onClick={()=>{setSelected(h);setError('');setNotice('');}}><span className={`ho-status ${h.acknowledgedBy?'received':''}`}>{h.acknowledgedBy?t('Acknowledged'):t('Awaiting next shift')}</span><strong>{h.snapshot.title}</strong><span>{h.authorName} · {time(h.publishedAt)}</span><small>{h.snapshot.tasks.length} {t('open at handover')}</small></button>)}<div className="ho-history-pages"><button disabled={!offset||history.loading} onClick={()=>setOffset(Math.max(0,offset-20))}>{t('Previous')}</button><span>{offset/20+1}</span><button disabled={!history.data?.hasMore||history.loading} onClick={()=>setOffset(offset+20)}>{t('Next')}</button></div></aside>
    <section className="ho-report">{current?<><div className="ho-preview-heading"><span className="ho-eyebrow">{t('PUBLISHED SHIFT BRIEF')}</span><span className={`ho-status ${current.acknowledgedBy?'received':''}`}>{current.acknowledgedBy?t('Acknowledged'):t('Awaiting next shift')}</span></div><h2>{current.snapshot.title}</h2><p className="ho-asof">{t('Published by')} {current.authorName} · {time(current.publishedAt)}</p><p className="ho-asof">{t('Operational snapshot')} · {time(current.snapshot.asOf)} · {t('Sri Lanka time')}</p><p className="ho-snapshot-warning">{t("Saved snapshot — check current tasks before acting.")}</p><Counts tasks={current.snapshot.tasks}/><div className="ho-preview-notes"><h3>{t('Shift notes')}</h3><p>{current.snapshot.notes||t('No additional notes.')}</p></div><TaskList tasks={current.snapshot.tasks}/><div className="ho-ack">{current.acknowledgedBy?<><strong>✓ {t('Received by')} {current.acknowledgedName}</strong><p>{time(current.acknowledgedAt!)}</p></>:current.authorId===profile?.userId?<><strong>{t('Ready for another person to receive')}</strong><p>{t('The author cannot acknowledge their own handover.')}</p></>:<><label><input type="checkbox" checked={reviewed} disabled={busy} onChange={e=>setReviewed(e.target.checked)}/>{t('I have reviewed the notes and outstanding work.')}</label><button className="ho-primary" disabled={busy||!reviewed} onClick={()=>void acknowledge()}>{busy?t('Working…'):t('Acknowledge handover')}</button></>}<p>{t('Acknowledgement confirms receipt of the brief. It does not mark operational tasks complete.')}</p><Link to={dispatcher?'/dispatcher/tasks':'/store-manager/tasks'}>{t('Open the current task queue')} ↗</Link></div></>:<div className="ho-empty"><span aria-hidden="true">↙</span><h3>{t('Choose a handover to review')}</h3><p>{t('Published briefs preserve what was known at the time. Check the current task queue before acting.')}</p></div>}</section></div>}
 </>;
}
function Counts({tasks}:{tasks:HandoverTask[]}){const {t}=useLocale();const c=handoverCounts(tasks);return <div className="ho-counts"><div><strong>{c.total}</strong><span>{t('Open at handover')}</span></div><div><strong>{c.critical}</strong><span>{t('Critical')}</span></div><div><strong>{c.high}</strong><span>{t('High priority')}</span></div><div><strong>{c.normal}</strong><span>{t('Normal')}</span></div></div>;}
function TaskList({tasks}:{tasks:HandoverTask[]}){const {t}=useLocale();return <div className="ho-tasks"><h3>{t('Outstanding work')} <span>{tasks.length}</span></h3>{!tasks.length?<p>{t('No open tasks were found in the checked sources. Add any untracked work in the shift notes.')}</p>:<ol>{tasks.map(task=><li key={task.id}><span className={`ho-priority ${task.priority.toLowerCase()}`}>{t(task.priority)}</span><div><strong>{t(task.title)}</strong><small>{task.reference}</small><p>{t(task.reason)}</p><Link to={task.to}>{t(task.action)} ↗</Link></div></li>)}</ol>}</div>;}
