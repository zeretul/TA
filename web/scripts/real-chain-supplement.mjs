/**
 * 真实链补充验收：剧情助手连续使用、资料库新实体增删改读、聊天草稿暂存。
 *
 * 这三项在《Claude_剩余任务统一收尾》第 8 节被列为「真实模型代表链」的一部分，
 * 但 real-model-acceptance.mjs 只覆盖到建角→开场→主线→重开→分支。这里补上剩下的
 * 三条，全部跑在隔离实例上，不碰正式安装。
 *
 * 生图不在本脚本里：隔离网关没有 /v1/images/generations，无法实测，单独在报告里
 * 标注为环境限制并引用 R07 的既有证据。
 *
 * 用法：
 *   TA_ACCEPT_BASE=http://127.0.0.1:19730 \
 *   TA_ACCEPT_PROJECT=project-xxx \
 *   TA_ACCEPT_STORY=st_xxx \
 *   node scripts/real-chain-supplement.mjs
 */
import { chromium } from '@playwright/test'
import { writeFileSync, mkdirSync } from 'node:fs'

const BASE = process.env.TA_ACCEPT_BASE || 'http://127.0.0.1:19730'
const PROJECT = process.env.TA_ACCEPT_PROJECT || ''
const STORY = process.env.TA_ACCEPT_STORY || ''
// 故事舞台打开时默认落在最新的故事线上，而验收用的那条往往不是最新的。
// 给标题就能在舞台上显式切回它，否则浏览器段落会停在一个空故事上。
const STORY_TITLE = process.env.TA_ACCEPT_STORY_TITLE || ''
const OUT = process.env.TA_ACCEPT_RESULT || './real-chain-supplement-result.json'
const ART = './test-results/real-chain-supplement'
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

async function req(method, path, data) {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: data ? JSON.stringify(data) : undefined,
  })
  const text = await res.text()
  let body
  try { body = JSON.parse(text) } catch { body = text }
  return { ok: res.ok, status: res.status, body }
}
const get = (p) => req('GET', p)
const post = (p, d) => req('POST', p, d)
const put = (p, d) => req('PUT', p, d)
const del = (p) => req('DELETE', p)

