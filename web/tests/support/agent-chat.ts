import { expect, type Locator, type Page } from './fixtures'

/** Submits after the selected conversation's configuration is ready. */
export async function submitAgentChatMessage(page: Page, composer: Locator, message: string): Promise<void> {
  await composer.fill(message)
  await page.locator('[data-action="send"]').filter({ visible: true }).click()
}

/** Surface a terminal runtime error immediately instead of timing out on missing prose. */
export async function expectAgentChatReply(page: Page, text: string): Promise<void> {
  const reply = page.getByText(text, { exact: true }).filter({ visible: true })
  const errors = page.getByRole('alert').filter({ visible: true })
  await expect(reply.or(errors).first()).toBeVisible()
  expect(await errors.allTextContents(), 'Agent execution reported an error').toEqual([])
  await expect(reply).toHaveCount(1)
}
