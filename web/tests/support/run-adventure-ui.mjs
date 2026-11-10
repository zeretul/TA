import { chromium } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
const output = path.resolve(process.env.TA_UI_EVIDENCE_DIR || '../work/cloud-completion-20261102/ui-evidence')
await mkdir(output, { recursive: true })
const browser = await chromium.launch({executablePath:process.env.TA_CHROMIUM_EXECUTABLE_PATH, headless: true, args: ['--disable-features=LocalNetworkAccessChecks,LocalNetworkAccessRestrictions'] })
const results = []
const story = { id: 'ui-story', title: '江湖旧事 · 一个用于核验长标题换行与保存对象的故事', protagonist: { mode: 'default' }, story_teller_id: 'rhythm', planning_mode: 'disabled', planning_template_id: 'adventure', module_refs: { actor_state_id: 'default', rule_system_id: 'd20' }, reply_target_chars: 2000, choice_count: 5, opening: { mode: 'ai' }, branches: 1, events: 0, turn_count: 0, pack_binding: { pack_id: 'world-a', scope: 'pack:a' } }
try {
for (const width of [375, 1280]) for (const theme of ['dark', 'light']) for (const locale of ['zh-CN', 'en-US']) {
 const page = await browser.newPage({ viewport: { width, height: 850 }, reducedMotion: 'reduce' })
 const errors = [];page.on('pageerror',error=>{if(errors.length<20)errors.push(String(error))})
 await page.route('**/@vite/client', route => route.fulfill({contentType:'application/javascript',body:'export function createHotContext(){return {accept(){},acceptExports(){},dispose(){},prune(){},invalidate(){},on(){},off(){},send(){},data:{}}};export function updateStyle(id,css){let el=document.getElementById(id);if(!el){el=document.createElement("style");el.id=id;document.head.appendChild(el)}el.textContent=css};export function removeStyle(id){document.getElementById(id)?.remove()};export function injectQuery(url){return url}'}))
 // The assistant is outside the settings-target check and normally owns a
 // live session. Keep this fixture from bootstrapping an unrelated runtime.
 await page.route('**/src/components/Chat/ConfigManagerChat.tsx*',route=>route.fulfill({contentType:'application/javascript',body:'export function ConfigManagerChat(){return null}'}))
 await page.route('**/src/components/Chat/ModelProfileSwitcher.tsx*',route=>route.fulfill({contentType:'application/javascript',body:'export function ModelProfileSwitcher(){return null}'}))
 await page.route('**/src/features/interactive/components/director-console/StoryTuningView.tsx*',route=>route.fulfill({contentType:'application/javascript',body:'export function StoryTuningView(){return null}'}))
 const english=locale==='en-US'
 const state = { head: 'head-1', family: width===375?'wuxia':'space', enemy_condition: 'wounded', actor: { health: 70, max_health: 100, energy: 80, max_energy: 100, skills: { skill1: 1 }, inventory: {}, assets: {}, battle: { id:'battle', scale: 'person', opponent_name: english?'The gatekeeper with an exceptionally long known title':'青石渡口守卫 · 一个用于检查窄屏长文本换行的对手称号', round: 1, status:'active' } }, skills: [{ id:'skill1',name:english?'Measured strike':'沉稳一击',kind:'attack',scale:'person',cost:10 }],items:[] }
 let submitted
 await page.route('**/__ui_checks*',r=>r.fulfill({contentType:'text/html',body:'<!doctype html><html><body style="margin:0"><div id="root"></div><script type="module">import RefreshRuntime from "/@react-refresh";RefreshRuntime.injectIntoGlobalHook(window);window.$RefreshReg$=()=>{};window.$RefreshSig$=()=>type=>type;window.__vite_plugin_react_preamble_installed__=true;</script><script type="module" src="/tests/support/adventure-ui-harness.tsx"></script></body></html>'}))
 await page.route('**/api/**', async route => {
   const url=new URL(route.request().url());let json={}
   if(url.pathname.endsWith('/adventure-window')) json=state
   if(url.pathname.endsWith('/adventure-action')) {submitted=route.request().postDataJSON();state.actor.battle.round=2;state.head='head-2';json={event_id:'round-2'}}
   if(url.pathname.endsWith('/stories')) json={stories:[story,{...story,id:'edit-story',title:english?'Other story':'另一条故事'}],current_story_id:'ui-story'}
   if(url.pathname.endsWith('/snapshot')) json={story_id:'edit-story',branch_id:'main',state:{},turns:[]}
   if(url.pathname.endsWith('/story-packs'))json={packs:[{id:'world-a',name:english?'Lan Chuan — long world name for scope validation':'澜川 · 用于检查编辑范围的长世界名称',settings_revision:'rev-a',settings:{},modules:[]},{id:'world-b',name:english?'Star Sea':'星海',settings_revision:'rev-b',settings:{},modules:[]}]}
   if(url.pathname.endsWith('/tellers'))json={tellers:[]}
   if(url.pathname.endsWith('/settings'))json={effective:{speech:{},agent_models:{},theme,locale},user:{},workspace:{},revisions:{}}
   if(url.pathname.includes('config'))json={capabilities:{},effective:{},revision:'1'}
   await route.fulfill({json})
 })
 const suffix=`theme=${theme}&locale=${locale}`
 await page.goto(`http://127.0.0.1:19924/__ui_checks?${suffix}`)
 await page.getByTestId('adventure-combat').waitFor()
 await page.getByRole('combobox').selectOption('skill:skill1')
 await page.getByRole('button',{name:english?'Play this round':'执行这一回合'}).click()
 await page.getByText(english?'Round 2':'第 2 回合',{exact:false}).waitFor()
 if(submitted.action.skill_id!=='skill1'||submitted.expected_head!=='head-1')throw new Error('Incorrect round request')
 if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw new Error('Horizontal overflow')
 await page.screenshot({path:path.join(output,`battle-${width}-${theme}-${locale}.png`)})
 state.actor.battle=undefined
 await page.reload();await page.getByText(english?/No encounter is underway/:/这里还没有交战/).waitFor()
 await page.screenshot({path:path.join(output,`empty-${width}-${theme}-${locale}.png`)})
 await page.goto(`http://127.0.0.1:19924/__ui_checks?mode=settings&${suffix}`)
 await page.getByText(english?/Editing: Lan Chuan/:/修改目标：澜川/).first().waitFor()
 await page.screenshot({path:path.join(output,`settings-world-${width}-${theme}-${locale}.png`)})
 const section=page.locator('#game-settings-narrative');await section.scrollIntoViewIfNeeded().catch(error=>{throw new Error(`${error}\nPage errors: ${errors.join('\n')}`)})
 await page.screenshot({path:path.join(output,`settings-story-${width}-${theme}-${locale}.png`)})
 if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw new Error('Settings horizontal overflow')
 if(errors.length)throw new Error(errors.join('\n'))
 results.push({width,theme,locale,roundRequest:submitted,checks:['battle','empty','settings-world','settings-story','no-overflow','no-page-errors']})
 await page.close()
}
await writeFile(path.join(output,'results.json'),JSON.stringify(results,null,2))
console.log(JSON.stringify({passed:results.length,output}))
} finally {await browser.close()}
