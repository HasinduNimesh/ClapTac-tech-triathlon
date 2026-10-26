// Run against tasks-preview.mjs. Requires PLAYWRIGHT_MODULE and optionally CHROME_PATH.
import assert from 'node:assert/strict';
import {mkdir} from 'node:fs/promises';
import {fileURLToPath,pathToFileURL} from 'node:url';
const {chromium}=await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE).href);
const output=fileURLToPath(new URL('../../../docs/media/priority-todo-agents/',import.meta.url));
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.CHROME_PATH||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'});
const errors=[];const requests=[];
const context=await browser.newContext({viewport:{width:1440,height:1050},deviceScaleFactor:1});
const page=await context.newPage();page.setDefaultTimeout(15000);page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>requests.push(r.url()));
async function count(n){console.log("Checking task count",n);await page.waitForFunction(n=>document.querySelectorAll('.pt-task').length===n,n,{timeout:15000}).catch(async e=>{console.log(await page.locator('.pt-page').innerText());throw e;});}
async function shot(name){await page.evaluate(()=>window.scrollTo(0,0));await page.waitForTimeout(350);await page.screenshot({path:output+name,fullPage:true});}
async function scenario(name){await page.evaluate(name=>{const url=new URL(location.href);url.searchParams.set('scenario',name);history.replaceState({},'',url);},name);await page.getByRole('button',{name:'Refresh tasks'}).click();}
try{
 await page.goto('http://127.0.0.1:4322/__preview?role=dispatch');await count(7);
 assert.match(await page.locator('.pt-task').first().innerText(),/Critical/);
 assert.equal(await page.locator('.pt-task').first().getByRole('link').getAttribute('href'),'/dispatcher/notifications?date=2026-10-27');
 await shot('01-dispatcher-priorities.png');
 await page.getByLabel('Search tasks',{exact:true}).fill('wpnb7134');await count(1);
 assert.match(await page.locator('.pt-task').innerText(),/WP NB 7134/);
 await shot('02-vehicle-search.png');
 await page.getByLabel('Search tasks',{exact:true}).fill('');await count(7);
 await page.getByRole('button',{name:/^High/}).click();await count(5);
 await page.getByLabel(/Filter by type/).selectOption('Recovery');await count(1);
 await page.getByRole('button',{name:/^Known tasks/}).click();await page.getByLabel(/Filter by type/).selectOption('');await count(7);
 await page.getByLabel(/Sort by/).selectOption('time');
 assert.match(await page.locator('.pt-task').first().innerText(),/Resolve a dock alert/);
 await page.getByLabel(/Sort by/).selectOption('priority');
 await scenario('partial');await page.getByRole('alert').waitFor();await count(7);
 assert.match(await page.locator('.pt-task').first().innerText(),/Needs refresh/);
 await shot('03-source-outage.png');
 await scenario('full');await page.waitForFunction(()=>!document.querySelector('.pt-warning'));await count(7);
 await scenario('complete');await count(0);await page.getByText('No tasks from the checked sources',{exact:true}).waitFor();
 await shot('04-resolved-tasks.png');
 requests.length=0;await page.goto('http://127.0.0.1:4322/__preview?role=store');await count(6);assert.equal(requests.some(url=>/api\/v1\/(fleet|loading)|api\/v1\/planning\/plans/.test(url)),false,'Store task queue must not fetch dispatcher-only sources');
 assert.match(await page.locator('.pt-task').first().innerText(),/ORD-1027-041/);
 assert.equal(await page.locator('.pt-task').first().getByRole('link').getAttribute('href'),'/store-manager/receipts?order=041');
 await shot('05-store-priorities.png');
 await scenario('tracking-failure');await page.getByRole('alert').waitFor();await count(6);
 assert.match(await page.locator('.pt-task').first().innerText(),/Needs refresh/);
 await scenario('complete');await count(0);
 await page.goto('http://127.0.0.1:4322/__preview?role=store&scenario=offline');await page.getByRole('alert').waitFor();
 await page.getByText('Waiting for complete data',{exact:true}).waitFor();assert.equal(await page.getByText('No tasks from the checked sources',{exact:true}).count(),0);
 await page.goto('http://127.0.0.1:4322/__preview?role=store');await count(6);
 await page.setViewportSize({width:390,height:844});await shot('06-mobile-store.png');
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Mobile must not overflow');
 await page.addInitScript(()=>{const NativeDate=Date;class FakeDate extends NativeDate{constructor(...args){super(...(args.length?args:['2040-01-01T00:00:00Z']));}static now(){return new NativeDate('2040-01-01T00:00:00Z').getTime();}}window.Date=FakeDate;});
 await page.reload();await count(6);assert.match(await page.locator('.pt-meta').innerText(),/27 Oct/);
 await page.setViewportSize({width:1440,height:1050});
 const many=Array.from({length:25},(_,i)=>({id:`bulk-${i}`,orderRef:`BULK-${String(i).padStart(3,'0')}`,outletId:'OUT001',requestedDeliveryDate:'2026-10-27'}));
 await page.route('**/api/v1/orders',route=>route.fulfill({json:{items:many}}));
 await page.route('**/api/v1/orders/*/tracking',route=>{const id=new URL(route.request().url()).pathname.split('/')[4];return route.fulfill({json:{tracking:{order:many.find(o=>o.id===id),stage:'DEFERRED',planning:{}}}});});
 await page.goto('http://127.0.0.1:4322/__preview?role=store&scenario=empty');await count(12);
 await page.getByRole('button',{name:'Next',exact:true}).click();await count(12);await page.getByRole('button',{name:'Next',exact:true}).click();await count(1);
 await page.getByLabel('Search tasks',{exact:true}).fill('BULK-000');await count(1);assert.match(await page.locator('.pt-task').innerText(),/BULK-000/);
 assert.deepEqual(errors,[]);
 console.log('Passed: both roles, priority and time sorting, combined filters, normalized vehicle search, date links, source failure retention, automatic resolution, first-load failure, device clock changes, mobile overflow, pagination, store-only sources; no runtime errors.');
} finally {await browser.close();}
