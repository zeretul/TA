import { expect, test, type Page, type APIRequestContext } from '../support/fixtures'
import { createAndOpenBook, createStartedStory, getStorySnapshot, getStoryBranches } from '../support/api'

function createMobileBook(request: APIRequestContext, title: string) {
  return createAndOpenBook(request, `${title} ${test.info().project.name}`)
}

async function navigate(page: Page, name: string) {
  await page.getByRole('button', { name: '导航菜单', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: '导航菜单', exact: true })
  await drawer.getByRole('button', { name, exact: true }).click()
  await expect(drawer).toBeHidden()
}

async function swipe(page: Page, from: [number, number], to: [number, number]) {
  await page.evaluate(({ from, to }) => {
    const target = document.elementFromPoint(...from)!
    const touch = (point: [number, number]) => ({ identifier: 1, target, clientX: point[0], clientY: point[1] })
    // WebKit does not expose a constructible Touch. Supply complete touch lists,
    // including changedTouches used by Radix's scroll lock during drawer dismissal.
    for (const [type, point] of [['touchstart', from], ['touchmove', to], ['touchend', to]] as const) {
      const event = new Event(type, { bubbles: true, cancelable: true })
      Object.assign(event, { touches: type === 'touchend' ? [] : [touch(point)], targetTouches: type === 'touchend' ? [] : [touch(point)], changedTouches: [touch(point)] })
      target.dispatchEvent(event)
    }
  }, { from, to })
}

async function expectInsideViewport(page: Page) {
  const width = page.viewportSize()!.width
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
  const shell = page.locator('[data-nova-mobile-shell="true"]')
  await expect(shell).toBeVisible()
  const bounds = await shell.boundingBox()
  expect(bounds!.width).toBeLessThanOrEqual(width)
}

async function capture(page: Page, name: string) {
  await expect(page.locator('[data-slot=loading-state]:visible')).toHaveCount(0)
  await page.screenshot({ path: test.info().outputPath(`${name}.png`), animations: 'disabled' })
}

// Full navigation at the narrowest and last compact width covers the layout
// boundaries. Intermediate portrait/landscape sizes are checked below without
// repeating all nine destination journeys and their backend setup.
for (const width of [320, 1023]) {
  test(`mobile destinations and sheets remain usable at ${width}px`, async ({ page, request }) => {
    test.setTimeout(60_000)
    await createMobileBook(request, `Mobile ${width} A long book title for navigation`)
    await page.setViewportSize({ width, height: 844 })
    await page.goto('/')
    await expect(page.locator('.nova-mobile-topbar')).toHaveCount(1)
    await expect(page.locator('.nova-mobile-nav')).toHaveCount(0)
    // The game build ships 游戏 / 资料库 / 世界包 plus 设置.
    for (const destination of ['游戏', '资料库', '世界包', '设置']) {
      await page.getByRole('button', { name: '导航菜单', exact: true }).click()
      const navigation = page.getByRole('dialog', { name: '导航菜单', exact: true })
      await expect(navigation.getByRole('button', { name: destination, exact: true })).toBeVisible()
      await expect(navigation.getByRole('button', { name: destination, exact: true }).locator('span').last()).toHaveText(destination)
      await navigation.getByRole('button', { name: destination, exact: true }).click()
      await expect(navigation).toBeHidden()
      const nav = page.getByRole('navigation', { name: '移动端工作台导航' })
      await page.getByRole('button', { name: '导航菜单', exact: true }).click()
      await expect(nav.locator('[aria-current="page"]')).toHaveCount(1)
      await expect(nav.getByRole('button', { name: destination, exact: true })).toHaveAttribute('aria-current', 'page')
      await navigation.getByRole('button', { name: '关闭', exact: true }).click()
      await expect(navigation).toBeHidden()
      await expect(page.locator('[data-slot=loading-state]:visible')).toHaveCount(0)
      await expect(page.getByRole('button', { name: /^关闭设置$/ })).toHaveCount(0)
      await expectInsideViewport(page)
    }
    for (const viewport of [{ width: 390, height: 844 }, { width: 768, height: 844 }, { width: 844, height: 390 }]) {
      await page.setViewportSize(viewport)
      await expectInsideViewport(page)
    }
    await page.setViewportSize({ width, height: 844 })
    await capture(page, `settings-${width}`)
  })
}

