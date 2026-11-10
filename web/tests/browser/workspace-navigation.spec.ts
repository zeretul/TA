import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

test.use({ reducedMotion: 'no-preference' })

for (const theme of ['dark', 'light']) {
  test(`primary navigation responds without background trailing in ${theme} mode`, async ({ page, request }) => {
    // This journey hydrates six destinations on slower CI runners.
    test.setTimeout(60_000)
    await createAndOpenBook(request, `Navigation Feedback Book ${theme}`)
    const settings = await (await request.get('/api/settings')).json()
    // Navigation can abort a settings request before route.fetch returns.
    // This journey only needs a fixed theme, so serve its snapshot directly.
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({
      json: { ...settings, effective: { ...settings.effective, theme } },
    }))
    await page.goto('/')
    const sidebar = page.getByLabel('工作台侧边栏')
    await expect(sidebar).toBeVisible()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)

    const game = sidebar.getByRole('button', { name: '游戏', exact: true })
    const lore = sidebar.getByRole('button', { name: '资料库', exact: true })
    const gameBounds = await game.boundingBox()
    const loreBounds = await lore.boundingBox()
    expect(loreBounds!.y - gameBounds!.y - gameBounds!.height).toBeGreaterThanOrEqual(2)

    // Sample the first painted frame after each click, before any color tween could settle.
    for (const destination of ['游戏', '资料库', '世界包', '游戏']) {
      const button = sidebar.getByRole('button', { name: destination, exact: true })
      await button.click()
      const feedback = await button.evaluate(async (element) => {
        await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
        const activeColor = getComputedStyle(element).getPropertyValue('--nova-active').trim()
        const probe = document.createElement('span')
        probe.style.backgroundColor = activeColor
        element.appendChild(probe)
        const expectedColor = getComputedStyle(probe).backgroundColor
        probe.remove()
        return { active: element.getAttribute('aria-current'), color: getComputedStyle(element).backgroundColor, expectedColor }
      })
      expect(feedback.active).toBe('page')
      expect(feedback.color).toBe(feedback.expectedColor)
      await expect(sidebar.locator('[aria-current="page"]')).toHaveCount(1)
    }
    await page.screenshot({ path: test.info().outputPath(`navigation-${theme}.png`) })
  })
}

test('keeps exactly one primary destination active across the game destinations', async ({ page, request }) => {
  // This journey hydrates the player destinations; each assertion keeps its
  // normal deadline, while the total budget includes cold route loads on CI
  // runners.
  test.setTimeout(90_000)
  await createAndOpenBook(request, 'Browser Navigation Book')
  const settings = await (await request.get('/api/settings')).json()
  await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({
    json: { ...settings, effective: { ...settings.effective, labs: { ...settings.effective?.labs, developer_mode: true } } },
  }))
  await page.goto('/')

  const sidebar = page.getByLabel('工作台侧边栏')
  await expect(sidebar).toBeVisible()
  // The player entry is 游戏 / 资料库 / 世界包 plus 设置; see the player-only
  // activity list in WorkbenchShell.
  for (const destination of ['游戏', '资料库', '游戏设置', 'AI 助手', '世界包', '系统设置']) {
    await test.step(destination, async () => {
      await sidebar.getByRole('button', { name: destination, exact: true }).click()
      await expect(sidebar.locator('[aria-current="page"]')).toHaveCount(1)
      await expect(sidebar.getByRole('button', { name: destination, exact: true })).toHaveAttribute('aria-current', 'page')
      await expect(page.locator('[data-slot=loading-state]:visible')).toHaveCount(0)
      await expect(page.getByRole('button', { name: '关闭设置', exact: true })).toHaveCount(0)
    })
  }
})

test('exposes the same primary destinations in English on a narrow viewport', async ({ page, request }) => {
  await createAndOpenBook(request, 'Mobile Navigation Book')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.addInitScript(() => {
    window.localStorage.setItem('nova.locale.configured', 'en-US')
  })
  const settings = await (await request.get('/api/settings')).json()
  await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({
    json: { ...settings, effective: { ...settings.effective, language: 'en-US' } },
  }))
  await page.goto('/')

  await page.getByRole('button', { name: 'Navigation', exact: true }).click()
  const navigation = page.getByRole('dialog')
  await expect(navigation.getByRole('button', { name: 'Game', exact: true })).toBeVisible()
  await expect(navigation.getByRole('button', { name: 'Lore', exact: true })).toBeVisible()
  await expect(navigation.getByRole('button', { name: 'World packs', exact: true })).toBeVisible()
  await navigation.getByRole('button', { name: 'System settings', exact: true }).click()
  await expect(navigation).toBeHidden()
  const categories = page.locator('.nova-mobile-topbar').getByRole('button', { name: 'Categories', exact: true })
  await categories.click()
  const categoryDrawer = page.getByRole('dialog', { name: 'Categories', exact: true })
  await expect(categoryDrawer).toHaveAttribute('data-side', 'right')
  await categoryDrawer.getByRole('button', { name: 'Appearance', exact: true }).click()
  await expect(categoryDrawer).toBeHidden()
  await expect(categories).toBeFocused()
})
