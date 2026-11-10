import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'

// Both themes and locales are exercised; theme and locale do not change the
// compaction protocol, so avoid their full Cartesian product.
for (const [theme, language] of [['dark', 'zh-CN'], ['light', 'en-US']] as const) {
  test(`game restores runtime compaction and plan cards in ${theme} ${language}`, async ({ page, request }) => {
    await createAndOpenBook(request, `Compaction ${theme} ${language}`)
    await createStartedStory(request, 'Runtime context')
    const settings = await (await request.get('/api/settings')).json()
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({
      json: { ...settings, effective: { ...settings.effective, theme, language } },
    }))
    const event = { id: 'runtime-compact', role: 'context_compaction', status: 'success', phase: 'model_step', runtime_managed: true }
    const taskText = language === 'zh-CN' ? '核对已保存章节与验收要求，确认长文本在窄屏中完整换行。'.repeat(4) : 'Verify the saved chapter and acceptance criteria, including readable long task text on narrow screens. '.repeat(4)
    const plan = { schema: 'agent.todo.v1', items: [{ id: '1', text: taskText, status: 'in_progress' }] }
    const planEvent = { id: 'runtime-plan', role: 'todo_updated', content: JSON.stringify(plan) }
    await page.route('**/api/interactive/stories/*/snapshot**', async route => {
      const response = await route.fetch()
      const snapshot = await response.json()
      await route.fulfill({ response, json: { ...snapshot, pending_display_events: [event, planEvent] } })
    })
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      if (width === 1440) await page.goto('/')
      else await page.reload()
      if (width === 1440) {
        const sidebar = page.getByLabel(/工作台侧边栏|Workbench sidebar/)
        await sidebar.getByRole('button', { name: /^(游戏|Game)$/, exact: true }).click()
      }
      const text = language === 'zh-CN'
        ? '运行时已压缩工作上下文，会话历史完整保留。'
        : 'The runtime compacted its working context. Conversation history is preserved.'
      await expect(page.getByText(text, { exact: true })).toHaveCount(1)
      await expect(page.getByText(text, { exact: true })).toBeVisible()
      await expect(page.getByText(language === 'zh-CN' ? '自动压缩' : 'Automatic compaction', { exact: true })).toBeVisible()
      await expect(page.getByText(/没有压缩摘要内容|No compaction summary content/)).toHaveCount(0)
      await expect(page.locator('li').getByText(taskText.trim(), { exact: true })).toBeVisible()
      await expect(page.getByText('0/1', { exact: true })).toBeVisible()
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await page.screenshot({ path: test.info().outputPath(`game-${theme}-${language}-${width}.png`), animations: 'disabled' })
    }
  })
}