for (const theme of ['dark', 'light']) {
  test(`mobile Game supports actions, readable history, and branching in ${theme}`, async ({ page, request }) => {
    test.setTimeout(60_000)
    await createMobileBook(request, `Mobile Game ${theme}`)
    const story = await createStartedStory(request, '旧车站：一段用于验证手机长标题布局的旅行故事')
    await page.setViewportSize({ width: 390, height: 844 })
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, async (route) => {
      const response = await route.fetch()
      const settings = await response.json()
      await route.fulfill({ response, json: { ...settings, effective: { ...settings.effective, theme } } })
    })
    await page.goto('/')
    // A fresh install opens the story surface; the journey starts from the game
    // shell it is testing.
    await expect(page.getByPlaceholder(/你要做什么/)).toBeVisible()
    await page.getByRole('button', { name: '导航菜单', exact: true }).click()
    await capture(page, `navigation-${theme}-390`)
    await page.getByRole('dialog', { name: '导航菜单', exact: true }).getByRole('button', { name: '设置', exact: true }).click()
    await expect(page.locator('.nova-mobile-topbar').getByRole('heading', { name: '设置', exact: true })).toBeVisible()
    await expect(page.getByRole('dialog', { name: '导航菜单', exact: true })).toBeHidden()
    await capture(page, `settings-${theme}-390`)
    await navigate(page, '游戏')
    const composer = page.getByPlaceholder(/你要做什么/)
    await expect(composer).toBeVisible()
    // One title bar owns navigation and story context; all other height belongs
    // to the conversation and composer, retaining 44px touch targets.
    for (const width of [320, 390]) {
      await page.setViewportSize({ width, height: 844 })
      for (const selector of ['.nova-mobile-topbar']) {
        const bounds = await page.locator(selector).boundingBox()
        expect(bounds!.height, `${selector} must leave room for the conversation`).toBeLessThanOrEqual(49)
      }
      for (const label of ['导航菜单', '选择故事线', '显示游戏控制台']) {
        const bounds = await page.getByRole('button', { name: label, exact: true }).boundingBox()
        expect(bounds!.height).toBeGreaterThanOrEqual(44)
        expect(bounds!.width).toBeGreaterThanOrEqual(44)
      }
      await expectInsideViewport(page)
    }
    const inputBounds = await composer.boundingBox()
    expect(inputBounds!.width).toBeGreaterThan(300)
    await composer.fill('推开石门')
    await composer.press('Enter')
    await composer.pressSequentially('观察灯光')
    expect((await getStorySnapshot(request, story.id)).turns).toHaveLength(1)
    await page.getByRole('button', { name: '发送', exact: true }).click()
    await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns).toHaveLength(2)
    await expect(page.getByText('石门缓缓开启，暖色灯光照亮了前方的旧车站。')).toBeVisible()
    await expect(page.getByRole('button', { name: '中断 AI 执行', exact: true })).toBeHidden()
    await expect(page.locator('[data-state-panel-mode=collapsed]')).toBeVisible()
    await capture(page, `game-${theme}-390`)
    await page.getByRole('button', { name: '选择故事线', exact: true }).click()
    await page.getByRole('button', { name: '剧情轮次导航', exact: true }).click()
    const history = page.getByRole('dialog', { name: '剧情轮次导航', exact: true })
    await expect(history.getByRole('button', { name: /跳转到第/ })).toHaveCount(2)
    await history.getByRole('button', { name: '跳转到第 1 轮', exact: true }).click()
    await expect(history).toBeHidden()
    await page.getByRole('button', { name: '获取行动选择', exact: true }).click()
    await page.getByText('走进旧车站', { exact: true }).click()
    await expect(composer).toContainText('走进旧车站')
    await composer.fill('')
    await page.getByRole('button', { name: '消息操作', exact: true }).last().click()
    await page.getByRole('button', { name: '从此处创建分支', exact: true }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByLabel('剧情线名称').fill('手机支线')
    await dialog.getByRole('button', { name: '创建并切换', exact: true }).click()
    await expect.poll(async () => getStoryBranches(request, story.id)).toContainEqual(expect.objectContaining({ title: '手机支线', current: true }))
    await page.getByRole('button', { name: '显示游戏控制台', exact: true }).click()
    const consoleSheet = page.getByRole('dialog', { name: '故事控制台', exact: true })
    await expect(consoleSheet).toBeVisible()
    await expect(consoleSheet).toHaveAttribute('data-side', 'right')
    await expect(consoleSheet.getByRole('button', { name: '关闭', exact: true })).toBeFocused()
    await consoleSheet.getByRole('tab', { name: /路线/ }).click()
    await capture(page, `console-${theme}-390`)
    await consoleSheet.getByRole('button', { name: '打开完整分支路线', exact: true }).click()
    await expect(consoleSheet).toBeHidden()
    await expect(page.getByRole('button', { name: '返回剧情', exact: true })).toBeVisible()
    await expect(page.getByRole('combobox', { name: '分支路线', exact: true })).toBeVisible()
    await expect(page.getByText('当前节点', { exact: true })).toHaveCount(1)
    await capture(page, `branches-${theme}-390`)
    await page.getByRole('button', { name: '返回剧情', exact: true }).click()
    await page.getByRole('button', { name: '显示游戏控制台', exact: true }).click()
    await page.keyboard.press('Escape')
    await expect(consoleSheet).toBeHidden()
    await page.setViewportSize({ width: 1440, height: 1000 })
    await expect(page.getByLabel('工作台侧边栏')).toBeVisible()
    await expect(page.locator('[data-nova-mobile-shell="true"]')).toHaveCount(0)
    await expect(page.locator('.nova-story-stage-header')).toBeVisible()
    await capture(page, `desktop-game-${theme}`)
  })
}

