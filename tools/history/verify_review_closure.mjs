import fs from 'node:fs/promises'
import path from 'node:path'
import {createRequire} from 'node:module'
import assert from 'node:assert/strict'
const require=createRequire(path.resolve('web/package.json'));const {chromium}=require('@playwright/test')
const base='http://127.0.0.1:18071',out=path.resolve('.ta-dev/history-complete/evidence/browser-closure')
const seed=JSON.parse(await fs.readFile(path.join(out,'seed.json'),'utf8'))
async function api(url,method='GET',body){const r=await fetch(base+url,{method,headers:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});assert(r.ok,`${url}: ${r.status}`);return r.json()}
const original=await api('/api/settings');assert.equal(path.resolve(original.paths.denova_dir).toLowerCase(),path.resolve('.ta-dev/history-complete/data').toLowerCase())
const browser=await chromium.launch({headless:true,executablePath:'E:\\游戏\\游戏开发\\TA-开发版\\.ta-dev\\playwright\\chromium-1234\\chrome-win64\\chrome.exe'})
try{
 await api('/api/settings','PATCH',{layer:'user',changes:{language:'zh-CN',theme:'light'},base_revision:original.revisions.user})
 let state=await api(seed.route)
 const order=state.world.orders.find(o=>o.kind==='policy'&&o.status==='active')
 const originalText=state.edicts.flatMap(e=>e.plan.orders||[]).find(o=>o.action==='create'&&o.text===order.text)?.text||order.text
 const newText='后续改为河南，保留既有累计支出，仅改变今后的施政范围。'
 if(order.text!==newText){const r=await api(seed.route+'/edict','POST',{command_id:'ui-amend-'+crypto.randomUUID(),expected_revision:state.revision,plan:{text:newText,orders:[{action:'revise',order_id:order.id,kind:order.kind,text:newText,minister_id:order.minister_id,targets:['henan'],limits:order.limits,condition:order.condition,depends_on:order.depends_on,not_before_turn:order.not_before_turn}]}});state=r.state}
 const context=await browser.newContext({viewport:{width:1440,height:1000}})
 await context.addInitScript(({project,campaign})=>localStorage.setItem(`ta:history:${project}:campaign`,campaign),seed)
 const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));await page.goto(base+'/?game=history')
 const all=page.getByRole('button',{name:'查看全部 9 项（另有 1 项）',exact:true});await all.click()
 assert(await page.getByText('陕西旱情延续与赈济办理',{exact:true}).first().isVisible())
 await page.screenshot({path:path.join(out,'all-unresolved.png')})
 await page.getByRole('tab',{name:'季度奏报',exact:true}).click();await page.locator('#history-world-report').selectOption(state.reports[0].id)
 const details=page.locator('#history-world-records details').filter({has:page.locator('summary').filter({hasText:'逐条政令结果'})});await details.locator('summary').click()
 assert(await details.getByText(originalText,{exact:true}).isVisible());assert.equal(await details.getByText(newText,{exact:true}).count(),0)
 await page.screenshot({path:path.join(out,'historical-order.png')})
 await page.getByRole('tab',{name:'召见与廷议',exact:true}).click()
 const chat=state.chats.find(c=>c.minister_ids.length===1&&!c.discarded);const person=state.ministers.find(m=>m.id===chat.minister_ids[0])
 await page.locator('.history-ministers button').filter({hasText:person.name}).click()
 const responsePromise=page.waitForResponse(r=>r.url().endsWith('/advice')&&r.request().method()==='POST')
 await page.getByRole('button',{name:'收藏这条建议',exact:true}).first().click()
 const saved=await(await responsePromise).json();const advice=saved.state.advice.at(-1)
 await page.getByRole('tab',{name:'颁诏',exact:true}).click()
 let card=page.locator(`[data-advice-id="${advice.id}"]`);await card.locator('summary').click();await card.getByRole('button',{name:'整理',exact:true}).click()
 await card.getByLabel('标题',{exact:true}).fill('陕西核查 · 待下季审议')
 await card.getByLabel('整理备注',{exact:true}).fill('仅收藏，不构成执行授权。'.repeat(35))
 const updatedPromise=page.waitForResponse(r=>r.url().endsWith('/advice')&&r.request().method()==='POST');await card.getByRole('button',{name:'保存整理',exact:true}).click();assert((await updatedPromise).ok())
 await page.reload();await page.getByRole('tab',{name:'颁诏',exact:true}).click();card=page.locator(`[data-advice-id="${advice.id}"]`);await card.locator('summary').click();assert(await card.getByText(advice.text,{exact:true}).isVisible())
 await page.screenshot({path:path.join(out,'saved-advice-desktop.png')})
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:path.join(out,'saved-advice-mobile.png')});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1))
 let before=await api(seed.route);const cp=await api(seed.route+'/checkpoints','POST',{command_id:'ui-checkpoint-'+crypto.randomUUID(),expected_revision:before.revision,label:'收藏整理后'});const checkpoint=cp.state.checkpoints.at(-1).id
 await page.reload();await page.getByRole('tab',{name:'颁诏',exact:true}).click();card=page.locator(`[data-advice-id="${advice.id}"]`);await card.locator('summary').click()
 const removedPromise=page.waitForResponse(r=>r.url().endsWith('/advice')&&r.request().method()==='POST');await card.getByRole('button',{name:'移出收藏',exact:true}).click();const removed=await(await removedPromise).json();assert(!removed.state.advice?.some(a=>a.id===advice.id));assert.equal(removed.state.chats.length,before.chats.length)
 const restored=await api(seed.route+'/restore','POST',{command_id:'ui-restore-'+crypto.randomUUID(),expected_revision:removed.state.revision,checkpoint_id:checkpoint});assert(restored.state.advice.some(a=>a.id===advice.id&&a.title==='陕西核查 · 待下季审议'))
 assert.equal(errors.length,0)
 await fs.writeFile(path.join(out,'result.json'),JSON.stringify({allNineUnresolved:true,historicalOrderUnchanged:true,bookmarkSaveEditReloadRemoveRestore:true,sourceChatPreserved:true,mobileOverflow:false,errors},null,2));console.log('PASS five review-closure UI flows')
}finally{await browser.close();const current=await api('/api/settings');await api('/api/settings','PATCH',{layer:'user',changes:{language:original.user.language??null,theme:original.user.theme??null},base_revision:current.revisions.user})}
