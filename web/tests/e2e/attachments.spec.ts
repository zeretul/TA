import { expect, test, type Page } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { submitAgentChatMessage } from '../support/agent-chat'
import { createRequire } from 'node:module'

test('uploads, previews, sends, and restores image attachments in Game', async ({ page, request }) => {
  // This journey isolates encoded-byte admission and original-image recovery.
  // Image-pressure compaction has its own smaller-window journey.
  const original = await (await request.get('/api/settings')).json() as { user: { model_profiles: Array<Record<string, unknown>> } }
  const configured = await request.patch('/api/settings', { data: { layer: 'user', changes: {
    model_profiles: original.user.model_profiles.map(profile => ({ ...profile, context_window_tokens: 400_000 })),
  } } })
  expect(configured.ok(), await configured.text()).toBe(true)
  try {
    const sharp = createRequire(import.meta.url)('sharp') as typeof import('sharp').default
    // Identical pixels can have very different lossless encodings. This valid
    // image used to be rejected as >590K text tokens before reaching the model.
    const image = await sharp({ create: { width: 768, height: 768, channels: 3, background: '#165cbd' } })
      .png({ compressionLevel: 0 }).toBuffer()
    const secondImage = await sharp({ create: { width: 768, height: 768, channels: 3, background: '#e3aa22' } })
      .png({ compressionLevel: 0 }).toBuffer()
    expect(image.length).toBeGreaterThan(1_200_000)
    await createAndOpenBook(request, 'Attachment E2E Book')
    await createStartedStory(request, 'Attachment E2E Story')
    await page.goto('/')

    let composer = await openGame(page)
    await attachImage(page, 'story-stage', 'game-e2e.png', image)
    await expect(page.getByTestId('story-stage').getByRole('button', { name: '预览 game-e2e.png' })).toBeVisible()
    await submitAgentChatMessage(page, composer, 'Follow the image signal. E2E_GAME_IMAGE_ATTACHMENT')
    await expect(page.getByText('图像中的蓝色信标亮起，旧车站的侧门随之打开。', { exact: true })).toBeVisible()
    await expect(page.getByTestId('story-stage').getByTestId('sent-message-attachments')).toHaveCount(1)
    await previewImage(page, 'story-stage', 'game-e2e.png')

    await attachImage(page, 'story-stage', 'game-second.png', secondImage)
    await submitAgentChatMessage(page, composer, 'Compare both signals. E2E_GAME_IMAGE_ATTACHMENT E2E_TWO_IMAGES')
    await expect(page.getByText('图像中的蓝色信标亮起，旧车站的侧门随之打开。', { exact: true })).toHaveCount(2)

    await page.reload()
    composer = await openGame(page)
    await expect(page.getByTestId('story-stage').getByTestId('sent-message-attachments')).toHaveCount(2)
    await previewImage(page, 'story-stage', 'game-e2e.png')

    await submitAgentChatMessage(page, composer, 'Continue using the original image. E2E_GAME_IMAGE_ATTACHMENT')
    await expect(page.getByText('图像中的蓝色信标亮起，旧车站的侧门随之打开。', { exact: true })).toHaveCount(3)
  } finally {
    const restored = await request.patch('/api/settings', { data: { layer: 'user', changes: { model_profiles: original.user.model_profiles } } })
    expect(restored.ok(), await restored.text()).toBe(true)
  }
})

async function openGame(page: Page) {
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const composer = page.getByPlaceholder(/你要做什么/)
  await expect(composer).toBeVisible()
  // Wait for the story snapshot to bind the draft before adding files. The
  // composer shell can mount before the selected story finishes hydrating.
  await expect(page.getByTestId('story-stage').getByText('暮色落在旧车站外，石门后的轨道传来遥远的回声。', { exact: true })).toHaveCount(1)
  return composer
}

async function attachImage(page: Page, surface: 'right' | 'story-stage', name: string, image: Buffer) {
  await page.getByTestId(surface).getByLabel('添加文件', { exact: true }).setInputFiles({
    name,
    mimeType: 'image/png',
    buffer: image,
  })
}

async function previewImage(page: Page, surface: 'right' | 'story-stage', name: string) {
  await page.getByTestId(surface).getByRole('button', { name: `预览 ${name}` }).click()
  const dialog = page.getByRole('dialog', { name })
  await expect(dialog.getByRole('img', { name })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
}

test('shows a localized transport error in Game after reload', async ({ page, request }, testInfo) => {
  await createAndOpenBook(request, 'Image limit E2E Book')
  await createStartedStory(request, 'Image limit E2E Story')
  await page.goto('/')
  const error = '请求超过供应商的传输大小限制，请减少或缩小附件，或新建会话。'
  const composer = await openGame(page)
  await submitAgentChatMessage(page, composer, 'E2E_IMAGE_TRANSPORT_LIMIT')
  await expect(page.getByText(error, { exact: false }).filter({ visible: true }).first()).toBeVisible()
  await page.reload()
  await openGame(page)
  await expect(page.getByText(error, { exact: false }).filter({ visible: true }).first()).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('game-transport-error.png') })
})