test('the compact shell fills the PWA viewport and recovers from stale keyboard measurements', async ({ page, request }) => {
  await createMobileBook(request, 'Mobile Keyboard')
  await createStartedStory(request, 'Keyboard story')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  await navigate(page, '游戏')
  const composer = page.getByPlaceholder(/你要做什么/)
  await composer.fill('草稿')
  await page.evaluate(() => {
    Object.defineProperty(window.visualViewport, 'height', { configurable: true, value: 460 })
    window.visualViewport!.dispatchEvent(new Event('resize'))
  })
  await expect(page.locator('[data-nova-mobile-shell="true"]')).toHaveCSS('height', '460px')
  await expect(page.locator('.nova-mobile-nav')).toBeHidden()
  const bounds = await page.getByRole('button', { name: '发送', exact: true }).boundingBox()
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(460)
  await expect(page.locator('.nova-story-input-float')).toHaveCSS('bottom', '0px')
  await page.evaluate(() => {
    delete (window.visualViewport as unknown as { height?: number }).height
    window.visualViewport!.dispatchEvent(new Event('resize'))
  })
  await expect.poll(async () => (await page.locator('[data-nova-mobile-shell="true"]').boundingBox())!.height).toBeCloseTo(844, 0)
  await expect(composer).toHaveText('草稿')
  // PWA resumption may report a shorter visual viewport even without a keyboard.
  await page.getByRole('button', { name: '选择故事线', exact: true }).focus()
  await page.evaluate(() => {
    Object.defineProperty(window.visualViewport, 'height', { configurable: true, value: 700 })
    Object.defineProperty(window.visualViewport, 'offsetTop', { configurable: true, value: 40 })
    window.dispatchEvent(new Event('pageshow'))
  })
  await expect.poll(async () => (await page.locator('[data-nova-mobile-shell="true"]').boundingBox())!.height).toBeCloseTo(844, 0)
  await expect(page.locator('[data-nova-mobile-shell="true"]')).toHaveCSS('top', '0px')
  const shell = await page.locator('[data-nova-mobile-shell="true"]').boundingBox()
  expect(shell!.y + shell!.height).toBeCloseTo(844, 0)
  const input = await page.locator('.nova-agent-composer').boundingBox()
  expect(844 - input!.y - input!.height).toBeLessThanOrEqual(12)
})

test('mobile story setup keeps start reachable and creates a custom protagonist story', async ({ page, request }) => {
  await createMobileBook(request, 'Mobile Story Setup')
  await page.setViewportSize({ width: 320, height: 740 })
  await page.goto('/')
  await navigate(page, '游戏')
  await page.getByRole('button', { name: '选择故事线', exact: true }).click()
  await page.getByRole('button', { name: '新建', exact: true }).click()
  await expect(page.getByRole('heading', { name: '新故事线', exact: true })).toBeVisible()
  const advanced = page.getByRole('button', { name: '高级设置', exact: true })
  await expect(advanced).toHaveAttribute('aria-expanded', 'false')
  await advanced.click()
  await expect(advanced).toHaveAttribute('aria-expanded', 'true')
  // Match the deterministic model's fixed schema and two submitted choices.
  await page.getByRole('combobox', { name: '状态结构', exact: true }).click()
  await page.getByRole('option', { name: '固定使用模板', exact: true }).click()
  await page.getByRole('spinbutton', { name: '候选行动数', exact: true }).fill('2')
  await advanced.click()
  await page.getByRole('radio', { name: '自定义角色', exact: true }).check()
  await page.getByRole('textbox', { name: '角色名称', exact: true }).fill('林川')
  await page.getByRole('textbox', { name: '角色设定', exact: true }).fill('一位来到旧车站的旅人，寻找失散的同伴。')
  await page.getByPlaceholder('可选。写下必须保留的场景、人物处境或第一事件；留空则自动生成。').fill('夜色降临，我走到旧车站门口。')
  await capture(page, 'new-story-320')
  const start = page.getByRole('button', { name: '开始故事', exact: true })
  await expect(start).toBeEnabled()
  const bounds = await start.boundingBox()
  expect(bounds!.y + bounds!.height).toBeLessThan(740)
  await start.click()
  await expect(page.getByPlaceholder(/你要做什么/)).toBeVisible()
  await expect(page.getByRole('button', { name: '中断 AI 执行', exact: true })).toBeHidden()
  await expect(page.getByText('暮色落在旧车站外，石门后的轨道传来遥远的回声。', { exact: true }).first()).toBeVisible()
  await expect(page.getByRole('heading', { name: '新故事线', exact: true })).toBeHidden()
  await expectInsideViewport(page)
})

