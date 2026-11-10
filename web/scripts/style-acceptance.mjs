/**
 * Style acceptance: STY-04 (purpose selection, hot switch, localization, "takes
 * effect next time") and STY-08 (the twelve styles are actually distinct, and a
 * non-cultivation world is written in a style that fits it).
 *
 * The product's own claim that a style reached the model is not evidence that it
 * did, so this driver reads the wire. It runs the backend against a recording
 * passthrough of the live gateway and asserts on the fragments that the recorded
 * request actually carried.
 *
 * Run with the backend already serving on TA_ACCEPT_BASE and the proxy
 * already recording into TA_ACCEPT_WIRE_DIR. Results go to
 * TA_ACCEPT_RESULT so output survives pipe buffering.
 */
import { chromium } from '@playwright/test'
import { randomUUID } from 'node:crypto'
import { mkdirSync, readFileSync, writeFileSync, existsSync } from 'node:fs'

const BASE = process.env.TA_ACCEPT_BASE || 'http://127.0.0.1:19740'
const WIRE = process.env.TA_ACCEPT_WIRE_DIR || './test-results/style/wire'
const OUT = process.env.TA_ACCEPT_RESULT || './style-acceptance-result.json'
const ART = './test-results/style'
mkdirSync(ART, { recursive: true })

const REQUEST_LOG = `${WIRE}/requests.jsonl`
const RESPONSE_LOG = `${WIRE}/responses.jsonl`

const steps = []
let failed = 0
function record(name, ok, detail = '') {
  steps.push({ name, ok: !!ok, detail: String(detail) })
  if (!ok) failed += 1
}

/** Prose the twelve migrated styles would each have to reach for. */
const CULTIVATION_TERMS = [
  '灵根', '炼气', '筑基', '修士', '丹药', '灵气', '宗门', '法器', '灵石', '丹田', '法宝', '散修',
]

/**
 * The world-fit vocabulary is read from the pack that was actually installed
 * rather than hardcoded here.
 *
 * A fixed list of period words answered a narrower question than the check
 * claims to ask. It measured whether the model happened to use words this file
 * guessed in advance, so a turn that named 沈曼青 and walked down 霞飞路 could
 * still be reported as not fitting the world — the world it was written in.
 * The pack's own lore names and keywords are the definition of that world, and
 * they are read from the workspace the acceptance just populated, so the check
 * follows the pack instead of the author's memory of it.
 */
function worldTerms(lorePath) {
  if (!existsSync(lorePath)) return { terms: [], names: [] }
  let parsed
  try { parsed = JSON.parse(readFileSync(lorePath, 'utf8')) } catch { return { terms: [], names: [] } }
  const items = Array.isArray(parsed) ? parsed : (parsed?.items ?? [])
  const names = []
  const terms = new Set()
  for (const item of items) {
    const name = String(item?.name ?? '').trim()
    if (name) { names.push(name); terms.add(name) }
    for (const keyword of Array.isArray(item?.keywords) ? item.keywords : []) {
      const word = String(keyword ?? '').trim()
      if (word) terms.add(word)
    }
  }
  return { terms: [...terms], names }
}

function readJsonl(path) {
  if (!existsSync(path)) return []
  return readFileSync(path, 'utf8')
    .split('\n')
    .filter((line) => line.trim())
    .map((line) => { try { return JSON.parse(line) } catch { return null } })
    .filter(Boolean)
}

function requests() { return readJsonl(REQUEST_LOG) }
function responses() { return readJsonl(RESPONSE_LOG) }

/** The last recorded request that carried a given purpose's fragment. */
function lastRequestWith(purpose) {
  const all = requests()
  for (let i = all.length - 1; i >= 0; i -= 1) {
    if ((all[i].styleSections || []).some((section) => section.purpose === purpose)) return all[i]
  }
  return null
}

function sectionOf(request, purpose) {
  return (request?.styleSections || []).find((section) => section.purpose === purpose) ?? null
}

/**
 * Whether a response is a planning checkpoint rather than narrative prose.
 *
 * One turn makes more than one model call: the turn-side call returns a
 * checkpoint document (its content opens with a "检查点" heading or a "Goal"
 * section), and a later call returns the prose the reader actually sees.
 */
function isCheckpoint(text) {
  const head = text.slice(0, 200)
  return head.includes('检查点') || /^\s*#+\s*(Goal|目标)\b/.test(head) || head.includes('## Goal')
}

