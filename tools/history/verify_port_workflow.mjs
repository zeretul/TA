// Exercise saved advice and map movement without issuing another decree.
import fs from 'node:fs/promises'
import path from 'node:path'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
const require=createRequire(path.resolve('web/package.json'))
const {chromium}=require('@playwright/test')
const base='http://127.0.0.1:18071'
const out=path.resolve('.ta-dev/history-complete/evidence/port-workflow')
await fs.mkdir(out,{recursive:true})
async function api(url,method='GET',body){const r=await fetch(base+url,{method,headers:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});assert(r.ok,`${url}: ${r.status}`);return r.json()}
const original=await api('/api/settings')
assert.equal(path.resolve(original.paths.denova_dir).toLowerCase(),path.resolve('.ta-dev/history-complete/data').toLowerCase())
const movement=JSON.parse(await fs.readFile('.ta-dev/history-complete/evidence/reference-assets-movement/result.json','utf8'))
const main=JSON.parse(await fs.readFile('.ta-dev/history-complete/evidence/final-ming-1627/result.json','utf8'))
const browser=await chromium.launch({headless:true,executablePath:'E:\\游戏\\游戏开发\\TA-开发版\\.ta-dev\\playwright\\chromium-1234\\chrome-win64\\chrome.exe'})
const errors=[]
try{
 await api('/api/settings','PATCH',{layer:'user',changes:{language:'zh-CN',theme:'light'},base_revision:original.revisions.user})
 const context=await browser.newContext({viewport:{width:1440,height:1000}})
 await context.addInitScript(({project,campaign})=>localStorage.setItem(`ta:history:${project}:campaign`,campaign),movement)
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 await page.goto(base+'/?game=history')
 await page.locator('.history-map-region[aria-label="北直隶"]').click()
 assert(await page.locator('.history-map-detail').getByText('山海关增援队',{exact:true}).isVisible())
 await page.screenshot({path:path.join(out,'persisted-arrival.png')})
 await page.evaluate(({project,campaign})=>localStorage.setItem(`ta:history:${project}:campaign`,campaign),main)
 // A fresh page is needed because the startup script intentionally binds a campaign.
 const mainContext=await browser.newContext({viewport:{width:1440,height:1000}})
 await mainContext.addInitScript(({project,campaign})=>localStorage.setItem(`ta:history:${project}:campaign`,campaign),main)
 const mainPage=await mainContext.newPage();mainPage.on('pageerror',e=>errors.push(e.message))
 const route=`/api/projects/${main.project}/history/campaigns/${main.campaign}`
 const before=await api(route)
 await mainPage.goto(base+'/?game=history')
 await mainPage.getByRole('tab',{name:'颁诏',exact:true}).click()
 const draft='请结合所选议事，拟一份只继续已授权查账与试制、不追加额度的草案，保留已有秘密的知情范围。'
 await mainPage.locator('#history-edict-text').fill(draft)
 await mainPage.locator('.history-edict-reference summary').filter({hasText:'近期议事'}).click()
 const responsePromise=mainPage.waitForResponse(r=>r.url().endsWith('/prepare-edict'),{timeout:0})
 await mainPage.getByRole('button',{name:'据此议事拟诏',exact:true}).first().click()
 const response=await responsePromise;assert(response.ok(),`Draft failed: ${response.status()}`)
 const prepared=await response.json()
 assert(prepared.plan.source_chat_id)
 assert.equal(prepared.plan.text,draft)
 assert(before.chats.some(chat=>chat.id===prepared.plan.source_chat_id))
 await mainPage.getByRole('heading',{name:'待批方案',exact:true}).waitFor()
 assert(await mainPage.locator('summary').filter({hasText:'议事来源'}).isVisible())
 assert.deepEqual(await api(route),before,'Preview unexpectedly mutated the campaign')
 await mainPage.screenshot({path:path.join(out,'saved-advice-preview.png')})
 await fs.writeFile(path.join(out,'prepared.json'),JSON.stringify(prepared,null,2))
 assert.equal(errors.length,0)
 await fs.writeFile(path.join(out,'result.json'),JSON.stringify({mapArrival:true,savedAdviceSource:prepared.plan.source_chat_id,draftRetained:true,noCommit:true,errors},null,2))
 console.log('PASS map arrival and saved-advice preview')
}finally{
 await browser.close()
 const current=await api('/api/settings')
 await api('/api/settings','PATCH',{layer:'user',changes:{language:original.user.language??null,theme:original.user.theme??null},base_revision:current.revisions.user})
}
