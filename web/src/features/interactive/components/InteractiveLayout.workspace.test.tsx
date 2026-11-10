import { act, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getInteractiveSnapshot, getInteractiveStories } from '../api'
import { useInteractiveStore } from '../stores/interactive-store'
import type { InteractiveSnapshotResponse, StoryIndex, StorySummary } from '../types'
import { InteractiveLayout } from './InteractiveLayout'

const runtime = vi.hoisted(() => ({ workspace: 'space', calls: [] as { workspace: string; storyId: string }[] }))
const toast = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }))
vi.mock('@/lib/toast', () => ({ toast }))
vi.mock('@/hooks/useIsMobile', () => ({ useIsMobile: () => false }))
vi.mock('../use-narrative-style-catalog', () => ({ useNarrativeStyleCatalog: (tellers: unknown[]) => tellers }))
vi.mock('../api', () => ({
  getInteractiveStories: vi.fn(),
  getInteractiveSnapshot: vi.fn(),
  getInteractiveBranches: vi.fn(async () => []),
  getInteractiveTellers: vi.fn(async () => []),
  getGamePlanningTemplates: vi.fn(async () => []),
  selectInteractiveStory: vi.fn(async () => undefined),
  switchInteractiveBranch: vi.fn(),
  createInteractiveBranch: vi.fn(),
  deleteInteractiveBranch: vi.fn(),
  deleteInteractiveStory: vi.fn(),
  updateInteractiveStory: vi.fn(),
}))
vi.mock('./StoryStage', () => ({ StoryStage: ({ storyId, snapshot, snapshotLoading }: { storyId: string; snapshot: { story_id: string } | null; snapshotLoading: boolean }) => <div data-testid="stage" data-story={storyId} data-snapshot={snapshot?.story_id || ''} data-loading={String(snapshotLoading)} /> }))
vi.mock('./StoryWorkspace', () => ({ StoryWorkspace: ({ story }: { story: ReactNode }) => <>{story}</> }))
vi.mock('./DirectorPanel', () => ({ DirectorPanel: () => null }))
vi.mock('./StoryPicker', () => ({ StoryPicker: () => null }))
vi.mock('./BranchTimeline', () => ({ BranchTimeline: () => null }))
vi.mock('./branching/CreateBranchDialog', () => ({ CreateBranchDialog: () => null }))

function story(id: string): StorySummary {
  return { id, title: id, origin: '', protagonist: { mode: 'default' }, story_teller_id: 'classic', planning_template_id: 'default', choice_count: 3, reply_target_chars: 2000, opening: { mode: 'ai' }, created_at: '', updated_at: '', branches: 1, events: 0, turn_count: 0 }
}
function index(id: string): StoryIndex { return { stories: [story(id)], current_story_id: id } }
function snapshot(id: string): InteractiveSnapshotResponse { return { story_id: id, branch_id: 'main', turns: [], state: {}, graph: { nodes: [], branches: [] }, turn_count: 0, turn_start: 0, has_earlier_turns: false } }
function view(workspace: string, active = true) { return <InteractiveLayout workspace={workspace} projectId={workspace} active={active} /> }

describe('Interactive layout workspace snapshot ownership', () => {
  beforeEach(() => {
    window.localStorage.clear()
    useInteractiveStore.getState().resetWorkspaceState()
    useInteractiveStore.getState().setSubmode('story')
    runtime.workspace = 'space'
    runtime.calls = []
    vi.mocked(getInteractiveStories).mockReset().mockImplementation(async () => index(runtime.workspace === 'space' ? 'space-c' : 'lanchuan-a'))
    vi.mocked(getInteractiveSnapshot).mockReset().mockImplementation(async id => {
      runtime.calls.push({ workspace: runtime.workspace, storyId: id })
      return snapshot(id)
    })
    toast.error.mockClear()
  })

  it('waits for the new workspace index instead of requesting its predecessor story on activation', async () => {
    const result = render(view('space', false))
    await waitFor(() => expect(useInteractiveStore.getState().currentStoryId).toBe('space-c'))
    let release!: (value: StoryIndex) => void
    vi.mocked(getInteractiveStories).mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    runtime.workspace = 'lanchuan'
    result.rerender(view('lanchuan'))
    // The workspace-reset effect and snapshot effect run from the same render;
    // its old story closure must not issue any request while the index waits.
    expect(runtime.calls).toEqual([])
    await act(async () => release(index('lanchuan-a')))
    await waitFor(() => expect(screen.getByTestId('stage')).toHaveAttribute('data-snapshot', 'lanchuan-a'))
    expect(runtime.calls.every(call => call.workspace === 'lanchuan' && call.storyId === 'lanchuan-a')).toBe(true)
    expect(toast.error).not.toHaveBeenCalled()
  })

  it.each(['resolve', 'reject'] as const)('discards a delayed old-workspace snapshot %s without overwriting the current world or leaking an error', async (outcome) => {
    let rejectOld!: (reason: Error) => void
    let resolveOld!: (value: InteractiveSnapshotResponse) => void
    vi.mocked(getInteractiveSnapshot).mockImplementation(async id => {
      runtime.calls.push({ workspace: runtime.workspace, storyId: id })
      if (id === 'space-c') return new Promise((resolve, reject) => { resolveOld = resolve; rejectOld = reject })
      return snapshot(id)
    })
    const result = render(view('space'))
    await waitFor(() => expect(rejectOld).toBeDefined())
    runtime.workspace = 'lanchuan'
    result.rerender(view('lanchuan'))
    await waitFor(() => expect(screen.getByTestId('stage')).toHaveAttribute('data-snapshot', 'lanchuan-a'))
    await act(async () => {
      if (outcome === 'resolve') resolveOld(snapshot('space-c'))
      else rejectOld(new Error('old workspace snapshot not found'))
    })
    expect(screen.getByTestId('stage')).toHaveAttribute('data-snapshot', 'lanchuan-a')
    expect(useInteractiveStore.getState().currentStoryId).toBe('lanchuan-a')
    expect(toast.error).not.toHaveBeenCalled()
    expect(runtime.calls).not.toContainEqual({ workspace: 'lanchuan', storyId: 'space-c' })
  })

  it('still reports a genuine current-workspace snapshot failure', async () => {
    const failure = new Error('current workspace snapshot could not be read')
    vi.mocked(getInteractiveSnapshot).mockRejectedValue(failure)
    const logged = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    try {
      render(view('space'))
      await waitFor(() => expect(toast.error).toHaveBeenCalledWith(expect.stringContaining(failure.message)))
      expect(logged).toHaveBeenCalledWith('[interactive-layout] Failed to refresh interactive snapshot', failure)
      expect(screen.getByTestId('stage')).toHaveAttribute('data-loading', 'false')
    } finally {
      logged.mockRestore()
    }
  })
})
