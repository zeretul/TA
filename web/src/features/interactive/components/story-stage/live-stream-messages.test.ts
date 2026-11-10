import { describe, expect, it } from 'vitest'
import { createAgentToolMessage } from '@/lib/agent-ui-message'
import { buildAgentMessageViews } from '@/lib/agent-message-view'
import { completeToolMessage, toolMessageInput, toolResultFailed, updateToolMessageInput } from './live-stream-messages'

describe('live tool input', () => {
  it('uses the canonical input field for streaming and completed tool state', () => {
    const partialInput = '{"path":"chapters/ch01.md"'
    const completeInput = '{"path":"chapters/ch01.md","content":"draft"}'
    const streaming = createAgentToolMessage({
      id: 'tool-write',
      name: 'write',
      state: 'input-streaming',
      input: partialInput,
    })

    expect(toolMessageInput(streaming)).toBe(partialInput)

    const updated = updateToolMessageInput(streaming, completeInput)
    const completed = completeToolMessage(updated, 'done')

    expect(completed.parts[0]).toMatchObject({
      state: 'output-available',
      input: { path: 'chapters/ch01.md', content: 'draft' },
      output: 'done',
    })
    expect(completed.parts[0]).not.toHaveProperty('inputText')
  })
})

// 自制-叙事演绎 §8.4 资料库故障、模型失败和提交失败是技术问题，明确提示并保留恢复能力.
// The live stream used to complete every tool call as 'output-available' no matter
// what the runtime reported, so a failed read looked successful until the turn was
// persisted and re-projected.
describe('live tool failure', () => {
  const streamingTool = () => createAgentToolMessage({
    id: 'tool-read',
    name: 'read_lore_items',
    state: 'input-streaming',
    input: '{"ids":["lore_missing"]}',
  })

  it('marks a failed call as an error instead of a success', () => {
    const failed = completeToolMessage(streamingTool(), 'cannot read lore because the current workspace is unavailable', true)

    expect(failed.parts[0]).toMatchObject({
      state: 'output-error',
      errorText: 'cannot read lore because the current workspace is unavailable',
    })
    expect(failed.parts[0]).not.toHaveProperty('output')
    expect(buildAgentMessageViews([failed])[0]).toMatchObject({ kind: 'tool', status: 'error' })
  })

  it('keeps the same projection a reloaded turn produces', () => {
    // The persisted path builds 'output-error' from status === 'error'
    // (use-story-stage-messages.ts). Live and reloaded cards must agree, or the
    // story appears to heal itself after a refresh.
    const live = completeToolMessage(streamingTool(), 'cannot read lore', true)
    const persisted = createAgentToolMessage({
      id: 'tool-read',
      name: 'read_lore_items',
      state: 'output-error',
      input: '{"ids":["lore_missing"]}',
      errorText: 'cannot read lore',
    })

    expect(buildAgentMessageViews([live])[0]).toMatchObject({
      kind: 'tool',
      status: buildAgentMessageViews([persisted])[0].status,
      errorText: 'cannot read lore',
    })
  })

  it('only treats a genuine runtime error as a failure', () => {
    // A blocked or skipped call never ran. Reporting it as a broken library would
    // tell the player the material is gone when nothing was attempted.
    expect(toolResultFailed('error')).toBe(true)
    expect(toolResultFailed('success')).toBe(false)
    expect(toolResultFailed('blocked')).toBe(false)
    expect(toolResultFailed('skipped')).toBe(false)
    expect(toolResultFailed(undefined)).toBe(false)
  })
})
