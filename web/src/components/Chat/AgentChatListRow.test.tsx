import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { buildAgentMessageViews } from '@/lib/agent-message-view'
import { TrajectoryNavigationProvider } from '@/features/trajectory/trajectory-navigation'
import { AgentChatListRow, type AgentChatListItem } from './AgentChatListRow'
import { buildAgentChatListItems, isVisibleActiveReply } from './AgentMessageList'
import { buildAgentRunPresentation } from './agent-run-presentation'

beforeEach(async () => { await i18next.changeLanguage('en-US') })

it('keeps historical turn media actions available without allowing narrative mutations', () => {
  const view = buildAgentMessageViews([{
    id: 'old-reply', role: 'assistant', metadata: { turn_id: 'old-turn', agent_kind: 'interactive_story', display_phase: 'final' },
    parts: [{ type: 'text', text: 'An earlier scene', state: 'done' }],
  }])[0]
  const generateImage = vi.fn()
  const readAloud = vi.fn()
  const renderRow = (streaming: boolean) => (
    <AgentChatListRow item={{ kind: 'message', key: view.key, view, sourceIndex: 0 }}
      executionTimings={new Map()} isStreaming={streaming} tailFollowActive={false}
      activeTraceDisplay="collapsed" subAgentPresentation="card" highlightDialogue={false}
      canMutateMessage={() => false} onEditAssistantReply={vi.fn()} onRegenerateMessage={vi.fn()}
      onGenerateInteractiveImage={generateImage} onReadAloud={readAloud} />
  )
  const { rerender } = render(renderRow(false))
  fireEvent.click(screen.getByRole('button', { name: 'Generate interactive image' }))
  fireEvent.click(screen.getByRole('button', { name: i18next.t('speech.read') }))
  expect(generateImage).toHaveBeenCalledWith(view)
  expect(readAloud).toHaveBeenCalledWith(view)
  expect(screen.queryByRole('button', { name: 'Edit AI reply' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Regenerate this turn' })).not.toBeInTheDocument()

  rerender(renderRow(true))
  expect(screen.queryByRole('button', { name: 'Generate interactive image' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: i18next.t('speech.read') })).toBeInTheDocument()
})

function runItem(messages: AgentUIMessage[], active: boolean): AgentChatListItem {
  const run = buildAgentRunPresentation(buildAgentMessageViews(messages), 0, active)!
  return { kind: 'run', key: run.key, runId: run.runID, sections: run.sections, sourceIndex: 0 }
}

function row(item: AgentChatListItem, active: boolean, nextItem?: AgentChatListItem) {
  return <AgentChatListRow projectId="project-a" item={item} nextItem={nextItem} executionTimings={new Map()} isStreaming={active} tailFollowActive={active} activeTraceDisplay="collapsed" subAgentPresentation="card" highlightDialogue={false} />
}

it.each(['ide', 'interactive_story'])('shows one Run reference only after the %s output stops', (agentKind) => {
  const open = vi.fn()
  const messages: AgentUIMessage[] = [{
    id: 'thinking', role: 'assistant', metadata: { run_id: 'current-run', agent_kind: agentKind },
    parts: [{ type: 'reasoning', text: 'Working through the request', state: 'done' }],
  }]
  const renderRun = (active: boolean) => (
    <TrajectoryNavigationProvider value={{ enabled: true, intent: null, open }}>
      {row(runItem(messages, active), active)}
    </TrajectoryNavigationProvider>
  )
  const { rerender } = render(renderRun(true))
  expect(screen.queryByRole('button', { name: 'Copy Run ID' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Open in Trajectory' })).not.toBeInTheDocument()

  // Cancellation can leave a reasoning/tool-only run without terminal prose.
  rerender(renderRun(false))
  expect(screen.getAllByRole('button', { name: 'Copy Run ID' })).toHaveLength(1)
  fireEvent.click(screen.getByRole('button', { name: 'Open in Trajectory' }))
  expect(open).toHaveBeenCalledWith({ projectId: 'project-a', runId: 'current-run' })

  messages.push({
    id: 'reply', role: 'assistant', metadata: { run_id: 'current-run', agent_kind: agentKind, display_phase: 'final' },
    parts: [{ type: 'text', text: 'Final answer', state: 'done' }],
  })
  rerender(renderRun(false))
  expect(screen.getByText('Final answer')).toBeInTheDocument()
  expect(screen.getAllByRole('button', { name: 'Copy Run ID' })).toHaveLength(1)
  expect(screen.getAllByRole('button', { name: 'Open in Trajectory' })).toHaveLength(1)
})

it('leaves the reference on the error message when it follows a failed run', () => {
  const views = buildAgentMessageViews([{
    id: 'error', role: 'assistant', metadata: { run_id: 'failed-run' },
    parts: [{ type: 'data-agent-error', data: { message: 'Request failed' } }],
  }])
  const error: AgentChatListItem = { kind: 'message', key: 'error', view: views[0], sourceIndex: 1 }
  const run = runItem([{
    id: 'thinking', role: 'assistant', metadata: { run_id: 'failed-run' },
    parts: [{ type: 'reasoning', text: 'Working', state: 'done' }],
  }], false)
  render(<>{row(run, false, error)}{row(error, false)}</>)
  expect(screen.getAllByRole('button', { name: 'Copy Run ID' })).toHaveLength(1)
})

it('hides Run actions while waiting for the model to emit content', () => {
  render(row({ kind: 'activity', key: 'waiting', content: 'Waiting for the model', runId: 'accepted-run' }, true))
  expect(screen.queryByRole('button', { name: 'Copy Run ID' })).not.toBeInTheDocument()
})

it('keeps Run actions hidden when execution remains active between streamed parts', () => {
  const item = runItem([{
    id: 'thinking', role: 'assistant', metadata: { run_id: 'active-run' },
    parts: [{ type: 'reasoning', text: 'Checking the result', state: 'done' }],
  }], true)
  render(row(item, false))
  expect(screen.queryByRole('button', { name: 'Copy Run ID' })).not.toBeInTheDocument()
})

// 自制-叙事演绎 §8.4 资料库故障、模型失败和提交失败是技术问题，明确提示并保留恢复能力.
// The collapsed trace is the default view, so a failure that only colours a hidden
// card tells the player nothing.
it('reports a failed tool inside the collapsed execution summary', () => {
  const item: AgentChatListItem = {
    kind: 'trace',
    key: 'trace-failed',
    activeStreamingTrace: false,
    views: buildAgentMessageViews([{
      id: 'tool-read', role: 'assistant', metadata: { run_id: 'run-failed' },
      parts: [{
        type: 'dynamic-tool', toolName: 'read_lore_items', toolCallId: 'tool-read',
        state: 'output-error', input: { ids: ['lore_missing'] },
        errorText: 'cannot read lore because the current workspace is unavailable',
      }],
    }]),
  }
  render(row(item, false))
  expect(screen.getByText(/1 tool failure/)).toBeInTheDocument()
})

it('keeps the execution summary neutral when every tool succeeded', () => {
  const item: AgentChatListItem = {
    kind: 'trace',
    key: 'trace-ok',
    activeStreamingTrace: false,
    views: buildAgentMessageViews([{
      id: 'tool-read', role: 'assistant', metadata: { run_id: 'run-ok' },
      parts: [{
        type: 'dynamic-tool', toolName: 'read_lore_items', toolCallId: 'tool-read',
        state: 'output-available', input: { ids: ['lore_known'] }, output: 'entry body',
      }],
    }]),
  }
  render(row(item, false))
  expect(screen.getByText(/1 tool call/)).toBeInTheDocument()
  expect(screen.queryByText(/tool failure/)).not.toBeInTheDocument()
})

// 自制-叙事演绎：故事舞台只留必须作答与真实失败，思考与工具不进前台。
// A quiet group with nothing left to show must not become a row at all — a row
// that exists but measures zero height makes Virtuoso report a broken item.
it('builds no row for a quiet trace group that only holds process', () => {
  const views = buildAgentMessageViews([{
    id: 'thinking', role: 'assistant', metadata: { run_id: 'run-quiet' },
    parts: [{ type: 'reasoning', text: 'Weighing the next scene', state: 'done' }],
  }])
  const quiet = buildAgentChatListItems({
    views, isStreaming: false, isExecutionActive: false, visibleActivityContent: '',
    collapseTraceGroups: true, tracePresentation: 'quiet', groupSubAgentTimeline: false, timelineAttachments: [],
  })
  expect(quiet).toHaveLength(0)

  // The same views still reach a full surface, where process is the point.
  const full = buildAgentChatListItems({
    views, isStreaming: false, isExecutionActive: false, visibleActivityContent: '',
    collapseTraceGroups: true, groupSubAgentTimeline: false, timelineAttachments: [],
  })
  expect(full).toHaveLength(1)
  expect(full[0].kind).toBe('run')
})

it('keeps the story waiting cue through hidden thinking and tools until readable prose streams', () => {
  const views = buildAgentMessageViews([
    { id: 'thinking', role: 'assistant', metadata: { run_id: 'run' }, parts: [{ type: 'reasoning', text: 'internal', state: 'streaming' }] },
    { id: 'tool', role: 'assistant', metadata: { run_id: 'run' }, parts: [{ type: 'dynamic-tool', toolName: 'read_lore_items', toolCallId: 'call', state: 'input-available', input: {} }] },
    { id: 'prose', role: 'assistant', metadata: { run_id: 'run' }, parts: [{ type: 'text', text: '门开了。', state: 'streaming' }] },
  ])
  expect(views.map(view => isVisibleActiveReply(view, 'run', 'quiet'))).toEqual([false, false, true])
  expect(isVisibleActiveReply(views[0], 'run', 'full')).toBe(true)
})

it('keeps the row when a quiet trace group holds a real failure', () => {
  const views = buildAgentMessageViews([{
    id: 'thinking', role: 'assistant', metadata: { run_id: 'run-quiet-error' },
    parts: [{ type: 'reasoning', text: 'Weighing the next scene', state: 'done' }],
  }, {
    id: 'error', role: 'assistant', metadata: { run_id: 'run-quiet-error' },
    parts: [{ type: 'data-agent-error', data: { message: 'Request failed' } }],
  }])
  const items = buildAgentChatListItems({
    views, isStreaming: false, isExecutionActive: false, visibleActivityContent: '',
    collapseTraceGroups: true, tracePresentation: 'quiet', groupSubAgentTimeline: false, timelineAttachments: [],
  })
  expect(items).toHaveLength(1)
  render(row(items[0], false))
  expect(screen.getByText(/Request failed/)).toBeInTheDocument()
  expect(screen.queryByText('Weighing the next scene')).not.toBeInTheDocument()
})
