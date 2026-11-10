import { expect, test } from '@playwright/test'
import { cast } from './fixtures/lore-social-data'

for (const language of ['zh-CN', 'en-US']) for (const theme of ['dark', 'light']) for (const width of [390, 1440]) {
  test(`known social records ${language} ${theme} ${width}`, async ({ page }, info) => {
    const zh = language === 'zh-CN'
    const errors: string[] = []
    const writes: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    await page.route('**/api/**', async route => {
      if (route.request().method() !== 'GET') writes.push(route.request().method())
      const path = new URL(route.request().url()).pathname
      let body: unknown = {}
      if (path.endsWith('/characters')) body = cast
      else if (path.endsWith('/journey')) body = { story_id: 'social-fixture', branch_id: 'main', routes: [] }
      else if (path.endsWith('/quests')) body = { story_id: 'social-fixture', branch_id: 'main', tasks: [], available: [] }
      else if (path.endsWith('/public-events')) body = { story_id: 'social-fixture', branch_id: 'main', events: [] }
      await route.fulfill({ json: body })
    })
    await page.setViewportSize({ width, height: 900 })
    await page.goto(`/tests/fixtures/lore-social.html?language=${language}&theme=${theme}`)
    await page.getByRole('tab', { name: zh ? '势力' : 'Powers', exact: true }).click()
    await page.locator('[data-lore-entry="world:势力记录:青山书院"]').click()
    await expect(page.getByText(zh ? '已知人物' : 'Known people', { exact: true })).toBeVisible()
    await expect(page.getByText(/最后听说在城外/).first()).toBeVisible()
    await page.screenshot({ path: info.outputPath('faction.png') })
    await page.getByRole('button', { name: /很长的姓名与已经公开/ }).click()
    await expect(page.getByText(zh ? '最后获知位置' : 'Last known whereabouts', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: zh ? '返回列表' : 'Back to list', exact: true }).click()
    await page.getByRole('tab', { name: zh ? '人物' : 'People', exact: true }).click()
    await page.getByRole('button', { name: zh ? '人物关系' : 'Relationships', exact: true }).click()
    await expect(page.getByRole('group', { name: zh ? '试玩者的已知关系图' : 'Known relationships of 试玩者' })).toBeVisible()
    await page.screenshot({ path: info.outputPath('graph.png') })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.getByRole('button', { name: zh ? '下一组' : 'Next', exact: true }).click()
    await page.getByRole('button', { name: zh ? '关系列表' : 'List', exact: true }).click()
    await expect(page.getByText('一起办过一件事，明确以朋友相称。')).toHaveCount(9)
    await page.getByRole('button', { name: zh ? '关系图' : 'Graph', exact: true }).click()
    await page.getByRole('button', { name: zh ? '查看同行者6' : 'View 同行者6', exact: true }).click()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('button', { name: zh ? '关系图' : 'Graph', exact: true })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('button', { name: 'Reopen' })).toBeVisible()
    await page.getByRole('button', { name: 'Reopen' }).click()
    await expect(page.getByRole('tab', { name: zh ? '人物' : 'People', exact: true })).toBeVisible()
    expect(writes).toEqual([])
    expect(errors).toEqual([])
  })
}
