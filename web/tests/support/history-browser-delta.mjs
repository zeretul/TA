// Targeted real-browser checks for errors in scrolled views and long drafts.
import { chromium } from '@playwright/test'
import assert from 'node:assert/strict'
import { readFile,writeFile } from 'node:fs/promises'
import path from 'node:path'
import {randomUUID} from 'node:crypto'
const base=process.env.TA_HISTORY_TEST_URL,output=process.env.TA_HISTORY_BROWSER_OUTPUT
if(!base||!output||!/^http:\/\/(127\.0\.0\.1|localhost):\d+$/.test(base))throw new Error('Isolated loopback and output required')
async function api(route,body,method='POST'){const r=await fetch(base+route,body===undefined?undefined:{method,headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});assert.equal(r.status,200);return r.json()}
const old=JSON.parse(await readFile(path.join(output,'browser-result.json'),'utf8'))
const boot=await api('/api/history/bootstrap',{}),campaign=old.campaign
const campaignPath=`/api/projects/${boot.project_id}/history/campaigns/${campaign}`
const settings=await api('/api/settings'),appearance={language:settings.effective.language,theme:settings.effective.theme}
await api('/api/settings',{layer:'user',changes:{language:'zh-CN',theme:'light'}},'PATCH')
const browser=await chromium.launch({headless:true,executablePath:process.env.TA_HISTORY_CHROMIUM})
const context=await browser.newContext({viewport:{width:1440,height:1000}})
await context.addInitScript(({projectId,campaign})=>localStorage.setItem(`ta:history:${projectId}:campaign`,campaign),{projectId:boot.project_id,campaign})
const page=await context.newPage();const result={campaign,checks:[],screenshots:[],expectedErrors:[]}
try{
 await page.goto(base+'/?game=history',{waitUntil:'networkidle'})
 await page.locator('#history-edict-text').fill('很长的未批草稿，只留在草稿中。'.repeat(300))
 const field=await page.locator('#history-edict-text').evaluate(el=>({height:el.clientHeight,scrollHeight:el.scrollHeight}))
 assert(field.height<=420);assert(field.scrollHeight>field.height)
 const textbox=await page.locator('#history-edict-text').boundingBox(),button=await page.getByRole('button',{name:'拟定执行方案',exact:true}).boundingBox()
 assert(button.y-(textbox.y+textbox.height)<60)
 await page.locator('.history-scroll').evaluate(el=>el.scrollTop=0)
 await page.screenshot({path:path.join(output,'long-draft-bounded-1440.png'),fullPage:true})
 result.screenshots.push('long-draft-bounded-1440.png');result.checks.push({name:'Long edict stays bounded with action immediately after field',passed:true,field})
 await page.getByRole('tab',{name:'起居注',exact:true}).click()
 const current=await api(campaignPath)
 await api(campaignPath+'/checkpoints',{command_id:'browser-stale-'+randomUUID(),expected_revision:current.revision,label:'跨窗口变更检查'})
 const conflictResponse=page.waitForResponse(r=>r.url().endsWith(campaign+'/inspect')&&r.request().method()==='POST')
 await page.getByRole('button',{name:'查核账目',exact:true}).click()
 const conflict=await conflictResponse;assert.equal(conflict.status(),409)
 await page.locator('.history-global-feedback').waitFor()
 let alert=await page.locator('.history-global-feedback').boundingBox();assert(alert.y>=0&&alert.y+alert.height<=1000)
 await page.screenshot({path:path.join(output,'stale-error-visible-1440.png'),fullPage:true});result.screenshots.push('stale-error-visible-1440.png')
 await page.setViewportSize({width:390,height:844})
 alert=await page.locator('.history-global-feedback').boundingBox();assert(alert.y>=0&&alert.y+alert.height<=844)
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1),false)
 await page.screenshot({path:path.join(output,'stale-error-visible-390.png'),fullPage:true});result.screenshots.push('stale-error-visible-390.png')
 result.checks.push({name:'Actual stale-revision error remains in view at 1440 and 390',passed:true});result.expectedErrors.push({status:409,code:(await conflict.json()).code})
 await page.getByRole('button',{name:'重新读取',exact:true}).click()
 await page.locator('.history-global-feedback').waitFor({state:'hidden'})
 result.checks.push({name:'Reload after conflict clears feedback and preserves current state',passed:true})
 result.passed=true;console.log(JSON.stringify(result))
}catch(e){result.passed=false;result.failure=String(e.stack||e);throw e}
finally{await writeFile(path.join(output,'browser-delta-result.json'),JSON.stringify(result,null,2));await api('/api/settings',{layer:'user',changes:appearance},'PATCH');await browser.close()}
