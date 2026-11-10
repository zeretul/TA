import { execFileSync } from 'node:child_process'
import { expect, test, type APIRequestContext, type Page } from '../support/fixtures'
import { createAndOpenBook, createProjectFile, createStartedStory, getStorySnapshot } from '../support/api'
import { submitAgentChatMessage } from '../support/agent-chat'
import { modelControlURL } from '../support/model'

// Run with TA_TEST_CODEX_EXE. No product API or runtime event is mocked.
// The ordinary core journey specs use the same installed runtime in this mode.
test.skip(!process.env.TA_TEST_CODEX_EXE, 'Installed Codex acceptance is opt-in')
test.beforeAll(async ({}, info) => {
  if (process.env.TA_TEST_CODEX_EXE) await info.attach('runtime-version', {
    body: execFileSync(process.env.TA_TEST_CODEX_EXE, ['--version']), contentType: 'text/plain',
  })
})

type WireRequest = { prompt_cache_key?: string; input?: unknown[]; messages?: unknown[]; tools?: Array<{ name: string }> }

async function wire(request: APIRequestContext, marker: string): Promise<WireRequest[]> {
  const response = await request.get(`${modelControlURL}/control/runtime-requests?marker=${encodeURIComponent(marker)}`)
  expect(response.ok()).toBe(true)
  return response.json()
}

async function openProduct(page: Page) {
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const composer = page.getByPlaceholder(/你要做什么/)
  await expect(composer).toBeVisible()
  return composer
}

async function settled(page: Page) {
  await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)
}

test('game uses Codex automatic compaction and resumes the compacted thread', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Codex automatic game')
  await createProjectFile(request, book.projectId, 'runtime-evidence.txt', 'Runtime evidence is verified.')
  await createStartedStory(request, 'Automatic runtime context')
  const marker = 'E2E_RUNTIME_AUTO_GAME'
  await page.goto('/')
  const composer = await openProduct(page)
  await submitAgentChatMessage(page, composer, marker)
  await expect(page.getByText(`${marker} completed after compaction.`, { exact: true })).toBeVisible()
  await settled(page)
  await expect(page.getByText('自动压缩', { exact: true })).toBeVisible()
  await expect(page.getByText('运行时已压缩工作上下文，会话历史完整保留。', { exact: true })).toBeVisible()
  const before = await wire(request, marker)
  const compactIndex = before.findIndex(call => JSON.stringify(call.input?.at(-1)).includes('CONTEXT CHECKPOINT COMPACTION'))
  expect(compactIndex).toBeGreaterThan(0)
  expect(JSON.stringify(before[compactIndex]?.input)).toContain('Runtime evidence is verified.')
  expect(before.at(-1)?.prompt_cache_key).toBe(before[0]?.prompt_cache_key)
  expect(JSON.stringify(before.at(-1)?.input)).toContain('read evidence verified')
  await page.reload()
  await submitAgentChatMessage(page, await openProduct(page), `${marker} CONTINUE_AFTER_COMPACT`)
  await expect(page.getByText(`${marker} continued.`, { exact: true })).toBeVisible()
  await settled(page)
  const after = await wire(request, marker)
  expect(after.at(-1)?.prompt_cache_key).toBe(before[0]?.prompt_cache_key)
  expect(JSON.stringify(after.at(-1)?.input)).toContain('read evidence verified')
  await expect(page.getByText('自动压缩', { exact: true })).toBeVisible()
  await expect(page.getByText(/^正在执行/)).toHaveCount(0)
  await page.getByText('自动压缩', { exact: true }).scrollIntoViewIfNeeded()
  await page.screenshot({ path: test.info().outputPath('game-codex-compaction.png') })
})

test('game restores the native Codex plan after reload', async ({ page, request }) => {
  await createAndOpenBook(request, 'Codex plan game')
  await createStartedStory(request, 'Native plan')
  const marker = 'E2E_RUNTIME_PLAN_GAME'
  await page.goto('/')
  await submitAgentChatMessage(page, await openProduct(page), marker)
  await expect(page.getByText(`${marker} planned.`, { exact: true })).toBeVisible()
  await settled(page)
  const calls = await wire(request, marker)
  expect(calls[0]?.tools?.some(tool => tool.name === 'update_plan')).toBe(true)
  expect(calls[0]?.tools?.some(tool => tool.name === 'todo')).toBe(false)
  await page.reload()
  await openProduct(page)
  await expect(page.locator('li').getByText('Verify the station evidence', { exact: true })).toBeVisible()
  await expect(page.getByText('1/1', { exact: true })).toBeVisible()
})

test('game preserves canonical history through Native, Codex, and Native', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Runtime history game')
  const story = await createStartedStory(request, 'Runtime switching')
  const binding = { mode: 'interactive', story_id: story.id, branch_id: 'main' }
  const configURL = `/api/projects/${book.projectId}/conversation-config`
  const query = new URLSearchParams(binding)
  const marker = 'E2E_RUNTIME_SWITCH_GAME'
  for (const [index, kind] of ['native', 'codex', 'native'].entries()) {
    const snapshot = await (await request.get(`${configURL}?${query}`)).json()
    const changed = await request.patch(configURL, { data: { binding, base_revision: snapshot.revision, changes: {
      runtime: kind === 'codex' ? { kind, codex: { profile_id: 'e2e-codex' } } : { kind },
    } } })
    expect(changed.ok(), await changed.text()).toBe(true)
    expect((await changed.json()).runtime_capabilities.goal).toBe(false)
    await page.goto('/')
    await submitAgentChatMessage(page, await openProduct(page), `${marker} STEP_${index + 1}`)
    await expect(page.getByText(`${marker} reply ${index + 1}.`, { exact: true })).toBeVisible()
    await settled(page)
  }
  const calls = await wire(request, marker)
  expect(calls.some(call => Array.isArray(call.input))).toBe(true)
  expect(calls.some(call => Array.isArray(call.messages))).toBe(true)
  await page.reload()
  await openProduct(page)
  for (const step of [1, 2, 3]) await expect(page.getByText(`${marker} reply ${step}.`, { exact: true })).toHaveCount(1)
  expect((await getStorySnapshot(request, story.id)).turns).toHaveLength(4)
})

test('game rejects Goal', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Codex Goal')
  const marker = 'E2E_RUNTIME_GOAL_GAME'
  const story = await createStartedStory(request, 'No game Goal')
  const goalURL = `/api/projects/${book.projectId}/conversation-goal`
  const unsupported = await request.post(goalURL, { data: { binding: { mode: 'interactive', story_id: story.id, branch_id: 'main' }, action: 'set', expected_revision: 0, objective: marker } })
  expect(unsupported.status()).toBe(400)
  expect(await unsupported.json()).toEqual({ error: '当前引擎不支持此操作或参数。' })
})
