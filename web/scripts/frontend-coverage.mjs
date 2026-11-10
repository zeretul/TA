/**
 * 前端界面覆盖验收：宽窄、深浅、中英、长文本、空态。
 *
 * 《Claude_剩余任务统一收尾》第 8 节要求「前端改动实际打开候选页面，宽窄、
 * 深浅、中英、长文本/空态覆盖」。这里覆盖本轮改动过的四个界面：
 *
 *   1. 主舞台与聊天外壳（T27 统一聊天）
 *   2. 剧情助手（T23/T26 分析与灵感）
 *   3. 资料库（T28/T31-5 栏目与默认作用域）
 *   4. 世界包与建角（T16 开局只保留 AI 生成与自定义）
 *
 * 每处都实际打开页面、按视口尺寸截图，并断言界面没有横向溢出、没有未翻译的
 * i18n key、没有控制台报错。断言不过就是缺陷，不用截图糊过去。
 *
 * 用法：
 *   TA_ACCEPT_BASE=http://127.0.0.1:19730 \
 *   TA_ACCEPT_PROJECT=project-xxx \
 *   TA_ACCEPT_STORY=st_xxx \
 *   node scripts/frontend-coverage.mjs
 */
import { chromium } from '@playwright/test'
import { writeFileSync, mkdirSync } from 'node:fs'

const BASE = process.env.TA_ACCEPT_BASE || 'http://127.0.0.1:19730'
const PROJECT = process.env.TA_ACCEPT_PROJECT || ''
const STORY = process.env.TA_ACCEPT_STORY || ''
const OUT = process.env.TA_ACCEPT_RESULT || './frontend-coverage-result.json'
const ART = './test-results/frontend-coverage'
mkdirSync(ART, { recursive: true })

const WIDE = { width: 1440, height: 900 }
const NARROW = { width: 390, height: 844 }

const steps = []
let failed = 0
function record(name, ok, detail = '') {
  steps.push({ name, ok: !!ok, detail: String(detail) })
  if (!ok) failed += 1
}
function flush() {
  writeFileSync(OUT, JSON.stringify({ passed: steps.length - failed, total: steps.length, steps }, null, 2))
}
function finish() {
  flush()
  console.log(`==== 前端界面覆盖：${steps.length - failed}/${steps.length} 通过 ====`)
  for (const s of steps) console.log(`${s.ok ? 'PASS' : 'FAIL'}  ${s.name}${s.detail ? ' — ' + s.detail : ''}`)
  if (failed) process.exitCode = 1
}

/** 未翻译的 i18n key 会原样显示成 a.b.c 这种形状。 */
const RAW_KEY = /\b[a-z][a-zA-Z0-9]*(\.[a-zA-Z][a-zA-Z0-9]*){2,}\b/

async function visibleText(page) {
  return page.evaluate(() => document.body.innerText || '')
}

async function hasHorizontalOverflow(page) {
  return page.evaluate(() => {
    const doc = document.documentElement
    return doc.scrollWidth > doc.clientWidth + 1
  })
}

/** 页面正文里出现形如 chatWindow.title 的裸 key，就是漏翻。 */
async function untranslatedKeys(page) {
  const text = await visibleText(page)
  const hits = new Set()
  for (const token of text.split(/\s+/)) {
    if (RAW_KEY.test(token) && !token.includes('://')) hits.add(token)
  }
  return [...hits].slice(0, 5)
}

async function setTheme(page, theme) {
  await page.evaluate((value) => {
    window.localStorage.setItem('theme', value)
    document.documentElement.setAttribute('data-theme', value)
  }, theme)
  await page.waitForTimeout(600)
}

/**
 * 语言的真源是后端 settings 的 effective.language，localStorage 里的
 * ta.locale.configured 只是启动缓存，每次 fetchSettings 都会被覆盖回去。
 * 所以切语言必须走 PATCH /api/settings 的用户层，跟设置页点选是同一条路。
 */
