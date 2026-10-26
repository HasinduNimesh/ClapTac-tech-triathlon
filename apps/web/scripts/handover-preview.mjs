// Development-only synthetic data for the real task screens. Never imported by the application.
import {createServer} from 'vite';
import react from '@vitejs/plugin-react';
import {fileURLToPath} from 'node:url';
const root=fileURLToPath(new URL('../',import.meta.url));
const serverTime='2026-10-27T04:00:00Z';
const incident={id:'incident-1',date:'2026-10-27',vehicleId:'WP_NA_4821',type:'breakdown',status:'open',reportedAt:'2026-10-27T03:30:00Z'};
const recovery={id:'recovery-1',planId:'plan-1',depot:'DEPOT_NORTH',status:'pending',incident,createdAt:serverTime,readyAt:'2026-10-27T04:30:00Z',expiresAt:'2026-10-27T04:15:00Z',preferences:[],candidates:[{id:'split',moves:[{outletId:'OUT001'}]}]};
const row=(id,stage,more={})=>({order:{id,orderRef:`ORD-1027-${id}`,requestedDeliveryDate:'2026-10-27',outletId:'OUT001',brand:'Keells',temperatureRequirement:'AMBIENT'},planning:{},stage,...more});
const rows=[row('041','DELIVERED',{receiptDue:{state:'overdue',reportBy:'2026-10-26T10:30:00Z'}}),row('056','PARTIAL',{receiptDue:{state:'due_today',reportBy:'2026-10-27T10:30:00Z'}}),row('062','OUT_FOR_DELIVERY',{planning:{plannedArrivalAt:'2026-10-27T04:40:00Z'}}),row('078','DEFERRED'),row('081','DELIVERED',{receiptDue:{state:'open',reportBy:'2026-10-28T10:30:00Z'}})];
const reports=new Map();const lost=new Set();
const plugin={name:'handover-evidence',resolveId(id){if(id==='/__handover.tsx')return id;},load(id){if(id==='/__handover.tsx')return `import React from 'react';import {createRoot} from 'react-dom/client';import {MemoryRouter,Routes,Route} from 'react-router-dom';import {LocaleProvider} from '/src/i18n';import {DispatcherLayout} from '/src/dispatcher/DispatcherLayout';import {StoreManagerLayout} from '/src/store-manager/StoreManagerLayout';import {ShiftHandoverPage} from '/src/handovers/ShiftHandoverPage';import '/src/index.css';const dispatch=new URLSearchParams(location.search).get('role')!=='store';createRoot(document.getElementById('root')).render(<LocaleProvider><MemoryRouter initialEntries={[dispatch?'/dispatcher/handover':'/store-manager/handover']}><Routes><Route element={dispatch?<DispatcherLayout/>:<StoreManagerLayout/>}><Route path='*' element={<ShiftHandoverPage dispatcher={dispatch}/>} /></Route></Routes></MemoryRouter></LocaleProvider>);`;},transform(source,id){
 if(id.endsWith('/auth/AuthContext.tsx'))return `export function useAuth(){const params=new URLSearchParams(location.search);const dispatch=params.get('role')!=='store';const incoming=params.get('person')==='incoming';const role=dispatch?'dispatch':'store';const person=incoming?'incoming':'outgoing';return {user:{access_token:'preview-'+role+'-'+person},profile:{userId:'PREVIEW-'+role+'-'+person,displayName:incoming?(dispatch?'Maya · Dispatch':'Dilani · Store manager'):(dispatch?'Aeshan · Dispatch':'Nimal · Store manager'),roles:[dispatch?'DISPATCHER':'STORE_MANAGER'],outletIds:['OUT001','OUT002'],depot:'DEPOT_NORTH'},logout:async()=>{}}}`;
 if(id.endsWith('/notifications/useRoleNotifications.ts'))return `const feed={items:[],unread:[],unreadCount:0,loading:false,error:'',markRead:()=>{},markAllRead:()=>{}};export function useDispatcherNotifications(){return feed}export function useStoreManagerNotifications(){return feed}`;
},configureServer(server){
 server.middlewares.use('/api/v1',async(req,res)=>{
  res.setHeader('Content-Type','application/json');
  const scenario=new URL(req.headers.referer||'http://localhost').searchParams.get('scenario')||'full';
  const url=new URL(req.url,'http://localhost');
  if(url.pathname.startsWith('/shared/handovers')){
   const role=url.searchParams.get('role'),scope=url.searchParams.get('scope');
   const incoming=req.headers.authorization?.endsWith('incoming');
   const actor='PREVIEW-'+(role==='dispatcher'?'dispatch':'store')+'-'+(incoming?'incoming':'outgoing');
   const name=incoming?(role==='dispatcher'?'Maya · Dispatch':'Dilani · Store manager'):(role==='dispatcher'?'Aeshan · Dispatch':'Nimal · Store manager');
   let raw='';for await(const chunk of req)raw+=chunk;const input=raw?JSON.parse(raw):{};
   if(req.method==='GET'){const items=[...reports.values()].filter(h=>h.role===role&&h.scope===scope).reverse();res.end(JSON.stringify({items,hasMore:false,serverTime}));return;}
   if(url.pathname.endsWith('/acknowledge')){
    const record=reports.get(url.pathname.split('/')[3]);
    if(!record||record.role!==role||record.scope!==scope){res.statusCode=404;res.end('{}');return;}
    if(record.authorId===actor){res.statusCode=409;res.end(JSON.stringify({error:'Another person must acknowledge'}));return;}
    record.acknowledgedBy=actor;record.acknowledgedName=name;record.acknowledgedAt=serverTime;
    res.end(JSON.stringify({handover:record}));return;
   }
   if(scenario==='stale'){res.statusCode=409;res.end(JSON.stringify({error:'Refresh the snapshot before publishing'}));return;}
   if(!reports.has(input.id))reports.set(input.id,{id:input.id,role,scope,authorId:actor,authorName:name,snapshot:input.snapshot,publishedAt:serverTime});
   if(scenario==='lost-response'&&!lost.has(input.id)){lost.add(input.id);res.statusCode=503;res.end(JSON.stringify({error:'Synthetic lost response after commit'}));return;}
   res.end(JSON.stringify({handover:reports.get(input.id)}));return;
  }
  const route=req.url.split('?')[0];
  if((scenario==='partial'&&route==='/loading/trips') || scenario==='offline' || (scenario==='tracking-failure'&&route.endsWith('/041/tracking'))){res.statusCode=503;res.end(JSON.stringify({error:'Synthetic source outage'}));return;}
  const complete=scenario==='complete';const empty=scenario==='empty';
  let body={items:[]};
  if(route==='/orders/cutoff')body={serverTime,cutoff:{localTime:'16:00'}};
  else if(route==='/planning/recovery')body={serverNow:serverTime,items:empty?[]:[{...recovery,status:complete?'approved':'pending'}]};
  else if(route==='/fleet/incidents')body={items:empty||complete?[]:[incident,{...incident,id:'incident-2',vehicleId:'CP_NC_2086',reportedAt:'2026-10-27T03:50:00Z'}]};
  else if(route==='/loading/trips')body={items:empty||complete?[]:[{tripId:'trip-1',vehicleId:'WP_NB_7134',depot:'DEPOT_NORTH',openShortfallCount:3,openChilledShortfall:true},{tripId:'trip-2',vehicleId:'WP_NC_3502',depot:'DEPOT_NORTH',openShortfallCount:1,openChilledShortfall:false}]};
  else if(route==='/loading/alerts')body={items:empty||complete?[]:[{id:'dock-1',orderRef:'ORD-1027-089',belongsVehicleId:'WP_NA_1068',createdAt:'2026-10-27T03:40:00Z'}]};
  else if(route==='/planning/plans')body={plan:{id:'plan-1',status:'draft'},orders:[{id:'order-a',orderRef:'ORD-1027-023',outletDeferralCount:3},{id:'order-b',orderRef:'ORD-1027-094',outletDeferralCount:0}],unallocated:empty||complete?[]:[{orderId:'order-a'},{orderId:'order-b'}]};
  else if(route==='/orders')body={items:empty?[]:rows.map(row=>row.order)};
  else if(route.endsWith('/tracking')){const id=route.split('/')[2];const found=rows.find(row=>row.order.id===id);body={tracking:complete?{...found,stage:'RECEIPT_CONFIRMED'}:found};}
  res.end(JSON.stringify(body));
 });
 server.middlewares.use('/__preview',async(req,res)=>{res.setHeader('Content-Type','text/html');res.end(await server.transformIndexHtml('/__preview','<!doctype html><html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><div id="root"></div><script type="module" src="/__handover.tsx"></script></body></html>'));});
}};
const server=await createServer({configFile:false,root,plugins:[plugin,react()],optimizeDeps:{entries:[]},server:{host:'127.0.0.1',port:4323,strictPort:true}});
await server.listen();console.log('Synthetic handover examples: http://127.0.0.1:4323/__preview?role=dispatch');