async function main() {
  if (!PROJECT || !STORY) {
    record('前置参数', false, '需要 TA_ACCEPT_PROJECT 与 TA_ACCEPT_STORY')
    return finish()
  }
  const storyPath = `/api/interactive/stories/${encodeURIComponent(STORY)}`

  const snap = await get(`${storyPath}/snapshot`)
  const branchId = snap.body?.branch_id || ''
  const turnId = snap.body?.current_turn?.id || ''
  const turnCount = snap.body?.turn_count || 0
  record('读取故事快照', !!branchId && !!turnId, `branch=${branchId} turn=${turnId} turns=${turnCount}`); flush()
  if (!branchId || !turnId) return finish()

  // ---------------------------------------------------------------
  // 1. 剧情助手：分析与灵感连续使用
  // ---------------------------------------------------------------
  const analyze = await post(`${storyPath}/assistant`, { branch_id: branchId, turn_id: turnId, action: 'analyze' })
  record('助手分析返回', analyze.ok && !!analyze.body?.read,
    `status=${analyze.status} read=${(analyze.body?.read || '').length}字 suggestions=${(analyze.body?.suggestions || []).length}`); flush()
  record('分析锚定同一回合', analyze.body?.turn_id === turnId && analyze.body?.context_revision === snap.body?.context_revision,
    `turn_id=${analyze.body?.turn_id} rev=${analyze.body?.context_revision} vs ${snap.body?.context_revision}`); flush()

  const inspire = await post(`${storyPath}/assistant`, { branch_id: branchId, turn_id: turnId, action: 'inspire' })
  record('助手灵感返回', inspire.ok && !!inspire.body?.read,
    `status=${inspire.status} read=${(inspire.body?.read || '').length}字 suggestions=${(inspire.body?.suggestions || []).length}`); flush()
  record('灵感锚定同一回合', inspire.body?.turn_id === turnId && inspire.body?.context_revision === snap.body?.context_revision,
    `turn_id=${inspire.body?.turn_id} rev=${inspire.body?.context_revision}`); flush()

  // 连续使用：第二次回答必须仍建立在这一回合上，不能因为第一次调用而失锚。
  record('连续调用未推进回合', (await get(`${storyPath}/snapshot`)).body?.turn_count === turnCount,
    `调用前后回合数 ${turnCount}`); flush()

  // T26：灵感必须给出既有选项之外的方向。
  const existingChoices = (snap.body?.current_turn?.turn_result?.choices || []).map((c) => (typeof c === 'string' ? c : c?.text || c?.label || ''))
  const inspired = (inspire.body?.suggestions || []).map((s) => (s.action || '').trim()).filter(Boolean)
  const outsideChoices = inspired.filter((action) => !existingChoices.includes(action))
  record('灵感给出既有选项之外的方向', inspired.length > 0 && outsideChoices.length === inspired.length,
    `${outsideChoices.length}/${inspired.length} 条不在既有 ${existingChoices.length} 个选项内`); flush()

  // 分析与灵感不写账：回合数、分支、当前回合都没有变化。
  const afterAssistant = (await get(`${storyPath}/snapshot`)).body
  record('助手两个动作都不产生回合',
    afterAssistant?.branch_id === branchId && afterAssistant?.current_turn?.id === turnId,
    `branch=${afterAssistant?.branch_id} turn=${afterAssistant?.current_turn?.id}`); flush()

  // ---------------------------------------------------------------
  // 2. 正式新实体与资料管理
  // ---------------------------------------------------------------
  const loreBase = `/api/projects/${encodeURIComponent(PROJECT)}/book/lore/items`
  const before = await get(loreBase)
  const beforeCount = (before.body?.items || []).length

  const stamp = Date.now().toString(36)
  const draft = {
    type: 'person',
    name: `验收临时人物${stamp}`,
    importance: 'minor',
    tags: ['验收', '临时'],
    brief_description: '资料库增删改读链路验收用条目',
    keywords: ['验收', stamp],
    load_mode: 'auto',
    content: '这是一条用于验收资料库新增、读取、更新与删除链路的临时条目，验收完成后删除。',
  }
  const created = await post(loreBase, draft)
  const createdItem = created.body?.item || {}
  record('资料库新增条目', created.ok && !!createdItem.id,
    `status=${created.status} id=${createdItem.id} name=${createdItem.name}`); flush()

  const listed = await get(loreBase)
  const found = (listed.body?.items || []).find((it) => it.id === createdItem.id)
  record('新增条目可读回', !!found && found.name === draft.name,
    `列表 ${beforeCount} → ${(listed.body?.items || []).length}，命中 ${found ? '是' : '否'}`); flush()
  // 空字符串就是共享作用域，而 omitempty 会把它从 JSON 里省略，所以两边都要归一。
  record('新增条目默认归当前作用域', (found?.scope ?? '') === (listed.body?.world_scope ?? ''),
    `条目 scope=${JSON.stringify(found?.scope ?? '')} 当前 world_scope=${JSON.stringify(listed.body?.world_scope ?? '')}`); flush()
  record('新增条目默认按需取用', found?.load_mode === 'auto',
    `load_mode=${found?.load_mode}`); flush()

  // 更新要求完整字段，并且可以带 base_revision 做乐观并发。
  const updatedContent = draft.content + ' 已更新。'
  const updateBody = {
    id: createdItem.id,
    enabled: true,
    type: draft.type,
    name: draft.name,
    importance: 'major',
    tags: draft.tags,
    brief_description: '更新后的简介',
    keywords: draft.keywords,
    load_mode: 'resident',
    content: updatedContent,
    base_revision: found?.updated_at,
  }
  const updated = await put(`${loreBase}/${encodeURIComponent(createdItem.id)}`, updateBody)
  record('资料库更新条目', updated.ok && updated.body?.item?.importance === 'major',
    `status=${updated.status} importance=${updated.body?.item?.importance} load_mode=${updated.body?.item?.load_mode}`); flush()
  record('更新内容已写入', updated.body?.item?.content === updatedContent,
    `${(updated.body?.item?.content || '').length} 字`); flush()

  // 用过期的 base_revision 必须被拒绝，而不是静默覆盖。
  const stale = await put(`${loreBase}/${encodeURIComponent(createdItem.id)}`, {
    ...updateBody, content: '过期写入', base_revision: '1970-01-01T00:00:00Z',
  })
  record('过期 revision 被拒绝', stale.status === 409,
    `status=${stale.status} body=${JSON.stringify(stale.body).slice(0, 160)}`); flush()

  // 更新缺字段必须被拒绝，不能把没提交的字段清空。
  const partial = await put(`${loreBase}/${encodeURIComponent(createdItem.id)}`, { name: draft.name })
  record('更新缺字段被拒绝', partial.status === 400,
    `status=${partial.status}`); flush()

  // 重名必须被拒绝，防止资料库里出现两个同名条目。
  const duplicate = await post(loreBase, { ...draft, name: draft.name })
  record('同名条目被拒绝', duplicate.status >= 400,
    `status=${duplicate.status}`); flush()

  const removed = await del(`${loreBase}/${encodeURIComponent(createdItem.id)}`)
  const afterDelete = await get(loreBase)
  record('资料库删除条目', removed.ok && (afterDelete.body?.items || []).length === beforeCount,
    `status=${removed.status} 列表 ${(afterDelete.body?.items || []).length}（原 ${beforeCount}）`); flush()

  // ---------------------------------------------------------------
  // 3. 聊天草稿暂存再发送（浏览器实机）
  // ---------------------------------------------------------------
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
    if (gameBtn) await gameBtn.click()
    await page.waitForTimeout(2500)

    // 舞台默认打开最新故事线，而那条可能是别的用例留下的空故事。按标题切回来，
    // 否则下面的输入框根本不会出现，会把「没切对故事」记成产品缺陷。
    if (STORY_TITLE) {
      const picker = page.getByRole('button', { name: '选择故事线' }).first()
      if (await picker.isVisible().catch(() => false)) {
        await picker.click()
        await page.waitForTimeout(1200)
        const option = page.locator(`button[aria-label="${STORY_TITLE}"]`).first()
        const found = await option.isVisible().catch(() => false)
        if (found) { await option.click(); await page.waitForTimeout(3000) }
        record('切换到验收故事线', found, found ? STORY_TITLE : `未找到「${STORY_TITLE}」`); flush()
      }
    }

    const composer = page.getByPlaceholder(/你要做什么/).first()
    const stageReady = await composer.waitFor({ state: 'visible', timeout: 45000 }).then(() => true).catch(() => false)
    record('主舞台可用', stageReady, stageReady ? '输入框可见' : '输入框未出现'); flush()
    if (!stageReady) return await finishBrowser(browser)

    // 工具栏入口的可访问名是「打开聊天窗口」，不是窗口标题「聊天」。
    const chatBtn = page.getByRole('button', { name: '打开聊天窗口' }).first()
    const chatVisible = await chatBtn.isVisible().catch(() => false)
    record('主舞台含聊天入口', chatVisible, chatVisible ? '找到按钮' : '未找到'); flush()
    if (!chatVisible) return await finishBrowser(browser)

    await chatBtn.click()
    await page.waitForTimeout(2500)
    const panel = page.locator('[data-testid="chat-window-panel"]').first()
    const panelVisible = await panel.waitFor({ state: 'visible', timeout: 20000 }).then(() => true).catch(() => false)
    record('聊天窗口打开', panelVisible, panelVisible ? '面板可见' : '面板未出现'); flush()
    if (!panelVisible) {
      await page.screenshot({ path: `${ART}/01-chat-missing.png` })
      return await finishBrowser(browser)
    }
    await page.screenshot({ path: `${ART}/01-chat-open.png` })

    // 会话名单是聊天外壳自己的行，正文才复用人物窗口面板。
    const rows = page.locator('[data-testid="chat-window-row"][data-chat-kind="person"]')
    const rowCount = await rows.count().catch(() => 0)
    record('聊天窗口列出会话', rowCount >= 1, `${rowCount} 行`); flush()
    if (rowCount < 1) {
      await page.screenshot({ path: `${ART}/02-no-roster.png` })
      return await finishBrowser(browser)
    }
    const characterId = await rows.first().getAttribute('data-chat-id').catch(() => null)
    await rows.first().click()
    await page.waitForTimeout(2500)

    const draftInput = page.getByPlaceholder(/^对 .+ 说/).first()
    const inputReady = await draftInput.isVisible().catch(() => false)
    record('聊天输入框可用', inputReady, inputReady ? '占位符匹配' : '未找到输入框'); flush()
    if (!inputReady) {
      await page.screenshot({ path: `${ART}/02-no-input.png` })
      return await finishBrowser(browser)
    }

    const draftText = `草稿暂存验收-${stamp}`
    await draftInput.fill(draftText)
    await page.waitForTimeout(600)
    record('草稿已填入', (await draftInput.inputValue().catch(() => '')) === draftText, draftText); flush()

    // 最小化：窗口不卸载，草稿必须还在。
    const minimize = page.getByRole('button', { name: '最小化' }).first()
    const minimizeVisible = await minimize.isVisible().catch(() => false)
    record('聊天窗口有最小化控件', minimizeVisible, minimizeVisible ? '找到按钮' : '未找到'); flush()
    if (minimizeVisible) {
      await minimize.click()
      await page.waitForTimeout(1500)
      const entry = page.locator('[data-testid="story-window-minimized-entry"]').first()
      const minimized = await entry.isVisible().catch(() => false)
      record('窗口已最小化', minimized, minimized ? '出现最小化入口' : '未出现入口'); flush()
      await page.screenshot({ path: `${ART}/02-minimized.png` })

      await entry.click()
      await page.waitForTimeout(1800)
      const restoredValue = await draftInput.inputValue().catch(() => '')
      record('最小化恢复后草稿仍在', restoredValue === draftText, `「${restoredValue}」`); flush()
      await page.screenshot({ path: `${ART}/03-restored-draft.png` })
    }

    // 现在真的发出去，并等回复。窗口里的消息条数由产品自己渲染，容易受
    // 分页影响，所以以服务端已提交的交谈条数为准。分支必须用当前分支，
    // main 上读不到这条故事线的交谈记录。
    const exchangesBefore = await exchangeCount(STORY, branchId, characterId)
    await page.locator('[data-testid="character-window-panel"] button[aria-label="发送"]').first().click()
    const exchangesAfter = await waitForExchangeCount(STORY, branchId, characterId, exchangesBefore + 2, 180000)
    record('草稿发送后收到回复', exchangesAfter >= exchangesBefore + 2,
      `${exchangesBefore} → ${exchangesAfter} 条（人物 ${characterId} 分支 ${branchId}）`); flush()
    await page.screenshot({ path: `${ART}/04-sent.png` })

    record('浏览器无未预期报错', consoleErrors.length === 0, consoleErrors.slice(0, 3).join(' | ') || '无'); flush()
  } finally {
    await finishBrowser(browser)
  }
}