/**
 * The narrative prose a turn's model calls produced, in order.
 *
 * The window's own request ids are passed in rather than a starting id: taking
 * everything recorded after the first call would concatenate this turn's
 * checkpoint, its prose, and every later turn's output — including other
 * styles' — so a verdict drawn from it would not be about this style at all.
 * Checkpoints are dropped for the same reason.
 */
function narrativeProse(requestIds) {
  const byId = new Map(responses().map((entry) => [entry.id, entry]))
  const parts = []
  for (const id of requestIds) {
    const text = byId.get(id)?.contentExcerpt || ''
    if (!text.trim() || isCheckpoint(text)) continue
    parts.push(text)
  }
  return parts.join('\n')
}

function termsIn(text, terms) {
  return terms.filter((term) => text.includes(term))
}

async function post(path, data) { return req('POST', path, data) }
async function patch(path, data) { return req('PATCH', path, data) }
async function get(path) { return req('GET', path) }

/**
 * Reads one story's persisted summary.
 *
 * There is no GET /api/interactive/stories/:id route; the product exposes the
 * collection and the snapshot. Reading through the collection is what the panel
 * itself does, and it keeps this driver honest: it sees exactly the persisted
 * record a reload would, not an optimistic in-memory copy.
 *
 * A miss is reported, never swallowed. Returning an empty object here once hid a
 * real read failure behind a bare "{}" in the results, which reads as "the field
 * is unset" when it actually means "this read did not work". The retry covers a
 * transient refusal during the story write that immediately precedes it.
 */
async function storySummary(storyId, attempts = 5) {
  let last = null
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    const res = await get('/api/interactive/stories')
    const index = res.body
    const stories = Array.isArray(index?.stories) ? index.stories : []
    const found = stories.find((story) => story.id === storyId)
    if (found) return found
    last = { status: res.status, ids: stories.map((s) => s.id), body: typeof index === 'string' ? index.slice(0, 200) : index }
    await new Promise((r) => setTimeout(r, 800))
  }
  // The caller compares fields, so an unreadable record has to be distinguishable
  // from a readable one whose fields are unset. `__unreadable` carries the reason.
  return { __unreadable: true, __last: last }
}

async function req(method, path, data) {
  const res = await fetch(`${BASE}${path}`, {
    method, headers: { 'Content-Type': 'application/json' },
    body: data ? JSON.stringify(data) : undefined,
  })
  const text = await res.text()
  let body; try { body = JSON.parse(text) } catch { body = text }
  return { ok: res.ok, status: res.status, body }
}

function flush(extra = {}) {
  writeFileSync(OUT, JSON.stringify({
    passed: steps.length - failed, total: steps.length, steps, ...extra,
  }, null, 2))
}

async function pollTurns(storyId, want, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  let count = 0
  while (Date.now() < deadline) {
    count = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body?.turns?.length || 0
    if (count >= want) return count
    await new Promise((r) => setTimeout(r, 3000))
  }
  return count
}

/**
 * Sends one turn through the real API and waits for it to be committed.
 *
 * The deadline is generous because the live gateway refuses a fraction of
 * requests with a 5xx and the product retries them; a turn that is retrying is
 * still a turn in progress, not a turn that failed. Too short a deadline here
 * reported "the turn did not advance" for turns that had in fact committed —
 * the snapshot showed them a minute later. Waiting longer costs only wall clock
 * on a run that is already slow, and it keeps the failure meaning what it says.
 */
