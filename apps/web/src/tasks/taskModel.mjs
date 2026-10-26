import { colomboDate, needsReceipt } from '../store-manager/orderStage.mjs';
import { isChilledShortfall, openShortfallCount } from '../dispatcher/shortfalls.mjs';

export const PRIORITIES = ['Critical', 'High', 'Normal'];
const stamp = value => Number.isFinite(Date.parse(value)) ? Date.parse(value) : Infinity;
const vehicle = id => String(id || '').replace(/_/g, ' ');
const link = (path, date) => `${path}?date=${encodeURIComponent(date)}`;

/** Read-only, deterministic work queues. No device clock, generated IDs, or hidden dismiss state. */
export function generateTasks(role, source, serverTime, outletIds = [], t = s => s) {
  const now = stamp(serverTime);
  if (!Number.isFinite(now)) return [];
  const date = colomboDate(now);
  const tasks = [];
  const add = (id, kind, priority, title, reason, reference, to, action, dueAt, sourceKey) => tasks.push({id, kind, priority, title:t(title), reason:t(reason), reference, to, action:t(action), dueAt, source:sourceKey});
  const recovery = (source.recovery || []).filter(c => c.incident.date === date && c.status !== 'superseded');
  if (role === 'dispatcher') {
    for (const c of recovery.filter(c => c.status === 'pending')) {
      const expired = Math.min(stamp(c.expiresAt), stamp(c.readyAt)) <= now;
      add(`recovery:${c.id}`, 'Recovery', 'High', expired ? 'Refresh recovery options' : 'Approve a recovery plan', expired ? 'Options expired. Generate fresh plans before approving.' : 'Compare fleet options and store preferences before publishing.', vehicle(c.incident.vehicleId), link('/dispatcher/recovery', date), 'Open recovery', c.expiresAt, 'recovery');
    }
    for (const i of source.incidents || []) {
      if (i.status !== 'open' || i.date !== date || recovery.some(c => c.incident.id === i.id)) continue;
      add(`incident:${i.id}`, 'Disruption', 'High', 'Review vehicle incident', 'An open incident may block deliveries. Check the affected route.', vehicle(i.vehicleId), link(i.type === 'breakdown' ? '/dispatcher/recovery' : '/dispatcher/live', date), i.type === 'breakdown' ? 'Open recovery' : 'Open live operations', i.reportedAt, 'incidents');
    }
    for (const trip of source.trips || []) {
      if (openShortfallCount(trip) <= 0) continue;
      const chilled = isChilledShortfall(trip);
      add(`shortfall:${trip.tripId}`, 'Loading', chilled ? 'Critical' : 'High', 'Decide a loading shortfall', chilled ? 'Chilled goods are short. Decide how to protect the delivery.' : 'Loading is waiting for a dispatcher decision.', `${vehicle(trip.vehicleId)} · ${openShortfallCount(trip)} ${t('open lines')}`, link('/dispatcher/notifications', date), 'Review shortfall', undefined, 'trips');
    }
    for (const alert of source.dockAlerts || []) {
      if (alert.resolvedAt) continue;
      add(`dock:${alert.id}`, 'Loading', 'High', 'Resolve a dock alert', 'Check the staged goods before the wrong vehicle departs.', `${alert.orderRef} · ${vehicle(alert.belongsVehicleId)}`, link('/dispatcher/notifications', date), 'Review dock alert', alert.createdAt, 'dockAlerts');
    }
    const plan = source.plan;
    for (const entry of plan?.unallocated || []) {
      const order = plan.orders?.find(o => o.id === entry.orderId);
      const repeat = (order?.outletDeferralCount || 0) >= 2;
      add(`unallocated:${entry.orderId}`, 'Planning', repeat ? 'High' : 'Normal', repeat ? 'Prioritize a repeat-deferred order' : 'Allocate or defer an order', repeat ? 'This outlet has repeated deferrals. Review a fair allocation.' : 'This order has no vehicle allocation in the current plan.', entry.orderRef || order?.orderRef || entry.orderId, link('/dispatcher/planning', date), 'Open planning', undefined, 'plan');
    }
  } else {
    const allowed = new Set(outletIds);
    for (const c of recovery.filter(c => c.status === 'pending')) {
      if (Math.min(stamp(c.expiresAt), stamp(c.readyAt)) <= now) continue;
      for (const outletId of allowed) {
        if (!c.candidates?.some(p => p.moves?.some(m => m.outletId === outletId))) continue;
        if (c.preferences?.some(p => p.outletId === outletId && c.candidates.some(candidate => candidate.id === p.candidateId))) continue;
        add(`preference:${c.id}:${outletId}`, 'Recovery', 'High', 'Choose your recovery preference', 'Dispatch needs your preference. Fleet changes still require dispatcher approval.', outletId, link('/store-manager/recovery', date), 'Compare options', c.expiresAt, 'recovery');
      }
    }
    for (const row of source.rows || []) {
      const order = row.order;
      if (!allowed.has(order.outletId)) continue;
      const ref = order.orderRef;
      if (needsReceipt(row.stage) && !row.receipt?.confirmedAt) {
        const urgent = ['overdue','due_today'].includes(row.receiptDue?.state);
        add(`receipt:${order.id}`, 'Receipts', urgent ? 'High' : 'Normal', 'Confirm delivery receipt', urgent ? 'The receipt is due today or overdue. Verify quantities and report issues.' : 'Delivery is recorded. Confirm what the store received.', ref, `/store-manager/receipts?order=${encodeURIComponent(order.id)}`, 'Confirm receipt', row.receiptDue?.reportBy, 'rows');
      } else if (row.stage === 'DEFERRED') {
        add(`deferred:${order.id}`, 'Orders', 'Normal', 'Review a deferred order', 'Check the reason and next-run target with dispatch.', ref, `/store-manager/orders/${encodeURIComponent(order.id)}/timeline`, 'View order', undefined, 'rows');
      } else if (['PLANNED','READY_FOR_DEPARTURE','OUT_FOR_DELIVERY'].includes(row.stage) && !row.delivery?.outcome) {
        const arrival = row.delivery?.arrivalPrediction?.estimatedArrivalAt || row.planning?.plannedArrivalAt;
        const minutes = (stamp(arrival) - now) / 60000;
        if (minutes >= 0 && minutes <= 60) add(`arrival:${order.id}`, 'Arrivals', 'Normal', 'Prepare to receive a delivery', 'Arrival is expected within an hour. Check staff and receiving space; the ETA is an estimate.', ref, `/store-manager/orders/${encodeURIComponent(order.id)}/track`, 'Track delivery', arrival, 'rows');
      }
    }
  }
  return sortTasks([...new Map(tasks.map(task => [task.id, task])).values()]);
}

export function sortTasks(tasks, sort = 'priority') {
  const priority = (a,b) => PRIORITIES.indexOf(a.priority) - PRIORITIES.indexOf(b.priority);
  const time = (a,b) => (stamp(a.dueAt) < stamp(b.dueAt) ? -1 : stamp(a.dueAt) > stamp(b.dueAt) ? 1 : 0);
  return [...tasks].sort((a,b) => (sort === 'time' ? time(a,b) || priority(a,b) : priority(a,b) || time(a,b)) || a.id.localeCompare(b.id));
}

export function filterTasks(tasks, {query = '', priority = '', kind = '', sort = 'priority'} = {}) {
  const normalize = s => s.toLowerCase().replace(/[_\s-]+/g, '');
  const needle = normalize(query.trim());
  return sortTasks(tasks.filter(task => (!priority || task.priority === priority) && (!kind || task.kind === kind) && (!needle || normalize(`${task.title} ${task.reference} ${task.reason}`).includes(needle))), sort);
}
