/**
 * Script pack acceptance: two packs must open, progress and restore
 * independently, through the game adapter and the browser.
 *
 * Run with the backend already serving on TA_ACCEPT_BASE.
 * Writes results after every step so output survives pipe buffering.
 */
import { chromium } from '@playwright/test'
import { randomUUID } from 'node:crypto'
import { writeFileSync, mkdirSync } from 'node:fs'

const BASE = process.env.TA_ACCEPT_BASE || 'http://127.0.0.1:19711'
const OUT = process.env.TA_ACCEPT_RESULT || './story-pack-acceptance-result.json'
const ART = './test-results/story-pack'
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
  const unique = randomUUID().slice(0, 8)

  // --- Catalog ---
  const catalog = await get('/api/story-packs')
  const packs = catalog.body?.packs || []
  record('剧本包目录可读取', catalog.ok && packs.length >= 2, `共 ${packs.length} 个`)
  const fanren = packs.find((p) => p.id === 'fanren-jie')
  const detective = packs.find((p) => p.id === 'wumian-zhentan')
  record('内置「凡人人界」包存在', !!fanren, fanren ? `${fanren.world_count} 条世界书` : '未找到')
  record('内置「雾夜侦探」包存在', !!detective, detective ? `${detective.world_count} 条世界书` : '未找到')
  flush()
  if (!fanren || !detective) return finish()

  const detail = await get('/api/story-packs/fanren-jie')
  record('剧本包详情含开局与绑定', detail.ok && !!detail.body?.story?.origin && !!detail.body?.story?.module_refs?.rule_system_id,
    detail.body?.story?.module_refs?.rule_system_id || '缺少绑定')
  flush()

  // --- One workspace holds both worlds ---
  const book = await post('/api/books/create', { title: `剧本包验收-${unique}`, author: 'Text Adventure' })
  record('创建工作区', book.ok && !!book.body.workspace, book.body.workspace || JSON.stringify(book.body).slice(0, 160))
  flush()
  if (!book.ok) return finish()
  const projectId = book.body.project_id
  const workspace = book.body.workspace
  await post('/api/workspace/switch', { path: workspace })

  const worlds = [
    { pack: fanren, key: 'fanren', name: '凡人人界', freeInput: '我向丹铺掌柜打听最近镇上发生的怪事。', label: '凡人' },
    { pack: detective, key: 'detective', name: '雾夜侦探', freeInput: '我接过照片，先问那位女士昨晚最后一次见到丈夫是什么时候。', label: '侦探' },
  ]
  const started = {}

  for (const world of worlds) {
    const applied = await post(`/api/projects/${encodeURIComponent(projectId)}/story-packs/${world.pack.id}/apply`, {})
    record(`[${world.label}] 载入剧本包并写入世界书`, applied.ok && applied.body.world_added > 0,
      `新增 ${applied.body?.world_added} 条，已存在 ${applied.body?.world_exists} 条`)
    flush()
    if (!applied.ok) continue

    const request = applied.body.request
    const story = await post('/api/interactive/stories', request)
    record(`[${world.label}] 按剧本包创建故事线`, story.ok && !!story.body.id, story.body?.id || JSON.stringify(story.body).slice(0, 160))
    flush()
    if (!story.ok) continue
    const storyId = story.body.id
    started[world.key] = { storyId, request }

    await post('/api/interactive/chat', {
      command_id: `pack-opening-${storyId}`, mode: 'story', story_id: storyId, branch: 'main', start_opening: true,
    })
    const snap1 = await pollSnapshot(storyId, 1, 150000)
    const opening = snap1.turns?.[0]?.narrative || ''
    record(`[${world.label}] 独立开局生成（真实模型）`, opening.length > 50, `${opening.length} 字`)
    record(`[${world.label}] 开局长度达目标量级`, opening.length >= 400, `${opening.length} 字`)
    flush()

    await post('/api/interactive/chat', {
      command_id: `pack-turn2-${storyId}`, mode: 'story', story_id: storyId, branch: 'main', message: world.freeInput,
    })
    const snap2 = await pollSnapshot(storyId, 2, 180000)
    const turn2 = snap2.turns?.[1]
    record(`[${world.label}] 独立推进一回合`, (turn2?.narrative?.length || 0) > 50, `第2轮 ${turn2?.narrative?.length ?? 0} 字`)
    flush()

    const reloaded = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
    record(`[${world.label}] 重读快照后回合保留`, (reloaded.turns?.length || 0) === 2, `${reloaded.turns?.length} 回合`)
    flush()
  }

  // --- The two worlds must not share state ---
  const ids = Object.values(started).map((s) => s.storyId)
  if (ids.length === 2) {
    const [a, b] = ids
    const snapA = (await get(`/api/interactive/stories/${a}/snapshot?branch=main`)).body
    const snapB = (await get(`/api/interactive/stories/${b}/snapshot?branch=main`)).body
    record('两条故事线互不串档', snapA.story_id !== snapB.story_id && snapA.turns?.length === 2 && snapB.turns?.length === 2,
      `A=${snapA.turns?.length} 回合, B=${snapB.turns?.length} 回合`)
    const textA = (snapA.turns || []).map((t) => t.narrative || '').join('')
    const textB = (snapB.turns || []).map((t) => t.narrative || '').join('')
    record('两条故事线正文彼此独立', textA.slice(0, 40) !== textB.slice(0, 40), '两段开局正文不同')
    flush()
  }

  // --- World book landed in the workspace ---
  const lore = await get(`/api/projects/${encodeURIComponent(projectId)}/book/lore/items`)
  const items = lore.body?.items || lore.body || []
  const names = (Array.isArray(items) ? items : []).map((i) => i.name)
  record('世界书写入工作区资料库', names.length >= 6, `资料 ${names.length} 条`)
  record('凡人人界世界书条目可见', names.includes('修行境界') && names.includes('青石镇'), names.slice(0, 6).join('、'))
  record('雾夜侦探世界书条目可见', names.includes('沪上租界') && names.includes('袁明远'), names.slice(0, 6).join('、'))
  flush()

  // --- Applying the same pack again must not duplicate or overwrite ---
  const reapplied = await post(`/api/projects/${encodeURIComponent(projectId)}/story-packs/fanren-jie/apply`, {})
  record('重复载入同一剧本包不重复写入', reapplied.ok && reapplied.body.world_added === 0 && reapplied.body.world_exists === 4,
    `新增 ${reapplied.body?.world_added}，已存在 ${reapplied.body?.world_exists}`)
  flush()

  // --- Browser: both worlds are reachable and restore ---
  const browser = await chromium.launch()
  const context = await browser.newContext({ baseURL: BASE, locale: 'zh-CN', serviceWorkers: 'block', reducedMotion: 'reduce' })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()) })
  page.on('pageerror', (e) => consoleErrors.push(`pageerror: ${e.message}`))

  try {
    await page.goto('/')
    await page.waitForTimeout(5000)
    const skip = page.getByRole('button', { name: '跳过引导' }).first()
    if (await skip.isVisible().catch(() => false)) { await skip.click(); await page.waitForTimeout(1200) }

    const gameBtn = await findGameButton(page)
    record('游戏入口可用', !!gameBtn, gameBtn ? '找到入口' : '未找到')
    if (gameBtn) await gameBtn.click()
    await page.waitForTimeout(2500)

    const picker = page.getByRole('button', { name: '选择故事线' }).first()
    if (await picker.isVisible().catch(() => false)) {
      await picker.click()
      await page.waitForTimeout(1200)
      const options = await page.locator('button[aria-label]').allTextContents()
      const hasFanren = options.some((text) => text.includes('凡人人界'))
      const hasDetective = options.some((text) => text.includes('雾夜侦探'))
      record('故事线列表含两条剧本包故事', hasFanren && hasDetective, `凡人=${hasFanren} 侦探=${hasDetective}`)
      flush()

      const target = page.locator('button[aria-label="雾夜侦探"]').first()
      if (await target.isVisible().catch(() => false)) {
        await target.click()
        await page.waitForTimeout(3500)
      }
      await page.getByPlaceholder(/你要做什么/).first().waitFor({ state: 'visible', timeout: 30000 })
      await page.waitForTimeout(2500)
      const detectiveText = probeSnippet((await get(`/api/interactive/stories/${started.detective?.storyId}/snapshot?branch=main`)).body
        ?.turns?.[1]?.narrative)
      const shown = detectiveText
        ? await page.getByText(detectiveText, { exact: false }).first().waitFor({ state: 'visible', timeout: 15000 }).then(() => true).catch(() => false)
        : false
      record('浏览器内可切到剧本包故事并回看正文', shown, `匹配「${detectiveText || 'n/a'}…」`)
      await page.screenshot({ path: `${ART}/01-detective-stage.png` })

      await page.reload()
      await page.waitForTimeout(5000)
      const gameBtn2 = await findGameButton(page)
      if (gameBtn2) await gameBtn2.click()
      await page.waitForTimeout(2500)
      const picker2 = page.getByRole('button', { name: '选择故事线' }).first()
      if (await picker2.isVisible().catch(() => false)) {
        await picker2.click()
        await page.waitForTimeout(1200)
        const target2 = page.locator('button[aria-label="凡人人界"]').first()
        if (await target2.isVisible().catch(() => false)) { await target2.click(); await page.waitForTimeout(3500) }
      }
      await page.getByPlaceholder(/你要做什么/).first().waitFor({ state: 'visible', timeout: 30000 })
      await page.waitForTimeout(2500)
      const fanrenText = probeSnippet((await get(`/api/interactive/stories/${started.fanren?.storyId}/snapshot?branch=main`)).body
        ?.turns?.[1]?.narrative)
      const shown2 = fanrenText
        ? await page.getByText(fanrenText, { exact: false }).first().waitFor({ state: 'visible', timeout: 15000 }).then(() => true).catch(() => false)
        : false
      record('刷新后切换另一剧本包故事并回看正文', shown2, `匹配「${fanrenText || 'n/a'}…」`)
      await page.screenshot({ path: `${ART}/02-fanren-after-reload.png` })
    }
  } catch (e) {
    record('浏览器剧本包链路验收', false, e.message.split('\n')[0])
    await page.screenshot({ path: `${ART}/99-ui-failure.png` }).catch(() => {})
  }

  const unexpected = consoleErrors.filter((t) => !/404|favicon|Failed to load resource/.test(t))
  record('浏览器无未预期报错', unexpected.length === 0, unexpected.slice(0, 3).join(' | '))
  flush()
  await browser.close()
  finish()
}

async function findGameButton(page) {
  const candidates = [
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

/**
 * Picks a snippet from the first paragraph of a narrative for an on-screen text
 * probe. The reader renders each paragraph as its own element, so a slice that
 * crosses a blank line never matches a single node even though the text is
 * displayed. Staying inside the first line keeps the probe a real assertion.
 */
function probeSnippet(narrative) {
  const firstLine = String(narrative || '').split('\n').map((line) => line.trim()).find(Boolean)
  return firstLine ? firstLine.slice(0, 12) : ''
}
function finish() {
  flush()
  console.log(`==== 剧本包验收：${steps.length - failed}/${steps.length} 通过 ====`)
  for (const s of steps) console.log(`${s.ok ? 'PASS' : 'FAIL'}  ${s.name}${s.detail ? ' — ' + s.detail : ''}`)
  if (failed) process.exitCode = 1
}

main().catch((e) => { record('脚本异常', false, e.message); flush(); console.error(e); process.exitCode = 1 })
