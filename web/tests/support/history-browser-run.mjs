// Real-browser acceptance against a disposable native candidate. No API mocks.
import assert from 'node:assert/strict'
import { chromium } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'

const base=process.env.TA_HISTORY_TEST_URL
const output=process.env.TA_HISTORY_BROWSER_OUTPUT
const executablePath=process.env.TA_HISTORY_CHROMIUM
if (!base || !output || !executablePath || process.env.TA_HISTORY_ALLOW_MODEL_TESTS !== '1') throw new Error('Explicit isolated URL, output, Chromium and model-test authorization are required')
if(!/^http:\/\/(127\.0\.0\.1|localhost):\d+$/.test(base))throw new Error('Loopback only')
await mkdir(output,{recursive:true})
const campaignName=`历史界面隔离验收 · ${String(Date.now()).slice(-6)}`
const secondName=`历史界面切换验收 · ${String(Date.now()).slice(-6)}`
const results={started:new Date().toISOString(),campaign:null,checks:[],screenshots:[],errors:[],failedRequests:[]}
async function persist(){await writeFile(path.join(output,'browser-result.json'),JSON.stringify(results,null,2))}
async function api(route,body,method='POST'){
 const r=await fetch(base+route,body===undefined?undefined:{method,headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})
 const json=await r.json();assert.equal(r.status,200,`${method} ${route}: ${r.status}`);return json
}
const originalSettings=await api('/api/settings')
const originalAppearance={language:originalSettings.user?.language ?? originalSettings.effective?.language ?? 'zh-CN',theme:originalSettings.user?.theme ?? originalSettings.effective?.theme ?? 'dark'}
const originalWorkspace=await api('/api/workspace/current')
const browser=await chromium.launch({executablePath,headless:true})
const context=await browser.newContext({viewport:{width:1440,height:1000},deviceScaleFactor:1})
const page=await context.newPage()
page.on('pageerror',error=>results.errors.push(error.message))
page.on('console',message=>{if(message.type()==='error')results.errors.push(message.text())})
page.on('requestfailed',request=>{if(!request.failure()?.errorText?.includes('ERR_ABORTED'))results.failedRequests.push({url:request.url().replace(base,''),failure:request.failure()?.errorText})})
page.setDefaultTimeout(15000)
let campaignPath
try{
 await api('/api/settings',{layer:'user',changes:{language:'zh-CN',theme:'light'}},'PATCH')
 await page.goto(base+'/?game=history',{waitUntil:'networkidle'})
 await page.getByTestId('history-game').waitFor()
 await page.getByRole('button',{name:'新开一局',exact:true}).click()
 await page.getByRole('textbox',{name:'存档名称',exact:true}).fill(campaignName)
 const createdResponse=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/history/campaigns'))
 await page.getByRole('button',{name:'建立新局',exact:true}).click()
 const created=await (await createdResponse).json()
 assert(created.id&&created.id!=='campaign-a5887debbd09b3c8939df991')
 results.campaign=created.id
 const boot=await api('/api/history/bootstrap',{})
 campaignPath=`/api/projects/${boot.project_id}/history/campaigns/${created.id}`
 await page.getByRole('heading',{name:campaignName,exact:true}).waitFor()
 results.checks.push({name:'Create independent campaign through UI',passed:true})
 await persist()
 if(process.env.TA_HISTORY_SKIP_MODEL_TEST !== '1'){
 // One real-model audience proves the visible conversation reaches the candidate backend.
 await page.getByRole('tab',{name:'召见与廷议',exact:true}).click()
 await page.getByRole('textbox',{name:'对大臣说',exact:true}).fill('毕卿，请简短报出当前国库可用银与官仓存粮，并说明这次问话不会动用钱粮。只按当前账目回答。')
 const chatResponse=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith(`${created.id}/chat`),{timeout:300000})
 await page.getByRole('button',{name:'传谕',exact:true}).click()
 const chat=await(await chatResponse).json()
 assert(chat.state?.chats.at(-1)?.replies[0]?.text.trim())
 assert.equal(chat.state.treasury,created.treasury);assert.equal(chat.state.grain,created.grain)
 await page.locator('.history-minister-message').last().waitFor()
 results.checks.push({name:'Real-model private audience via UI, no resource charge',passed:true,reply:chat.state.chats.at(-1).replies[0].text})
 await persist()
 } else { results.modelSkipped=true; await persist() }
 // Explicit checkpoint plus cost-free inspection and restore.
 await page.getByRole('button',{name:'存档与回溯',exact:true}).first().click()
 await page.getByRole('textbox',{name:'节点名称',exact:true}).fill('界面验收节点')
 const checkpointResponse=page.waitForResponse(r=>r.url().endsWith(`${created.id}/checkpoints`)&&r.request().method()==='POST')
 await page.getByRole('button',{name:'留存节点',exact:true}).click()
 const checkpoint=await(await checkpointResponse).json()
 assert(checkpoint.state.checkpoints.some(c=>c.label==='界面验收节点'))
 await page.keyboard.press('Escape')
 await page.getByRole('tab',{name:'起居注',exact:true}).click()
 const inspectionResponse=page.waitForResponse(r=>r.url().endsWith(`${created.id}/inspect`)&&r.request().method()==='POST')
 await page.getByRole('button',{name:'查核账目',exact:true}).click()
 const inspection=await(await inspectionResponse).json()
 assert.equal(inspection.state.treasury,created.treasury);assert.equal(inspection.state.grain,created.grain)
 await page.getByText('最新查核',{exact:false}).waitFor()
 await page.getByRole('button',{name:'存档与回溯',exact:true}).first().click()
 await page.getByRole('button',{name:'回到此处',exact:true}).click()
 const restoreResponse=page.waitForResponse(r=>r.url().endsWith(`${created.id}/restore`)&&r.request().method()==='POST')
 await page.getByRole('button',{name:'确认回溯',exact:true}).click()
 const restored=await(await restoreResponse).json()
 assert(restored.state.inspections.at(-1)?.discarded)
 assert.equal(restored.state.treasury,created.treasury)
 await page.getByRole('dialog').waitFor({state:'hidden'})
 assert.equal(await page.locator('.history-records').getByText('最新查核',{exact:false}).count(),0)
 results.checks.push({name:'Checkpoint, inspection without resource charge, restore hides discarded inspection',passed:true})
 // A second independently created save tests the switching UI without changing root's test save.
 await page.getByRole('button',{name:'新开一局',exact:true}).click()
 await page.getByRole('textbox',{name:'存档名称',exact:true}).fill(secondName)
 const secondResponse=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/history/campaigns'))
 await page.getByRole('button',{name:'建立新局',exact:true}).click()
 const second=await(await secondResponse).json();results.secondCampaign=second.id
 await page.getByRole('heading',{name:secondName,exact:true}).waitFor()
 await page.getByRole('button',{name:'存档与回溯',exact:true}).first().click()
 const saveRow=page.getByRole('dialog',{name:'历史存档',exact:true}).locator('li').filter({has:page.getByText(campaignName,{exact:true})})
 await saveRow.getByRole('button',{name:'切换存档',exact:true}).click()
 await page.getByRole('heading',{name:campaignName,exact:true}).waitFor()
 results.checks.push({name:'Switch own historical saves via UI',passed:true})
 // Long draft survives a cold page reload and changing game destinations.
 await page.getByRole('tab',{name:'拟诏',exact:true}).click()
 const draft='此为未批行的长诏草稿，先核赈务、再议水利，不准挪用内帑。'.repeat(90)
 await page.getByRole('textbox',{name:'诏令原文',exact:true}).fill(draft)
 await page.reload({waitUntil:'networkidle'})
 assert.equal(await page.getByRole('textbox',{name:'诏令原文',exact:true}).inputValue(),draft)
 // Capture 8 real rendered combinations; each retains the same isolated campaign.
 for(const locale of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [1440,390]){
  await api('/api/settings',{layer:'user',changes:{language:locale,theme}},'PATCH')
  await page.setViewportSize({width,height:width===390?844:1000})
  await page.reload({waitUntil:'networkidle'})
  await page.getByRole('heading',{name:campaignName,exact:true}).waitFor()
  await page.waitForFunction(({locale,theme})=>document.documentElement.lang===locale&&document.documentElement.getAttribute('data-theme')===theme,{locale,theme})
  assert.equal(await page.locator('#history-edict-text').inputValue(),draft)
  const overflow=await page.evaluate(()=>({document:document.documentElement.scrollWidth>innerWidth+1,game:document.querySelector('.history-game').scrollWidth>document.querySelector('.history-game').clientWidth+1}))
  assert.equal(overflow.document,false,`document horizontal overflow ${locale}/${theme}/${width}`)
  assert.equal(overflow.game,false,`game horizontal overflow ${locale}/${theme}/${width}`)
  const file=`history-${locale}-${theme}-${width}.png`
  await page.screenshot({path:path.join(output,file),fullPage:true})
  results.screenshots.push(file);results.checks.push({name:`Layout ${locale} ${theme} ${width}`,passed:true,overflow})
  if(width===390){
   await page.getByRole('tab',{name:locale==='zh-CN'?'召见与廷议':'Audience & council',exact:true}).click()
   await page.screenshot({path:path.join(output,`court-${locale}-${theme}-${width}.png`),fullPage:true})
   results.screenshots.push(`court-${locale}-${theme}-${width}.png`)
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1),false)
  }
  await persist()
 }
 await api('/api/settings',{layer:'user',changes:{language:'zh-CN',theme:'light'}},'PATCH')
 await page.setViewportSize({width:1440,height:1000});await page.reload({waitUntil:'networkidle'})
 const beforeAdventure=await api('/api/workspace/current')
 await page.getByRole('navigation',{name:'游戏玩法',exact:true}).getByRole('button',{name:'冒险',exact:true}).click()
 // Empty adventure workspace should reach a neutral chooser; it must not create/select a Book.
 await page.getByRole('heading',{name:'从哪一段故事开始',exact:true}).waitFor()
 await page.screenshot({path:path.join(output,'game-chooser.png'),fullPage:true});results.screenshots.push('game-chooser.png')
 const afterAdventure=await api('/api/workspace/current')
 assert.equal(afterAdventure.project_id,beforeAdventure.project_id);assert.equal(afterAdventure.workspace,beforeAdventure.workspace)
 await page.getByRole('button',{name:'进入历史推演',exact:true}).click()
 await page.getByRole('heading',{name:campaignName,exact:true}).waitFor()
 assert.equal(await page.getByRole('textbox',{name:'诏令原文',exact:true}).inputValue(),draft)
 results.checks.push({name:'Neutral game chooser and return preserve selected history, draft and adventure workspace',passed:true})
 const finalWorkspace=await api('/api/workspace/current')
 assert.equal(finalWorkspace.project_id,originalWorkspace.project_id);assert.equal(finalWorkspace.workspace,originalWorkspace.workspace)
 assert.equal(results.errors.length,0,'No browser errors expected')
 assert.equal(results.failedRequests.length,0,'No failed network requests expected')
 results.passed=true;results.finished=new Date().toISOString();await persist()
 console.log(JSON.stringify({passed:true,campaign:results.campaign,checks:results.checks.length,screenshots:results.screenshots.length}))
}catch(error){results.passed=false;results.failure=String(error?.stack||error);await persist();await page.screenshot({path:path.join(output,'failure.png'),fullPage:true}).catch(()=>{});throw error}
finally{await api('/api/settings',{layer:'user',changes:originalAppearance},'PATCH').catch(()=>{});await browser.close()}
