// Development-only UI evidence harness. Run the real Go examples first (see docs).
import { createServer } from 'vite';
import react from '@vitejs/plugin-react';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const root = fileURLToPath(new URL('../', import.meta.url));
const fixtures = process.env.RECOVERY_EXAMPLE_DIR;
if (!fixtures) throw new Error('Set RECOVERY_EXAMPLE_DIR to exported Go recovery fixtures.');
const examples = Object.fromEntries(['full', 'partial', 'no-capacity'].map(k => [k, JSON.parse(readFileSync(path.join(fixtures, k + '.json'), 'utf8'))]));
const cases = structuredClone(examples);
const serverNow = '2026-10-26T04:00:00Z';
const plugin = {
  name: 'recovery-evidence',
  resolveId(id) { if (id === '/__recovery.tsx') return id; },
  load(id) {
    if (id === '/__recovery.tsx') return `import React from 'react';import {createRoot} from 'react-dom/client';import {MemoryRouter,Routes,Route} from 'react-router-dom';import {LocaleProvider} from '/src/i18n';import {DispatcherLayout} from '/src/dispatcher/DispatcherLayout';import {StoreManagerLayout} from '/src/store-manager/StoreManagerLayout';import {RecoveryPage} from '/src/recovery/RecoveryPage';import '/src/index.css';const dispatch=new URLSearchParams(location.search).get('role')!=='store';createRoot(document.getElementById('root')).render(<LocaleProvider><MemoryRouter initialEntries={[dispatch?'/dispatcher/recovery':'/store-manager/recovery']}><Routes><Route element={dispatch?<DispatcherLayout/>:<StoreManagerLayout/>}><Route path='*' element={<RecoveryPage dispatcher={dispatch}/>} /></Route></Routes></MemoryRouter></LocaleProvider>);`;
  },
  transform(source, id) {
    if (id.endsWith('/auth/AuthContext.tsx')) return `export function useAuth(){const dispatch=new URLSearchParams(location.search).get('role')!=='store';return {user:{access_token:'preview-'+(dispatch?'dispatch':'store')},profile:{userId:'PREVIEW',displayName:dispatch?'Aeshan · Dispatch':'Nimal · Store manager',roles:[dispatch?'DISPATCHER':'STORE_MANAGER'],outletIds:['OUT001'],depot:'DEPOT_NORTH'},logout:async()=>{}}}`;
    if (id.endsWith('/notifications/useRoleNotifications.ts')) return `const feed={items:[],unread:[],unreadCount:0,loading:false,error:'',refresh:async()=>{},markRead:()=>{},markAllRead:()=>{}};export function useDispatcherNotifications(){return feed}export function useStoreManagerNotifications(){return feed}`;
  },
  configureServer(server) {
    server.middlewares.use('/api/v1', async (req, res) => {
      res.setHeader('Content-Type', 'application/json');
      const referer = new URL(req.headers.referer || 'http://localhost');
      const scenario = referer.searchParams.get('scenario') || 'full';
      const role = req.headers.authorization?.includes('store') ? 'store' : 'dispatch';
      const route = req.url.split('?')[0];
      let c = cases[scenario] || cases.full;
      let body = ''; for await (const chunk of req) body += chunk;
      const input = body ? JSON.parse(body) : {};
      if (route.endsWith('/preference')) {
        c.preferences = [{outletId: input.outletId, candidateId: input.candidateId, note: input.note, updatedAt:serverNow}];
        res.end(JSON.stringify({saved:true})); return;
      }
      if (route.endsWith('/approve')) {
        if (referer.searchParams.has('stale')) {res.statusCode=409;res.end(JSON.stringify({detail:'conflict: fleet, delivery progress or plan changed; generate fresh options'}));return;}
        c.status='approved';c.selectedId=input.candidateId;c.publishedVersion=4;c.approvalNote=input.note;
        res.end(JSON.stringify(c));return;
      }
      if (route === '/planning/recovery' && req.method === 'POST') { cases[scenario]=structuredClone(examples[scenario]); res.end(JSON.stringify(cases[scenario]));return; }
      if (route === '/planning/recovery') {
        c=structuredClone(c);
        if (role==='store') {
          c.incident.description='';c.approvalNote='';
          for (const o of c.candidates) {
            o.moves=o.moves.filter(m=>m.outletId==='OUT001');o.recovered=o.moves.filter(m=>!m.deferred).length;o.deferred=o.moves.filter(m=>m.deferred).length;o.vehicles=new Set(o.moves.filter(m=>!m.deferred).map(m=>m.vehicleId)).size;o.delayMinutes=o.moves.reduce((n,m)=>n+m.delayMinutes,0);o.distanceKm=0;
          }
        }
        res.end(JSON.stringify({items:[c],serverNow}));return;
      }
      if (route==='/planning/plans') {res.end(JSON.stringify({plan:{id:c.planId,status:'confirmed'}}));return;}
      if (route==='/fleet/incidents') {res.end(JSON.stringify({items:[c.incident]}));return;}
      res.end(JSON.stringify({items:[]}));
    });
    server.middlewares.use('/__preview', async (req,res)=>{
      res.setHeader('Content-Type','text/html');res.end(await server.transformIndexHtml('/__preview','<!doctype html><html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><div id="root"></div><script type="module" src="/__recovery.tsx"></script></body></html>'));
    });
  },
};
const server = await createServer({configFile:false,root,plugins:[plugin,react()],optimizeDeps:{entries:[]},server:{host:'127.0.0.1',port:4321,strictPort:true}});
await server.listen();
console.log('Synthetic recovery examples: http://127.0.0.1:4321/__preview?role=dispatch&scenario=full');
