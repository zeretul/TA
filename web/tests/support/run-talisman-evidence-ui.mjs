import { chromium } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
const output=path.resolve(process.env.TA_UI_EVIDENCE_DIR || '../work/cloud-completion-20261102/ui-evidence')
await mkdir(output,{recursive:true})
const browser=await chromium.launch({executablePath:process.env.TA_CHROMIUM_EXECUTABLE_PATH,headless:true,args:['--disable-features=LocalNetworkAccessChecks,LocalNetworkAccessRestrictions']})
const results=[]
try {
for(const width of [375,1280])for(const locale of ['zh-CN','en-US']){
 const theme=width===375?'dark':'light', english=locale==='en-US'
 const page=await browser.newPage({viewport:{width,height:850},reducedMotion:'reduce'})
 const errors=[];page.on('pageerror',error=>errors.push(String(error)))
 await page.route('**/@vite/client',route=>route.fulfill({contentType:'application/javascript',body:'export function createHotContext(){return {accept(){},acceptExports(){},dispose(){},prune(){},invalidate(){},on(){},off(){},send(){},data:{}}};export function updateStyle(id,css){let el=document.getElementById(id);if(!el){el=document.createElement("style");el.id=id;document.head.appendChild(el)}el.textContent=css};export function removeStyle(id){document.getElementById(id)?.remove()};export function injectQuery(url){return url}'}))
 await page.route('**/__ui_checks*',route=>route.fulfill({contentType:'text/html',body:'<!doctype html><html><body style="margin:0"><div id="root"></div><script type="module">import RefreshRuntime from "/@react-refresh";RefreshRuntime.injectIntoGlobalHook(window);window.$RefreshReg$=()=>{};window.$RefreshSig$=()=>type=>type;window.__vite_plugin_react_preamble_installed__=true;</script><script type="module" src="/tests/support/adventure-ui-harness.tsx"></script></body></html>'}))
 const state={story_id:'ui-story',branch_id:'main',scene:{node_id:'n',title:english?'Mountain path':'山道',location:english?'Mountain path':'山道'},session_id:'fight',round:1,ended:false,has_more:false,sides:[{actor_id:'protagonist',name:english?'Traveler':'行旅',side:'player',exact:true,standing:'standing',health:40,health_max:60,mana:12,mana_max:30,has_implement:true,skills:[],items:[{instance_id:'ward-copy',def_id:'talisman.ward.lower',name_key:'item.talisman.ward_lower.name',quantity:1,purpose:'shield',shield_amount:12,duration_exchanges:3,usable:true}]},{actor_id:'wolf',name:english?'Wolf':'山狼',side:'enemy',exact:false,standing:'standing',observed:{condition:'combatWindow.condition.hurt'},has_implement:false}]}
 let command
 await page.route('**/api/**',async route=>{
  const url=new URL(route.request().url())
  if(url.pathname.endsWith('/combat/rounds')){command=route.request().postDataJSON();state.round=2;state.sides[0].shield=12;state.sides[0].items=[];state.last_round={event_id:'r2',outcome:{round:2,steps:[]},narration:english?'The ward catches the blow.':'护符挡住了袭来的劲力。'}}
  await route.fulfill({json:state})
 })
 const suffix=`theme=${theme}&locale=${locale}`
 await page.goto(`http://127.0.0.1:19924/__ui_checks?mode=talisman&${suffix}`)
 const ward=page.getByRole('button',{name:english?/Ward Talisman/:/护身符/}).first();await ward.waitFor()
 await page.getByText(english?/Shield 12.*3/:/护盾 12，持续 3 轮/).first().waitFor()
 await page.screenshot({path:path.join(output,`talisman-offer-${width}-${locale}.png`)})
 await ward.click();await page.getByRole('button',{name:english?'Resolve this round':'结算本轮',exact:true}).click()
 await page.waitForFunction(()=>document.body.textContent?.includes('12'))
 for(let i=0;i<50&&!command;i++)await page.waitForTimeout(50)
 if(command?.actions?.[0]?.kind!=='use_item'||command.actions[0].item_instance_id!=='ward-copy')throw new Error('Ward identity was not submitted')
 await page.screenshot({path:path.join(output,`talisman-used-${width}-${locale}.png`)})
 await page.goto(`http://127.0.0.1:19924/__ui_checks?mode=evidence&${suffix}`)
 await page.getByText(english?'View result details':'查看依据',{exact:true}).click()
 await page.getByText(english?'Saved roll 1d20: 14':'已保存掷骰 1d20：14').waitFor()
 if(await page.getByText(english?'Settled total:':'已结算总值：',{exact:false}).count())throw new Error('Hidden total must not be rendered')
 await page.screenshot({path:path.join(output,`result-evidence-${width}-${locale}.png`)})
 if(errors.length)throw new Error(errors.join('\n'))
 results.push({width,theme,locale,command});await page.close()
}
await writeFile(path.join(output,'talisman-evidence-results.json'),JSON.stringify(results,null,2));console.log(JSON.stringify({passed:results.length,output}))
}finally{await browser.close()}
