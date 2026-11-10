/**
 * A3 packaged verification: the character chat panel must keep its input and
 * send bar inside the viewport at 375x860 when matters and history are long.
 *
 * Runs against the isolated packaged instance (game build, real entry) and
 * exits non-zero on any failure, so the shell reports the real exit code.
 *
 * The panel's own data comes from the packaged instance's story. The content
 * that makes the scroll region tall is inserted into that region as equal-height
 * rows: the assertion under test is a CSS constraint (the composer must sit
 * outside the scrolling remainder), and only a deterministic height can pin it.
 * No product behaviour is stubbed.
 */
import { chromium } from '@playwright/test'
import { devOutput } from '../../scripts/dev-output.mjs'

const BASE = process.env.TA_BASE || 'http://127.0.0.1:19812'
const OUT = process.env.TA_OUT || devOutput('verify-052b/a3')

const checks = []
function emit(name, ok, detail) {
  checks.push({ name, ok: Boolean(ok), detail })
  console.log(JSON.stringify({ name, ok: Boolean(ok), detail }))
}

const PANEL = '[data-testid="character-window-panel"]'
const ROSTER_ROW = '[data-testid="character-window-roster-row"]'

/** The window's markup can stay mounted while it is closed, so ask for what is shown. */
async function panelShown(page) {
  const panel = page.locator(PANEL).first()
  if (!(await panel.count())) return false
  return panel.isVisible()
}

/** The window shows either the cast list or one conversation; the composer marks the latter. */
async function chatShown(page) {
  if (!(await panelShown(page))) return false
  return (await page.locator(PANEL + ' textarea').count()) > 0
}

async function openCharacterChat(page) {
  if (await chatShown(page)) return true
  if (!(await panelShown(page))) {
    await page.evaluate(() => {
      const entry = [...document.querySelectorAll('button')].find((b) => (b.innerText || '').trim() === '人物')
      if (entry) entry.click()
    })
    await page.waitForTimeout(1500)
  }
  if (await chatShown(page)) return true
  const row = page.locator(ROSTER_ROW).first()
  if (!(await row.count())) return false
  await row.click()
  await page.waitForTimeout(2000)
  return chatShown(page)
}

async function measure(page, label) {
  const value = await page.evaluate((panelSel) => {
    const panel = document.querySelector(panelSel)
    if (!panel) return null
    const scroll = panel.querySelector('.min-h-0.flex-1.overflow-y-auto')
    const composer = panel.querySelector('textarea')
    const textarea = [...panel.querySelectorAll('textarea')].pop()
    const send = panel.querySelector('button[aria-label="发送"]')
    const box = (el) => {
      if (!el) return null
      const r = el.getBoundingClientRect()
      return { y: Math.round(r.y), bottom: Math.round(r.bottom), h: Math.round(r.height) }
    }
    const pr = panel.getBoundingClientRect()
    return {
      panel: { y: Math.round(pr.y), bottom: Math.round(pr.bottom), h: Math.round(pr.height) },
      scroll: scroll
        ? { y: Math.round(scroll.getBoundingClientRect().y), bottom: Math.round(scroll.getBoundingClientRect().bottom),
            clientH: scroll.clientHeight, scrollH: scroll.scrollHeight }
        : null,
      textarea: box(textarea),
      send: box(send),
      hasComposer: Boolean(composer),
      viewportH: window.innerHeight,
      docScrollH: document.documentElement.scrollHeight,
    }
  }, PANEL)
  return { label, ...value }
}

async function stressScroll(page) {
  await page.evaluate((panelSel) => {
    const panel = document.querySelector(panelSel)
    const scroll = panel && panel.querySelector('.min-h-0.flex-1.overflow-y-auto')
    if (!scroll) return
    for (let i = 0; i < 40; i += 1) {
      const row = document.createElement('div')
      row.className = 'px-3 py-2 text-sm'
      row.textContent = '事项 ' + (i + 1) + '：' + '这一段刻意写得很长，用来把列表撑高。'.repeat(3)
      scroll.appendChild(row)
    }
  }, PANEL)
  await page.waitForTimeout(400)
}

