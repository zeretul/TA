/**
 * W01–W06 wording cleanup, checked on the real page against the current source.
 *
 * The dev server proxies /api to the isolated 19815 backend, which is a clean
 * copy of a real 凡人人界 story. Each check states what a player sees, not what
 * a grep counted: a heading, a rendered label, a value that must not move.
 */
import { chromium } from '@playwright/test'
import { devOutput } from '../../scripts/dev-output.mjs'

const BASE = process.env.TA_BASE || 'http://127.0.0.1:19733'
const OUT = process.env.TA_OUT || devOutput('verify-w07')
const fs = await import('node:fs')
fs.mkdirSync(OUT, { recursive: true })

const checks = []
function emit(name, ok, detail) {
  checks.push({ name, ok: Boolean(ok), detail })
  console.log(JSON.stringify({ name, ok: Boolean(ok), detail }))
}

const PANEL = '[data-testid="character-window-panel"]'

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

async function clickButton(page, text) {
  return page.evaluate((label) => {
    const b = [...document.querySelectorAll('button')].find((x) => (x.innerText || '').trim() === label)
    if (!b) return false
    b.click()
    return true
  }, text)
}

async function bodyText(page) {
  return page.evaluate(() => document.body.innerText)
}

async function main() {
  const browser = await chromium.launch()
  const context = await browser.newContext({ viewport: { width: 1280, height: 860 }, locale: 'zh-CN', colorScheme: 'dark' })
  const page = await context.newPage()
  const errors = []
  page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()) })
  page.on('pageerror', (e) => errors.push(String(e)))

  await page.goto(BASE, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(5000)

  let text = await bodyText(page)
  emit('game shell loads', text.length > 200, { length: text.length })
  await page.screenshot({ path: OUT + '\\01-load.png' })

  // W06: 资料总览 shelves. The world entries must be filed under 世界观 / 规则,
  // not under 其他 beside the protagonist templates.
  const openedLore = await clickButton(page, '资料总览')
  await page.waitForTimeout(2500)
  const lorePanel = page.locator('[data-testid="lore-overview-panel"]').first()
  const loreVisible = (await lorePanel.count()) > 0 && (await lorePanel.isVisible())
  emit('lore overview opens', openedLore && loreVisible, { openedLore, loreVisible })
  if (loreVisible) {
    const loreText = await lorePanel.innerText()
    const tabs = await page.evaluate(() => [...document.querySelectorAll('[data-testid="lore-overview-panel"] [role="tab"], [data-testid="lore-overview-panel"] button')].map((b) => (b.innerText || '').trim()).filter(Boolean))
    emit('lore tabs render', tabs.length > 0, { tabs: tabs.slice(0, 12) })
    emit('lore shows no raw setting type', !/setting/i.test(loreText), { sample: loreText.slice(0, 300) })
    emit('lore shows no authoring numbers', !/N8-|NAR-|br_|fr-/.test(loreText), { sample: loreText.slice(0, 300) })
    await page.screenshot({ path: OUT + '\\02-lore.png' })
  }

  // close whatever opened
  await page.keyboard.press('Escape')
  await page.waitForTimeout(600)

  // W02: the state ledger. Open it and read the rendered rows.
  const ledgerOpened = await page.evaluate(() => {
    const b = [...document.querySelectorAll('button')].find((x) => /状态|详情/.test((x.innerText || '').trim()))
    if (!b) return false
    b.click()
    return true
  })
  await page.waitForTimeout(2000)
  text = await bodyText(page)
  emit('state surface reachable', (await page.locator('[data-ledger-field]:visible').count()) > 0, { ledgerOpened })
  emit('no definition id in the page', !/定义[:：]|版本[:：]\s*1\b/.test(text), { hit: (text.match(/定义[:：]|版本[:：]\s*1\b/g) || []).slice(0, 5) })
  emit('no raw json in the page', !/\{"[^"]+":/.test(text), { hit: (text.match(/\{"[^"]+":/g) || []).slice(0, 3) })
  await page.screenshot({ path: OUT + '\\03-state.png', fullPage: true })

  // W01: the character's aptitude line reads as prose, and the tier word is
  // still there (the cultivation rules read it back).
  emit('aptitude reads as a sentence', /资质(较差|普通|良好|优秀|极佳)/.test(text), { sample: (text.match(/[^\n]*资质[^\n]*/g) || []).slice(0, 3) })
  emit('no rule disclaimer in aptitude', !/不代表已学会|只是经历与机会的倾向/.test(text), {})
  emit('no doubled 一般悟性', !/一般悟性一般/.test(text), {})

  // W03/W04: the engineering-tone strings the audit named.
  const banned = [
    '由同一套规则结算',
    '此处不会替你行动',
    '这个提交身份已经被另一份内容占用',
    '编辑 AI 回复',
    'AI 回复正文',
  ]
  for (const phrase of banned) {
    emit(`page does not say: ${phrase}`, !text.includes(phrase), {})
  }

  // W04: thinking level must be worded, never echoed as high/medium/off.
  emit('no raw thinking level', !/(思考(?:强度)?|thinking)[^\n]{0,12}\b(high|medium|low|off)\b/i.test(text), { hit: (text.match(/[^\n]{0,20}\b(high|medium|off)\b[^\n]{0,20}/gi) || []).slice(0, 4) })

  // W06: no br_ branch id drawn on the page.
  emit('no branch id on the page', !/br_[0-9a-z]/.test(text), { hit: (text.match(/br_[0-9a-z]+/g) || []).slice(0, 3) })

  // Narrow screen: the character-chat composer stays on screen. The send
  // control is an icon button with an aria-label, so it is found by that label
  // rather than by its text — and the panel has to be opened the way a player
  // opens it: the 人物 tab, then a roster row.
  await page.setViewportSize({ width: 375, height: 860 })
  await page.waitForTimeout(1500)
  const openedChat = await openChat(page)
  emit('character window opens at 375x860', openedChat, { openedChat })
  const sendOnScreen = await page.evaluate((sel) => {
    const panel = document.querySelector(sel)
    const send = panel && panel.querySelector('button[aria-label="发送"]')
    if (!send) return { found: false }
    const r = send.getBoundingClientRect()
    return {
      found: true,
      top: Math.round(r.top), bottom: Math.round(r.bottom),
      left: Math.round(r.left), right: Math.round(r.right),
      inView: r.top >= 0 && r.bottom <= window.innerHeight && r.left >= 0 && r.right <= window.innerWidth,
    }
  }, PANEL)
  emit('send button found at 375x860', sendOnScreen.found, sendOnScreen)
  emit('send button stays on a 375x860 screen', sendOnScreen.found && sendOnScreen.inView, sendOnScreen)

  // A long draft must not push the composer off the bottom either.
  if (openedChat) {
    const input = page.locator(PANEL + ' textarea').last()
    await input.fill('这是一段很长的草稿，用来验证换行后的输入区仍然把发送按钮留在屏幕内。'.repeat(6))
    await page.waitForTimeout(400)
    const longDraft = await page.evaluate((sel) => {
      const panel = document.querySelector(sel)
      const send = panel && panel.querySelector('button[aria-label="发送"]')
      if (!send) return { found: false }
      const r = send.getBoundingClientRect()
      return { found: true, bottom: Math.round(r.bottom), inView: r.bottom <= window.innerHeight }
    }, PANEL)
    emit('long draft keeps send on a 375x860 screen', longDraft.found && longDraft.inView, longDraft)
  }
  await page.screenshot({ path: OUT + '\\04-narrow.png' })

  // Light theme, wide screen: the same wording must hold when nothing is
  // cramped. The colour scheme is emulated rather than left to the OS so the
  // run is the same on any machine.
  await page.emulateMedia({ colorScheme: 'light' })
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.waitForTimeout(1500)
  const lightText = await bodyText(page)
  emit('light theme still renders the game', lightText.length > 200, { length: lightText.length })
  emit('light theme keeps the engineering tone out', !/由同一套规则结算|此处不会替你行动|编辑 AI 回复|AI 回复正文/.test(lightText), {})
  emit('light theme keeps definition ids out', !/定义[:：]|版本[:：]\s*1\b/.test(lightText), {})
  emit('light theme keeps the branch id out', !/br_[0-9a-z]/.test(lightText), {})
  emit('light theme keeps raw thinking level out', !/(思考(?:强度)?|thinking)[^\n]{0,12}\b(high|medium|low|off)\b/i.test(lightText), { hit: (lightText.match(/[^\n]{0,20}\b(high|medium|off)\b[^\n]{0,20}/gi) || []).slice(0, 4) })
  await page.screenshot({ path: OUT + '\\05-light-wide.png' })

  emit('no page errors', errors.filter((e) => !/favicon|ResizeObserver/i.test(e)).length === 0, { errors: errors.slice(0, 6) })

  await browser.close()
  const failed = checks.filter((c) => !c.ok)
  console.log(JSON.stringify({ total: checks.length, failed: failed.map((c) => c.name) }))
  fs.writeFileSync(OUT + '\\w07-result.json', JSON.stringify(checks, null, 2))
  process.exit(failed.length === 0 ? 0 : 1)
}

main().catch((e) => { console.error(e); process.exit(2) })
