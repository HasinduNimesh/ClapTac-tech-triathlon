import { generateTasks } from '../tasks/taskModel.mjs';

export function prepareHandover(data, dispatcher, scope) {
  if (data.failed.length) throw new Error('Refresh all sources before preparing a handover.');
  const tasks = generateTasks(dispatcher ? 'dispatcher' : 'store', data.source, data.serverTime, dispatcher ? [] : [scope]);
  if (tasks.length > 500) throw new Error('This handover exceeds the 500-task limit. Resolve or review the queue before publishing.');
  // Store only the reviewable report, not raw API payloads or other outlets' data.
  return {asOf:data.serverTime,tasks:tasks.map(({id,priority,title,reason,reference,action,to,dueAt})=>({id,priority,title,reason,reference,action,to,...(dueAt ? {dueAt} : {})}))};
}
export function handoverCounts(tasks) {
  return {total:tasks.length,critical:tasks.filter(t=>t.priority==='Critical').length,high:tasks.filter(t=>t.priority==='High').length,normal:tasks.filter(t=>t.priority==='Normal').length};
}
export function snapshotFresh(asOf, now) {
  const time=Date.parse(asOf);
  return Number.isFinite(time) && Number.isFinite(now) && time<=now+30_000 && now-time<=300_000;
}
