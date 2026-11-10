// Real selected-discussion -> AI draft -> explicit issue -> cold read acceptance.
import {chromium} from '@playwright/test'
import assert from 'node:assert/strict'
import {writeFile} from 'node:fs/promises'
import path from 'node:path'
const base=process.env.TA_HISTORY_TEST_URL,output=process.env.TA_HISTORY_BROWSER_OUTPUT
if(!base||!output||process.env.TA_HISTORY_ALLOW_MODEL_TESTS!=='1'||!/^http:\/\/(127\.0\.0\.1|localhost):\d+$/.test(base))throw new Error('Explicit isolated loopback model-test authorization required')
async function api(route,body,method='POST'){const r=await fetch(base+route,body===undefined?undefined:{method,headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});assert.equal(r.status,200,`${route}: ${r.status}`);return r.json()}
const settings=await api('/api/settings'),appearance={language:settings.effective.language,theme:settings.effective.theme},workspaceBefore=await api('/api/workspace/current')
const name=`议事拟诏隔离验收 · ${String(Date.now()).slice(-6)}`
const result={started:new Date().toISOString(),name,checks:[],errors:[]}
await api('/api/settings',{layer:'user',changes:{language:'zh-CN',theme:'light'}},'PATCH')
const browser=await chromium.launch({headless:true,executablePath:process.env.TA_HISTORY_CHROMIUM})
const context=await browser.newContext({viewport:{width:1440,height:1000}}),page=await context.newPage()
page.setDefaultTimeout(15000)
page.on('pageerror',error=>result.errors.push(error.message));page.on('console',message=>{if(message.type()==='error')result.errors.push(message.text())})
const persist=()=>writeFile(path.join(output,'discussion-result.json'),JSON.stringify(result,null,2))
const responseFor=(suffix)=>page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith(suffix),{timeout:300000})
function mentionsNumber(text,value){const clean=text.replaceAll(',','').replaceAll('，','');if(clean.includes(String(value)))return true;return [...clean.matchAll(/(\d+(?:\.\d+)?)\s*(万|千)/g)].some(m=>Number(m[1])*(m[2]==='万'?10000:1000)===value)}
try{
 await page.goto(base+'/?game=history',{waitUntil:'networkidle'})
 assert.equal(await page.getByText('尚未选择世界',{exact:true}).count(),0,'History must not show the adventure world picker')
 await page.getByRole('button',{name:'新开一局',exact:true}).click()
 await page.getByRole('textbox',{name:'存档名称',exact:true}).fill(name)
 const createResponse=responseFor('/history/campaigns')
 await page.getByRole('button',{name:'建立新局',exact:true}).click()
 const createHTTP=await createResponse;assert.equal(createHTTP.status(),200);const initial=await createHTTP.json()
 result.campaign=initial.id
 const boot=await api('/api/history/bootstrap',{}),campaignPath=`/api/projects/${boot.project_id}/history/campaigns/${initial.id}`
 await page.getByRole('heading',{name,exact:true}).waitFor()
 // Keep an independent hand-written draft to exercise preservation in the real UI.
 const manualDraft='另有练兵事项尚未决定，保留此手写草稿。'
 await page.getByRole('textbox',{name:'诏令原文',exact:true}).fill(manualDraft)
 await page.getByRole('tab',{name:'召见与廷议',exact:true}).click()
 const prompt='陕西赈灾请给我一个现在能承担的明确方案，说清负责人、具体银粮、几季办完，只提一个方案，不要执行。银子不要超过十万两。'
 await page.getByRole('textbox',{name:'对大臣说',exact:true}).fill(prompt)
 const chatResponse=responseFor(initial.id+'/chat')
 await page.getByRole('button',{name:'传谕',exact:true}).click()
 const chatHTTP=await chatResponse;result.chatStatus=chatHTTP.status();const chatBody=await chatHTTP.json();result.chatResponse=chatBody;await persist();assert.equal(chatHTTP.status(),200)
 const chat=chatBody.state.chats.at(-1),reply=chat.replies[0].text
 assert(chat.id&&reply.trim());result.sourceChatId=chat.id;result.proposal=reply
 assert.equal(chatBody.state.projects.length,0);assert.equal(chatBody.state.treasury,initial.treasury);assert.equal(chatBody.state.grain,initial.grain)
 await page.locator('.history-pending').waitFor({state:'hidden'})
 const exchange=page.locator('.history-exchange').filter({has:page.getByText(prompt,{exact:true})})
 const prepareResponse=responseFor(initial.id+'/prepare-edict')
 await exchange.getByRole('button',{name:'根据议事拟诏',exact:true}).click()
 const prepareHTTP=await prepareResponse,resultPlan=await prepareHTTP.json();result.prepareStatus=prepareHTTP.status();result.prepareRequest=prepareHTTP.request().postDataJSON();result.prepared=resultPlan;await persist();assert.equal(prepareHTTP.status(),200)
 assert.equal(result.prepareRequest.source_chat_id,chat.id);assert.equal(resultPlan.plan.source_chat_id,chat.id)
 const relief=resultPlan.plan.instructions.find(order=>order.kind==='relief');assert(relief,'Minister proposal must result in a relief instruction')
 assert(relief.budget>0&&relief.budget<=100000);assert(relief.grain>0)
 assert(mentionsNumber(reply,relief.budget),'Draft silver must be stated in the minister reply')
 assert(mentionsNumber(reply,relief.grain),'Draft grain must be stated in the minister reply')
 assert(!prompt.includes(String(relief.grain)),'The player did not supply an exact grain amount')
 assert(resultPlan.outcomes.some(item=>item.instruction_id===relief.id&&item.status==='ready'))
 const beforeIssue=await api(campaignPath)
 assert.equal(beforeIssue.projects.length,0);assert.equal(beforeIssue.treasury,initial.treasury);assert.equal(beforeIssue.grain,initial.grain);assert.equal(beforeIssue.revision,chatBody.state.revision)
 await page.getByRole('heading',{name:'待批方案',exact:true}).waitFor()
 const sourceSummary=page.locator('.history-manuscript summary').filter({hasText:'议事来源'})
 await sourceSummary.click()
 assert.equal(await page.locator('.history-manuscript').getByText(reply,{exact:true}).count(),1)
 assert.equal(await page.locator('.history-manuscript').getByText(prompt,{exact:true}).count(),1)
 await page.locator('.history-pending').waitFor({state:'hidden'})
 await sourceSummary.scrollIntoViewIfNeeded()
 await page.screenshot({path:path.join(output,'discussion-plan-source-1440.png'),fullPage:true})
 result.checks.push({name:'Selected canonical discussion supplies minister amounts, reviewed before any projects or spending',passed:true,budget:relief.budget,grain:relief.grain})
 const issueResponse=responseFor(initial.id+'/edict')
 await page.getByRole('button',{name:'批行此诏',exact:true}).click()
 const issueHTTP=await issueResponse,issued=await issueHTTP.json();result.issueStatus=issueHTTP.status();result.issueResponse=issued;await persist();assert.equal(issueHTTP.status(),200)
 assert.equal(issued.state.edicts.at(-1).plan.source_chat_id,chat.id)
 assert(issued.state.projects.some(project=>project.kind==='relief'&&project.budget===relief.budget&&project.grain===relief.grain))
 await page.locator('.history-pending').waitFor({state:'hidden'})
 assert.equal(await page.getByRole('textbox',{name:'诏令原文',exact:true}).inputValue(),manualDraft)
 await page.reload({waitUntil:'networkidle'})
 const cold=await api(campaignPath)
 assert.equal(cold.edicts.at(-1).plan.source_chat_id,chat.id)
 assert(cold.projects.some(project=>project.kind==='relief'&&project.budget===relief.budget&&project.grain===relief.grain))
 assert.equal(await page.getByRole('textbox',{name:'诏令原文',exact:true}).inputValue(),manualDraft)
 assert.equal(await page.getByText('尚未选择世界',{exact:true}).count(),0)
 await page.locator('.history-scroll').evaluate(el=>el.scrollTop=0)
 await page.screenshot({path:path.join(output,'discussion-issued-cold-1440.png'),fullPage:true})
 result.checks.push({name:'Explicit issue persists source provenance and project, cold reload keeps the unrelated manual draft',passed:true})
 await page.getByRole('navigation',{name:'游戏玩法',exact:true}).getByRole('button',{name:'冒险',exact:true}).click()
 await page.getByRole('heading',{name:'从哪一段故事开始',exact:true}).waitFor()
 assert.equal(await page.getByRole('button',{name:'选择冒险世界',exact:true}).count(),1)
 assert.equal(await page.getByText('尚未选择世界',{exact:true}).count(),0)
 const workspaceAfter=await api('/api/workspace/current');assert.equal(workspaceAfter.project_id,workspaceBefore.project_id);assert.equal(workspaceAfter.workspace,workspaceBefore.workspace)
 result.checks.push({name:'Historical and neutral game pages hide adventure context, adventure entry remains available',passed:true})
 assert.equal(result.errors.length,0)
 result.passed=true;result.finished=new Date().toISOString();console.log(JSON.stringify({passed:true,campaign:initial.id,source:chat.id,reply,budget:relief.budget,grain:relief.grain,checks:result.checks.length}))
}catch(error){result.passed=false;result.failure=String(error.stack||error);await page.screenshot({path:path.join(output,'discussion-failure.png'),fullPage:true}).catch(()=>{});throw error}
finally{await persist();await api('/api/settings',{layer:'user',changes:appearance},'PATCH');await browser.close()}
