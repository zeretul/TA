import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'

// Engine processes are covered at the application/tool boundary in Go. This
// browser test isolates the composer contract without requiring CLI login.
for (const theme of ['dark', 'light'] as const) {
  test(`game switches runtimes without losing draft in ${theme}`, async ({ page, request }) => {
    test.setTimeout(90_000)
    const current = await (await request.get('/api/settings')).json()
    const seeded = await request.patch('/api/settings', { data: { layer: 'user', base_revision: current.revisions.user,
      changes: { theme, agent_runtimes: { interactive_story: { selected: 'native' } } } } })
    expect(seeded.ok(), await seeded.text()).toBe(true)
    const projectId = (await createAndOpenBook(request, `Runtime game ${theme}`)).projectId
    const storyId = (await createStartedStory(request, `Runtime ${theme}`)).id
    const params = new URLSearchParams({ mode: 'interactive', story_id: storyId, branch_id: 'main' })
    const configPath = `/api/projects/${projectId}/conversation-config`
    let snapshot = await (await request.get(`${configPath}?${params}`)).json()
    expect(snapshot.runtime_capabilities.goal).toBe(false)
    const gameGoalRequests: string[] = []
    page.on('request', request => {
      const url = new URL(request.url())
      if (url.pathname.endsWith('/conversation-goal') && url.searchParams.get('mode') === 'interactive') gameGoalRequests.push(url.href)
    })
    const switches: string[] = []
    await page.route('**/conversation-config**', async route => {
      if (!route.request().url().includes(configPath)) { await route.continue(); return }
      if (route.request().method() === 'PATCH') {
        const body = route.request().postDataJSON()
        expect(body.binding.mode).toBe('interactive')
        expect(body.binding.story_id || '').toBe(storyId)
        expect(Object.keys(body.changes)).toEqual(['runtime'])
        switches.push(body.changes.runtime.kind)
        snapshot = { ...snapshot, revision: snapshot.revision + 1, runtime: body.changes.runtime,
          runtime_capabilities: { cancel: true, goal: false, queue: true, pause: true } }
      }
      await route.fulfill({ json: snapshot })
    })
    await page.route('**/api/agent-runtimes/*/models', route => route.fulfill({ json: { default_id: 'test-external', items: [{ id: 'test-external', display_name: `External model ${'long label '.repeat(12)}`, efforts: ['medium'] }] } }))
    const statuses: Record<string, string> = { native: 'ready', codex: 'ready', claude: 'ready' }
    await page.route('**/api/agent-runtimes/codex/check', route => { statuses.codex = 'ready'; return route.fulfill({ json: { id: 'codex', status: 'ready' } }) })
    await page.route('**/api/agent-runtimes', route => route.fulfill({ json: { items: ['native', 'codex', 'claude'].map(id => ({ id, name_key: `agentRuntime.${id}`, status: statuses[id] })) } }))
    await page.goto('/')
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
    const editor = page.getByPlaceholder(/你要做什么/)
    await expect(editor).toBeVisible()
    const trigger = page.locator('[data-model-profile-trigger]').filter({ visible: true })
    await expect(trigger).toBeEnabled()
    await editor.fill('Keep this draft / 保留这段草稿')
    await expect(page.locator('[data-action="send"]').filter({ visible: true })).toBeEnabled()
    for (const [engine, previous, width] of [['Codex', 'Native', 1440], ['Claude Code', 'Codex', 390], ['Native', 'Claude Code', 390]] as const) {
      await page.setViewportSize({ width, height: 960 })
      await trigger.click()
      const runtimeLink = page.getByRole('menuitem', { name: `运行时：${previous}`, exact: true })
      await expect(runtimeLink).toBeVisible()
      await expect(page.getByText('切换运行时', { exact: true })).toHaveCount(0)
      expect((await runtimeLink.locator('span').boundingBox())?.height).toBeLessThan(30)
      await page.screenshot({ path: test.info().outputPath(`game-${theme}-${width}-model-menu.png`), animations: 'disabled' })
      await runtimeLink.click()
      const option = page.getByRole('menuitem', { name: engine, exact: true })
      await expect(option).toBeVisible()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await expect.poll(async () => {
        const box = await page.locator('[data-slot="dropdown-menu-sub-content"]').boundingBox()
        return Boolean(box && box.x >= 0 && box.x + box.width <= width)
      }).toBe(true)
      await page.screenshot({ path: test.info().outputPath(`game-${theme}-${width}-${engine}.png`), animations: 'disabled' })
      await option.click()
      const currentRuntime = page.getByRole('menuitem', { name: `运行时：${engine}`, exact: true })
      await expect(currentRuntime).toBeVisible()
      // A changed label can render before the submenu finishes closing.
      await expect(page.locator('[data-slot="dropdown-menu-sub-content"]')).toHaveCount(0)
      await currentRuntime.press('Escape')
      await expect(trigger).toHaveAttribute('aria-expanded', 'false')
      await expect(page.getByRole('menu')).toHaveCount(0)
      await expect(editor).toHaveText('Keep this draft / 保留这段草稿')
      await page.getByRole('button', { name: '输入动作', exact: true }).filter({ visible: true }).click()
      await expect(page.getByText('切换运行时', { exact: true })).toHaveCount(0)
      await expect(page.getByRole('menuitem', { name: '上下文分析', exact: true })).toHaveCount(engine === 'Native' ? 1 : 0)
      await expect(page.getByRole('menuitemcheckbox', { name: '目标', exact: true })).toHaveCount(0)
      await page.keyboard.press('Escape')
      await expect(page.getByRole('menu')).toHaveCount(0)
    }
    expect(switches).toEqual(['codex', 'claude', 'native'])
    expect(gameGoalRequests).toEqual([])
    await trigger.click()
    await page.getByRole('menuitem', { name: '配置', exact: true }).click()
    // The game build has no Agent profile page. A configuration reference is
    // routed to the presets directory, the surface the player can still edit
    // (ModeRouter.openToolNavigationTarget). Assert that reachable
    // destination, and that the menu released the pointer lock, instead of
    // the agent-configuration sections this product does not ship.
    await expect(page.locator('.preset-workspace')).toBeVisible()
    await expect(page.locator('body')).not.toHaveCSS('pointer-events', 'none')
    await expect(page.locator('[data-agent-configuration-section]')).toHaveCount(0)
  })
}