test('mobile navigation and pane dismissal use English consistently', async ({ page, request }) => {
  await createMobileBook(request, 'English Mobile Workspace')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, async (route) => {
    const response = await route.fetch()
    const settings = await response.json()
    await route.fulfill({ response, json: { ...settings, effective: { ...settings.effective, language: 'en-US' } } })
  })
  await page.goto('/')
  await expect(page.locator('.nova-mobile-topbar')).toBeVisible()
  await page.getByRole('button', { name: 'Navigation', exact: true }).click()
  const navigation = page.getByRole('dialog', { name: 'Navigation', exact: true })
  for (const name of ['Game', 'Lore', 'World packs', 'Settings']) {
    await expect(navigation.getByRole('button', { name, exact: true })).toBeVisible()
  }
  await capture(page, 'navigation-en-390')
})

test('mobile edge gestures open the navigation and console without taking vertical scrolling', async ({ page, request }) => {
  await createMobileBook(request, 'Mobile Drawers')
  await createStartedStory(request, 'Drawer story')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  await navigate(page, '游戏')
  await expect(page.getByPlaceholder(/你要做什么/)).toBeVisible()
  await swipe(page, [12, 220], [16, 330])
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await swipe(page, [12, 220], [120, 225])
  const navigation = page.getByRole('dialog', { name: '导航菜单', exact: true })
  await expect(navigation).toBeVisible()
  await expect(navigation).toHaveAttribute('data-side', 'left')
  await expect(navigation.getByRole('button', { name: '命令', exact: true })).toBeVisible()
  // The game drawer carries 游戏 / 资料库 / 世界包 plus 设置 and the world switch.
  await expect(navigation.getByRole('button', { name: /切换世界/ })).toBeVisible()
  await swipe(page, [180, 24], [60, 24])
  await expect(navigation).toBeHidden()
  await page.getByPlaceholder(/你要做什么/).fill('剧情输入草稿')
  await swipe(page, [378, 220], [270, 225])
  const consoleSheet = page.getByRole('dialog', { name: '故事控制台', exact: true })
  await expect(consoleSheet).toBeVisible()
  await expect(consoleSheet).toHaveAttribute('data-side', 'right')
  await swipe(page, [170, 24], [290, 24])
  await expect(consoleSheet).toBeHidden()
  await expect(page.getByPlaceholder(/你要做什么/)).toHaveText('剧情输入草稿')
  await page.setViewportSize({ width: 1440, height: 1000 })
  await expect(page.getByPlaceholder(/你要做什么/)).toHaveText('剧情输入草稿')
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByPlaceholder(/你要做什么/)).toHaveText('剧情输入草稿')
})

