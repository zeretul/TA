/**
 * 0.5.3 packaged re-check, isolated instance.
 *
 * Drives the packaged game entry against an isolated data directory. It checks
 * the four things this round and the last one changed on the surfaces a player
 * can see:
 *   - the workspace read takes /book and returns JSON, with no scope mismatch
 *   - the character-chat composer stays inside a 375x860 viewport with long matters
 *   - a draft survives closing and reopening the window
 *   - the page itself does not scroll away from the composer
 *
 * No product behaviour is stubbed. The only thing inserted is equal-height rows
 * inside the scroll region, because the assertion is a CSS constraint and only a
 * deterministic height can pin it.
 */
import { chromium } from '@playwright/test'
import { devOutput } from '../../scripts/dev-output.mjs'

const BASE = process.env.TA_BASE || 'http://127.0.0.1:19814'
const OUT = process.env.TA_OUT || devOutput('verify-053/out')

const checks = []
function emit(name, ok, detail) {
  checks.push({ name, ok: Boolean(ok), detail })
  console.log(JSON.stringify({ name, ok: Boolean(ok), detail }))
}

const PANEL = '[data-testid="character-window-panel"]'

async function clickButton(page, text) {
  return page.evaluate((label) => {
    const b = [...document.querySelectorAll('button')].find((x) => (x.innerText || '').trim() === label)
    if (!b) return false
    b.click()
    return true
  }, text)
}

async function chatShown(page) {
  const p = page.locator(PANEL).first()
  if (!(await p.count())) return false
  if (!(await p.isVisible())) return false
  return (await page.locator(PANEL + ' textarea').count()) > 0
}

async function openChat(page) {
  if (await chatShown(page)) return true
  const p = page.locator(PANEL).first()
  if (!(await p.count()) || !(await p.isVisible())) {
    await clickButton(page, '人物')
    await page.waitForTimeout(1800)
  }
  if (await chatShown(page)) return true
  const row = page.locator('[data-testid="character-window-roster-row"]').first()
  if (!(await row.count())) return false
  await row.click()
  await page.waitForTimeout(2200)
  return chatShown(page)
}

async function runViewport(browser, width, height, theme) {
  const tag = width + 'x' + height + '-' + theme
  const context = await browser.newContext({ viewport: { width, height }, locale: 'zh-CN', colorScheme: theme })
  const page = await context.newPage()
  const consoleErrors = []
  const bookResponses = []
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()) })
  page.on('response', async (r) => {
    const url = r.url()
    if (!/\/api\/projects\/[^/]+\/book(\?|$)/.test(url)) return
    let head = ''
    try { head = (await r.text()).slice(0, 100) } catch { head = '<unreadable>' }
    bookResponses.push({ url, status: r.status(), contentType: r.headers()['content-type'] || '', head })
  })

  await page.goto(BASE, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(4000)

  const toolbar = await page.evaluate(() =>
    [...document.querySelectorAll('button')].map((b) => (b.innerText || '').trim()).filter(Boolean))
  const wanted = ['人物', '战斗', '群聊', '秘境', '见闻与事务', '资料总览']
  const missing = wanted.filter((n) => !toolbar.includes(n))
  emit(tag + ' game entry loads', missing.length === 0, { missing })

  emit(tag + ' workspace read used /book', bookResponses.length > 0, { count: bookResponses.length })
  emit(tag + ' workspace read returned JSON',
    bookResponses.length > 0 && bookResponses.every((r) => r.status === 200 && /json/.test(r.contentType)),
    bookResponses.slice(0, 2))
  emit(tag + ' no scope mismatch in console',
    !consoleErrors.some((t) => /scope mismatch/i.test(t)),
    { errors: consoleErrors.slice(0, 5) })

  if (!(await openChat(page))) {
    emit(tag + ' character chat reachable', false, { note: 'no roster row on this surface' })
    await page.screenshot({ path: OUT + '\\' + tag + '-no-panel.png' })
    await context.close()
    return
  }
  emit(tag + ' character chat reachable', true, {})

  const measure = () => page.evaluate((sel) => {
    const panel = document.querySelector(sel)
    if (!panel) return null
    const scroll = panel.querySelector('.min-h-0.flex-1.overflow-y-auto')
    const ta = [...panel.querySelectorAll('textarea')].pop()
    const send = panel.querySelector('button[aria-label="发送"]')
    const box = (el) => {
      if (!el) return null
      const r = el.getBoundingClientRect()
      return { y: Math.round(r.y), bottom: Math.round(r.bottom) }
    }
    return {
      scroll: scroll ? { clientH: scroll.clientHeight, scrollH: scroll.scrollHeight } : null,
      textarea: box(ta), send: box(send),
      docScrollH: document.documentElement.scrollHeight, viewportH: window.innerHeight,
    }
  }, PANEL)

  const empty = await measure()
  emit(tag + ' composer inside viewport with no matters',
    Boolean(empty.textarea) && empty.textarea.bottom <= height, empty.textarea)

  await page.evaluate((sel) => {
    const panel = document.querySelector(sel)
    const scroll = panel && panel.querySelector('.min-h-0.flex-1.overflow-y-auto')
    if (!scroll) return
    for (let i = 0; i < 40; i += 1) {
      const row = document.createElement('div')
      row.className = 'px-3 py-2 text-sm'
      row.textContent = '事项 ' + (i + 1) + '：' + '这一段刻意写得很长，用来把列表撑高。'.repeat(3)
      scroll.appendChild(row)
    }
  }, PANEL)
  await page.waitForTimeout(500)

  const stressed = await measure()
  emit(tag + ' long matters really overflow',
    Boolean(stressed.scroll) && stressed.scroll.scrollH > stressed.scroll.clientH + 200, stressed.scroll)
  emit(tag + ' input bottom inside ' + height,
    Boolean(stressed.textarea) && stressed.textarea.bottom <= height, stressed.textarea)
  emit(tag + ' send bottom inside ' + height,
    Boolean(stressed.send) && stressed.send.bottom <= height, stressed.send)
  emit(tag + ' send visible without scrolling',
    await page.locator(PANEL + ' button[aria-label="发送"]').first().isVisible(), {})
  emit(tag + ' page itself does not scroll', stressed.docScrollH <= height + 1,
    { docScrollH: stressed.docScrollH, height })

  const input = page.locator(PANEL + ' textarea').last()
  await input.fill('这是一段很长的草稿，用来验证换行后的输入区仍然把发送按钮留在屏幕内。'.repeat(6))
  await page.waitForTimeout(400)
  const longDraft = await measure()
  emit(tag + ' long draft keeps send inside ' + height,
    Boolean(longDraft.send) && longDraft.send.bottom <= height, longDraft.send)

  const draftText = '草稿保留检查 ' + tag
  await input.fill(draftText)
  await page.waitForTimeout(300)
  await page.evaluate((sel) => {
    const close = document.querySelector(sel + ' button[aria-label="关闭人物窗口"]')
    if (close) close.click()
  }, PANEL)
  await page.waitForTimeout(1400)
  const reopened = await openChat(page)
  const value = reopened ? await page.locator(PANEL + ' textarea').last().inputValue().catch(() => '') : ''
  emit(tag + ' draft survives close and reopen', reopened && value === draftText, { value, reopened })

  const after = await measure()
  emit(tag + ' panel reopens with its scroll region', Boolean(after.scroll), after.scroll)

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

main().catch((e) => {
  console.error(JSON.stringify({ fatal: String((e && e.stack) || e) }))
  process.exit(1)
})