async function runViewport(browser, width, height, theme) {
  const tag = width + 'x' + height + '-' + theme
  const context = await browser.newContext({
    viewport: { width, height },
    locale: 'zh-CN',
    colorScheme: theme,
  })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(msg.text())
  })

  await page.goto(BASE, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(3000)

  // The packaged build ships the game shell: its toolbar carries these entries.
  const toolbar = await page.evaluate(() =>
    [...document.querySelectorAll('button')].map((b) => (b.innerText || '').trim()).filter(Boolean))
  const wanted = ['人物', '战斗', '群聊', '秘境', '见闻与事务', '资料总览']
  const missing = wanted.filter((name) => !toolbar.includes(name))
  emit(tag + ' game entry loads', missing.length === 0, { missing, sample: toolbar.slice(0, 10) })
  emit(tag + ' no scope mismatch on load',
    !consoleErrors.some((t) => /scope mismatch/i.test(t)),
    { errors: consoleErrors.slice(0, 5) })

  if (!(await openCharacterChat(page))) {
    emit(tag + ' character chat reachable', false, { note: 'no roster row or panel on this surface' })
    await page.screenshot({ path: OUT + '\\' + tag + '-no-panel.png' })
    await context.close()
    return
  }
  emit(tag + ' character chat reachable', true, {})

  // Empty data first: the composer must already sit inside the viewport.
  const empty = await measure(page, 'empty')
  emit(tag + ' composer inside viewport with no matters', Boolean(empty.textarea) && empty.textarea.bottom <= height,
    empty)

  await stressScroll(page)
  const stressed = await measure(page, 'stressed')
  emit(tag + ' long matters really overflow the scroll region',
    Boolean(stressed.scroll) && stressed.scroll.scrollH > stressed.scroll.clientH + 200,
    stressed.scroll)
  emit(tag + ' input bottom stays inside ' + height,
    Boolean(stressed.textarea) && stressed.textarea.bottom <= height, stressed.textarea)
  emit(tag + ' send button bottom stays inside ' + height,
    Boolean(stressed.send) && stressed.send.bottom <= height, stressed.send)
  emit(tag + ' send button is visible without scrolling',
    await page.locator(PANEL + ' button[aria-label="发送"]').first().isVisible(), {})
  emit(tag + ' page itself does not scroll',
    stressed.docScrollH <= height + 1, { docScrollH: stressed.docScrollH, height })

  // A long draft must not push the send button out either.
  const input = page.locator(PANEL + ' textarea').last()
  await input.fill('这是一段很长的草稿，用来验证换行后的输入区仍然把发送按钮留在屏幕内。'.repeat(6))
  await page.waitForTimeout(400)
  const longDraft = await measure(page, 'long-draft')
  emit(tag + ' long draft keeps the send button inside ' + height,
    Boolean(longDraft.send) && longDraft.send.bottom <= height, longDraft.send)

  // Draft survives closing and reopening the window.
  const draftText = '草稿保留检查 ' + tag
  await input.fill(draftText)
  await page.waitForTimeout(300)
  await page.evaluate((panelSel) => {
    const close = document.querySelector(panelSel + ' button[aria-label="关闭人物窗口"]')
    if (close) close.click()
  }, PANEL)
  await page.waitForTimeout(1200)
  if (await openCharacterChat(page)) {
    const value = await page.locator(PANEL + ' textarea').last().inputValue().catch(() => '')
    emit(tag + ' draft survives close and reopen', value === draftText, { value })
  } else {
    emit(tag + ' draft survives close and reopen', false, { note: 'panel did not reopen' })
  }

  // History is still there after the reopen: the composer is not the only thing kept.
  const reopened = await measure(page, 'reopened')
  emit(tag + ' panel reopens with its scroll region', Boolean(reopened.scroll), reopened.scroll)

  await page.screenshot({ path: OUT + '\\' + tag + '.png' })
  await context.close()
}

async function main() {
  const browser = await chromium.launch()
  await runViewport(browser, 375, 860, 'dark')
  await runViewport(browser, 375, 860, 'light')
  await runViewport(browser, 1280, 800, 'dark')
  await browser.close()
  const failed = checks.filter((c) => !c.ok)
  console.log(JSON.stringify({ checks, failed: failed.map((c) => c.name) }))
  process.exit(failed.length === 0 ? 0 : 1)
}

main().catch((error) => {
  console.error(JSON.stringify({ fatal: String((error && error.stack) || error) }))
  process.exit(1)
})
