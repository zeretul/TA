// Isolated component-browser check. API responses are explicit synthetic data;
// combat settlement and privacy are tested separately against the real Go store.
import { chromium } from '@playwright/test'
import { spawn } from 'node:child_process'
import { mkdir, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const root = fileURLToPath(new URL('../../', import.meta.url))
const output = path.resolve(root, process.env.TA_UI_EVIDENCE_DIR || '../docs/verification-0617-combat')
const port = process.env.TA_COMBAT_UI_PORT || '15183'
const base = `http://127.0.0.1:${port}`
await mkdir(output, { recursive: true })
const server = spawn(process.execPath, [path.join(root, 'node_modules/vite/bin/vite.js'), '--host', '127.0.0.1', '--port', port, '--strictPort'], { cwd: root, env: { ...process.env, TA_TEST_VITE_CACHE_DIR: `node_modules/.vite-combat-${port}` }, stdio: ['ignore', 'pipe', 'pipe'] })
let serverLog = ''
server.stdout.on('data', data => { serverLog += data })
server.stderr.on('data', data => { serverLog += data })
let browser
try {
  const deadline = Date.now() + 30000
  while (true) {
    if (server.exitCode !== null) throw new Error(serverLog)
    try { if ((await fetch(base)).ok) break } catch {}
    if (Date.now() > deadline) throw new Error(`Vite did not start: ${serverLog}`)
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  browser = await chromium.launch({ executablePath: process.env.TA_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', headless: true })
  const results = []
  for (const width of [375, 1280]) for (const theme of ['dark', 'light']) for (const locale of ['zh-CN', 'en-US']) {
    const page = await browser.newPage({ viewport: { width, height: 850 }, reducedMotion: 'reduce' })
    const errors = []
    page.on('pageerror', error => errors.push(String(error)))
    const english = locale === 'en-US'
    const name = english ? 'Measured ward of the north gate, as actually seen in this exchange' : '北门回风护体诀·这一回合实际观察到的招式名称'
    const state = { head: 'head-1', family: width === 375 ? 'wuxia' : 'space', enemy_condition: 'wounded', actor: { health: 70, max_health: 100, energy: 80, max_energy: 100, injury: 0, skills: {}, inventory: {}, assets: {}, battle: { id: 'battle', scale: 'person', opponent_name: english ? 'North gatekeeper' : '北门守卫', round: 0, status: 'active' } }, skills: [], items: [] }
    let submissions = 0
    await page.route('**/__combat_ui*', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><html><body style="margin:0"><div id="root"></div><script type="module">import RefreshRuntime from "/@react-refresh";RefreshRuntime.injectIntoGlobalHook(window);window.$RefreshReg$=()=>{};window.$RefreshSig$=()=>type=>type;window.__vite_plugin_react_preamble_installed__=true;</script><script type="module" src="/tests/support/adventure-ui-harness.tsx"></script></body></html>' }))
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url())
      let json = {}
      if (url.pathname.endsWith('/adventure-window')) json = state
      if (url.pathname.endsWith('/adventure-action')) {
        const request = route.request().postDataJSON()
        if (request.expected_head !== state.head) throw new Error('Stale command head')
        submissions++
        state.head = `head-${submissions + 1}`
        state.actor.battle.round = submissions
        state.actor.battle.last_enemy_skill_name = submissions === 1 ? name : undefined
        json = { event_id: `round-${submissions}` }
      }
      await route.fulfill({ json })
    })
    await page.goto(`${base}/__combat_ui?theme=${theme}&locale=${locale}`)
    await page.getByTestId('adventure-combat').waitFor()
    const button = page.getByRole('button', { name: english ? 'Play this round' : '执行这一回合' })
    await button.waitFor()
    if (await page.getByTestId('enemy-last-skill').count()) throw new Error('Future move exposed')
    await button.click()
    const summary = page.getByTestId('enemy-last-skill')
    await summary.waitFor()
    const text = await summary.innerText()
    if (text !== (english ? 'Opponent used: ' : '对手刚刚施展：') + name) throw new Error(`Wrong locale or observed name: ${text}`)
    if (await page.getByRole('option', { name: new RegExp(name) }).count()) throw new Error('Observed skill granted to player')
    if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)) throw new Error('Horizontal overflow')
    await page.screenshot({ path: path.join(output, `enemy-tactic-${width}-${theme}-${locale}.png`) })
    await page.reload()
    await page.getByTestId('enemy-last-skill').waitFor()
    await page.getByRole('button', { name: english ? 'Play this round' : '执行这一回合' }).click()
    await page.getByTestId('enemy-last-skill').waitFor({ state: 'detached' })
    state.actor.battle = undefined
    await page.reload()
    await page.getByText(english ? /No encounter is underway/ : /这里还没有交战/).waitFor()
    if (errors.length) throw new Error(errors.join('\n'))
    results.push({ width, theme, locale, checks: ['hidden-before-action', 'observed-skill-only', 'translated-label', 'long-name-wrap', 'reopen', 'clear-next-round', 'empty-state', 'no-page-errors'] })
    await page.close()
  }
  await writeFile(path.join(output, 'results.json'), JSON.stringify({ evidence: 'real component in Chromium; synthetic API, no model calls or real player data', results }, null, 2))
  console.log(JSON.stringify({ passed: results.length, output }))
} finally {
  await browser?.close()
  server.kill('SIGTERM')
}
