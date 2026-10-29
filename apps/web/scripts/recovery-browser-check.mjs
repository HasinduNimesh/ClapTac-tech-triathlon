// Run against recovery-preview.mjs. Requires a local Playwright installation.
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert=require('node:assert/strict');
const output = fileURLToPath(new URL('../../../docs/media/disruption-recovery-agent/', import.meta.url));
(async()=>{
 const browser=await chromium.launch({headless:true,...(process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : {})});
 const page=await browser.newPage({viewport:{width:1600,height:1100},deviceScaleFactor:1});const errors=[];page.on('pageerror',e=>errors.push(e.message));
 const base='http://127.0.0.1:4321/__preview';
 async function open(query){await page.goto(base+'?'+query);await page.getByRole('radiogroup',{name:'Recovery options'}).waitFor();}
 async function shot(name){await page.evaluate(()=>window.scrollTo(0,0));await page.waitForTimeout(350);await page.screenshot({path:output+name+'.png',fullPage:true});}
 await open('role=dispatch&scenario=full');
 await page.getByRole('button',{name:'Generate recovery options',exact:true}).click();await page.getByText('Recovery options are ready. Stores can now share their preference.').waitFor();
 assert.equal(await page.getByRole('radio').count(),3);
 assert(await page.getByRole('button',{name:'Approve and publish recovery'}).isDisabled());
 await page.getByRole('radio').nth(1).check();assert(await page.getByRole('radio').nth(1).isChecked());
 assert.equal(await page.locator('tbody tr').count(),4);
 await open('role=store&scenario=full');
 assert.equal(await page.locator('tbody tr').count(),1);assert.equal(await page.getByRole('button',{name:'Approve and publish recovery'}).count(),0);
 assert.equal(await page.getByText('ORD-1048',{exact:true}).count(),0);
 await page.getByLabel('What matters to your store? (optional)').fill('Please prioritise our chilled stock before the lunch rush. The receiving team is ready.');
 await page.getByRole('button',{name:'Send preference'}).click();await page.getByText('Preference sent to dispatch. Your delivery changes only after dispatcher approval.').waitFor();
 await shot('02-store-preference');
 await open('role=dispatch&scenario=full');await page.getByRole('heading',{name:'Store preferences',exact:true}).waitFor();
 await page.getByLabel('Decision note',{exact:true}).fill('Split the remaining load across three ready vehicles to reduce delay. Chilled deliveries go first; depot handover and all replacement drivers are confirmed.');
 await page.getByRole('checkbox').check();assert(await page.getByRole('button',{name:'Approve and publish recovery'}).isEnabled());
 await shot('01-dispatcher-comparison');
 await page.getByRole('button',{name:'Approve and publish recovery'}).click();await page.getByText('Recovery plan published',{exact:true}).waitFor();await shot('03-approved-plan');
 await open('role=dispatch&scenario=partial');assert.equal(await page.locator('tbody .rc-deferred').count(),2);await shot('04-partial-recovery');
 await open('role=dispatch&scenario=no-capacity');assert.equal(await page.locator('tbody .rc-deferred').count(),4);
 await shot('05-no-capacity');
 // A stale approval stays pending and gives an actionable conflict.
 await open('role=dispatch&scenario=partial&stale=1');await page.getByLabel('Decision note',{exact:true}).fill('Crew confirmed for this example');await page.getByRole('checkbox').check();await page.getByRole('button',{name:'Approve and publish recovery'}).click();await page.getByRole('alert').filter({hasText:'Fleet, delivery progress or plan changed'}).waitFor();assert.equal(await page.getByText('Recovery plan published',{exact:true}).count(),0);
 // The application expiry clock is anchored to server time, not Date.now.
 await page.evaluate(()=>{Date.now=()=>0});assert.equal(await page.getByText('Options expired',{exact:true}).count(),0);
 await page.setViewportSize({width:390,height:844});await open('role=store&scenario=partial');
 assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'mobile horizontal page overflow');
 await page.getByLabel('What matters to your store? (optional)').fill('Chilled stock is our priority.');await shot('06-mobile-store');
 assert.deepEqual(errors,[]);console.log('PASS: generation, comparison selection, store isolation, preference, approval gate, published result, partial/no-capacity, stale conflict, clock independence, mobile overflow; zero runtime errors.');
 await browser.close();
})().catch(e=>{console.error(e);process.exit(1)});