async function turn(storyId, want, message, timeoutMs = 420000) {
  await post('/api/interactive/chat', {
    command_id: `sty-${randomUUID().slice(0, 8)}`,
    mode: 'story', story_id: storyId, branch: 'main', message,
  })
  return pollTurns(storyId, want, timeoutMs)
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

/** Opens the game stage and selects a story line by title. */
async function openStory(page, title) {
  const game = await findGameButton(page)
  if (game) await game.click()
  await page.waitForTimeout(2500)
  const picker = page.getByRole('button', { name: '选择故事线' }).first()
  if (await picker.isVisible().catch(() => false)) {
    await picker.click()
    await page.waitForTimeout(1200)
    const option = page.locator(`button[aria-label="${title}"]`).first()
    if (await option.isVisible().catch(() => false)) {
      await option.click()
      await page.waitForTimeout(2500)
    }
  }
}

/** Opens the story console's control tab, where the style pickers live. */
async function openControls(page) {
  const show = page.getByRole('button', { name: '显示游戏控制台' }).first()
  if (await show.isVisible().catch(() => false)) {
    await show.click()
    await page.waitForTimeout(1500)
  }
  const controls = page.getByRole('tab', { name: '控制' }).first()
  if (await controls.isVisible().catch(() => false)) {
    await controls.click()
    await page.waitForTimeout(1500)
  }
}

/** Picks an option in one of the three purpose selects, by its aria-label. */
async function choosePurpose(page, label, optionName) {
  const trigger = page.getByLabel(label, { exact: true }).first()
  await trigger.waitFor({ state: 'visible', timeout: 20000 })
  await trigger.click()
  await page.waitForTimeout(900)
  const option = page.getByRole('option', { name: optionName }).first()
  await option.waitFor({ state: 'visible', timeout: 15000 })
  await option.click()
  await page.waitForTimeout(2500)
}

async function main() {
  const unique = randomUUID().slice(0, 8)
  const wireStart = requests().length
  const run = { unique, wireStart, phases: {} }

  // --- Workspace ---
  const book = await post('/api/books/create', { title: `文风验收-${unique}`, author: 'Text Adventure' })
  if (!book.ok) { record('创建工作区', false, JSON.stringify(book.body).slice(0, 200)); flush(run); return }
  const projectId = book.body.project_id
  await post('/api/workspace/switch', { path: book.body.workspace })
  record('创建工作区', true, book.body.workspace)

  // --- Phase A: a non-cultivation world (STY-08) ---
  const detective = await post(`/api/projects/${encodeURIComponent(projectId)}/story-packs/wumian-zhentan/apply`, {})
  record('载入雾夜侦探剧本包（非修仙）', detective.ok && detective.body.world_added > 0, `新增 ${detective.body?.world_added} 条`)
  if (!detective.body?.request) { flush(run); return }

  const storyA = await post('/api/interactive/stories', {
    ...detective.body.request, title: `雾夜-${unique}`,
  })
  record('按非修仙剧本包创建故事线', storyA.ok && !!storyA.body.id, storyA.body?.id || JSON.stringify(storyA.body).slice(0, 200))
  if (!storyA.ok) { flush(run); return }
  const storyAId = storyA.body.id
  record('新建故事默认不写入文风（跟随底座默认）',
    !storyA.body.style_purposes?.dialogue && !storyA.body.style_purposes?.combat,
    `story_teller_id=${storyA.body.story_teller_id || '(empty)'}`)

  // The pack ships a preset opening, so the first model call is the first turn.
  const aBefore = requests().length
  const aTurns = await turn(storyAId, 1, '我请那位女士坐下，先问她丈夫失踪前最后见过谁，再问她照片是谁塞进门的。')
  record('非修仙故事真实推进一回合', aTurns >= 1, `${aTurns} 回合`)
  const aReqs = requests().slice(aBefore)
  const aMain = aReqs.map((r) => sectionOf(r, 'main')).find(Boolean)
  record('非修仙世界的主叙事请求确实带上了文风片段', !!aMain,
    aMain ? `主叙事片段 ${aMain.chars} 字` : '未捕获到主叙事片段')

  const aProse = narrativeProse(aReqs.map((r) => r.id))
  const aCultivation = termsIn(aProse, CULTIVATION_TERMS)
  // The world's own cast and places, read from the pack this run installed. A
  // narrative that names the client and walks the street the pack defines is
  // written in that world; a narrative that only shares the era's vocabulary is
  // not, so the names are reported separately from the general keywords.
  const world = worldTerms(`${book.body.workspace}/setting/lore/items.json`)
  const aWorldNames = termsIn(aProse, world.names)
  const aWorldTerms = termsIn(aProse, world.terms)
  record('非修仙正文未出现修仙词汇', aCultivation.length === 0,
    aCultivation.length ? `出现 ${aCultivation.join('、')}` : `命中本世界词汇 ${aWorldTerms.join('、') || '（无）'}`)
  record('非修仙正文贴合该世界', aWorldNames.length >= 1 && aWorldTerms.length >= 2,
    `人物/地点 ${aWorldNames.join('、') || '（无）'}；世界词 ${aWorldTerms.join('、') || '（无）'}`)
  run.phases.nonCultivation = {
    mainStyleChars: aMain?.chars ?? null,
    mainStyleSha: aMain?.sha256 ?? null,
    proseChars: aProse.length,
    cultivationTerms: aCultivation,
    worldNames: aWorldNames,
    worldTerms: aWorldTerms,
    worldVocabularySize: world.terms.length,
    proseExcerpt: aProse.slice(0, 400),
  }
  flush(run)

  // --- Phase B: STY-04 — switch purposes in the real UI, then check the wire ---
  const browser = await chromium.launch()
  const context = await browser.newContext({ baseURL: BASE, locale: 'zh-CN', serviceWorkers: 'block', reducedMotion: 'reduce' })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()) })
  page.on('pageerror', (e) => consoleErrors.push(`pageerror: ${e.message}`))
  run.phases.switch = {}
  try {
    await page.goto('/')
    await page.waitForTimeout(5000)
    const skip = page.getByRole('button', { name: '跳过引导' }).first()
    if (await skip.isVisible().catch(() => false)) {
      await skip.click()
      await page.waitForTimeout(1200)
    }
    await openStory(page, `雾夜-${unique}`)
    await openControls(page)

    // The panel must show what the story actually runs on: both override
    // purposes unset, so both read as following the main narrative.
    const dialogueTrigger = page.getByLabel('对白与聊天', { exact: true }).first()
    await dialogueTrigger.waitFor({ state: 'visible', timeout: 20000 })
    const dialogueBefore = (await dialogueTrigger.textContent()) || ''
    record('对白用途初始显示为跟随主叙事', dialogueBefore.includes('跟随主叙事'), dialogueBefore.trim())
    const combatBefore = ((await page.getByLabel('战斗描写', { exact: true }).first().textContent()) || '')
    record('战斗用途初始显示为跟随主叙事', combatBefore.includes('跟随主叙事'), combatBefore.trim())
    await page.screenshot({ path: `${ART}/01-purposes-inherit.png` })

    // Two different styles in the two override purposes. If a single change
    // moved both, the purposes would not be independent and this would fail.
    await choosePurpose(page, '对白与聊天', '轻佻吐槽')
    await choosePurpose(page, '战斗描写', '热血战斗')

    const afterDialogue = ((await page.getByLabel('对白与聊天', { exact: true }).first().textContent()) || '')
    const afterCombat = ((await page.getByLabel('战斗描写', { exact: true }).first().textContent()) || '')
    record('对白用途已改为轻佻吐槽', afterDialogue.includes('轻佻吐槽'), afterDialogue.trim())
    record('战斗用途已改为热血战斗', afterCombat.includes('热血战斗'), afterCombat.trim())
    await page.screenshot({ path: `${ART}/02-purposes-set.png` })

    // The persisted story is what generation reads, so check it directly rather
    // than trusting the panel's own optimistic state.
    const storyNow = await storySummary(storyAId)
    const purposes = storyNow.style_purposes || {}
    record('分用途选择已落盘', purposes.dialogue === 'flippant-banter' && purposes.combat === 'hotblooded-combat',
      storyNow.__unreadable ? `故事记录读取失败 ${JSON.stringify(storyNow.__last)}` : JSON.stringify(purposes))
    record('主叙事未被分用途改动牵连', !storyNow.__unreadable && (storyNow.story_teller_id || '') === (storyA.body.story_teller_id || ''),
      storyNow.__unreadable ? '故事记录读取失败' : `${storyA.body.story_teller_id} → ${storyNow.story_teller_id}`)

    // STY-04's "takes effect on the next generation": the switch happened in the
    // browser, so the next turn is the next generation and must carry both.
    const bBefore = requests().length
    const bTurns = await turn(storyAId, aTurns + 1,
      '我追问那位女士：昨夜雾大，她丈夫出门前有没有带伞，有没有提过要去码头见谁。')
    record('改文风后真实推进一回合', bTurns >= aTurns + 1, `${aTurns} → ${bTurns} 回合`)
    const bReqs = requests().slice(bBefore)
    const bDialogue = bReqs.map((r) => sectionOf(r, 'dialogue')).find(Boolean)
    const bCombat = bReqs.map((r) => sectionOf(r, 'combat')).find(Boolean)
    const bMain = bReqs.map((r) => sectionOf(r, 'main')).find(Boolean)
    record('下一请求携带对白文风片段', !!bDialogue, bDialogue ? `${bDialogue.chars} 字` : '未携带')
    record('下一请求携带战斗文风片段', !!bCombat, bCombat ? `${bCombat.chars} 字` : '未携带')
    record('下一请求仍携带主叙事片段（分用途不替换主叙事）', !!bMain, bMain ? `${bMain.chars} 字` : '未携带')
    record('对白与战斗片段互不相同（两个用途独立生效）',
      !!bDialogue && !!bCombat && bDialogue.sha256 !== bCombat.sha256,
      bDialogue && bCombat ? `${bDialogue.sha256.slice(0, 12)} vs ${bCombat.sha256.slice(0, 12)}` : '缺少片段')
    // A purpose that inherits must not gain a fragment of its own, so the
    // dialogue fragment's text has to differ from the main fragment's text.
    record('对白片段文本不同于主叙事片段（确为覆盖而非继承）',
      !!bDialogue && !!bMain && bDialogue.sha256 !== bMain.sha256,
      bDialogue && bMain ? `${bDialogue.sha256.slice(0, 12)} vs ${bMain.sha256.slice(0, 12)}` : '缺少片段')
    run.phases.switch = {
      before: { dialogue: dialogueBefore.trim(), combat: combatBefore.trim() },
      after: { dialogue: afterDialogue.trim(), combat: afterCombat.trim() },
      persisted: purposes,
      request: {
        dialogue: bDialogue ? { chars: bDialogue.chars, sha: bDialogue.sha256, excerpt: bDialogue.excerpt } : null,
        combat: bCombat ? { chars: bCombat.chars, sha: bCombat.sha256, excerpt: bCombat.excerpt } : null,
        main: bMain ? { chars: bMain.chars, sha: bMain.sha256 } : null,
      },
    }
    flush(run)

    // Clear one purpose back to inheriting; the other must be left alone.
    //
    // What is asserted is the meaning, not a spelling of it: a cleared purpose
    // must name no style. The record carries `omitempty`, so "follow the main
    // narrative" arrives as an absent key rather than as an empty string, and an
    // earlier version of this check demanded the empty string and so failed a
    // record that was correct. The journal-level proof that combat really
    // inherits is the next request: it must carry no combat fragment.
    await choosePurpose(page, '战斗描写', '跟随主叙事')
    const storyCleared = await storySummary(storyAId)
    const clearedPurposes = storyCleared.style_purposes || {}
    record('清除战斗用途后不再指定文风（改为跟随主叙事）',
      !storyCleared.__unreadable && !clearedPurposes.combat,
      storyCleared.__unreadable ? `故事记录读取失败 ${JSON.stringify(storyCleared.__last)}` : JSON.stringify(clearedPurposes))
    record('清除一项不重置另一项', !storyCleared.__unreadable && clearedPurposes.dialogue === 'flippant-banter',
      storyCleared.__unreadable ? '故事记录读取失败' : JSON.stringify(clearedPurposes))

    const cBefore = requests().length
    const cTurns = await turn(storyAId, bTurns + 1, '我问她：照片背面有没有字，是谁的字迹。')
    const cReqs = requests().slice(cBefore)
    const cDialogue = cReqs.map((r) => sectionOf(r, 'dialogue')).find(Boolean)
    const cCombat = cReqs.map((r) => sectionOf(r, 'combat')).find(Boolean)
    record('清除后对白覆盖仍然生效', !!cDialogue, cDialogue ? `${cDialogue.chars} 字` : '未携带')
    // Inheriting combat now means the main style's own combat fragment, which for
    // the base style is empty — so no combat fragment is the correct outcome.
    record('清除后战斗不再作为覆盖片段出现', !cCombat, cCombat ? `仍携带 ${cCombat.chars} 字` : '未携带')
    run.phases.clear = { turns: cTurns, dialogue: !!cDialogue, combat: !!cCombat, persisted: storyCleared.style_purposes }
    flush(run)

    // Reload the browser: the choices must come back as saved.
    await page.reload()
    await page.waitForTimeout(5000)
    await openStory(page, `雾夜-${unique}`)
    await openControls(page)
    const reloadedDialogue = ((await page.getByLabel('对白与聊天', { exact: true }).first().textContent()) || '')
    const reloadedCombat = ((await page.getByLabel('战斗描写', { exact: true }).first().textContent()) || '')
    record('刷新后对白用途仍为轻佻吐槽', reloadedDialogue.includes('轻佻吐槽'), reloadedDialogue.trim())
    record('刷新后战斗用途仍为跟随主叙事', reloadedCombat.includes('跟随主叙事'), reloadedCombat.trim())
    await page.screenshot({ path: `${ART}/03-after-reload.png` })

    const unexpected = consoleErrors.filter((t) => !/404|favicon|Failed to load resource/.test(t))
    record('浏览器无未预期报错', unexpected.length === 0, unexpected.slice(0, 3).join(' | '))
  } catch (error) {
    record('浏览器链路验收', false, String(error.message || error).split('\n')[0])
    await page.screenshot({ path: `${ART}/99-failure.png` }).catch(() => {})
  }
  await browser.close()
  flush(run)

  // --- Phase C: STY-08 — each of the twelve styles reaches the model distinctly ---
  // Twelve styles would be twelve more model turns if each were generated, and
  // what STY-08 has to show first is that they are actually different voices
  // rather than twelve names over one prompt. The recorded fragments answer that
  // directly; the prose samples below answer whether the writing differs.
  const distinct = await checkTwelveStyles(storyAId, unique)
  run.phases.twelveStyles = distinct
  flush(run)

  console.log(`==== 文风验收：${steps.length - failed}/${steps.length} 通过 ====`)
  for (const step of steps) console.log(`${step.ok ? 'PASS' : 'FAIL'}  ${step.name}${step.detail ? ' — ' + step.detail : ''}`)
  if (failed) process.exitCode = 1
}

