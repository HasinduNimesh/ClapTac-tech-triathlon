import test from 'node:test';
import assert from 'node:assert/strict';
import { generateTasks, filterTasks } from '../src/tasks/taskModel.mjs';
const now='2026-10-27T04:00:00Z';
const incident={id:'incident',date:'2026-10-27',status:'open',type:'breakdown',vehicleId:'WP_NA_1234'};
const recovery={id:'case',status:'pending',incident,readyAt:'2026-10-27T05:00:00Z',expiresAt:'2026-10-27T04:15:00Z',candidates:[{id:'option',moves:[{outletId:'own'}]}],preferences:[]};
const row=(id,stage,extra={})=>({order:{id,orderRef:`ORD-${id}`,outletId:'own'},stage,planning:{},...extra});
const dispatcher=source=>generateTasks('dispatcher',source,now);
const store=source=>generateTasks('store',source,now,['own']);

test('chilled shortfalls lead, ordinary incidents are not falsely critical',()=>{
 const tasks=dispatcher({incidents:[incident],trips:[{tripId:'ambient',openShortfallCount:1},{tripId:'chilled',openShortfallCount:1,openChilledShortfall:true}]});
 assert.equal(tasks[0].id,'shortfall:chilled');assert.equal(tasks[0].priority,'Critical');assert.equal(tasks.find(t=>t.id==='incident:incident').priority,'High');
});
test('decided shortfalls and resolved dock alerts disappear without marking tasks done',()=>{
 assert.equal(dispatcher({trips:[{tripId:'t',shortfallCount:3,openShortfallCount:0}],dockAlerts:[{id:'alert',resolvedAt:now}]}).length,0);
});
test('recovery replaces duplicate incident work and published recovery clears the decision',()=>{
 assert.deepEqual(dispatcher({incidents:[incident],recovery:[recovery]}).map(t=>t.id),['recovery:case']);
 assert.equal(dispatcher({incidents:[incident],recovery:[{...recovery,status:'approved'}]}).length,0);
});
test('expired recovery asks dispatcher to refresh, never asks store to pick stale options',()=>{
 const expired={...recovery,expiresAt:now};
 assert.equal(dispatcher({recovery:[expired]})[0].title,'Refresh recovery options');assert.equal(store({recovery:[expired]}).length,0);
 assert.equal(store({recovery:[{...recovery,readyAt:now}]}).length,0);
});
test('closed and other-day incidents are outside today’s queue',()=>{
 assert.equal(dispatcher({incidents:[{...incident,status:'closed'},{...incident,date:'2026-10-26'}]}).length,0);
});
test('store preference task disappears only after a valid own-outlet choice',()=>{
 assert.equal(store({recovery:[recovery]}).length,1);
 assert.equal(store({recovery:[{...recovery,preferences:[{outletId:'other',candidateId:'option'}]}]}).length,1);
 assert.equal(store({recovery:[{...recovery,preferences:[{outletId:'own',candidateId:'obsolete'}]}]}).length,1);
 assert.equal(store({recovery:[{...recovery,preferences:[{outletId:'own',candidateId:'option'}]}]}).length,0);
});
test('outlet scope also guards against accidentally mixed tracking/recovery payloads',()=>{
 assert.equal(generateTasks('store',{rows:[row('1','DELIVERED')],recovery:[recovery]},now,[]).length,0);
 assert.equal(store({rows:[row('1','DELIVERED',{order:{id:'other',outletId:'other'}})],recovery:[{...recovery,candidates:[{id:'option',moves:[{outletId:'other'}]}]}]}).length,0);
});
test('all authorized outlets receive independent preference tasks',()=>{
 const c={...recovery,candidates:[{id:'option',moves:[{outletId:'own'},{outletId:'second'}]}]};
 assert.equal(generateTasks('store',{recovery:[c]},now,['own','second']).length,2);
});
test('receipt priorities follow server receipt-due state and confirmed receipts clear',()=>{
 const tasks=store({rows:[row('later','DELIVERED',{receiptDue:{state:'open'}}),row('today','PARTIAL',{receiptDue:{state:'due_today'}}),row('overdue','DELIVERED',{receiptDue:{state:'overdue'}}),row('done','RECEIPT_CONFIRMED'),row('recorded','DELIVERED',{receipt:{confirmedAt:now}})]});
 assert.equal(tasks.length,3);assert.equal(tasks.find(t=>t.id==='receipt:later').priority,'Normal');assert.equal(tasks.find(t=>t.id==='receipt:overdue').priority,'High');
});
test('upcoming arrival uses latest ETA and only creates a task within the next hour',()=>{
 const rows=[row('soon','OUT_FOR_DELIVERY',{planning:{plannedArrivalAt:'2026-10-27T04:45:00Z'}}),row('late','OUT_FOR_DELIVERY',{planning:{plannedArrivalAt:'2026-10-27T03:45:00Z'}}),row('changed','OUT_FOR_DELIVERY',{planning:{plannedArrivalAt:'2026-10-27T04:45:00Z'},delivery:{arrivalPrediction:{estimatedArrivalAt:'2026-10-27T06:00:00Z'}}}),row('done','DELIVERED',{planning:{plannedArrivalAt:'2026-10-27T04:45:00Z'},receipt:{confirmedAt:now}})];
 assert.deepEqual(store({rows}).map(t=>t.id),['arrival:soon']);
});
test('fairness priority requires both repeat deferrals and current unallocated state',()=>{
 const tasks=dispatcher({plan:{orders:[{id:'a',outletDeferralCount:2},{id:'b',outletDeferralCount:0},{id:'assigned',outletDeferralCount:9}],unallocated:[{orderId:'b'},{orderId:'a'}]}});
 assert.deepEqual(tasks.map(t=>t.priority),['High','Normal']);assert.equal(tasks.length,2);
});
test('stable IDs de-duplicate records and break equal-priority/time ties',()=>{
 const rows=[row('b','DEFERRED'),row('a','DEFERRED'),row('b','DEFERRED')];
 assert.deepEqual(store({rows}).map(t=>t.id),['deferred:a','deferred:b']);
 assert.deepEqual(store({rows}).map(t=>t.id),store({rows:[...rows].reverse()}).map(t=>t.id));
});
test('search normalizes vehicle separators; priority/type filters combine without mutating input',()=>{
 const tasks=dispatcher({incidents:[incident],trips:[{tripId:'t',openShortfallCount:1,openChilledShortfall:true}]});
 assert.equal(filterTasks(tasks,{query:'wp-na1234',priority:'High',kind:'Disruption'}).length,1);
 assert.equal(filterTasks(tasks,{query:'not present'}).length,0);assert.equal(tasks.length,2);
});
test('only server time determines day, expiry and arrival priority',()=>{
 const old=Date.now;Date.now=()=>Date.parse('2040-01-01T00:00:00Z');
 try {assert.equal(store({recovery:[recovery]}).length,1);assert.equal(generateTasks('store',{recovery:[recovery]},'invalid',['own']).length,0);} finally {Date.now=old;}
 const midnight={...incident,date:'2026-10-28'};
 assert.equal(generateTasks('dispatcher',{incidents:[midnight]},'2026-10-27T18:30:00Z').length,1);
});
test('time sort puts dated work first; missing dates sort last',()=>{
 const tasks=store({rows:[row('undated','DEFERRED'),row('dated','DELIVERED',{receiptDue:{state:'open',reportBy:'2026-10-28T12:00:00Z'}})]});
 assert.equal(filterTasks(tasks,{sort:'time'})[0].id,'receipt:dated');
});

test('task deep links validate real calendar dates',async()=>{
 const {requestedDate}=await import('../src/api/date.mjs');
 assert.equal(requestedDate('2026-10-27'),'2026-10-27');
 for(const value of [null,'bad','2026-02-30','2026-13-01','2026-1-01']) assert.equal(requestedDate(value),undefined);
});
