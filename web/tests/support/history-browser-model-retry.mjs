// Retest the original real-model UI failure after the native JSON-mode fix.
import {chromium} from '@playwright/test'
import assert from 'node:assert/strict'
import {readFile,writeFile} from 'node:fs/promises'
import path from 'node:path'
const base=process.env.TA_HISTORY_TEST_URL,output=process.env.TA_HISTORY_BROWSER_OUTPUT
if(!base||!output||process.env.TA_HISTORY_ALLOW_MODEL_TESTS!=='1'||!/^http:\/\/(127\.0\.0\.1|localhost):\d+$/.test(base))throw new Error('Explicit loopback model-test authorization required')
async function api(route,body,method='POST'){const r=await fetch(base+route,body===undefined?undefined:{method,headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});assert.equal(r.status,200);return r.json()}
const first=JSON.parse(await readFile(path.join(output,'first-model-browser-result.json'),'utf8'))
const boot=await api('/api/history/bootstrap',{}),campaign=first.campaign
const campaignPath=`/api/projects/${boot.project_id}/history/campaigns/${campaign}`
const before=await api(campaignPath),settings=await api('/api/settings'),appearance={language:settings.effective.language,theme:settings.effective.theme}
await api('/api/settings',{layer:'user',changes:{language:'zh-CN',theme:'light'}},'PATCH')
const result={campaign,started:new Date().toISOString(),checks:[],errors:[]}
const browser=await chromium.launch({headless:true,executablePath:process.env.TA_HISTORY_CHROMIUM})
const context=await browser.newContext({viewport:{width:1440,height:1000}})
await context.addInitScript(({projectId,campaign})=>localStorage.setItem(`ta:history:${projectId}:campaign`,campaign),{projectId:boot.project_id,campaign})
const page=await context.newPage();page.setDefaultTimeout(15000)
page.on('pageerror',error=>result.errors.push(error.message));page.on('console',message=>{if(message.type()==='error')result.errors.push(message.text())})
try{
 await page.goto(base+'/?game=history',{waitUntil:'networkidle'})
 await page.getByRole('tab',{name:'召见与廷议',exact:true}).click()
 const prompt='毕卿，请简短报出当前国库可用银与官仓存粮，并说明这次问话不会动用钱粮。只按当前账目回答。'
 await page.getByRole('textbox',{name:'对大臣说',exact:true}).fill(prompt)
 const responsePromise=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith(campaign+'/chat'),{timeout:300000})
 await page.getByRole('button',{name:'传谕',exact:true}).click()
 const response=await responsePromise,resultBody=await response.json()
 result.status=response.status();result.response=resultBody
 assert.equal(response.status(),200,'Original query should now return a valid JSON-backed receipt')
 const reply=resultBody.state.chats.at(-1).replies[0].text
 assert(reply.trim());assert(/1,000,000|1000000|100万|一百万/.test(reply),'Current treasury should be reported')
 assert(/630,000|630000|63万|六十三万/.test(reply),'Current grain should be reported')
 assert.equal(resultBody.state.treasury,before.treasury);assert.equal(resultBody.state.grain,before.grain);assert.equal(resultBody.state.privy_purse,before.privy_purse)
 await page.locator('.history-pending').waitFor({state:'hidden'})
 await page.locator('.history-minister-message').last().scrollIntoViewIfNeeded()
 await page.screenshot({path:path.join(output,'model-retry-success-1440.png'),fullPage:true})
 result.checks.push({name:'Original prompt returns truthful current resources and causes no spending',passed:true,reply})
 await page.reload({waitUntil:'networkidle'})
 await page.getByRole('tab',{name:'召见与廷议',exact:true}).click()
 assert.equal(await page.locator('.history-minister-message').last().locator('p').innerText(),reply)
 assert.equal(await page.getByRole('textbox',{name:'对大臣说',exact:true}).inputValue(),'')
 await page.setViewportSize({width:390,height:844})
 await page.locator('.history-minister-message').last().scrollIntoViewIfNeeded()
 await page.screenshot({path:path.join(output,'model-retry-success-390.png'),fullPage:true})
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1),false)
 assert.equal(result.errors.length,0)
 result.checks.push({name:'Actual reply survives page reload and remains readable on 390px screen',passed:true})
 result.passed=true;console.log(JSON.stringify({passed:true,campaign,reply,checks:result.checks.length}))
}catch(error){result.passed=false;result.failure=String(error.stack||error);await page.screenshot({path:path.join(output,'model-retry-failure.png'),fullPage:true}).catch(()=>{});throw error}
finally{await writeFile(path.join(output,'browser-model-retry.json'),JSON.stringify(result,null,2));await api('/api/settings',{layer:'user',changes:appearance},'PATCH');await browser.close()}
