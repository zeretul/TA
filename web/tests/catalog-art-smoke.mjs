// Run with the isolated Vite fixture at TA_ART_TEST_URL. Uses actual shipped
// artwork bytes with a transport fixture; does not claim an earned game state.
import { chromium } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'
const root = path.resolve(import.meta.dirname, '../..')
const manifest = JSON.parse(fs.readFileSync(path.join(root, 'internal/catalogart/assets/manifest.json'), 'utf8'))
const definition = manifest.entries.find(e => e.kind === 'definition' && e.id === 'lanchuan-i01')
const npc = manifest.entries.find(e => e.kind === 'npc' && e.id === 'lanchuan-p01')
const bytes = sha => fs.readFileSync(path.join(root, 'internal/catalogart/assets/images', `${sha}.png`))
const out = process.env.TA_ART_SCREENSHOTS || path.join(root, 'web/test-results/catalog-art')
fs.mkdirSync(out, { recursive: true })
const browser = await chromium.launch({ executablePath: process.env.TA_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage'] })
const report = []
try {
  for (const theme of ['dark', 'light']) for (const width of [375, 1280]) for (const locale of ['zh-CN', 'en-US']) {
    const page = await browser.newPage({ viewport: { width, height: 900 }, reducedMotion: 'reduce' })
    const errors = []
    page.on('pageerror', e => errors.push(e.message))
    await page.route('**/api/catalog-art/*', route => {
      const id = route.request().url().split('/').pop()
      if (id === definition.variants.illustration.sha256 + '.png') return route.fulfill({ status: 200, contentType: 'image/png', body: bytes(definition.variants.illustration.sha256) })
      if (id === npc.variants.avatar.sha256 + '.png') return route.fulfill({ status: 200, contentType: 'image/png', body: bytes(npc.variants.avatar.sha256) })
      return route.fulfill({ status: 404, body: '' })
    })
    await page.goto(`${process.env.TA_ART_TEST_URL || 'http://127.0.0.1:15191'}/tests/support/catalog-art.html?theme=${theme}&locale=${locale}`)
    await page.locator('[data-testid=dedicated] img').first().waitFor()
    await page.waitForFunction(() => document.querySelector('[data-testid=dedicated] img')?.naturalWidth > 0)
    await page.getByText(locale === 'en-US' ? 'No illustration yet' : '暂无配图', { exact: true }).first().waitFor()
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)
    if (overflow || errors.length) throw new Error(JSON.stringify({ theme, width, locale, overflow, errors }))
    const image = await page.locator('[data-testid=dedicated] img').first().evaluate(el => ({ width: el.naturalWidth, height: el.naturalHeight }))
    if (image.width !== 768 || image.height !== 768) throw new Error('Dedicated source bytes did not render')
    await page.screenshot({ path: path.join(out, `${theme}-${width}-${locale}.png`), fullPage: true })
    report.push({ theme, width, locale, overflow, image, errors })
    await page.close()
  }
} finally { await browser.close() }
fs.writeFileSync(path.join(out, 'results.json'), JSON.stringify(report, null, 2))
console.log(`Passed ${report.length} catalog artwork visual combinations`)
