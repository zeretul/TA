// Run against an isolated Vite presentation server; no game facts are created.
import { chromium } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'
const out = process.env.TA_BRAND_EVIDENCE || '/tmp/ta-brand-evidence'
fs.mkdirSync(out, { recursive: true })
const browser = await chromium.launch({ executablePath: process.env.TA_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage'] })
const results = []
try {
  for (const theme of ['dark', 'light']) for (const width of [390, 1280]) for (const locale of ['zh-CN', 'en-US']) {
    const page = await browser.newPage({ viewport: { width, height: 900 }, reducedMotion: 'reduce' })
    const errors = []
    page.on('pageerror', e => errors.push(e.message))
    await page.route('**/api/update/status', route => route.fulfill({ json: { current_version: 'brand-visual-test', channel_configured: false, staged: false, apply_ready: false } }))
    await page.goto(`${process.env.TA_BRAND_TEST_URL || 'http://127.0.0.1:15193'}/tests/support/branding.html?theme=${theme}&locale=${locale}`)
    await page.locator('[data-testid=about-updates] img').waitFor()
    await page.waitForFunction(() => [...document.querySelectorAll('img')].every(img => img.naturalWidth > 0))
    if (width < 768) await page.getByRole('button', { name: 'Open navigation', exact: true }).click()
    const mark = page.locator('[data-slot=sidebar-header] img')
    await mark.waitFor({ state: 'visible' })
    if ((await mark.evaluate(el => el.naturalWidth)) !== 512) throw new Error('Selected brand image did not load')
    if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)) throw new Error('Horizontal overflow')
    const prefix = `${theme}-${width}-${locale}`
    await page.screenshot({ path: path.join(out, `${prefix}-expanded.png`), fullPage: true })
    const toggle = page.locator('[data-activity-bar-toggle]')
    await toggle.click()
    if (width < 768) {
      await page.getByRole('dialog').waitFor({ state: 'hidden' })
      await page.screenshot({ path: path.join(out, `${prefix}-about.png`), fullPage: true })
      await page.getByRole('button', { name: 'Open navigation', exact: true }).click()
    } else {
      await page.waitForSelector('[data-collapsible=icon]')
      await mark.waitFor({ state: 'visible' })
      await page.screenshot({ path: path.join(out, `${prefix}-collapsed.png`), fullPage: true })
      await toggle.click()
      await page.waitForSelector('[data-state=expanded]')
    }
    await mark.waitFor({ state: 'visible' })
    if (errors.length) throw new Error(JSON.stringify(errors))
    results.push({ theme, width, locale, brandLoaded: true, navigationToggleAndReturn: true, errors })
    await page.close()
  }
} finally { await browser.close() }
fs.writeFileSync(path.join(out, 'branding-browser-results.json'), JSON.stringify({ fixture: 'Actual sidebar and About components; mocked update-status transport', results }, null, 2))
console.log(`Passed ${results.length} TA branding theme / language / viewport combinations, including toggle and return.`)
