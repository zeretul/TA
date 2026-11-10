import { chromium, expect } from '@playwright/test'
import path from 'node:path'
import { mkdir,writeFile } from 'node:fs/promises'
const base='http://127.0.0.1:19927'
const output=path.resolve('../.ta-dev/070-repair/browser')
await mkdir(output,{recursive:true})
const browser=await chromium.launch({executablePath:path.resolve('../.ta-dev/playwright/chromium-1234/chrome-win64/chrome.exe'),headless:true})
const results=[]
try {
 for(const locale of ['zh-CN','en-US']) for(const width of [375,1440]){
  const context=await browser.newContext({viewport:{width,height:960},locale,serviceWorkers:'block'})
  const page=await context.newPage()
  const settings=await(await context.request.get(base+'/api/settings')).json()
  await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/,r=>r.fulfill({json:{...settings,effective:{...settings.effective,language:locale,theme:width===375?'light':'dark'}}}))
  await page.addInitScript(()=>localStorage.setItem('nova:mode','books'))
  await page.goto(base)
  const packs=(await(await context.request.get(base+'/api/story-packs',{headers:{'X-TA-Locale':locale}})).json()).packs
  for(const pack of packs.filter(p=>['fanren-jie','xinghai-689','lanchuan-qunxia','sui-guan-realms'].includes(p.id))){
   await page.getByRole('button',{name:(locale==='zh-CN'?'选择':'Select ')+pack.name,exact:true}).click()
   const load=page.getByRole('button',{name:locale==='zh-CN'?'加载世界包':'Load world pack',exact:true})
   if(await load.count())await load.click()
   const create=page.getByRole('button',{name:locale==='zh-CN'?'创建角色':'Create a character',exact:true})
   await expect(create).toBeVisible()
   const toast=page.getByRole('button',{name:'Close toast',exact:true});if(await toast.count())await toast.first().click()
   await create.click()
   const dialog=page.getByRole('dialog')
   await expect(dialog).toBeVisible()
   await expect(dialog.getByLabel(locale==='zh-CN'?'落脚地点':'Starting place',{exact:true})).toHaveCount(0)
   const origin=pack.creation.origins.find(o=>/heir|young-master/.test(o.id))
   if(!origin)throw new Error('Missing strong origin '+pack.id)
   await dialog.getByRole('textbox',{name:locale==='zh-CN'?'姓名':'Name',exact:true}).fill('叶行舟')
   await dialog.getByRole('radio',{name:new RegExp('^'+origin.name)}).click()
   if(pack.id==='fanren-jie'){
    await expect(dialog.getByText('叶氏',{exact:true})).toBeVisible()
    await expect(dialog.getByRole('combobox',{name:locale==='zh-CN'?'所属':'Affiliation',exact:true})).toHaveCount(0)
    await dialog.getByRole('radio',{name:locale==='zh-CN'?'金':'Metal',exact:true}).click()
   }
   const generate=dialog.getByRole('button',{name:locale==='zh-CN'?'生成身份':'Generate identity',exact:true})
   await generate.click()
   await expect(dialog.getByText(locale==='zh-CN'?/无法连接当前游戏模型服务/:/Cannot connect to the current game model service/)).toBeVisible({timeout:15000})
   await expect(dialog.getByRole('textbox',{name:locale==='zh-CN'?'姓名':'Name',exact:true})).toHaveValue('叶行舟')
   await dialog.getByRole('textbox',{name:locale==='zh-CN'?'姓名':'Name',exact:true}).scrollIntoViewIfNeeded()
   await page.screenshot({path:path.join(output,`${locale}-${width}-${pack.id}-creation.png`)})
   expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)).toBe(false)
   results.push({locale,width,pack:pack.id,strongOrigin:origin.id,originCount:pack.creation.origins.length,errorLocalized:true,formPreserved:true})
   await dialog.getByRole('button',{name:locale==='zh-CN'?'取消':'Cancel',exact:true}).click()
  }
  await context.close()
 }
 await writeFile(path.join(output,'creation-results.json'),JSON.stringify(results,null,2))
 console.log(JSON.stringify({passed:results.length}))
}finally{await browser.close()}

