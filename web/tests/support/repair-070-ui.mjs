import { chromium, expect } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
const base = process.env.TA_UI_BASE_URL || 'http://127.0.0.1:19927'
const output = path.resolve('../.ta-dev/070-repair/browser')
await mkdir(output, { recursive: true })
const browser = await chromium.launch({ executablePath: path.resolve('../.ta-dev/playwright/chromium-1234/chrome-win64/chrome.exe'), headless: true })
const results = []
try {
 for (const locale of ['zh-CN','en-US']) for (const theme of ['dark','light']) for (const width of [375,1440]) {
  const context = await browser.newContext({ viewport: {width,height:960}, locale, reducedMotion:'reduce', serviceWorkers:'block' })
  const page = await context.newPage()
  const errors=[]; page.on('pageerror',e=>errors.push(e.message))
  const settings = await (await context.request.get(base+'/api/settings')).json()
  await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({ json:{...settings,effective:{...settings.effective,theme,language:locale}} }))
  await page.addInitScript(() => localStorage.setItem('nova:mode','game-settings'))
  await page.goto(base)
  const mainLabel=locale==='zh-CN'?'主叙事':'Main narrative'
  const main=page.getByRole('combobox',{name:mainLabel,exact:true}).first()
  await expect(main).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('data-theme',theme)
  await main.click()
  await expect(page.getByRole('option')).toHaveCount(6)
  await page.screenshot({path:path.join(output,`${locale}-${theme}-${width}-styles.png`)})
  await page.keyboard.press('Escape')
  const planning=locale==='zh-CN'?'故事规划':'Story planning'
  await page.getByRole('button',{name:new RegExp('^'+planning)}).first().click()
  const trigger=page.getByRole('combobox',{name:locale==='zh-CN'?'规划模板':'Planning template',exact:true}).first()
  await expect(trigger).toBeVisible()
  const labelText=(await trigger.innerText()).trim()
  if(labelText.length>65)throw new Error('Planning description leaked into the trigger: '+labelText)
  const size=await trigger.boundingBox(); if(size.height>45)throw new Error('Planning trigger overflow')
  await page.screenshot({path:path.join(output,`${locale}-${theme}-${width}-settings.png`)})
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>window.innerWidth+1)
  expect(overflow).toBe(false)
  if(width===1440){
   await page.getByRole('button',{name:locale==='zh-CN'?'AI 助手':'AI assistant',exact:true}).click()
   await expect(page.getByRole('heading',{name:locale==='zh-CN'?'AI 助手':'AI assistant',exact:true})).toBeVisible()
   await expect(page.getByText(locale==='zh-CN'?'快捷配置':'Quick Configuration',{exact:true})).toHaveCount(0)
   await expect(page.locator('[data-slot="sidebar-container"] [aria-current="page"]')).toHaveCount(1)
   await page.screenshot({path:path.join(output,`${locale}-${theme}-assistant.png`)})
   await page.getByRole('button',{name:locale==='zh-CN'?'资料库':'Lore',exact:true}).click()
   await expect(page.getByRole('button',{name:locale==='zh-CN'?'创作偏好与红线':'Writing preferences and boundaries',exact:true})).toHaveCount(0)
  }
  expect(errors).toEqual([])
  results.push({locale,theme,width,styles:6,planningLabel:labelText,overflow,errors})
  await context.close()
 }
 await writeFile(path.join(output,'results.json'),JSON.stringify(results,null,2))
 console.log(JSON.stringify({passed:results.length,output}))
} finally {await browser.close()}
