/**
 * The English creation page, end to end (real backend, real browser).
 *
 * Two halves, both of which a Chinese page would pass by accident:
 *   1. the form and the preview read as English, and the root hint is singular
 *      when one attribute is wanted;
 *   2. a refusal the pack itself writes is read in English too — the sentence
 *      comes from the pack's message table, not from the page.
 *
 * Run with the backend already serving on TA_CREATION_BASE and the page's own
 * language set to English.
 */
import { chromium } from '@playwright/test'
import { mkdirSync } from 'node:fs'

const BASE = process.env.TA_CREATION_BASE || 'http://127.0.0.1:19730'
const SHOTS = process.env.TA_CREATION_SHOTS || './test-results/creation-en'
mkdirSync(SHOTS, { recursive: true })

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok: !!ok, detail: String(detail) })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  ' + detail : ''}`)
}

const hanPattern = /[一-鿿]/

// The refusal is asked of the API directly, because the page's own input limits
// keep every rejected combination from ever reaching the server. This is the
// half a Chinese page cannot prove: the sentence is written by the pack.
const draftBody = JSON.stringify({
  choice: {
    name: '',
    gender: 'female',
    age: 20,
    origin_id: 'sect',
    affiliation_id: 'yue.huangfeng',
    location_id: 'yue.huangfeng.approach',
    root_category_id: 'waste',
    aptitude_id: 'normal',
  },
  rewrite: false,
})
for (const [locale, want, mustNot] of [
  ['en-US', 'Enter a name.', hanPattern],
  ['zh-CN', '请填写姓名', null],
]) {
  const response = await fetch(`${BASE}/api/story-packs/fanren-jie/identity-draft`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-TA-Locale': locale },
    body: draftBody,
  })
  const payload = await response.json()
  const text = payload.error ?? ''
  check(`refusal in ${locale}`, response.status === 400 && text === want, `status=${response.status} text=${JSON.stringify(text)}`)
  if (mustNot) check(`refusal in ${locale} carries no Chinese`, !mustNot.test(text), text)
}

const browser = await chromium.launch()
const ctx = await browser.newContext({ locale: 'en-US', viewport: { width: 1440, height: 1200 }, serviceWorkers: 'block' })
const page = await ctx.newPage()
const consoleErrors = []
page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()) })
page.on('pageerror', (e) => consoleErrors.push(String(e)))

// The app holds an SSE stream open, so networkidle never fires here.
await page.goto(BASE, { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(4500)
// The creation entry lives on the world pack page, not the landing stage.
for (const name of ['World packs', '世界包']) {
  const nav = page.getByRole('button', { name, exact: true }).first()
  if (await nav.isVisible().catch(() => false)) { await nav.click(); await page.waitForTimeout(3000); break }
}
await page.getByText('Mortal Cultivation', { exact: false }).first().click()
await page.waitForTimeout(2500)
const load = page.getByRole('button', { name: /^Load/i })
if (await load.count()) { await load.first().click(); await page.waitForTimeout(4000) }
await page.getByRole('button', { name: /Create a character/i }).first().click({ force: true })
await page.waitForTimeout(1500)

// The first input on the page is a hidden file picker; the name field is
// addressed by its own placeholder.
await page.getByPlaceholder(/Name your character/).first().fill('Lin Zhao')
await page.getByRole('radio', { name: /^Wood$/ }).first().click()
await page.waitForTimeout(400)

// One attribute is all a single root takes, so the hint has to be singular.
const hint = await page.evaluate(() => {
  const m = document.body.innerText.match(/Choose \d+ attributes?/)
  return m ? m[0] : null
})
check('the root hint is singular for one attribute', hint === 'Choose 1 attribute', String(hint))
await page.screenshot({ path: `${SHOTS}/form.png` })

await page.getByRole('button', { name: /Generate identity/i }).first().click()
let preview = false
for (let i = 0; i < 60; i++) {
  await page.waitForTimeout(2000)
  if (await page.getByRole('button', { name: /Start story/i }).count()) { preview = true; break }
}
check('the draft reached the preview', preview)

const text = await page.evaluate(() => document.body.innerText)
const start = text.indexOf('Confirm this character')
const shown = start >= 0 ? text.slice(start) : text
check('the preview is written in English', !hanPattern.test(shown), hanPattern.test(shown) ? shown.slice(0, 200) : '')
check('the page reported no console errors', consoleErrors.length === 0, consoleErrors.join(' | '))
await page.screenshot({ path: `${SHOTS}/preview.png` })
await browser.close()

const failed = results.filter((r) => !r.ok).length
console.log(`\n${results.length - failed}/${results.length} passed`)
process.exit(failed === 0 ? 0 : 1)