test('cached destinations contribute only the active title bar and preserve the game draft', async ({ page, request }) => {
  await createMobileBook(request, 'Mobile Header Ownership')
  await createStartedStory(request, 'Header story')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  await navigate(page, '游戏')
  await page.getByPlaceholder(/你要做什么/).fill('保留在游戏里的草稿')
  await navigate(page, '设置')
  const header = page.locator('.nova-mobile-topbar')
  await expect(header.locator('[data-mobile-header-content]')).toHaveCount(1)
  await expect(header.getByRole('heading', { name: '设置', exact: true })).toBeVisible()
  await expect(page.locator('.nova-settings-view [data-slot="feature-page-header"]')).toBeHidden()
  await expect(page.getByRole('button', { name: '关闭设置', exact: true })).toHaveCount(0)
  const categories = header.getByRole('button', { name: '设置分类', exact: true })
  const content = page.locator('[data-nova-settings-content]')
  await expect(content.getByRole('button', { name: '设置分类', exact: true })).toHaveCount(0)
  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 })
    const bounds = (await categories.boundingBox())!
    expect(bounds.width).toBeGreaterThanOrEqual(44)
    expect(bounds.height).toBeGreaterThanOrEqual(44)
    expect(bounds.x).toBeGreaterThan(width - 72)
    await categories.focus()
    await categories.press('Enter')
    const drawer = page.getByRole('dialog', { name: '设置分类', exact: true })
    await expect(drawer).toHaveAttribute('data-side', 'right')
    await drawer.getByRole('button', { name: '路径', exact: true }).click()
    await expect(drawer).toBeHidden()
    await expect(content.getByRole('button', { name: /公共配置\s*路径/ })).toBeInViewport()
    await expect(categories).toBeFocused()
    await expect(categories).toHaveAttribute('aria-expanded', 'false')
    await categories.click()
    await expect(drawer.getByRole('button', { name: '路径', exact: true })).toHaveAttribute('aria-current', 'page')
    await drawer.getByRole('button', { name: '关闭', exact: true }).click()
    await expect(drawer).toBeHidden()
  }
  await navigate(page, '资料库')
  await expect(header.locator('[data-mobile-header-content]')).toHaveCount(1)
  await navigate(page, '游戏')
  await expect(header.locator('[data-mobile-header-content]')).toHaveCount(1)
  await expect(page.getByPlaceholder(/你要做什么/)).toHaveText('保留在游戏里的草稿')
  await page.getByRole('button', { name: '导航菜单', exact: true }).click()
  await page.getByRole('dialog', { name: '导航菜单', exact: true }).getByRole('button', { name: '命令', exact: true }).click()
  const commands = page.getByRole('dialog', { name: '命令面板', exact: true })
  await expect(page.getByRole('dialog', { name: '导航菜单', exact: true })).toBeHidden()
  await commands.getByRole('combobox').fill('搜索')
  await expect(commands.getByRole('option', { name: /打开全局搜索/ })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(commands).toBeHidden()
})

// 资料库 is a resource editor with a retained Agent pane, so its draft survives
// a view switch. 世界包 opens the world-pack library instead: it has no Agent
// pane, no 目录 drawer and no content/Agent tabs, so only 资料库 can carry this
// journey.
for (const destination of ['资料库']) {
  test(`mobile ${destination} retains its Agent draft across full-page views and primary navigation`, async ({ page, request }) => {
    await createMobileBook(request, `Resource ${destination}`)
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/')
    await navigate(page, destination)
    const header = page.locator('.nova-mobile-topbar')
    await expect(header.getByRole('tablist')).toHaveCount(1)
    const directoryButton = header.getByRole('button', { name: /目录$/ })
    const bounds = (await directoryButton.boundingBox())!
    expect(bounds.x).toBeGreaterThan(318)
    await directoryButton.click()
    const directory = page.getByRole('dialog')
    await expect(directory).toHaveAttribute('data-side', 'right')
    await directory.getByRole('button', { name: '关闭', exact: true }).click()
    await expect(directory).toBeHidden()
    await header.getByRole('tab', { name: 'Agent', exact: true }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    const composer = page.getByPlaceholder(/输入消息/)
    await expect(composer).toBeVisible()
    const composerBounds = (await composer.boundingBox())!
    expect(composerBounds.width).toBeGreaterThan(300)
    await composer.fill(`保留 ${destination} 的草稿`)
    const contentTab = header.getByRole('tab').first()
    await contentTab.click()
    await expect(composer).toBeHidden()
    await header.getByRole('tab', { name: 'Agent', exact: true }).click()
    await expect(composer).toHaveText(`保留 ${destination} 的草稿`)
    await navigate(page, '设置')
    await navigate(page, destination)
    await expect(composer).toHaveText(`保留 ${destination} 的草稿`)
    // Moving the retained Agent between a phone view and desktop split must preserve its input.
    await page.setViewportSize({ width: 1440, height: 1000 })
    await expect(page.locator('.nova-mobile-topbar')).toHaveCount(0)
    await page.setViewportSize({ width: 320, height: 740 })
    await expect(composer).toHaveText(`保留 ${destination} 的草稿`)
    await expectInsideViewport(page)
    await capture(page, `resource-${destination}-320`)
  })
}
