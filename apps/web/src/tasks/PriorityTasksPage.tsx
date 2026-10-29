import { useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { useLocale } from '../i18n';
import { usePolledResource } from '../notifications/usePolledResource';
import { generateTasks, filterTasks, PRIORITIES } from './taskModel.mjs';
import { loadTasks, SOURCE_LABELS, type TaskSnapshot } from './loadTasks';
import './tasks.css';

const formatTime = (value: string) => new Intl.DateTimeFormat('en-GB', {timeZone:'Asia/Colombo', day:'2-digit',month:'short',hour:'2-digit',minute:'2-digit'}).format(new Date(value));
export function PriorityTasksPage({dispatcher = false}: {dispatcher?:boolean}) {
  const { user, profile } = useAuth();
  // Remount the queue on identity/role change; no cached work from another account can flash on screen.
  return <TaskQueue key={`${profile?.userId}:${user?.access_token}:${dispatcher}`} dispatcher={dispatcher} />;
}
function TaskQueue({dispatcher}: {dispatcher:boolean}) {
  const {user,profile} = useAuth();
  const {t} = useLocale();
  const token = user?.access_token || '';
  const previous = useRef<TaskSnapshot>();
  const [query,setQuery] = useState('');
  const [priority,setPriority] = useState('');
  const [kind,setKind] = useState('');
  const [sort,setSort] = useState('priority');
  const [page,setPage] = useState(0);
  const feed = usePolledResource(async () => {
    const snapshot = await loadTasks(token,dispatcher,previous.current);
    previous.current = snapshot;
    return snapshot;
  }, Boolean(token), `${profile?.userId}:${dispatcher}`, 30_000);
  const data = feed.data;
  const incomplete = Boolean(feed.error || data?.failed.length);
  const tasks = useMemo(() => data ? generateTasks(dispatcher?'dispatcher':'store',data.source,data.serverTime,profile?.outletIds || [],t) : [], [data,dispatcher,profile?.outletIds,t]);
  const filtered = filterTasks(tasks,{query,priority,kind,sort});
  const pageCount = Math.max(1,Math.ceil(filtered.length / 12));
  const currentPage = Math.min(page,pageCount-1);
  const shown = filtered.slice(currentPage*12,(currentPage+1)*12);
  function resetFilters() {setQuery('');setPriority('');setKind('');setPage(0);}
  return <div className="pt-page">
    <header className="pt-heading"><div><p className="pt-eyebrow">{dispatcher?t('AGENT 02 · DISPATCHER'):t('AGENT 03 · STORE MANAGER')}</p><h1>{t('Priority tasks')}</h1><p>{dispatcher?t('Keep deliveries moving, one decision at a time.'):t('Know what needs your attention at the store.')}</p></div><button className="pt-button" onClick={feed.reload} disabled={feed.loading}>{t('Refresh tasks')} <span aria-hidden="true">↻</span></button></header>
    <section className="pt-intro"><span className="pt-spark" aria-hidden="true">✦</span><div><strong>{t('Your next steps, automatically organized')}</strong><p>{t('Tasks come from operational records and clear when the underlying work is resolved. Nothing is submitted or approved automatically.')}</p></div></section>
    <div className="pt-meta"><span>{dispatcher?t('Today · all accessible depots'):t('Your authorized stores · open work')}</span><span>{data ? `${t('Checked')} ${formatTime(data.serverTime)} · ${t('Sri Lanka time')}` : t('Connecting to operational sources…')}</span></div>
    {(feed.error || data?.failed.length) ? <div className="pt-warning" role="alert"><strong>{t('Task list may be incomplete or out of date.')}</strong><p>{feed.error?t('Could not refresh. Previous tasks remain visible; retry before relying on this list.'):t('Some sources could not be refreshed. Previously known tasks are retained where available.')}</p>{Boolean(data?.failed.length) && <p>{data!.failed.map(key=>t(SOURCE_LABELS[key])).join(' · ')}</p>}</div> : null}
    <section className="pt-stats" aria-label={t('Task priorities')}>
      <button className={!priority?'selected':''} onClick={()=>{setPriority('');setPage(0);}} aria-pressed={!priority}><span>{t('Known tasks')}</span><strong>{tasks.length}</strong><small>{t('Open work')}</small></button>
      {PRIORITIES.map(p=><button key={p} className={`${p.toLowerCase()} ${priority===p?'selected':''}`} onClick={()=>{setPriority(priority===p?'':p);setPage(0);}} aria-pressed={priority===p}><span><i aria-hidden="true"/>{t(p)}</span><strong>{tasks.filter(task=>task.priority===p).length}</strong><small>{t(p==='Critical'?'Protect chilled deliveries':p==='High'?'Needs attention soon':'Keep work on track')}</small></button>)}
    </section>
    <section className="pt-workspace" aria-label={t('Task list')}>
      <div className="pt-toolbar"><label className="pt-search">{t('Search tasks')}<input type="search" value={query} placeholder={t('Order, vehicle or task…')} onChange={e=>{setQuery(e.target.value);setPage(0);}} /></label><label>{t('Filter by type')}<select value={kind} onChange={e=>{setKind(e.target.value);setPage(0);}}><option value="">{t('All task types')}</option>{[...new Set(tasks.map(task=>task.kind))].sort().map(k=><option key={k} value={k}>{t(k)}</option>)}</select></label><label>{t('Sort by')}<select value={sort} onChange={e=>{setSort(e.target.value);setPage(0);}}><option value="priority">{t('Priority first')}</option><option value="time">{t('Earliest task time')}</option></select></label></div>
      <div className="pt-list-heading"><h2>{priority?t(priority):t('Your work queue')} <span>{filtered.length}</span></h2><small>{t(sort==='time'?'Earliest task time':'Priority, then task time')}</small></div>
      {feed.loading ? <div className="pt-empty" role="status">{t('Finding your next tasks…')}</div> : !tasks.length ? <div className="pt-empty"><span aria-hidden="true">{incomplete?'!':'✓'}</span><h3>{incomplete?t('Waiting for complete data'):t('No tasks from the checked sources')}</h3><p>{incomplete?t('Refresh to check for work that may be missing.'):t('New tasks will appear as operational records change.')}</p></div> : !filtered.length ? <div className="pt-empty"><h3>{t('No tasks match these filters')}</h3><button className="pt-button" onClick={resetFilters}>{t('Clear filters')}</button></div> : <ol className="pt-list">{shown.map((task,index)=><li key={task.id} className={`pt-task ${task.priority.toLowerCase()}`}><span className="pt-rank" aria-hidden="true">{String(currentPage*12+index+1).padStart(2,'0')}</span><div className="pt-task-main"><div className="pt-tags"><span className={`pt-priority ${task.priority.toLowerCase()}`}>{t(task.priority)}</span><span>{t(task.kind)}</span>{(feed.error || data?.failed.includes(task.source)) && <span className="pt-stale">{t('Needs refresh')}</span>}</div><h3>{task.title}</h3><p className="pt-reference">{task.reference}</p><p className="pt-reason">{task.reason}</p>{task.dueAt && Number.isFinite(Date.parse(task.dueAt)) && <small>{t(task.kind==='Recovery'?'Options expire':task.kind==='Arrivals'?'Estimated arrival':task.kind==='Receipts'?'Report by':'Reported')} · {formatTime(task.dueAt)}</small>}</div><Link className="pt-action" to={task.to}>{task.action}<span aria-hidden="true"> ↗</span></Link></li>)}</ol>}
      {pageCount>1 && <div className="pt-pagination"><button className="pt-button" disabled={currentPage===0} onClick={()=>setPage(currentPage-1)}>{t('Previous')}</button><span>{currentPage+1} / {pageCount}</span><button className="pt-button" disabled={currentPage===pageCount-1} onClick={()=>setPage(currentPage+1)}>{t('Next')}</button></div>}
    </section>
    <details className="pt-explain"><summary>{t('How these agents prioritize work')}</summary><p>{t('Critical: chilled loading shortfalls. High: disruptions, recovery decisions, loading blockers, repeat deferrals and receipts due today or overdue. Normal: remaining allocation, receipt, deferral and arrival work.')}</p><p>{t('Equal priorities use the earliest available task time, then a stable task ID. Times come from the server; the list refreshes every 30 seconds while visible. These are rule-based suggestions, not a complete operational checklist.')}</p><p>{dispatcher?t('Dispatcher tasks cover today’s incidents, recovery cases, loading and unallocated orders across accessible depots.'):t('Store tasks use your authorized orders and recovery cases. Submitting a preference clears that task; dispatch still approves the fleet plan.')}</p></details>
  </div>;
}
