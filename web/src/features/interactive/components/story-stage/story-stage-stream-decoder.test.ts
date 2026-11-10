import { describe, expect, it } from 'vitest'
import { decodeInteractiveStreamEvent } from './story-stage-stream-decoder'

describe('interactive stream decoder', () => {
  it('keeps the reported tool outcome so a failed read is not shown as a success', () => {
    // 自制-叙事演绎 §13 NAR-13 分开正常未命中与真实系统故障. The consumer cannot draw
    // that line if the decoder drops the status the runtime already sent.
    expect(decodeInteractiveStreamEvent({
      event: 'tool_result',
      data: JSON.stringify({ id: 'read-1', name: 'read_lore_items', content: 'workspace unavailable', status: 'error' }),
    })).toMatchObject({ kind: 'handled', event: { type: 'tool_result', data: { id: 'read-1', status: 'error' } } })

    expect(decodeInteractiveStreamEvent({
      event: 'tool_result',
      data: JSON.stringify({ id: 'read-2', name: 'read_lore_items', content: 'no such entry', status: 'success' }),
    })).toMatchObject({ kind: 'handled', event: { type: 'tool_result', data: { id: 'read-2', status: 'success' } } })
  })

  it('rejects an outcome the runtime never reports', () => {
    expect(decodeInteractiveStreamEvent({
      event: 'tool_result',
      data: JSON.stringify({ id: 'read-3', content: 'x', status: 'failed' }),
    })).toMatchObject({ kind: 'invalid', type: 'tool_result' })
  })

  it('preserves validated payload metadata for handled events', () => {
    expect(decodeInteractiveStreamEvent({
      event: 'chunk',
      data: JSON.stringify({
        content: '继续前进',
        run_id: 'run-1',
        agent_kind: 'interactive_story',
        custom_trace_field: 'kept',
      }),
    })).toEqual({
      kind: 'handled',
      event: {
        type: 'chunk',
        data: {
          content: '继续前进',
          run_id: 'run-1',
          agent_kind: 'interactive_story',
          custom_trace_field: 'kept',
        },
      },
    })
  })

  it('accepts exact terminal SubAgent statuses and rejects non-terminal ones', () => {
    expect(decodeInteractiveStreamEvent({
      event: 'subagent_settled',
      data: JSON.stringify({
        status: 'blocked',
        reason: 'needs input',
        subagent: true,
        subagent_session_id: 'child-session',
      }),
    })).toMatchObject({
      kind: 'handled',
      event: {
        type: 'subagent_settled',
        data: { status: 'blocked', subagent_session_id: 'child-session' },
      },
    })
    expect(decodeInteractiveStreamEvent({
      event: 'subagent_settled',
      data: JSON.stringify({ status: 'running' }),
    })).toMatchObject({ kind: 'invalid', type: 'subagent_settled' })
  })

  it('isolates invalid JSON and schema violations', () => {
    expect(decodeInteractiveStreamEvent({ event: 'thinking', data: '{broken' })).toMatchObject({
      kind: 'invalid',
      type: 'thinking',
    })
    expect(decodeInteractiveStreamEvent({
      event: 'agent_cycle_started',
      data: JSON.stringify({ command_id: 'command-1' }),
    })).toMatchObject({
      kind: 'invalid',
      type: 'agent_cycle_started',
    })
  })

  it('does not parse intentionally ignored or unknown event payloads', () => {
    expect(decodeInteractiveStreamEvent({ event: 'workspace_change', data: '{ignored' })).toEqual({
      kind: 'ignored',
      type: 'workspace_change',
    })
    expect(decodeInteractiveStreamEvent({ event: 'future_event', data: '{unknown' })).toEqual({
      kind: 'unknown',
      type: 'future_event',
    })
  })
})
