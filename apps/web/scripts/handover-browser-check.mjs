// Actual React workflow with synthetic peer responses; see handover-preview.mjs.
import assert from 'node:assert/strict';
import {mkdir} from 'node:fs/promises';
import {fileURLToPath,pathToFileURL} from 'node:url';
const {chromium}=await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE).href);
const output=fileURLToPath(new URL('../../../docs/media/shift-handover-agent/',import.meta.url));await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.CHROME_PATH||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'});
const context=await browser.newContext({viewport:{width:1500,height:1080},deviceScaleFactor:1});const page=await context.newPage();page.setDefaultTimeout(15000);const errors=[];page.on('pageerror',e=>errors.push(e.message));
const url=(role,extra='')=>`http://127.0.0.1:4323/__preview?role=${role}${extra}`;
async function screenshot(name){await page.evaluate(()=>window.scrollTo(0,0));await page.waitForTimeout(350);await page.screenshot({path:output+name,fullPage:true});}
async function generate(role,title,notes,extra=''){
 await page.goto(url(role,extra));await page.getByLabel('Shift title',{exact:true}).fill(title);await page.getByLabel('Notes for the next shift',{exact:true}).fill(notes);await page.getByRole('button',{name:'Generate handover brief',exact:true}).click();await page.locator('.ho-counts').waitFor();
 await page.waitForFunction(()=>!document.querySelector('.ho-publish button')?.disabled);
}
async function incoming(role,title){await page.goto(url(role,'&person=incoming'));await page.getByRole('button',{name:/Team handovers/}).click();await page.getByRole('button',{name:new RegExp(title)}).click();await page.locator('.ho-report h2').waitFor();}
try{
 await generate('dispatch','Morning → afternoon dispatch','WP NA 4821: cargo handover is being coordinated at Peliyagoda. Review the store preference before approving recovery.\nAsk the loading lead to confirm the three chilled short lines first.');
 assert.equal(await page.locator('.ho-tasks li').count(),7);await screenshot('01-dispatcher-brief.png');
 await page.getByRole('button',{name:'Publish to next shift',exact:true}).click();await page.getByText('Ready for another person to receive',{exact:true}).waitFor();
 assert.equal(await page.getByRole('button',{name:'Acknowledge handover',exact:true}).count(),0);
 await incoming('dispatch','Morning → afternoon dispatch');
 assert.equal(await page.getByRole('button',{name:'Acknowledge handover',exact:true}).isDisabled(),true);
 await screenshot('02-incoming-dispatcher.png');
 await page.getByRole('checkbox').check();await page.getByRole('button',{name:'Acknowledge handover',exact:true}).click();await page.getByText('✓ Received by Maya · Dispatch',{exact:true}).waitFor();await screenshot('03-acknowledged-dispatcher.png');
 assert.equal(await page.locator('.ho-tasks li').count(),7,'Acknowledgement must not complete tasks');
 await page.reload();await page.getByRole('button',{name:/Team handovers/}).click();await page.getByRole('button',{name:/Morning → afternoon dispatch/}).click();await page.getByText('✓ Received by Maya · Dispatch',{exact:true}).waitFor();
 await generate('store','Store afternoon handover','Confirm the overdue receipt for ORD-1027-041 after the quantity check.\nKeep receiving space free for ORD-1027-062, expected at 10:10. The recovery choice still needs our preference.');
 assert.equal(await page.locator('.ho-tasks li').count(),6);await screenshot('04-store-brief.png');
 await page.getByRole('button',{name:'Publish to next shift',exact:true}).click();await page.getByText('Ready for another person to receive',{exact:true}).waitFor();
 await incoming('store','Store afternoon handover');await screenshot('05-incoming-store.png');
 await page.setViewportSize({width:390,height:844});await screenshot('06-mobile-handover.png');
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Mobile overflow');
 await page.getByRole('checkbox').check();await page.getByRole('button',{name:'Acknowledge handover',exact:true}).click();await page.getByText('✓ Received by Dilani · Store manager',{exact:true}).waitFor();
 await page.getByLabel('Handover store',{exact:true}).selectOption('OUT002');await page.getByRole('button',{name:/Team handovers/}).click();await page.getByText('No published handovers for this audience yet.',{exact:true}).waitFor();assert.equal(await page.locator('.ho-report h2').count(),0,'Changing store clears selected report');
 await page.setViewportSize({width:1500,height:1080});
 await page.goto(url('dispatch','&scenario=partial'));await page.getByRole('button',{name:'Generate handover brief',exact:true}).click();await page.getByRole('alert').waitFor();assert.equal(await page.getByRole('button',{name:'Publish to next shift',exact:true}).count(),0,'Incomplete sources cannot publish');
 await generate('dispatch','Retry-safe handover','A simulated response is lost after the server saves the report.','&scenario=lost-response');
 await page.getByRole('button',{name:'Publish to next shift',exact:true}).click();await page.getByRole('button',{name:'Retry publication',exact:true}).waitFor();assert.equal(await page.getByLabel('Shift title',{exact:true}).isDisabled(),true);
 await page.getByRole('button',{name:'Retry publication',exact:true}).click();await page.getByText('Ready for another person to receive',{exact:true}).waitFor();assert.equal(await page.locator('.ho-history-item').filter({hasText:'Retry-safe handover'}).count(),1,'Retry should create one report');
 await page.addInitScript(()=>{const NativeDate=Date;class FakeDate extends NativeDate{constructor(...args){super(...(args.length?args:['2040-01-01T00:00:00Z']));}static now(){return new NativeDate('2040-01-01T00:00:00Z').getTime();}}window.Date=FakeDate;});
 await generate('store','Server-clock check','The device clock is in 2040.');assert.equal(await page.locator('.ho-tasks li').count(),6);assert.match(await page.locator('.ho-asof').innerText(),/27 Oct/);
 await generate('store','Stale snapshot check','','&scenario=stale');await page.getByRole('button',{name:'Publish to next shift',exact:true}).click();await page.getByRole('alert').waitFor();assert.equal(await page.getByLabel('Shift title',{exact:true}).isDisabled(),false,'Known rejection unlocks the draft for refreshing');
 assert.deepEqual(errors,[]);console.log('Passed: both roles, generation, review/publication, different-person acknowledgement, self-ack gating, persistence after reload, unchanged task counts, outlet switching, source failure blocking, idempotent retry, server/device clock separation, stale rejection, mobile overflow; no runtime errors.');
}finally{await browser.close();}
