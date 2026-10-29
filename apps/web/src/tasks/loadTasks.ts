import { ApiError, apiJSON } from '../api/client';
import { colomboDate } from '../store-manager/orderStage.mjs';
import { fetchOrderTrackings, forgetSharedTrackings } from '../store-manager/useOrderTrackings';
import type { TaskSource } from './taskModel.mjs';

export type TaskSnapshot = {source: TaskSource; serverTime: string; date: string; failed: string[]};
export const SOURCE_LABELS: Record<string,string> = {recovery:'Recovery options', incidents:'Vehicle incidents', trips:'Loading shortfalls', dockAlerts:'Dock alerts', plan:'Daily plan', rows:'Order tracking'};

/** Peers enforce role/outlet access; never fetch the dispatcher feeds for a store account. */
export async function loadTasks(token: string, dispatcher: boolean, previous?: TaskSnapshot): Promise<TaskSnapshot> {
  const get = <T,>(path: string) => apiJSON<T>(path, token, {cache:'no-store', signal:AbortSignal.timeout(15_000)});
  const clock = await get<{serverTime:string}>('/orders/cutoff');
  if (!clock.serverTime || !Number.isFinite(Date.parse(clock.serverTime))) throw new Error('Server time is unavailable');
  const date = colomboDate(clock.serverTime);
  const jobs: [keyof TaskSource, () => Promise<unknown>][] = [
    ['recovery', async () => (await get<{items:TaskSource['recovery']}>(`/planning/recovery?date=${date}`)).items || []],
  ];
  if (dispatcher) jobs.push(
    ['incidents', async () => (await get<{items:TaskSource['incidents']}>('/fleet/incidents?openOnly=true')).items || []],
    ['trips', async () => (await get<{items:TaskSource['trips']}>(`/loading/trips?date=${date}`)).items || []],
    ['dockAlerts', async () => (await get<{items:TaskSource['dockAlerts']}>(`/loading/alerts?date=${date}`)).items || []],
    ['plan', async () => {try {return await get(`/planning/plans?date=${date}`);} catch(e) {if (e instanceof ApiError && e.status===404) return null; throw e;}}],
  );
  else jobs.push(['rows', async () => {
    forgetSharedTrackings();
    const result = await fetchOrderTrackings(token);
    // Preserve earlier rows if only some tracking endpoints fail; do not pretend those tasks resolved.
    if (result.unavailable.length) {
      const unavailable = new Set(result.unavailable.map(o => o.id));
      return {rows:[...result.rows, ...(previous?.source.rows || []).filter(r => unavailable.has(r.order.id))], partial:true};
    }
    return {rows:result.rows, partial:false};
  }]);
  const results = await Promise.allSettled(jobs.map(([,job]) => job()));
  const source: TaskSource = previous?.date === date ? {...previous.source} : {};
  const failed: string[] = [];
  results.forEach((result,i) => {
    const key = jobs[i][0];
    if (result.status === 'rejected') { failed.push(key); return; }
    if (key === 'rows') {
      const value = result.value as {rows:TaskSource['rows'];partial:boolean};
      source.rows = value.rows;
      if (value.partial) failed.push(key);
    } else Object.assign(source, {[key]:result.value});
  });
  return {source,serverTime:clock.serverTime,date,failed};
}
