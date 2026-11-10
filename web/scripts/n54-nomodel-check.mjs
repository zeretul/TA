import { chromium } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { devOutput } from '../../scripts/dev-output.mjs'

const BASE = 'http://127.0.0.1:19734'
const shotsDir = devOutput('n54-check/shots')
mkdirSync(shotsDir, { recursive: true })
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 900 } })
await page.goto(BASE, { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(6000)
const gameEntry = page.getByRole('button', { name: /^游戏$/ }).first()
if (await gameEntry.isVisible().catch(() => false)) { await gameEntry.click(); await page.waitForTimeout(2500) }
await page.getByText(/世界包/).first().click()
await page.waitForTimeout(2500)
await page.getByRole('button', { name: /创建世界/ }).first().click()
await page.waitForTimeout(4000)
const warning = page.getByText(/还差一个能写世界的模型/).first()
const shown = await warning.isVisible().catch(() => false)
const configure = page.getByRole('button', { name: /去设置模型/ }).first()
const configureShown = await configure.isVisible().catch(() => false)
console.log(shown && configureShown ? 'PASS  未配模型时提示缺模型，并给出设置入口' : `FAIL  shown=${shown} configure=${configureShown}`)
await page.screenshot({ path: `${shotsDir}/nomodel-warning.png` })
await browser.close()
process.exit(shown && configureShown ? 0 : 1)
