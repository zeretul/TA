import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'

// Controlled projections exercise the real built UI and real local image routes.
// They are not evidence of a model spontaneously recognizing a character.
for (const dpr of [1, 2]) {
  test.describe(`portrait DPR ${dpr}`, () => {
    test.use({ deviceScaleFactor: dpr })
    for (const theme of ['dark', 'light']) {
      test(`keeps explicit image consent and identity in ${theme}`, async ({ page, request, browserDiagnostics }) => {
        await page.setViewportSize({ width: theme === 'dark' ? 1440 : 390, height: 1000 })
        await createAndOpenBook(request, `Portrait ${dpr} ${theme}`)
        const story = await createStartedStory(request, '画像访问验证')
        const settings = await (await request.get('/api/settings')).json()
        await request.patch('/api/settings', { data: { layer: 'user', base_revision: settings.revisions.user, changes: { theme, language: 'zh-CN' } } })
        const thumbnail = '/api/portraits/canon_chen_qiaoqian.png'
        const display = `${thumbnail}?size=display`
        const protectedURL = `/api/interactive/stories/${story.id}/media/denied-test-image?branch=main&head=stale&revision=1&actor=protected`
        const reads: string[] = []
        page.on('request', req => { if (req.resourceType() === 'image') reads.push(new URL(req.url()).pathname + new URL(req.url()).search) })
        browserDiagnostics.allow(/Failed to load resource:.*404.*denied-test-image/)
        await page.route(`**/api/interactive/stories/${story.id}/lore-overview*`, async route => {
          const response = await route.fetch()
          const body = await response.json()
          const person = { name: '同名人物', present: true, channel: 'scene', unread_messages: 0, open_matters: 0, affinity: 0, affinity_band: 'neutral' }
          body.characters.characters = [
            { ...person, id: 'author', source_key: 'canon_chen_qiaoqian', identity_source: 'author_catalog', avatar: thumbnail, avatar_display: display },
            { ...person, id: 'invented', identity_source: 'story' },
            { ...person, id: 'protected', name: '需另行查看的人物', identity_source: 'author_catalog', avatar_display: protectedURL, image_caption: '隔离测试画像' },
          ]
          await route.fulfill({ response, json: body })
        })
        await page.goto('/')
        const nav = page.getByLabel('工作台侧边栏')
        const menu = page.getByRole('button', { name: '导航菜单', exact: true })
        await expect.poll(async () => (await nav.isVisible()) || (await menu.isVisible())).toBe(true)
        if (await nav.isVisible()) await nav.getByRole('button', { name: '游戏', exact: true }).click()
        else {
          await page.getByRole('button', { name: '导航菜单', exact: true }).click()
          await page.getByRole('dialog', { name: '导航菜单', exact: true }).getByRole('button', { name: '游戏', exact: true }).click()
        }
        const trigger = page.locator('[data-lore-overview-trigger]')
        await trigger.click()
        const panel = page.getByTestId('lore-overview-panel')
        await panel.getByRole('tab', { name: '人物', exact: true }).click()
        await expect(panel.locator('[data-lore-entry="author"]')).toBeVisible()
        expect(reads).not.toContain(display)
        expect(reads).not.toContain(protectedURL)
        await panel.locator('[data-lore-entry="author"]').click()
        const portrait = panel.locator('img').first()
        await expect(portrait).toHaveAttribute('src', display)
        await expect.poll(() => portrait.evaluate(img => (img as HTMLImageElement).naturalWidth)).toBeGreaterThan(0)
        await page.screenshot({ path: test.info().outputPath(`portrait-${dpr}-${theme}.png`) })
        await panel.getByRole('button', { name: '返回', exact: true }).click()
        await panel.locator('[data-lore-entry="invented"]').click()
        await expect(panel.locator('img')).toHaveCount(0)
        await panel.getByRole('button', { name: '返回', exact: true }).click()
        await panel.locator('[data-lore-entry="protected"]').click()
        await expect(panel.locator('img')).toHaveCount(0)
        expect(reads).not.toContain(protectedURL)
        const toggle = panel.getByTestId('lore-overview-portrait-toggle')
        const publicReads = reads.filter(url => url.startsWith('/api/portraits/')).length
        await toggle.click()
        await expect.poll(() => reads.includes(protectedURL)).toBe(true)
        await expect(panel.locator('img')).toHaveCount(0)
        await expect(panel.getByText('隔离测试画像')).toHaveCount(0)
        expect(reads.filter(url => url.startsWith('/api/portraits/')).length).toBe(publicReads)
        await panel.getByRole('button', { name: '收起资料总览', exact: true }).click()
        await trigger.click()
        await panel.getByRole('tab', { name: '人物', exact: true }).click()
        await panel.locator('[data-lore-entry="protected"]').click()
        await expect(toggle).toHaveAttribute('aria-expanded', 'false')
        await expect(panel.locator('img')).toHaveCount(0)
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      })
    }
  })
}