/**
 * Sets each of the twelve migrated styles on the main purpose through the real
 * API and records the fragment each one produces, then samples prose from a
 * representative subset. The fragment check is exhaustive because it is cheap
 * and catches the "twelve names, one prompt" failure; the prose check is
 * sampled because each sample is a real model turn.
 */
async function checkTwelveStyles(storyId, unique) {
  const list = (await get('/api/interactive/tellers')).body
  const tellers = Array.isArray(list) ? list : (list.tellers || [])
  // The title is read so the style PATCH below can resend it unchanged: the
  // update request is not partial for the title, so omitting it would rename
  // the story to the default.
  const currentTitle = (await storySummary(storyId)).title || ''
  const migrated = [
    'source-drama', 'marketplace-light', 'strange-tales', 'straight-comedy',
    'flippant-banter', 'deadpan-humor', 'restrained-hard', 'intrigue-ensemble',
    'slice-banter', 'hotblooded-combat', 'cinematic-shots', 'dialogue-piece',
  ]
  const present = migrated.filter((id) => tellers.some((teller) => teller.id === id))
  record('十二类文风全部可从接口取到', present.length === 12, `取到 ${present.length}/12`)

  const fragmentByStyle = {}
  const sampledProse = {}
  // Two styles far apart in register, so a "they all read the same" result would
  // be visible in the prose and not only in the fragment hashes.
  const proseSamples = ['strange-tales', 'flippant-banter']

  let turns = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body?.turns?.length || 0
  for (const id of present) {
    // The style is set through the same update the console panel uses. The
    // title is sent unchanged: the update request is not partial for the title,
    // so omitting it would rename the story.
    const updated = await patch(`/api/interactive/stories/${storyId}`, { title: currentTitle, story_teller_id: id })
    if (!updated.ok) { record(`切换主叙事文风 ${id}`, false, JSON.stringify(updated.body).slice(0, 120)); continue }
    const before = requests().length
    turns = await turn(storyId, turns + 1, `我继续追这条线索，留意周围人的反应。(${unique}-${id})`)
    const turnRequests = requests().slice(before)
    const main = turnRequests.map((r) => sectionOf(r, 'main')).find(Boolean)
    fragmentByStyle[id] = main ? { chars: main.chars, sha: main.sha256 } : null
    if (proseSamples.includes(id)) {
      // This turn's own calls only: the sample has to be prose this style
      // produced, not everything the run recorded afterwards.
      const prose = narrativeProse(turnRequests.map((r) => r.id))
      sampledProse[id] = { chars: prose.length, excerpt: prose.slice(0, 300) }
    }
  }

  const missing = present.filter((id) => !fragmentByStyle[id])
  record('每一类都产生了自己的主叙事片段', missing.length === 0, missing.length ? `缺失 ${missing.join('、')}` : `${present.length} 类`)
  const shas = Object.values(fragmentByStyle).filter(Boolean).map((entry) => entry.sha)
  const uniqueShas = new Set(shas)
  record('十二类主叙事片段两两不同（不是同一份提示换名）',
    uniqueShas.size === shas.length && shas.length === 12,
    `${uniqueShas.size} 个不同片段 / ${shas.length} 类`)

  const samples = Object.entries(sampledProse)
  const identical = samples.length === 2 && samples[0][1].excerpt === samples[1][1].excerpt
  record('抽样两类正文并非同一段文字', samples.length === 2 && !identical,
    samples.map(([id, s]) => `${id}:${s.chars}字`).join(' '))

  return { present, fragmentByStyle, sampledProse }
}

main().catch((error) => {
  record('脚本异常', false, String(error.message || error))
  flush()
  console.error(error)
  process.exitCode = 1
})
