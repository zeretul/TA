import {chromium,expect} from '@playwright/test'
import {readFile,writeFile} from 'node:fs/promises'
import path from 'node:path'
const base='http://127.0.0.1:19927', out=path.resolve('../.ta-dev/070-repair')
const project=JSON.parse(await readFile(path.join(out,'qa-project.json'),'utf8'))
const fileURL=base+`/api/projects/${project.project_id}/files/file?path=CREATOR.md`
const browser=await chromium.launch({executablePath:path.resolve('../.ta-dev/playwright/chromium-1234/chrome-win64/chrome.exe'),headless:true})
try{
 const context=await browser.newContext({viewport:{width:1440,height:960},serviceWorkers:'block'})
 const original=await(await context.request.get(fileURL)).json()
 const page=await context.newPage()
 await page.addInitScript(()=>localStorage.setItem('nova:mode','game-settings'))
 await page.goto(base)
 await page.getByRole('button',{name:'创作偏好与红线',exact:true}).click()
 const field=page.locator('#game-settings-creator textarea')
 await expect(field).toHaveValue(original.content)
 const changed=original.content+'\n\n隔离验收：保留既有要求，人物对白自然回应。'
 await field.fill(changed)
 await expect.poll(async()=>(await(await context.request.get(fileURL)).json()).content).toBe(changed)
 await page.reload()
 await page.getByRole('button',{name:'创作偏好与红线',exact:true}).click()
 await expect(field).toHaveValue(changed)
 await page.screenshot({path:path.join(out,'browser/creator-preferences-readback.png')})
 await field.fill(original.content)
 await expect.poll(async()=>(await(await context.request.get(fileURL)).json()).content).toBe(original.content)
 await writeFile(path.join(out,'creator-readback.json'),JSON.stringify({saved:true,reloaded:true,restored:true,project_id:project.project_id}))
 console.log('Writing preferences saved, reloaded and restored in the isolated project.')
}finally{await browser.close()}
