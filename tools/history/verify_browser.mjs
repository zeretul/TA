// System-level browser verification for the isolated candidate only.
import fs from 'node:fs/promises'
import path from 'node:path'
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'
const require=createRequire(path.resolve('web/package.json'))
const {chromium}=require('@playwright/test')
const batch=process.env.TA_TEST_BATCH || 'history-complete'
assert(/^history-[a-z0-9-]+$/.test(batch))
const base=`http://127.0.0.1:${Number(process.env.TA_TEST_PORT || 18071)}`
const output=path.resolve('.ta-dev',batch,'evidence/browser')
await fs.mkdir(output,{recursive:true})
async function api(resource,method='GET',body,locale='zh-CN'){
  const response=await fetch(base+resource,{method,headers:{'Content-Type':'application/json','X-TA-Locale':locale},body:body===undefined?undefined:JSON.stringify(body)})
  if(!response.ok)throw new Error(`${resource}: HTTP ${response.status}`)
  return response.json()
}
const original=await api('/api/settings')
assert.equal(path.resolve(original.paths.denova_dir).toLowerCase(),path.resolve('.ta-dev',batch,'data').toLowerCase(),'Refusing to modify a non-isolated application')
const browser=await chromium.launch({headless:true,executablePath:process.env.TA_TEST_CHROMIUM || 'E:\\游戏\\游戏开发\\TA-开发版\\.ta-dev\\playwright\\chromium-1234\\chrome-win64\\chrome.exe'})
const checks=[]
const errors=[]
try{
  for(const locale of ['zh-CN','en-US']){
    const settings=await api('/api/settings')
    await api('/api/settings','PATCH',{layer:'user',changes:{language:locale},base_revision:settings.revisions?.user})
    const bootstrap=await api('/api/history/bootstrap','POST',{},locale)
    const state=await api(`/api/projects/${bootstrap.project_id}/history/campaigns`,'POST',{name:locale==='zh-CN'?'山河新政 · 界面验收':'A Realm Rebuilt',scenario_id:locale==='zh-CN'?'ming-1627':'song-1127',locale,command_id:`browser-${crypto.randomUUID()}`},locale)
    for(const theme of ['light','dark'])for(const width of [1440,390]){
      console.log(`Checking ${locale}/${theme}/${width}`)
      const themeSettings=await api('/api/settings')
      await api('/api/settings','PATCH',{layer:'user',changes:{theme},base_revision:themeSettings.revisions?.user})
      const context=await browser.newContext({viewport:{width,height:width===390?844:1000},locale,colorScheme:theme})
      await context.addInitScript(({theme,key,id})=>{localStorage.setItem('theme',theme);localStorage.setItem(key,id)}, {theme,key:`ta:history:${bootstrap.project_id}:campaign`,id:state.id})
      const page=await context.newPage()
      page.on('pageerror',error=>errors.push(error.message))
      await page.goto(base+'/?game=history')
      await page.getByRole('heading',{name:state.name,exact:true}).waitFor()
      await page.waitForFunction(({locale,theme})=>document.documentElement.lang===locale&&document.documentElement.dataset.theme===theme,{locale,theme})
      await page.evaluate(src=>new Promise((resolve,reject)=>{const image=new Image();image.onload=()=>resolve(true);image.onerror=reject;image.src=src}),state.scenario.presentation.backdrop)
      await page.screenshot({path:path.join(output,`${locale}-${theme}-${width}-opening.png`)})
      const dimensions=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth}))
      assert(dimensions.scroll<=dimensions.width+1,`Horizontal overflow: ${locale}/${theme}/${width}`)
      if(locale==='en-US'){
        assert(await page.getByText('Court treasury',{exact:true}).isVisible())
        assert(await page.getByText('A court seeking stability',{exact:true}).isVisible())
        assert(!(await page.getByText('内帑',{exact:true}).count()))
      }
      await page.getByRole('group',{name:locale==='zh-CN'?'山河舆图':'Strategic map',exact:true}).waitFor()
      const mapButtons=page.locator('.history-map-region')
      assert((await mapButtons.count())>=13,'Scenario map missing regions')
      await mapButtons.nth(1).focus();await page.keyboard.press('Enter')
      assert.equal(await mapButtons.nth(1).getAttribute('aria-pressed'),'true')
      await page.screenshot({path:path.join(output,`${locale}-${theme}-${width}.png`)})
      await page.getByRole('tab',{name:locale==='zh-CN'?'召见与廷议':'Court',exact:true}).click()
      await page.waitForFunction(()=>[...document.querySelectorAll('.history-ministers img')].length>0&&[...document.querySelectorAll('.history-ministers img')].every(image=>image.complete&&image.naturalWidth>0))
      assert((await page.locator('.history-ministers img').count())>=(locale==='zh-CN'?100:12),'Missing minister portraits')
      await page.screenshot({path:path.join(output,`${locale}-${theme}-${width}-court.png`)})

      await page.getByRole('tab',{name:locale==='zh-CN'?'政令簿':'Standing orders',exact:true}).click()
      assert(await page.locator('#history-world-orders').getByText(locale==='zh-CN'?'尚未颁诏。可以先召见大臣，也可以直接写下决定。':'No decrees yet. Consult a minister or write your decision directly.',{exact:true}).isVisible())
      await page.getByRole('tab',{name:locale==='zh-CN'?'颁诏':'Decrees',exact:true}).click()
      await page.locator('#history-edict-text').fill((locale==='zh-CN'?'先清查账目，赈济灾民，保留已有授权，不动内帑。':'Audit accounts, relieve hardship and respect existing authorizations. ').repeat(30))
      assert(await page.getByRole('button',{name:locale==='zh-CN'?'直接颁诏':'Issue directly',exact:true}).isVisible())
      const widthAfter=await page.evaluate(()=>document.documentElement.scrollWidth)
      assert(widthAfter<=width+1,'Long decree overflow')
      await page.screenshot({path:path.join(output,`${locale}-${theme}-${width}-decree.png`)})
      checks.push({locale,theme,width,scenario:state.scenario_id,overflow:false,emptyOrders:true,longDraft:true})
      await context.close()
    }
  }
  assert.equal(errors.length,0,errors.join('\n'))
  await fs.writeFile(path.join(output,'results.json'),JSON.stringify({checks,errors},null,2))
  console.log(JSON.stringify({passed:checks.length,errors},null,2))
}finally{
  await browser.close()
  const settings=await api('/api/settings')
  await api('/api/settings','PATCH',{layer:'user',changes:{language:original.user.language??null,theme:original.user.theme??null},base_revision:settings.revisions?.user})
}
