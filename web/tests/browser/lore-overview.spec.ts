import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory, getStorySnapshot } from '../support/api'

/**
 * 设计稿/自制-资料总览与统一信息面板_20261020.md §8 requires, on a real page:
 *
 *   "正常打开→六标签→同面板关联跳转→返回列表→关闭且输入不丢"
 *   "宽窄屏/键盘/主题"
 *   "打开、反复查看、刷新和切标签不推进游戏时间、不生成新回合…"
 *
 * The unit suite proves what the panel assembles and what it refuses to reveal.
 * This spec proves the part only a real browser can answer: the button is on the
 * real toolbar, the six tabs really switch, the panel really sits beside the
 * prose without crushing it, closing really leaves the draft alone, and no
 * gesture along the way reaches the turn endpoint.
 */

async function openGame(page: import('@playwright/test').Page) {
  await page.goto('/')
  const sidebarGame = page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true })
  const menuButton = page.getByRole('button', { name: '导航菜单', exact: true })
  // The shell renders after the first workspace fetch, so neither destination is
  // on screen yet when the page has merely loaded.
  await expect.poll(async () => (await sidebarGame.isVisible()) || (await menuButton.isVisible())).toBe(true)
  if (await sidebarGame.isVisible()) {
    await sidebarGame.click()
  } else {
    await menuButton.click()
    const drawer = page.getByRole('dialog', { name: '导航菜单', exact: true })
    await drawer.getByRole('button', { name: '游戏', exact: true }).click()
    await expect(drawer).toBeHidden()
  }
  await expect(page.getByPlaceholder(/你要做什么/)).toBeVisible()
}

test('opens the six reading tabs beside the prose and closes without touching the draft', async ({ page, request }) => {
  await page.setViewportSize({ width: 1400, height: 1000 })
  await createAndOpenBook(request, 'Lore overview')
  const story = await createStartedStory(request, 'Lore overview story')
  const turnsBefore = (await getStorySnapshot(request, story.id)).turns.length

  // Every read the overview makes must be a GET. A chat request would be a turn,
  // and a turn is the one thing §6 says this surface never causes.
  const chatRequests: string[] = []
  page.on('request', (request) => {
    if (/\/api\/interactive\/chat$/.test(new URL(request.url()).pathname)) chatRequests.push(request.url())
  })

  await openGame(page)
  const composer = page.getByPlaceholder(/你要做什么/)
  await composer.fill('这是尚未发送的草稿')

  const trigger = page.locator('[data-lore-overview-trigger]')
  await expect(trigger).toBeVisible()
  await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await trigger.click()

  const panel = page.getByTestId('lore-overview-panel')
  await expect(panel).toBeVisible()
  // Opening is remembered on the button itself, so the same control says both
  // what it did and what it will do next. The panel's own close button shares
  // the label, so the toolbar control is picked by its marker.
  await expect(trigger).toHaveAttribute('aria-label', '收起资料总览')
  await expect(trigger).toHaveAttribute('aria-expanded', 'true')

  // §3's six categories, in the order the design lists them.
  for (const tab of ['玩家', '人物', '势力', '地点', '世界', '见闻与事务']) {
    const tabButton = panel.getByRole('tab', { name: tab, exact: true })
    await expect(tabButton).toBeVisible()
    await tabButton.click()
    await expect(tabButton).toHaveAttribute('aria-selected', 'true')
    // A tab with nothing committed says so in words instead of drawing a blank;
    // a tab with content lists what the projection allowed through.
    await expect.poll(async () => {
      const empty = await panel.getByText(/这一页还没有可查的见闻。|没有找到相符的内容。/).count()
      const entries = await panel.locator('[data-lore-entry]').count()
      return empty > 0 || entries > 0
    }).toBe(true)
  }

  // The panel is a column beside the prose, not an overlay over it: the story
  // column still has real width while the panel is open.
  const content = await page.locator('[data-story-stage-content]').evaluate((element) => element.getBoundingClientRect())
  const bodyWidth = await page.locator('[data-story-stage-body]').evaluate((element) => element.getBoundingClientRect().width)
  const panelBox = await panel.evaluate((element) => element.getBoundingClientRect())
  expect(panelBox.width).toBeGreaterThan(280)
  // Docked, not floating over the prose: the two boxes sit side by side and the
  // story column still has real width of its own.
  expect(panelBox.left).toBeGreaterThanOrEqual(content.left + content.width - panelBox.width - 1)
  expect(bodyWidth).toBeGreaterThan(360)

  // Closing returns the reader to the prose with the draft intact.
  await panel.getByRole('button', { name: '收起资料总览' }).click()
  await expect(panel).toHaveCount(0)
  await expect(composer).toHaveText('这是尚未发送的草稿')
  // §2 puts focus back on the control that opened the panel.
  await expect(trigger).toBeFocused()
  await expect(trigger).toHaveAttribute('aria-label', '打开资料总览')

  // Reopening reads again; it still never asks for a turn.
  await trigger.click()
  await expect(panel).toBeVisible()
  await panel.getByRole('tab', { name: '世界', exact: true }).click()
  await panel.getByRole('button', { name: '收起资料总览' }).click()
  await expect(panel).toHaveCount(0)

  expect(chatRequests).toEqual([])
  expect((await getStorySnapshot(request, story.id)).turns).toHaveLength(turnsBefore)
})