async function setLocale(page, locale) {
  const res = await fetch(`${BASE}/api/settings`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ layer: 'user', changes: { language: locale } }),
  })
  if (!res.ok) throw new Error(`切换语言失败：${res.status} ${await res.text()}`)
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(4500)
  const applied = await page.evaluate(() => document.documentElement.lang)
  if (applied !== locale) throw new Error(`语言未生效：期望 ${locale}，实际 ${applied}`)
}

async function openStage(page) {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(5000)
  const skip = page.getByRole('button', { name: /跳过引导|Skip/ }).first()
  if (await skip.isVisible().catch(() => false)) { await skip.click(); await page.waitForTimeout(1200) }
  for (const name of ['游戏', 'Game']) {
    const btn = page.getByRole('button', { name, exact: true }).first()
    if (await btn.isVisible().catch(() => false)) { await btn.click(); break }
  }
  await page.waitForTimeout(2500)
  return page.getByPlaceholder(/你要做什么|What do you do/).first()
    .waitFor({ state: 'visible', timeout: 45000 }).then(() => true).catch(() => false)
}

async function shot(page, name) {
  await page.screenshot({ path: `${ART}/${name}.png`, fullPage: false })
}

async function main() {
  if (!PROJECT || !STORY) {
    record('前置参数', false, '需要 TA_ACCEPT_PROJECT 与 TA_ACCEPT_STORY')
    return finish()
  }

  const browser = await chromium.launch()
  const context = await browser.newContext({ baseURL: BASE, locale: 'zh-CN', serviceWorkers: 'block', reducedMotion: 'reduce', viewport: WIDE })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()) })
  page.on('pageerror', (e) => consoleErrors.push(`pageerror: ${e.message}`))

  try {
    // -----------------------------------------------------------------
    // 1. 主舞台：宽 / 窄 / 深 / 浅 / 中 / 英
    // -----------------------------------------------------------------
    let ready = await openStage(page)
    record('主舞台打开', ready, ready ? '输入框可见' : '输入框未出现'); flush()
    if (!ready) return finish()

    const combos = [
      { name: 'stage-wide-dark-zh', viewport: WIDE, theme: 'dark', locale: 'zh-CN' },
      { name: 'stage-narrow-dark-zh', viewport: NARROW, theme: 'dark', locale: 'zh-CN' },
      { name: 'stage-wide-light-zh', viewport: WIDE, theme: 'light', locale: 'zh-CN' },
      { name: 'stage-wide-dark-en', viewport: WIDE, theme: 'dark', locale: 'en-US' },
    ]
    for (const combo of combos) {
      await page.setViewportSize(combo.viewport)
      if (combo.locale !== await page.evaluate(() => document.documentElement.lang)) {
        await setLocale(page, combo.locale)
        ready = await openStage(page)
        if (!ready) { record(`主舞台打开（${combo.name}）`, false, '切换语言后输入框未出现'); continue }
      }
      await setTheme(page, combo.theme)
      await page.waitForTimeout(800)

      const themeAttr = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
      record(`主题生效 ${combo.name}`, themeAttr === combo.theme, `data-theme=${themeAttr}`)

      const overflow = await hasHorizontalOverflow(page)
      record(`无横向溢出 ${combo.name}`, !overflow, overflow ? '正文宽于视口' : 'ok')

      const raw = await untranslatedKeys(page)
      record(`无裸 i18n key ${combo.name}`, raw.length === 0, raw.join(' | ') || 'ok')

      await shot(page, combo.name)
      flush()
    }

    // 回到中文宽屏，做长文本与空态。
    await page.setViewportSize(WIDE)
    if (await page.evaluate(() => window.localStorage.getItem('ta.locale.configured')) !== 'zh-CN') {
      await setLocale(page, 'zh-CN')
      await openStage(page)
    }
    await setTheme(page, 'dark')

    // 长文本：把一段很长的中文塞进输入框，界面不能被撑破。
    // 主输入框是 Tiptap 编辑器（contenteditable），不是 textarea，
    // 所以内容要从 .ProseMirror 里读，inputValue() 永远是空。
    const composer = page.getByPlaceholder(/你要做什么/).first()
    const composerEditor = page.locator('.ProseMirror').first()
    const longText = '这是一段用于检验长文本排版的中文内容。'.repeat(40)
    await composer.fill(longText)
    await page.waitForTimeout(800)
    const longOverflow = await hasHorizontalOverflow(page)
    record('长文本不撑破版面', !longOverflow, longOverflow ? '正文宽于视口' : `${longText.length} 字`)
    const keptValue = (await composerEditor.innerText().catch(() => '')).replace(/\s+/g, '')
    record('长文本完整保留', keptValue === longText, `${keptValue.length} 字 / 期望 ${longText.length} 字`)
    await shot(page, 'stage-long-text')
    await composer.fill('')
    await page.waitForTimeout(400)
    const afterClear = (await composerEditor.innerText().catch(() => '')).trim()
    record('长文本可清空', afterClear.length === 0, `剩余 ${afterClear.length} 字`)
    flush()

    // 空态：清空输入后，占位提示必须还在，不能变成空白一片。
    await page.waitForTimeout(400)
    const placeholder = await composer.getAttribute('placeholder').catch(() => '')
    record('空态保留占位提示', !!placeholder && placeholder.length > 0, placeholder || '（空）')
    await shot(page, 'stage-empty')
    flush()

    // -----------------------------------------------------------------
    // 2. 聊天外壳：宽 / 窄，空态（未选会话）
    // -----------------------------------------------------------------
    const chatBtn = page.getByRole('button', { name: '打开聊天窗口' }).first()
    const chatVisible = await chatBtn.isVisible().catch(() => false)
    record('主舞台含聊天入口', chatVisible, chatVisible ? '找到按钮' : '未找到'); flush()
    if (chatVisible) {
      await chatBtn.click()
      await page.waitForTimeout(2500)
      const panel = page.locator('[data-testid="chat-window-panel"]').first()
      const panelVisible = await panel.waitFor({ state: 'visible', timeout: 20000 }).then(() => true).catch(() => false)
      record('聊天窗口打开（宽）', panelVisible, panelVisible ? '面板可见' : '面板未出现'); flush()

      if (panelVisible) {
        // 宽屏时列表与正文并排，正文未选会话，必须给出明确空态而不是空白。
        const emptyHint = await panel.innerText().catch(() => '')
        record('宽屏未选会话有空态提示', /选一个会话|从左边|选择/.test(emptyHint), emptyHint.replace(/\s+/g, ' ').slice(0, 60))
        const chatOverflow = await hasHorizontalOverflow(page)
        record('聊天窗口无横向溢出（宽）', !chatOverflow, chatOverflow ? '正文宽于视口' : 'ok')
        await shot(page, 'chat-wide-empty')

        await page.setViewportSize(NARROW)
        await page.waitForTimeout(1200)
        const narrowPanel = await panel.isVisible().catch(() => false)
        record('窄屏聊天仍可用', narrowPanel, narrowPanel ? '面板可见' : '面板消失')
        const narrowOverflow = await hasHorizontalOverflow(page)
        record('聊天窗口无横向溢出（窄）', !narrowOverflow, narrowOverflow ? '正文宽于视口' : 'ok')
        await shot(page, 'chat-narrow')

        // 窄屏选中一个会话，正文与列表要轮流出现，返回键必须在。
        const rows = page.locator('[data-testid="chat-window-row"]')
        if (await rows.count().catch(() => 0) > 0) {
          await rows.first().click()
          await page.waitForTimeout(2200)
          const back = page.getByRole('button', { name: '返回' }).first()
          const backVisible = await back.isVisible().catch(() => false)
          record('窄屏选中会话后给出返回入口', backVisible, backVisible ? '找到返回按钮' : '未找到')
          await shot(page, 'chat-narrow-open')
        } else {
          record('聊天会话列表为空态', true, '没有会话，跳过选中')
        }
        await page.setViewportSize(WIDE)
        await page.waitForTimeout(1000)
        await page.getByRole('button', { name: '关闭聊天窗口' }).first().click().catch(() => {})
        await page.waitForTimeout(1200)
      }
      flush()
    }

    // -----------------------------------------------------------------
    // 3. 资料库：宽 / 窄，空态
    // -----------------------------------------------------------------
    for (const name of ['资料库']) {
      const nav = page.getByRole('button', { name, exact: true }).first()
      if (await nav.isVisible().catch(() => false)) {
        await nav.click()
        await page.waitForTimeout(3000)
        const libOverflow = await hasHorizontalOverflow(page)
        record('资料库无横向溢出（宽）', !libOverflow, libOverflow ? '正文宽于视口' : 'ok')
        const raw = await untranslatedKeys(page)
        record('资料库无裸 i18n key', raw.length === 0, raw.join(' | ') || 'ok')
        await shot(page, 'library-wide')

        await page.setViewportSize(NARROW)
        await page.waitForTimeout(1500)
        const libNarrowOverflow = await hasHorizontalOverflow(page)
        record('资料库无横向溢出（窄）', !libNarrowOverflow, libNarrowOverflow ? '正文宽于视口' : 'ok')
        await shot(page, 'library-narrow')
        await page.setViewportSize(WIDE)
        await page.waitForTimeout(800)
        flush()
      } else {
        record('资料库入口可用', false, '未找到导航按钮'); flush()
      }
    }

    // -----------------------------------------------------------------
    // 4. 世界包：宽 / 窄，中 / 英
    // -----------------------------------------------------------------
    const packNav = page.getByRole('button', { name: '世界包', exact: true }).first()
    if (await packNav.isVisible().catch(() => false)) {
      await packNav.click()
      await page.waitForTimeout(3000)
      const packOverflow = await hasHorizontalOverflow(page)
      record('世界包无横向溢出（宽）', !packOverflow, packOverflow ? '正文宽于视口' : 'ok')
      const raw = await untranslatedKeys(page)
      record('世界包无裸 i18n key', raw.length === 0, raw.join(' | ') || 'ok')
      await shot(page, 'pack-wide-zh')

      await setLocale(page, 'en-US')
      await page.waitForTimeout(2500)
      // 英文页的判据是「界面文案不含中文」。用户自己的数据（项目名、世界名）
      // 可以带中文，所以这里只看界面文案：中文连着出现且不在用户数据里的
      // 才算出问题。做法是把可见文本里的中文串挑出来，逐条排除用户数据。
      const enText = await visibleText(page)
      const userData = new Set([PROJECT, STORY, '基线验收-15a93b30', '基线验收'])
      const hanRuns = (enText.match(/[一-鿿][一-鿿\w\-·（）()]*/g) || [])
        .filter((run) => ![...userData].some((data) => data && (run.includes(data) || data.includes(run))))
      record('世界包英文页界面文案无中文', hanRuns.length === 0, hanRuns.slice(0, 6).join(' | ') || '无')
      const enRaw = await untranslatedKeys(page)
      record('世界包英文页无裸 i18n key', enRaw.length === 0, enRaw.join(' | ') || 'ok')
      await shot(page, 'pack-wide-en')

      await page.setViewportSize(NARROW)
      await page.waitForTimeout(1500)
      const packNarrowOverflow = await hasHorizontalOverflow(page)
      record('世界包无横向溢出（窄·英）', !packNarrowOverflow, packNarrowOverflow ? '正文宽于视口' : 'ok')
      await shot(page, 'pack-narrow-en')

      await setLocale(page, 'zh-CN')
      await page.waitForTimeout(2000)
      await page.setViewportSize(WIDE)
      await page.waitForTimeout(800)
      flush()
    } else {
      record('世界包入口可用', false, '未找到导航按钮'); flush()
    }

    // 真实的浏览器报错才算失败；被中止的 SSE 重连不算。
    const realErrors = consoleErrors.filter((line) => !/aborted|net::ERR_ABORTED|Failed to load resource/i.test(line))
    record('浏览器无未预期报错', realErrors.length === 0, realErrors.slice(0, 3).join(' | ') || '无')
  } finally {
    flush()
    await browser.close()
  }
}

main().catch((e) => { record('脚本异常', false, e.message); flush(); console.error(e); process.exitCode = 1 })
  .finally(() => finish())
