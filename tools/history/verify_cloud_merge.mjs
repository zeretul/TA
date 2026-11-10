// Cloud merge regression: isolated data only, with no external model calls.
// Run from the repository root against the built candidate server.
import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import path from 'node:path'
import { createRequire } from 'node:module'
const require = createRequire(path.resolve('web/package.json'))
const { chromium, expect } = require('@playwright/test')
const batch = process.env.TA_TEST_BATCH || 'history-cloud'
assert(/^history-[a-z0-9-]+$/.test(batch))
const base = `http://127.0.0.1:${Number(process.env.TA_TEST_PORT || 18071)}`
const output = path.resolve('.ta-dev', batch, 'evidence/cloud-merge')
await fs.mkdir(output, { recursive: true })
async function api(resource, method = 'GET', body, locale = 'zh-CN') {
  const response = await fetch(base + resource, { method, headers: { 'Content-Type': 'application/json', 'X-TA-Locale': locale }, body: body === undefined ? undefined : JSON.stringify(body) })
  assert(response.ok, `${method} ${resource}: HTTP ${response.status}`)
  return response.json()
}
const original = await api('/api/settings')
assert.equal(path.resolve(original.paths.denova_dir), path.resolve('.ta-dev', batch, 'data'), 'Refusing a non-isolated data directory')
const originalWorkspace = await api('/api/workspace/current')
const result = { checks: [], errors: [], model: 'not called; slow advance intercepted in the browser', passed: false }
const browser = await chromium.launch({ headless: true, executablePath: process.env.TA_TEST_CHROMIUM || '/usr/bin/chromium' })
let page
let pendingAdvance
try {
  await api('/api/settings', 'PATCH', { layer: 'user', changes: { language: 'zh-CN', theme: 'light' }, base_revision: original.revisions.user })
  const bootstrap = await api('/api/history/bootstrap', 'POST', {})
  const created = await api(`/api/projects/${bootstrap.project_id}/history/campaigns`, 'POST', { name: 'Cloud merge regression', scenario_id: 'ming-1627', locale: 'zh-CN', command_id: `cloud-${crypto.randomUUID()}` })
  const campaignPath = `/api/projects/${bootstrap.project_id}/history/campaigns/${created.id}`
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'zh-CN', reducedMotion: 'reduce', serviceWorkers: 'block' })
  await context.addInitScript(({ project, campaign }) => localStorage.setItem(`ta:history:${project}:campaign`, campaign), { project: bootstrap.project_id, campaign: created.id })
  page = await context.newPage()
  page.on('pageerror', error => result.errors.push(error.message))
  // Fail closed if a test accidentally requests a model-backed action.
  await page.route(/\/history\/campaigns\/[^/]+\/(chat|prepare-edict|direct-edict|advance)$/, async route => {
    if (route.request().url().endsWith('/advance')) { pendingAdvance = route; return }
    result.errors.push(`Unexpected model-backed action: ${route.request().url()}`)
    await route.abort()
  })
  await page.goto(base + '/?game=history')
  await expect(page.getByRole('heading', { name: created.name, exact: true })).toBeVisible()
  const originalMarker = await page.evaluate(() => window.__historyMergeMarker = crypto.randomUUID())
  await page.getByRole('tab', { name: '颁诏', exact: true }).click()
  const draft = '未颁布的草稿：核对公开账目，不动内帑。'.repeat(40)
  await page.locator('#history-edict-text').fill(draft)
  await page.getByRole('button', { name: '新开一局', exact: true }).click()
  await expect(page.locator('#history-new-scenario option').first()).toHaveText(bootstrap.scenarios[0].name)
  await page.keyboard.press('Escape')
  // Change language through the actual settings UI, without a reload.
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '系统设置', exact: true }).click()
  await page.getByRole('combobox', { name: '界面语言', exact: true }).click()
  const localizedCatalog = page.waitForResponse(response => response.url().endsWith('/api/history/bootstrap') && response.request().headers()['x-ta-locale'] === 'en-US')
  await page.getByRole('option', { name: 'English', exact: true }).click()
  const englishBootstrap = await (await localizedCatalog).json()
  await page.getByLabel('Workbench sidebar').getByRole('button', { name: 'Game', exact: true }).click()
  await expect(page.getByRole('heading', { name: created.name, exact: true })).toBeVisible()
  await expect(page.locator('#history-edict-text')).toHaveValue(draft)
  assert.equal(await page.evaluate(() => window.__historyMergeMarker), originalMarker)
  await page.getByRole('button', { name: 'New game', exact: true }).click()
  await expect(page.locator('#history-new-scenario option').first()).toHaveText(englishBootstrap.scenarios[0].name)
  assert.notEqual(englishBootstrap.scenarios[0].name, bootstrap.scenarios[0].name)
  await page.keyboard.press('Escape')
  assert.deepEqual(await api(campaignPath), created)
  result.checks.push('Live language switch refreshes only the scenario catalog and preserves campaign, draft and page identity')
  // Hold a controlled slow request before it reaches the backend. Cancellation
  // still uses the actual cancel endpoint and canonical-state GET.
  await page.getByRole('button', { name: 'Advance a season', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Conclude & simulate', exact: true }).click()
  await expect(dialog.getByRole('button', { name: 'Cancel simulation', exact: true })).toBeEnabled()
  const cancelled = page.waitForResponse(response => response.url().endsWith(`${created.id}/cancel`))
  const canonical = page.waitForResponse(response => response.url().endsWith(campaignPath) && response.request().method() === 'GET')
  await dialog.getByRole('button', { name: 'Cancel simulation', exact: true }).click()
  assert((await cancelled).ok())
  assert.deepEqual(await (await canonical).json(), created)
  await expect(dialog).toBeHidden()
  await pendingAdvance?.abort().catch(() => {})
  pendingAdvance = undefined
  await expect(page.locator('#history-edict-text')).toHaveValue(draft)
  result.checks.push('Slow advance exposes an actionable modal cancel and rereads unchanged canonical state')
  await page.screenshot({ path: path.join(output, 'cancelled-advance.png') })
  // Real backend checkpoint and restore through the visible controls.
  await page.getByRole('button', { name: 'Saves & checkpoints', exact: true }).click()
  await page.getByRole('textbox', { name: 'Checkpoint name', exact: true }).fill('Cloud checkpoint')
  const checkpointResponse = page.waitForResponse(response => response.url().endsWith(`${created.id}/checkpoints`) && response.request().method() === 'POST')
  await page.getByRole('button', { name: 'Create checkpoint', exact: true }).click()
  const checkpoint = await (await checkpointResponse).json()
  assert(checkpoint.state.checkpoints.some(item => item.label === 'Cloud checkpoint'))
  await page.keyboard.press('Escape')
  const inspected = await api(campaignPath + '/inspect', 'POST', { command_id: `cloud-${crypto.randomUUID()}`, expected_revision: checkpoint.state.revision, target: 'treasury', locale: 'en-US' })
  assert(inspected.state.inspections.length > (created.inspections || []).length)
  await page.reload()
  await expect(page.getByRole('heading', { name: created.name, exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Saves & checkpoints', exact: true }).click()
  await page.getByRole('button', { name: 'Return here', exact: true }).click()
  const restoredResponse = page.waitForResponse(response => response.url().endsWith(`${created.id}/restore`) && response.request().method() === 'POST')
  await page.getByRole('button', { name: 'Restore checkpoint', exact: true }).click()
  const restored = await (await restoredResponse).json()
  assert(restored.state.inspections.at(-1).discarded)
  assert.deepEqual(restored.state.world.accounts, created.world.accounts)
  await expect(page.getByRole('dialog')).toBeHidden()
  result.checks.push('Checkpoint, cold reload and rewind retain account balances and discard the later inspection')
  // Navigation must not select/create an adventure project as a side effect.
  await page.getByRole('navigation', { name: 'Games', exact: true }).getByRole('button', { name: 'Adventure', exact: true }).click()
  const activeGameNavigation = page.locator('section[aria-hidden="false"] > nav').filter({ has: page.getByRole('button', { name: 'Historical simulation', exact: true }) })
  await expect(activeGameNavigation).toBeVisible()
  await activeGameNavigation.getByRole('button', { name: 'Historical simulation', exact: true }).click()
  await expect(page.getByRole('heading', { name: created.name, exact: true })).toBeVisible()
  await page.getByRole('tab', { name: 'Decrees', exact: true }).click()
  await expect(page.locator('#history-edict-text')).toHaveValue(draft)
  const sidebar = page.getByLabel('Workbench sidebar')
  await expect(sidebar.locator('[aria-current="page"]')).toHaveCount(1)
  await expect(sidebar.getByRole('button', { name: 'Game', exact: true })).toHaveAttribute('aria-current', 'page')
  const finalWorkspace = await api('/api/workspace/current')
  assert.equal(finalWorkspace.project_id, originalWorkspace.project_id)
  assert.equal(finalWorkspace.workspace, originalWorkspace.workspace)
  result.checks.push('Adventure chooser round trip retains history draft and save without changing the adventure workspace')
  assert.equal(result.errors.length, 0, result.errors.join('\n'))
  result.passed = true
  console.log(JSON.stringify(result, null, 2))
} catch (error) {
  result.failure = String(error.stack || error)
  await page?.screenshot({ path: path.join(output, 'failure.png'), fullPage: true }).catch(() => {})
  throw error
} finally {
  await pendingAdvance?.abort().catch(() => {})
  await fs.writeFile(path.join(output, 'results.json'), JSON.stringify(result, null, 2))
  await browser.close()
  const current = await api('/api/settings')
  await api('/api/settings', 'PATCH', { layer: 'user', changes: { language: original.user.language ?? null, theme: original.user.theme ?? null }, base_revision: current.revisions.user })
}