async function exchangeCount(storyId, branchId, characterId) {
  const q = `?branch=${encodeURIComponent(branchId)}&character_id=${encodeURIComponent(characterId)}&limit=200`
  const body = (await get(`/api/interactive/stories/${storyId}/characters${q}`)).body
  return (body?.exchanges || []).length
}

async function waitForExchangeCount(storyId, branchId, characterId, want, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  let count = 0
  while (Date.now() < deadline) {
    count = await exchangeCount(storyId, branchId, characterId)
    if (count >= want) return count
    await new Promise((r) => setTimeout(r, 2500))
  }
  return count
}

async function findGameButton(page) {
  const candidates = [
    page.getByRole('button', { name: '游戏', exact: true }),
    page.locator('button', { hasText: /^游戏$/ }),
    page.locator('[data-mode="story"]'),
  ]
  for (const c of candidates) {
    if (await c.first().isVisible().catch(() => false)) return c.first()
  }
  return null
}

async function finishBrowser(browser) {
  await browser.close()
  finish()
}

function finish() {
  flush()
  console.log(`==== 真实链补充验收：${steps.length - failed}/${steps.length} 通过 ====`)
  for (const s of steps) console.log(`${s.ok ? 'PASS' : 'FAIL'}  ${s.name}${s.detail ? ' — ' + s.detail : ''}`)
  if (failed) process.exitCode = 1
}

main().catch((e) => { record('脚本异常', false, e.message); flush(); console.error(e); process.exitCode = 1 })
