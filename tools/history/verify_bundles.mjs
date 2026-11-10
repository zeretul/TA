// Native browser coverage for complete scenario bundles and large campaign lists.
import fs from 'node:fs/promises'
import path from 'node:path'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
const require=createRequire(path.resolve('web/package.json'))
const {chromium}=require('@playwright/test')
const root=path.resolve('.ta-dev/history-limits'),out=path.join(root,'evidence/bundles'),base='http://127.0.0.1:18071'
async function api(p,body){const r=await fetch(base+p,{method:body===undefined?'GET':'POST',headers:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});assert(r.ok,`${p}: ${r.status} ${await (!r.ok?r.text():Promise.resolve(''))}`);return r.json()}
const settings=await api('/api/settings');assert.equal(path.resolve(settings.paths.denova_dir),path.join(root,'data'))
const b=await api('/api/history/bootstrap',{}),prefix=`/api/projects/${b.project_id}/history/scenarios`
const browser=await chromium.launch({headless:true,executablePath:'E:\\游戏\\游戏开发\\TA-开发版\\.ta-dev\\playwright\\chromium-1234\\chrome-win64\\chrome.exe'})
const errors=[]
try{
 const page=await browser.newPage({viewport:{width:1440,height:1000}});page.on('pageerror',e=>errors.push(e.message))
 await page.goto(base+'/?game=history');await page.getByRole('button',{name:'新开一局',exact:true}).click()
 await page.locator('#history-new-scenario').selectOption('ming-1627')
 const downloadEvent=page.waitForEvent('download');await page.getByRole('button',{name:'导出剧本',exact:true}).click();const download=await downloadEvent;assert.equal(download.suggestedFilename(),'ming-1627.zip');await download.saveAs(path.join(out,'ui-export.zip'))
 for(const id of ['a','b']){
  await page.getByLabel('导入剧本',{exact:true}).setInputFiles(path.join(out,`portable-${id}.zip`))
  await page.waitForFunction(id=>document.querySelector('#history-new-scenario')?.value===`portable-${id}`,id)
  assert(await page.getByText('剧本已导入，可用于新局。',{exact:true}).isVisible())
 }
 const first=await api(prefix+'/portable-a'),second=await api(prefix+'/portable-b');assert.notEqual(first.presentation.backdrop,second.presentation.backdrop)
 for(const p of [first,second]){const r=await fetch(base+p.presentation.backdrop);assert(r.ok);assert.equal(r.headers.get('content-type'),'image/png')}
 await page.locator('#history-new-name').fill('图文迁移原生验收');await page.getByRole('button',{name:'建立新局',exact:true}).click();await page.getByRole('heading',{name:'图文迁移原生验收',exact:true}).waitFor()
 await page.evaluate(src=>new Promise((resolve,reject)=>{const image=new Image();image.onload=()=>resolve(true);image.onerror=reject;image.src=src}),second.presentation.backdrop)
 await page.getByRole('tab',{name:'召见与廷议',exact:true}).click();await page.waitForFunction(()=>[...document.querySelectorAll('.history-ministers img')].length>=100&&[...document.querySelectorAll('.history-ministers img')].every(i=>i.complete&&i.naturalWidth>0))
 await page.screenshot({path:path.join(out,'imported-court.png')})
 const before=await api('/api/history/bootstrap',{})
 for(const name of ['missing','traversal']){const r=await fetch(base+prefix,{method:'POST',headers:{'Content-Type':'application/zip'},body:await fs.readFile(path.join(out,name+'.zip'))});assert.equal(r.status,400)}
 assert.deepEqual((await api('/api/history/bootstrap',{})).scenarios,before.scenarios)
 const legacy=await api(prefix+'/ming-1627');legacy.id='legacy-json-test';const imported=await api(prefix,legacy);assert.equal(imported.id,legacy.id)
 const repeat=await fetch(base+prefix,{method:'POST',headers:{'Content-Type':'application/zip'},body:await fs.readFile(path.join(out,'ui-export.zip'))});assert(repeat.ok,'Builtin export/reimport must be idempotent')
 const fixture=JSON.parse(await fs.readFile(path.join(root,'evidence/long-fixture.json'),'utf8'))
 await page.evaluate(({key,id})=>localStorage.setItem(key,id),{key:`ta:history:${b.project_id}:campaign`,id:fixture.id});await page.reload();await page.getByRole('heading',{name:'长局容量隔离验收',exact:true}).waitFor()
 await page.locator('#history-entity-kind').selectOption('person');assert.equal(await page.locator('.history-atlas-grid details').count(),60)
 const firstName=await page.locator('.history-atlas-grid strong').first().textContent();await page.getByRole('button',{name:'下一页',exact:true}).click();assert.notEqual(await page.locator('.history-atlas-grid strong').first().textContent(),firstName)
 await page.locator('.history-atlas-controls input').fill('旧任文书699');assert.equal(await page.locator('.history-atlas-grid details').count(),1);await page.locator('.history-atlas-grid summary').click();assert(await page.getByText(/古井晴川/).count())
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:path.join(out,'long-world-search-mobile.png')});assert((await page.evaluate(()=>document.documentElement.scrollWidth))<=391)
 assert.equal(errors.length,0);await fs.writeFile(path.join(out,'browser-result.json'),JSON.stringify({zipDownload:true,zipImport:true,legacyJSON:true,idempotent:true,imagesIntact:true,filenameCollisionIsolated:true,rejectedInvalidPackages:true,longWorldSearchAndPages:true,errors},null,2));console.log('PASS scenario bundle browser checks')
}finally{await browser.close()}
