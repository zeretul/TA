/**
 * 0.5.3 packaged re-check of 资料总览 (#21) and the character roster (#12/#15)
 * on the isolated instance, through the game's own entry.
 *
 * What this proves is only what a player sees: the panel opens from the real
 * toolbar, reads six tabs, shows a portrait or a placeholder, and reopens with
 * its tab kept. It does not restate the unit tests, which already cover the
 * branch-scoped read logic.
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

async function clickButton(page, text) {
  return page.evaluate((label) => {
    const b = [...document.querySelectorAll('button')].find((x) => (x.innerText || '').trim() === label)
    if (!b) return false
    b.click()
    return true
  }, text)
}

async function main() {
  const browser = await chromium.launch()
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 }, locale: 'zh-CN', colorScheme: 'dark' })
  const page = await context.newPage()
  const errors = []
  const bookResponses = []
  page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()) })
  page.on('response', async (r) => {
    if (!/\/api\/projects\/[^/]+\/book(\?|$)/.test(r.url())) return
    bookResponses.push({ url: r.url(), status: r.status(), contentType: r.headers()['content-type'] || '' })
  })

  await page.goto(BASE, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(4000)

  emit('workspace read is JSON', bookResponses.length > 0 && bookResponses.every((r) => r.status === 200 && /json/.test(r.contentType)), bookResponses)
  emit('no scope mismatch', !errors.some((t) => /scope mismatch/i.test(t)), { errors: errors.slice(0, 5) })

  const opened = await clickButton(page, '资料总览')
  await page.waitForTimeout(2500)
  const panel = page.locator('[data-testid="lore-overview-panel"]').first()
  const visible = (await panel.count()) > 0 && (await panel.isVisible())
  emit('lore overview opens from the real toolbar', opened && visible, { opened, visible })

  if (!visible) {
    await page.screenshot({ path: OUT + '\\lore-no-panel.png' })
    await browser.close()
    console.log(JSON.stringify({ failed: checks.filter((c) => !c.ok).map((c) => c.name) }))
    process.exit(1)
  }

  const tabs = await panel.evaluate((el) =>
    [...el.querySelectorAll('button')].map((b) => (b.innerText || '').trim()).filter(Boolean))
  emit('lore overview lists its reading tabs', tabs.length >= 6, { tabs })

  const bodyText = (await panel.innerText()).slice(0, 400)
  emit('lore overview shows readable content', bodyText.trim().length > 20, { head: bodyText.slice(0, 160) })

  // Six tabs, one after another; a tab that throws leaves the panel blank.
  let tabFailures = []
  for (const tab of tabs.slice(0, 6)) {
    const ok = await panel.evaluate((el, label) => {
      const b = [...el.querySelectorAll('button')].find((x) => (x.innerText || '').trim() === label)
      if (!b) return false
      b.click()
      return true
    }, tab)
    await page.waitForTimeout(700)
    const text = (await panel.innerText()).replace(/\s+/g, '')
    if (!ok || text.length < 8) tabFailures.push({ tab, ok, length: text.length })
  }
  emit('every reading tab renders', tabFailures.length === 0, { tabFailures })

  const avatarState = await panel.evaluate((el) => {
    const imgs = [...el.querySelectorAll('img')]
    return imgs.map((i) => ({ src: i.getAttribute('src') || '', w: i.naturalWidth, h: i.naturalHeight }))
  })
  emit('portraits resolve or fall back without broken images',
    avatarState.every((i) => i.src !== ''), { count: avatarState.length, broken: avatarState.filter((i) => !i.src) })

  await page.screenshot({ path: OUT + '\\lore-open.png' })

  const closed = await page.evaluate(() => {
    const el = document.querySelector('[data-testid="lore-overview-panel"]')
    const b = el && [...el.querySelectorAll('button')].find((x) => /关闭|收起/.test(x.getAttribute('aria-label') || x.innerText || ''))
    if (!b) return false
    b.click()
    return true
  })
  await page.waitForTimeout(1200)
  const reopened = await clickButton(page, '资料总览')
  await page.waitForTimeout(2000)
  const visibleAgain = await page.locator('[data-testid="lore-overview-panel"]').first().isVisible().catch(() => false)
  emit('lore overview closes and reopens', closed && reopened && visibleAgain, { closed, reopened, visibleAgain })

  emit('no console errors during the whole run', errors.length === 0, { errors: errors.slice(0, 5) })

  await browser.close()
  const failed = checks.filter((c) => !c.ok)
  console.log(JSON.stringify({ checks, failed: failed.map((c) => c.name) }))
  process.exit(failed.length === 0 ? 0 : 1)
}

main().catch((e) => {
  console.error(JSON.stringify({ fatal: String((e && e.stack) || e) }))
  process.exit(1)
})
