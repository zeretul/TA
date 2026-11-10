import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory, getStorySnapshot } from '../support/api'

/**
 * 设计稿/Claude_返修优先与剧情助手续工_20261018.md:51:
 *
 *   "聊天与战斗浮窗目前只有左下角透明16px缩放区，测试只断言其存在。先正常页面复现用户
 *    问题，再修为可发现、易拖动的边/角缩放，真实验证拖拽前后宽高变化、内容布局、窄屏
 *    与长文本、最小化恢复和多窗不遮挡。拖窗不推进游戏、不重发、不丢草稿/在途回复。"
 *
 * 设计稿/战斗窗口_UI与Sonnet优化施工稿_20261018.md:56 adds: the resize edges must
 * have visible or hover feedback and a real hit area, the title and close stay in
 * the viewport, and the window must converge with the stage.
 *
 * The unit suite proves the geometry arithmetic. This spec proves the thing the
 * user actually reported: on a real page, dragging the affordance changes the
 * painted width and height, and none of the window gestures is a story action.
 */

const gameCharacterCastMarker = 'E2E_GAME_CHARACTER_CAST'

/**
 * Opens the Game destination. The sidebar is the desktop presentation; below
 * 1024px the same destinations live behind the 导航菜单 drawer, so the helper
 * takes the path the viewport actually offers instead of assuming one.
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

async function floatCharacterWindow(page: import('@playwright/test').Page) {
  await page.getByRole('button', { name: '打开人物窗口' }).click()
  const dialog = page.getByRole('dialog', { name: '人物' })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: '改为浮窗' }).click()
  await expect(dialog).toHaveAttribute('data-story-window', 'floating')
  return dialog
}

test('resizes a floating window by dragging a visible edge, and never as a story action', async ({ page, request }) => {
  // A desktop-sized stage, so the window has room to grow in both directions.
  // At the default 1280x720 the window already fills the stage's height, and a
  // downward drag would prove only that the clamp works, not that resizing does.
  await page.setViewportSize({ width: 1400, height: 1000 })
  await createAndOpenBook(request, 'Floating window resize')
  const story = await createStartedStory(request, 'Floating window resize story')
  const turnsBefore = (await getStorySnapshot(request, story.id)).turns.length

  // Any story turn would arrive as a chat request. Window gestures must not
  // produce one, so the test watches the wire rather than only the turn count.
  const chatRequests: string[] = []
  page.on('request', (request) => {
    if (/\/api\/interactive\/chat$/.test(new URL(request.url()).pathname)) chatRequests.push(request.url())
  })

  await openGame(page)
  const dialog = await floatCharacterWindow(page)

  // The affordances are visible and identifiable, not an anonymous transparent
  // box: a corner grip plus a left and a bottom edge.
  const corner = dialog.locator('[data-window-resize-handle="corner"]')
  const leftEdge = dialog.locator('[data-window-resize-handle="left"]')
  const bottomEdge = dialog.locator('[data-window-resize-handle="bottom"]')
  await expect(corner).toBeVisible()
  await expect(leftEdge).toBeVisible()
  await expect(bottomEdge).toBeVisible()
  await expect(corner).toHaveAttribute('title', '调整窗口大小')
  // The grip is drawn, so it can be found without hovering the exact pixel.
  await expect(corner.locator('svg')).toBeVisible()

  // A draft in the main composer must survive every window gesture.
  const composer = page.getByPlaceholder(/你要做什么/)
  await composer.fill('这是尚未发送的草稿')

  const before = await dialog.boundingBox()
  expect(before).not.toBeNull()
  // A window can only grow as far as the stage's bottom edge, and the default
  // height already fills much of it, so the honest expectation is the smaller
  // of the drag and the room the stage still has.
  const stage = await dialog.evaluate((element) => ({
    width: element.parentElement?.clientWidth ?? 0,
    height: element.parentElement?.clientHeight ?? 0,
  }))
  const roomToGrow = stage.height - 16 - before!.height
  expect(roomToGrow).toBeGreaterThan(40)

  // Drag the corner outward: the painted width and height both grow.
  const cornerBox = await corner.boundingBox()
  expect(cornerBox).not.toBeNull()
  await page.mouse.move(cornerBox!.x + cornerBox!.width / 2, cornerBox!.y + cornerBox!.height / 2)
  await page.mouse.down()
  await page.mouse.move(cornerBox!.x + cornerBox!.width / 2 - 120, cornerBox!.y + cornerBox!.height / 2 + 90, { steps: 8 })
  await page.mouse.up()

  const after = await dialog.boundingBox()
  expect(after!.width).toBeGreaterThan(before!.width + 100)
  expect(after!.height).toBeCloseTo(before!.height + Math.min(90, roomToGrow), 0)
  // The right edge is anchored, so growing the window did not slide it off the
  // stage; the header controls are still on screen.
  await expect(dialog.getByRole('button', { name: '停靠回侧栏' })).toBeVisible()
  await expect(dialog.getByRole('button', { name: '最小化' })).toBeVisible()
  await expect(dialog.getByRole('button', { name: '关闭人物窗口' })).toBeVisible()

  // The left edge alone changes the width, not the height.
  const leftBox = await leftEdge.boundingBox()
  const beforeLeftEdge = await dialog.boundingBox()
  await page.mouse.move(leftBox!.x + leftBox!.width / 2, leftBox!.y + leftBox!.height / 2)
  await page.mouse.down()
  await page.mouse.move(leftBox!.x + leftBox!.width / 2 - 60, leftBox!.y + leftBox!.height / 2, { steps: 6 })
  await page.mouse.up()
  const afterLeftEdge = await dialog.boundingBox()
  expect(afterLeftEdge!.width).toBeGreaterThan(beforeLeftEdge!.width + 50)
  expect(afterLeftEdge!.height).toBeCloseTo(beforeLeftEdge!.height, 0)

  // The bottom edge alone changes the height, not the width.
  const bottomBox = await bottomEdge.boundingBox()
  const beforeBottomEdge = await dialog.boundingBox()
  await page.mouse.move(bottomBox!.x + bottomBox!.width / 2, bottomBox!.y + bottomBox!.height / 2)
  await page.mouse.down()
  await page.mouse.move(bottomBox!.x + bottomBox!.width / 2, bottomBox!.y + bottomBox!.height / 2 + 60, { steps: 6 })
  await page.mouse.up()
  const afterBottomEdge = await dialog.boundingBox()
  expect(afterBottomEdge!.height).toBeGreaterThan(beforeBottomEdge!.height + 50)
  expect(afterBottomEdge!.width).toBeCloseTo(beforeBottomEdge!.width, 0)

  // The panel inside still renders its own content after the geometry changed.
  await expect(dialog).toContainText('人物')

  // Minimizing and restoring keeps the size the player chose.
  const resized = await dialog.boundingBox()
  await dialog.getByRole('button', { name: '最小化' }).click()
  await expect(page.getByTestId('story-window-minimized-entry')).toBeVisible()
  await page.getByTestId('story-window-minimized-entry').click()
  await expect(dialog).toHaveAttribute('data-story-window', 'floating')
  const restored = await dialog.boundingBox()
  expect(Math.round(restored!.width)).toBe(Math.round(resized!.width))
  expect(Math.round(restored!.height)).toBe(Math.round(resized!.height))

  // Dragging by the header moves the window without changing its size.
  const header = dialog.locator('header').first()
  const headerBox = await header.boundingBox()
  const beforeDrag = await dialog.boundingBox()
  await page.mouse.move(headerBox!.x + 12, headerBox!.y + headerBox!.height / 2)
  await page.mouse.down()
  await page.mouse.move(headerBox!.x + 12 - 80, headerBox!.y + headerBox!.height / 2 + 40, { steps: 6 })
  await page.mouse.up()
  const afterDrag = await dialog.boundingBox()
  expect(afterDrag!.x).toBeLessThan(beforeDrag!.x - 60)
  expect(afterDrag!.y).toBeGreaterThan(beforeDrag!.y + 20)
  expect(Math.round(afterDrag!.width)).toBe(Math.round(beforeDrag!.width))

  // None of the gestures was a story action: the draft is still there, no turn
  // was sent, and no request went out.
  await expect(composer).toHaveText('这是尚未发送的草稿')
  expect(chatRequests).toEqual([])
  expect((await getStorySnapshot(request, story.id)).turns).toHaveLength(turnsBefore)
})

test('keeps two floating windows apart and converges with the stage', async ({ page, request }) => {
  await createAndOpenBook(request, 'Two floating windows')
  await createStartedStory(request, 'Two floating windows story')

  await openGame(page)
  const character = await floatCharacterWindow(page)

  await page.getByRole('button', { name: '打开战斗窗口' }).click()
  const combat = page.getByRole('dialog', { name: '战斗' })
  await expect(combat).toBeVisible()
  await combat.getByRole('button', { name: '改为浮窗' }).click()
  await expect(combat).toHaveAttribute('data-story-window', 'floating')

  // The second window is offset, so neither header is buried under the other.
  const characterBox = await character.boundingBox()
  const combatBox = await combat.boundingBox()
  expect(combatBox!.x).toBeLessThan(characterBox!.x)
  expect(combatBox!.y).toBeGreaterThan(characterBox!.y)
  // Both headers stay reachable: each window's own controls are visible.
  await expect(character.getByRole('button', { name: '最小化' })).toBeVisible()
  await expect(combat.getByRole('button', { name: '最小化' })).toBeVisible()

  // Narrowing the stage pulls the windows back inside it instead of leaving
  // their headers off screen.
  await page.setViewportSize({ width: 1100, height: 700 })
  for (const dialog of [character, combat]) {
    const box = await dialog.boundingBox()
    expect(box!.x).toBeGreaterThanOrEqual(0)
    expect(box!.x + box!.width).toBeLessThanOrEqual(1100)
    await expect(dialog.getByRole('button', { name: '最小化' })).toBeInViewport()
  }
})

test('uses the sheet, not a floating frame, on a narrow screen', async ({ page, request }) => {
  await createAndOpenBook(request, 'Narrow screen windows')
  await createStartedStory(request, 'Narrow screen windows story')
  await page.setViewportSize({ width: 420, height: 820 })

  await openGame(page)
  await page.getByRole('button', { name: '打开人物窗口' }).click()
  const dialog = page.getByRole('dialog', { name: '人物' })
  await expect(dialog).toBeVisible()
  // The sheet is the narrow-screen presentation: there is no floating frame and
  // therefore no resize handle to hunt for.
  await expect(page.locator('[data-story-window="floating"]')).toHaveCount(0)
  await expect(page.locator('[data-window-resize-handle="corner"]')).toHaveCount(0)
  // Its own close affordance is still reachable.
  await expect(dialog.getByRole('button', { name: '关闭人物窗口' })).toBeVisible()
})

test('keeps long panel content scrollable inside the smallest window', async ({ page, request }) => {
  await createAndOpenBook(request, 'Long content window')
  const story = await createStartedStory(request, 'Long content window story')

  // The roster has to be real content the story committed, not a fixture list
  // injected into the page: the point of this test is that a genuinely long
  // panel body scrolls instead of pushing its own header out of the frame.
  const cast = await request.post('/api/interactive/chat', { data: {
    command_id: `cast-${story.id}`, mode: 'story', story_id: story.id, branch: 'main',
    message: `我走进候车厅，环视四周的人。 ${gameCharacterCastMarker}`,
  } })
  expect(cast.ok(), await cast.text()).toBe(true)
  await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns).toHaveLength(2)

  await openGame(page)
  const dialog = await floatCharacterWindow(page)
  await expect(dialog.locator('[data-testid="character-window-roster-row"]')).toHaveCount(8)

  // Shrink to the minimum from the corner, then the far corner again: the
  // geometry clamps instead of collapsing, and the panel scrolls its own body.
  const corner = dialog.locator('[data-window-resize-handle="corner"]')
  const cornerBox = await corner.boundingBox()
  await page.mouse.move(cornerBox!.x + cornerBox!.width / 2, cornerBox!.y + cornerBox!.height / 2)
  await page.mouse.down()
  await page.mouse.move(cornerBox!.x + 600, cornerBox!.y - 600, { steps: 10 })
  await page.mouse.up()

  const box = await dialog.boundingBox()
  expect(box!.width).toBeGreaterThanOrEqual(300)
  expect(box!.height).toBeGreaterThanOrEqual(240)
  // The header's controls still fit and the panel body scrolls rather than
  // pushing the header out of the frame.
  await expect(dialog.getByRole('button', { name: '停靠回侧栏' })).toBeInViewport()
  const scrolls = await dialog.evaluate((element) => {
    const overflowing = Array.from(element.querySelectorAll('*')).filter((node) => {
      const style = getComputedStyle(node)
      return (style.overflowY === 'auto' || style.overflowY === 'scroll') && node.scrollHeight > node.clientHeight
    })
    return overflowing.length
  })
  expect(scrolls).toBeGreaterThan(0)
  // Scrolling that body actually reveals the rest of the cast.
  const lastRow = dialog.locator('[data-testid="character-window-roster-row"]').last()
  await lastRow.scrollIntoViewIfNeeded()
  await expect(lastRow).toBeInViewport()
})
