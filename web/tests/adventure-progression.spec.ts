import { expect, test } from '@playwright/test'
for (const family of ['space', 'wuxia']) for (const language of ['zh-CN', 'en-US']) for (const theme of ['dark', 'light']) for (const width of [390, 1440]) {
  test(`growth ${family} ${language} ${theme} ${width}`, async ({ page }, info) => {
    const english = language === 'en-US'
    const errors: string[] = []
    const writes: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('request', request => { if (request.url().includes('/api/') && request.method() !== 'GET') writes.push(request.method()) })
    await page.setViewportSize({ width, height: 900 })
    await page.goto(`/tests/fixtures/adventure-progression.html?family=${family}&language=${language}&theme=${theme}`)
    await expect(page.getByTestId('adventure-progression')).toBeVisible()
    await expect(page.getByTestId('adventure-progression')).toContainText(english ? 'game-specific adaptation' : '本游戏自制成长适配')
    await page.screenshot({ path: info.outputPath('growth-card.png') })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.getByRole('button', { name: english ? 'Collapse the battle window' : '收起战斗窗口' }).click()
    await expect(page.getByRole('button', { name: 'Reopen', exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Reopen', exact: true }).click()
    await expect(page.getByTestId('adventure-progression')).toBeVisible()
    await page.getByRole('button', { name: 'Character ledger', exact: true }).click()
    const ledger = page.locator('[data-state-panel-mode]')
    if (await ledger.getAttribute('data-state-panel-mode') === 'collapsed') await ledger.locator('header button[aria-expanded]').first().click()
    await expect(ledger).toContainText(family === 'space' ? '41 / 60' : '31 / 40')
    await page.screenshot({ path: info.outputPath('growth-ledger.png') })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(writes).toEqual([])
    expect(await page.evaluate(() => (window as Window & { progressionFixtureWrites?: string[] }).progressionFixtureWrites)).toEqual([])
    expect(errors).toEqual([])
  })
}

for (const family of ['space', 'wuxia']) {
  test(`growth ceiling ${family}`, async ({ page }, info) => {
    await page.setViewportSize({ width: 390, height: 900 })
    await page.goto(`/tests/fixtures/adventure-progression.html?family=${family}&language=en-US&theme=light&ceiling=1`)
    const card = page.getByTestId('adventure-progression')
    await expect(card).toContainText('Current growth ceiling reached. Exploration and everyday life can continue.')
    await expect(card).not.toContainText('Request refinement hours')
    await page.screenshot({ path: info.outputPath('growth-ceiling.png') })
  })
}
