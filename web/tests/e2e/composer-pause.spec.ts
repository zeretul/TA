import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { getModelStatus, releaseDelayedRequest } from '../support/model'

for (const theme of ['dark', 'light'] as const) {
  test(`game pauses from the composer and reveals settled actions in ${theme}`, async ({ page, request }) => {
    page.setDefaultTimeout(10_000)
    await createAndOpenBook(request, `Composer pause ${theme}`)
    await createStartedStory(request, 'Composer pause story')
    const width = theme === 'dark' ? 1440 : 390
    const settings = await (await request.get('/api/settings')).json()
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({ json: {
      ...settings, effective: { ...settings.effective, theme, language: 'zh-CN' },
    } }))
    await page.addInitScript(() => {
      localStorage.setItem('nova.locale.configured', 'zh-CN')
      localStorage.setItem('nova:onboarding:v1', JSON.stringify({ version: 1, skipped: true }))
    })
    await page.setViewportSize({ width, height: 960 })
    await page.goto('/')
    if (width < 800) {
      await page.getByRole('button', { name: '导航菜单', exact: true }).click()
      await page.getByRole('dialog').getByRole('button', { name: '游戏', exact: true }).click()
    } else {
      await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
    }
    const composer = page.getByPlaceholder(/你要做什么/).filter({ visible: true })
    await expect(composer).toBeVisible()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    // The started Game already has one settled opening run on desktop.
    // Wait for hydration before counting it; the composer mounts earlier.
    const previousRunActions = width >= 800 ? 1 : 0
    await expect(page.getByRole('button', { name: '复制 Run ID', exact: true })).toHaveCount(previousRunActions)
    const marker = 'E2E_COMPOSER_PAUSE'
    await composer.fill(`检查房间后的窗台。${marker}`)
    await page.locator('[data-action="send"]').filter({ visible: true }).click()
    try {
      await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[marker] ?? 0).toBe(1)
      await expect(page.getByText(/正在检查房间后的窗台，接下来会继续描写/).last()).toBeVisible()
      await expect(page.getByRole('button', { name: '暂停任务', exact: true })).toHaveCount(0)
      await expect(page.getByRole('button', { name: '复制 Run ID', exact: true })).toHaveCount(previousRunActions)
      const output = page.locator('[data-nova-chat-item="run"]').last()
      await expect(output.getByRole('button', { name: '复制消息', exact: true })).toHaveCount(0)
      await page.screenshot({ path: test.info().outputPath('streaming.png') })
      const pauseRequest = page.waitForRequest(value => value.method() === 'POST' && value.postData()?.includes('"suspend"') === true)
      await page.locator('[data-action="stop"]').filter({ visible: true }).click()
      await pauseRequest
      await expect(page.getByRole('button', { name: '继续任务', exact: true })).toBeVisible()
      await output.hover()
      await expect(output.getByRole('button', { name: '复制消息', exact: true })).toBeVisible()
      if (width < 800) await output.getByRole('button', { name: '消息操作', exact: true }).click()
      await expect(page.getByRole('button', { name: '复制 Run ID', exact: true })).toHaveCount(previousRunActions + 1)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
      await page.screenshot({ path: test.info().outputPath('paused.png') })
    } finally {
      await releaseDelayedRequest(request, marker)
    }
  })
}
