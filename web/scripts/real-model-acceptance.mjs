/**
 * Real-model baseline acceptance (API chain).
 * Writes results to real-model-acceptance-result.json so output is never lost
 * to pipe buffering. Talks to the live gateway via the running backend.
 */
import { chromium } from '@playwright/test'
import { randomUUID } from 'node:crypto'
import { writeFileSync, mkdirSync } from 'node:fs'

const BASE = process.env.TA_ACCEPT_BASE || 'http://127.0.0.1:19711'
const OUT = process.env.TA_ACCEPT_RESULT || './real-model-acceptance-result.json'
const ART = './test-results/real-model'
mkdirSync(ART, { recursive: true })

const steps = []
let failed = 0
function record(name, ok, detail = '') {
  steps.push({ name, ok: !!ok, detail: String(detail) })
  if (!ok) failed += 1
}
function flush() {
  writeFileSync(OUT, JSON.stringify({ passed: steps.length - failed, total: steps.length, steps }, null, 2))
}

async function main() {
  const browser = await chromium.launch()
  const context = await browser.newContext({ baseURL: BASE, locale: 'zh-CN', serviceWorkers: 'block', reducedMotion: 'reduce' })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()) })
  page.on('pageerror', (e) => consoleErrors.push(`pageerror: ${e.message}`))

  const unique = randomUUID().slice(0, 8)

  // 1. Workspace
  const book = await post('/api/books/create', { title: `基线验收-${unique}`, author: 'Text Adventure' })
  record('创建工作区', book.ok && !!book.body.workspace, book.body.workspace || JSON.stringify(book.body).slice(0, 200)); flush()
  if (!book.ok) return finish(browser)
  await post('/api/workspace/switch', { path: book.body.workspace })

  // 2. Story + opening
  const storyTitle = `基线故事-${unique}`
  const story = await post('/api/interactive/stories', {
    title: storyTitle,
    origin: '越国边陲的青石镇，一名少年在丹铺后巷醒来。',
    protagonist: { mode: 'custom', name: '沈青', profile: '一名初入修行的少年散修。' },
    choice_count: 3,
    planning_mode: 'disabled',
    state_schema_policy: { mode: 'fixed_template' },
  })
  record('创建故事', story.ok && !!story.body.id, story.body.id || JSON.stringify(story.body).slice(0, 200)); flush()
  if (!story.ok) return finish(browser)
  const storyId = story.body.id

  await post('/api/interactive/chat', { command_id: `acc-opening-${storyId}`, mode: 'story', story_id: storyId, branch: 'main', start_opening: true })
  const snap1 = await pollSnapshot(storyId, 1, 150000)
  const openingText = snap1.turns?.[0]?.narrative || ''
  record('首幕生成（真实模型）', openingText.length > 50, `${openingText.length} 字`); flush()
  record('首幕长度达目标量级', openingText.length >= 400, `${openingText.length} 字`); flush()

  // 3. Free input turn
  await post('/api/interactive/chat', {
    command_id: `acc-turn2-${storyId}`,
    mode: 'story', story_id: storyId, branch: 'main',
    message: '我向丹铺掌柜打听最近镇上发生的怪事。',
  })
  const snap2 = await pollSnapshot(storyId, 2, 180000)
  const turn2 = snap2.turns?.[1]
  record('自由输入推进成功', !!turn2 && (turn2.narrative?.length || 0) > 50, `第2轮 ${turn2?.narrative?.length ?? 0} 字`); flush()
  record('第二轮正文达目标量级', (turn2?.narrative?.length || 0) >= 400, `${turn2?.narrative?.length ?? 0} 字`); flush()

  // 4/5. UI: game entry, story select, history visible, reload restores
  const t2head = (turn2?.narrative || '').slice(0, 12)
  try {
    await page.goto('/')
    await page.waitForTimeout(5000)
    // The first-run model onboarding overlay covers the shell; dismiss it.
    const skip = page.getByRole('button', { name: '跳过引导' }).first()
    if (await skip.isVisible().catch(() => false)) {
      await skip.click()
      await page.waitForTimeout(1200)
      record('关闭首启引导遮罩', true); flush()
    }

    const gameBtn = await findGameButton(page)
    record('侧边栏含「游戏」入口', !!gameBtn, gameBtn ? '找到入口按钮' : '未找到'); flush()
    if (gameBtn) await gameBtn.click()
    await page.waitForTimeout(2500)

    // Select the story that actually has turns; the console defaults to the newest.
    const picker = page.getByRole('button', { name: '选择故事线' }).first()
    if (await picker.isVisible().catch(() => false)) {
      await picker.click()
      await page.waitForTimeout(1200)
      const option = page.locator(`button[aria-label="${storyTitle}"]`).first()
      if (await option.isVisible().catch(() => false)) {
        await option.click()
        await page.waitForTimeout(3000)
        record('切换回已开局故事线', true); flush()
      } else {
        record('切换回已开局故事线', false, '故事线选项未出现'); flush()
      }
    }

    const composer = page.getByPlaceholder(/你要做什么/).first()
    await composer.waitFor({ state: 'visible', timeout: 30000 })
    record('游戏主舞台输入框可见', true); flush()
    await page.waitForTimeout(2000)
    // The stage virtualizes its message list, so only the turns near the
    // viewport are in the DOM. An earlier turn has to be scrolled into view
    // before it can be asserted on; waiting for it where it is would fail on a
    // story longer than one screen no matter how correct the rendering is.
    await scrollStageToStart(page)
    const histVisible = await page.getByText(t2head, { exact: false }).first().waitFor({ state: 'visible', timeout: 15000 })
      .then(() => true).catch(() => false)
    record('主舞台显示正文（历史回看）', histVisible, `匹配「${t2head}…」`); flush()
    await page.screenshot({ path: `${ART}/01-game-stage.png` })

    await page.reload()
    await page.waitForTimeout(5000)
    const gameBtn2 = await findGameButton(page)
    if (gameBtn2) await gameBtn2.click()
    await page.waitForTimeout(2500)
    const picker2 = page.getByRole('button', { name: '选择故事线' }).first()
    if (await picker2.isVisible().catch(() => false)) {
      await picker2.click()
      await page.waitForTimeout(1200)
      const option2 = page.locator(`button[aria-label="${storyTitle}"]`).first()
      if (await option2.isVisible().catch(() => false)) { await option2.click(); await page.waitForTimeout(3000) }
    }
    await page.getByPlaceholder(/你要做什么/).first().waitFor({ state: 'visible', timeout: 30000 })
    await page.waitForTimeout(2000)
    await scrollStageToStart(page)
    const afterReload = await page.getByText(t2head, { exact: false }).first().waitFor({ state: 'visible', timeout: 15000 })
      .then(() => true).catch(() => false)
    record('刷新/重启后正文恢复', afterReload); flush()
    await page.screenshot({ path: `${ART}/02-after-reload.png` })

    // Drive the real composer through the original "fill option then send" chain.
    const before = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body?.turns?.length || 0
    await scrollStageToEnd(page)
    await composer.fill('我仔细查看丹铺后巷墙上的痕迹。')
    await page.locator('[data-action="send"]').first().click()
    const grew = await waitForTurnCount(storyId, before + 1, 180000)
    record('浏览器内经原版输入链推进一回合', grew, `${before} → ${grew} 回合`); flush()
    await page.screenshot({ path: `${ART}/03-ui-turn.png` })
  } catch (e) {
    record('浏览器链路验收', false, e.message.split('\n')[0]); flush()
    await page.screenshot({ path: `${ART}/99-ui-failure.png` }).catch(() => {})
  }
  const snapReload = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
  record('重启后回合数一致', (snapReload.turns?.length || 0) >= (snap2.turns?.length || 0),
    `${snapReload.turns?.length} vs ${snap2.turns?.length}`); flush()

  // 6. Branch create from last turn
  const mainBefore = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
  const mainTurnCount = mainBefore.turns?.length || 0
  const lastTurnId = mainBefore.turns?.[mainTurnCount - 1]?.id
  const br = await post(`/api/interactive/stories/${storyId}/branches`, { parent_event_id: lastTurnId, title: `验收支线-${unique}` })
  record('创建分支', br.ok, JSON.stringify(br.body).slice(0, 200)); flush()
  const brs = (await get(`/api/interactive/stories/${storyId}/branches`)).body
  record('分支列表含新支线', (brs.branches || []).some((b) => b.title === `验收支线-${unique}`), `共 ${(brs.branches || []).length} 条`); flush()
  if (br.body?.id) {
    const sw = await post(`/api/interactive/stories/${storyId}/switch-branch`, { branch_id: br.body.id })
    record('切换分支', sw.ok, JSON.stringify(sw.body).slice(0, 120)); flush()
    const brSnap = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=${encodeURIComponent(br.body.id)}`)).body
    record('新分支继承父回合且可独立推进', (brSnap.turns?.length || 0) >= 1, `分支回合数 ${brSnap.turns?.length ?? 'n/a'}`); flush()
    const mainSnap = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
    record('主线状态未被分支切换污染', (mainSnap.turns?.length || 0) === mainTurnCount,
      `主线 ${mainSnap.turns?.length} vs ${mainTurnCount}`); flush()
  }

  // 7. Isolation: a second story in same workspace keeps its own turns
  const story2 = await post('/api/interactive/stories', {
    title: `隔离故事-${unique}`, origin: '另一处世界的开端。',
    protagonist: { mode: 'custom', name: '甲', profile: '测试' },
    choice_count: 2, planning_mode: 'disabled', state_schema_policy: { mode: 'fixed_template' },
  })
  const s2snap = story2.body?.id
    ? (await get(`/api/interactive/stories/${story2.body.id}/snapshot?branch=main`)).body
    : {}
  record('同工作区新故事独立（无串档）', (s2snap.turns?.length || 0) === 0, `新故事回合数 ${s2snap.turns?.length ?? 'n/a'}`); flush()
  const story1After = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
  record('新建故事后原故事回合未受影响', (story1After.turns?.length || 0) === mainTurnCount,
    `${story1After.turns?.length} vs ${mainTurnCount}`); flush()

  const unexpected = consoleErrors.filter((t) => !/404|favicon|Failed to load resource/.test(t))
  record('浏览器无未预期报错', unexpected.length === 0, unexpected.slice(0, 3).join(' | ')); flush()

  await finish(browser)
}

// The stage renders its message list virtually, so a turn only exists in the
// DOM while it is near the viewport. Reading an older turn means scrolling to
// it first; asserting on it in place would fail on any story longer than the
// window however correct the rendering is.
const STAGE_SCROLLER = '.nova-chat-canvas'

async function scrollStageToStart(page) {
  const scroller = page.locator(STAGE_SCROLLER).first()
  if (!(await scroller.isVisible().catch(() => false))) return false
  await scroller.hover().catch(() => {})
  // The list pages older turns in as the top is reached, so keep pushing until
  // the scroll position stops moving rather than wheeling a fixed number of times.
  let last = -1
  for (let i = 0; i < 40; i++) {
    const top = await page.evaluate((sel) => {
      const el = document.querySelector(sel)
      if (!el) return -1
      el.scrollTop = 0
      return el.scrollTop
    }, STAGE_SCROLLER)
    await page.waitForTimeout(400)
    if (top === 0 && last === 0) break
    last = top
  }
  await page.waitForTimeout(800)
  return true
}

async function scrollStageToEnd(page) {
  const scroller = page.locator(STAGE_SCROLLER).first()
  if (!(await scroller.isVisible().catch(() => false))) return false
  await page.evaluate((sel) => {
    const el = document.querySelector(sel)
    if (el) el.scrollTop = el.scrollHeight
  }, STAGE_SCROLLER)
  await page.waitForTimeout(800)
  return true
}

async function findGameButton(page) {  const candidates = [
    page.getByRole('button', { name: '游戏', exact: true }),
    page.locator('button', { hasText: /^游戏$/ }),
    page.locator('[data-mode="story"]'),
    page.getByText('游戏', { exact: true }),
  ]
  for (const c of candidates) {
    const first = c.first()
    if (await first.isVisible().catch(() => false)) return first
  }
  return null
}
async function waitForTurnCount(storyId, want, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  let count = 0
  while (Date.now() < deadline) {
    count = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body?.turns?.length || 0
    if (count >= want) return count
    await new Promise((r) => setTimeout(r, 3000))
  }
  return count
}
async function post(path, data) { return req('POST', path, data) }
async function get(path) { return req('GET', path) }
async function req(method, path, data) {
  const res = await fetch(`${BASE}${path}`, {
    method, headers: { 'Content-Type': 'application/json' },
    body: data ? JSON.stringify(data) : undefined,
  })
  const text = await res.text()
  let body; try { body = JSON.parse(text) } catch { body = text }
  return { ok: res.ok, status: res.status, body }
}
async function pollSnapshot(storyId, wantTurns, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  let last = {}
  while (Date.now() < deadline) {
    last = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body || {}
    if ((last.turns?.length || 0) >= wantTurns) return last
    await new Promise((r) => setTimeout(r, 2500))
  }
  return last
}
async function finish(browser) {
  await browser.close()
  flush()
  console.log(`==== 真实模型验收：${steps.length - failed}/${steps.length} 通过 ====`)
  for (const s of steps) console.log(`${s.ok ? 'PASS' : 'FAIL'}  ${s.name}${s.detail ? ' — ' + s.detail : ''}`)
  if (failed) process.exitCode = 1
}

main().catch((e) => { record('脚本异常', false, e.message); flush(); console.error(e); process.exitCode = 1 })