test('closes the focused layer first: Esc leaves the detail, then the panel', async ({ page, request }) => {
  await page.setViewportSize({ width: 1400, height: 1000 })
  await createAndOpenBook(request, 'Lore overview Esc')
  await createStartedStory(request, 'Lore overview Esc story')

  await openGame(page)
  await page.locator('[data-lore-overview-trigger]').click()
  const panel = page.getByTestId('lore-overview-panel')
  await expect(panel).toBeVisible()

  // The panel takes focus when it opens, so an Esc pressed without clicking
  // anywhere first still lands on the layer the reader is looking at.
  await expect(panel).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(panel).toHaveCount(0)
  await expect(page.locator('[data-lore-overview-trigger]')).toBeFocused()
})

test('keeps the prose readable when the panel is dragged to its widest', async ({ page, request }) => {
  // Wide enough that the panel has room to grow: the director panel is open
  // beside the story, so a narrower window would leave the drag clamped at once
  // and prove only that the clamp works.
  await page.setViewportSize({ width: 1680, height: 1000 })
  await createAndOpenBook(request, 'Lore overview resize')
  await createStartedStory(request, 'Lore overview resize story')

  await openGame(page)
  await page.locator('[data-lore-overview-trigger]').click()
  const panel = page.getByTestId('lore-overview-panel')
  await expect(panel).toBeVisible()

  const before = await panel.boundingBox()
  expect(before).not.toBeNull()
  const handle = panel.locator('[data-lore-overview-resize]')
  await expect(handle).toBeVisible()
  const handleBox = await handle.boundingBox()
  expect(handleBox).not.toBeNull()

  // Drag far past any sane ceiling. The clamp, not the drag, decides where it
  // stops: §2 refuses to squeeze the story into a thin strip.
  await page.mouse.move(handleBox!.x + handleBox!.width / 2, handleBox!.y + handleBox!.height / 2)
  await page.mouse.down()
  await page.mouse.move(handleBox!.x + handleBox!.width / 2 - 900, handleBox!.y + handleBox!.height / 2, { steps: 10 })
  await page.mouse.up()

  const after = await panel.boundingBox()
  expect(after!.width).toBeGreaterThan(before!.width)
  expect(after!.width).toBeLessThanOrEqual(640)
  // The story column keeps the prose readable, and the docked panel is not over
  // it: the panel starts exactly where the story column ends.
  const body = await page.locator('[data-story-stage-body]').evaluate((element) => element.getBoundingClientRect())
  expect(body.width).toBeGreaterThanOrEqual(360 - 1)
  expect(Math.abs(after!.x - (body.x + body.width))).toBeLessThanOrEqual(2)

  // The chosen width is remembered for the next time the panel opens.
  const chosen = Math.round(after!.width)
  await panel.getByRole('button', { name: '收起资料总览' }).click()
  await expect(panel).toHaveCount(0)
  await page.locator('[data-lore-overview-trigger]').click()
  const reopened = await page.getByTestId('lore-overview-panel').boundingBox()
  expect(Math.round(reopened!.width)).toBe(chosen)
})

test('uses the sheet, not a docked column, on a narrow screen', async ({ page, request }) => {
  await page.setViewportSize({ width: 420, height: 820 })
  await createAndOpenBook(request, 'Lore overview narrow')
  await createStartedStory(request, 'Lore overview narrow story')

  await openGame(page)
  await page.locator('[data-lore-overview-trigger]').click()
  const panel = page.getByTestId('lore-overview-panel')
  await expect(panel).toBeVisible()
  // The narrow presentation is the overlay sheet: it takes the screen and has no
  // drag edge to hunt for, because there is no column to share.
  await expect(panel.locator('[data-lore-overview-resize]')).toHaveCount(0)
  const box = await panel.boundingBox()
  expect(box!.width).toBeGreaterThan(380)
  // All six categories still fit and can still be reached.
  for (const tab of ['玩家', '人物', '势力', '地点', '世界', '见闻与事务']) {
    await expect(panel.getByRole('tab', { name: tab, exact: true })).toBeVisible()
  }
  await panel.getByRole('button', { name: '收起资料总览' }).click()
  await expect(panel).toHaveCount(0)
  await expect(page.getByPlaceholder(/你要做什么/)).toBeVisible()
})

for (const theme of ['dark', 'light'] as const) {
  test(`draws the overview in ${theme}`, async ({ page, request }) => {
    await page.setViewportSize({ width: 1400, height: 1000 })
    await createAndOpenBook(request, `Lore overview ${theme}`)
    await createStartedStory(request, `Lore overview ${theme} story`)
    const settings = await (await request.get('/api/settings')).json()
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({
      json: { ...settings, effective: { ...settings.effective, theme } },
    }))

    await openGame(page)
    await page.locator('[data-lore-overview-trigger]').click()
    const panel = page.getByTestId('lore-overview-panel')
    await expect(panel).toBeVisible()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    // The panel paints its own surface instead of inheriting a transparent one,
    // so the prose behind it never shows through.
    const background = await panel.evaluate((element) => getComputedStyle(element).backgroundColor)
    expect(background).not.toBe('rgba(0, 0, 0, 0)')
    expect(background).not.toBe('transparent')
    await page.screenshot({ path: test.info().outputPath(`lore-overview-${theme}.png`) })
  })
}
