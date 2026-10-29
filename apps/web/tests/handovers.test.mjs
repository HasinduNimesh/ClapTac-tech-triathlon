import test from 'node:test';
import assert from 'node:assert/strict';
import {prepareHandover,handoverCounts,snapshotFresh} from '../src/handovers/model.mjs';
const serverTime='2026-10-27T04:00:00Z';
const row=(id,outletId)=>({order:{id,outletId,orderRef:`ORD-${id}`},stage:'DELIVERED',planning:{}});
const data={serverTime,date:'2026-10-27',failed:[],source:{rows:[row('a','OUT001'),row('b','OUT002')]}};
test('store handovers contain only the selected authorized outlet and no raw peer payloads',()=>{
 const report=prepareHandover(data,false,'OUT001');assert.equal(report.tasks.length,1);assert.equal(report.tasks[0].reference,'ORD-a');assert.equal(report.tasks[0].source,undefined);assert.equal(JSON.stringify(report).includes('OUT002'),false);
});
test('incomplete source checks cannot become a misleading handover',()=>{
 assert.throws(()=>prepareHandover({...data,failed:['rows']},false,'OUT001'),/Refresh all sources/);
});
test('handover generation is a stable report, not a mutable alias to task-source data',()=>{
 const source=structuredClone(data);const report=prepareHandover(source,false,'OUT001');source.source.rows[0].order.orderRef='CHANGED';assert.equal(report.tasks[0].reference,'ORD-a');assert.equal(report.asOf,serverTime);
});
test('counts distinguish urgent work without inventing completed deliveries',()=>{
 assert.deepEqual(handoverCounts([{priority:'Critical'},{priority:'High'},{priority:'Normal'}]),{total:3,critical:1,high:1,normal:1});
 assert.deepEqual(handoverCounts([]),{total:0,critical:0,high:0,normal:0});
});
test('snapshot freshness is server anchored, with invalid and future times rejected',()=>{
 const now=Date.parse(serverTime);assert.equal(snapshotFresh(serverTime,now+299999),true);assert.equal(snapshotFresh(serverTime,now+300001),false);assert.equal(snapshotFresh(serverTime,NaN),false);assert.equal(snapshotFresh('invalid',now),false);assert.equal(snapshotFresh('2026-10-27T04:01:00Z',now),false);
});
test('oversized queues are rejected rather than silently omitting handover work',()=>{
 const rows=Array.from({length:501},(_,i)=>row(String(i),'OUT001'));assert.throws(()=>prepareHandover({...data,source:{rows}},false,'OUT001'),/500-task limit/);
});
