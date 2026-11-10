import type { ContextAnalysis } from '../../src/lib/api'
import { expect, test, type APIRequestContext, type Page } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { submitAgentChatMessage } from '../support/agent-chat'
import { getCapturedModelRequest, type E2EModelRequest } from '../support/model'

const contextAnalysisProbe = '[Text Adventure context analysis probe]'
const gameFlowMarker = 'E2E_GAME_PROMPT_FLOW_PARITY'
const gameContextMarker = 'E2E_GAME_PROMPT_CONTEXT_PARITY'

test('keeps Agent prompts, context analysis, and real model input aligned for Game', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Agent Prompt Context E2E Book')
  await createStartedStory(request, 'Agent Prompt Context E2E Story')

  // The game build has no Agents settings surface; the same override is applied
  // through the workspace settings layer that surface used to write.
  const gamePrompt = { flow_prompt: gameFlowMarker, system_prompt: gameContextMarker }
  const configured = await request.patch(`/api/projects/${encodeURIComponent(book.projectId)}/settings`, {
    data: { layer: 'workspace', changes: { agent_prompts: { interactive_story: gamePrompt } } },
  })
  expect(configured.ok(), await configured.text()).toBe(true)
  await expect.poll(async () => projectWorkspacePrompts(request, book.projectId)).toEqual(gamePrompt)

  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const gameComposer = page.getByPlaceholder(/你要做什么/)
  await expect(gameComposer).toBeVisible()
  const gameAnalysis = await openContextAnalysis(page, '/api/interactive/chat/context-analysis')
  await page.keyboard.press('Escape')
  await submitAgentChatMessage(page, gameComposer, contextAnalysisProbe)
  await expect(page.getByText('石门缓缓开启，暖色灯光照亮了前方的旧车站。', { exact: true })).toBeVisible()
  const gameRequest = await waitForCapturedRequest(request, gameContextMarker)
  expectPromptContextParity(gameAnalysis, gameRequest, gamePrompt)
})

interface PromptConfiguration {
  flow_prompt: string
  system_prompt: string
}

interface ComparableModelMessage {
  role: string
  content: string
}

async function projectWorkspacePrompts(
  request: APIRequestContext,
  projectId: string,
): Promise<PromptConfiguration | undefined> {
  const response = await request.get(`/api/projects/${encodeURIComponent(projectId)}/settings`)
  const failureDetails = response.ok() ? undefined : await response.text()
  expect(response.ok(), failureDetails).toBe(true)
  const snapshot = await response.json() as {
    workspace?: { agent_prompts?: Record<string, PromptConfiguration | undefined> }
  }
  return snapshot.workspace?.agent_prompts?.interactive_story
}

async function openContextAnalysis(page: Page, endpoint: string): Promise<ContextAnalysis> {
  const responsePromise = page.waitForResponse((response) => (
    response.request().method() === 'POST' && new URL(response.url()).pathname.endsWith(endpoint)
  ))
  await page.getByRole('button', { name: '输入动作' }).filter({ visible: true }).click()
  await page.getByRole('menuitem', { name: '上下文分析', exact: true }).click()
  const response = await responsePromise
  const failureDetails = response.ok() ? undefined : await response.text()
  expect(response.ok(), failureDetails).toBe(true)
  await expect(page.getByRole('dialog', { name: '上下文分析' })).toBeVisible()
  return response.json()
}

async function waitForCapturedRequest(request: APIRequestContext, marker: string): Promise<E2EModelRequest> {
  await expect.poll(async () => getCapturedModelRequest(request, marker)).not.toBeNull()
  const captured = await getCapturedModelRequest(request, marker)
  if (captured === null) {
    throw new Error(`Model request with marker ${marker} disappeared after capture`)
  }
  return captured
}

function expectPromptContextParity(
  analysis: ContextAnalysis,
  modelRequest: E2EModelRequest,
  configured: PromptConfiguration,
): void {
  const messages = modelRequest.messages.map(comparableCapturedMessage)
  const systemPrompt = messages
    .filter((message) => message.role === 'system')
    .map((message) => message.content)
    .join('\n\n')
  const contextMessages = messages.filter((message) => message.role !== 'system')

  expect(systemPrompt).toBe(analysis.system_prompt)
  expect(normalizeRuntimeCapture(contextMessages)).toEqual(
    normalizeRuntimeCapture(
      analysis.context_messages.map((message) => ({ role: message.role, content: message.content })),
    ),
  )
  expect(analysis.system_prompt_parts.find((part) => part.id === 'builtin_base')?.content)
    .toContain(configured.flow_prompt)
  expect(analysis.system_prompt_parts.find((part) => part.id === 'agent_custom_rules')?.content)
    .toBe(`\n\n## Agent Custom Rules\n\n${configured.system_prompt}`)
}

function comparableCapturedMessage(message: E2EModelRequest['messages'][number]): ComparableModelMessage {
  const toolCalls = message.tool_calls || []
  if (toolCalls.length === 0) {
    expect(typeof message.content).toBe('string')
    return { role: message.role, content: message.content as string }
  }

  const renderedCalls = toolCalls.map((call, index) => {
    const name = call.function?.name?.trim() || 'unknown_tool'
    const id = call.id?.trim()
    const suffix = id ? ` (id: ${id})` : ''
    return `${index + 1}. ${name}${suffix}\narguments:\n${call.function?.arguments?.trim() || ''}`
  }).join('\n')
  const renderedToolCalls = `[工具调用]\n${renderedCalls}`
  const content = typeof message.content === 'string' ? message.content : ''
  return {
    role: message.role,
    content: content.trim() ? `${content.replace(/\n+$/, '')}\n\n${renderedToolCalls}` : renderedToolCalls,
  }
}

function normalizeRuntimeCapture(messages: ComparableModelMessage[]): ComparableModelMessage[] {
  return messages.map((message) => ({
    ...message,
    content: message.content.replace(
      /^- Captured at: [^\r\n]+$/m,
      '- Captured at: <turn timestamp>',
    ),
  }))
}
