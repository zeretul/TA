import { render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { StoryPackSummary } from '@/lib/api-client'
import { HomeView } from './HomeView'

const api = vi.hoisted(() => ({
  listStoryPacks: vi.fn(async (): Promise<StoryPackSummary[]> => []),
  getStoryPack: vi.fn(),
  saveStoryPackSettings: vi.fn(),
}))

const interactiveApi = vi.hoisted(() => ({
  deleteInteractiveStory: vi.fn(async () => undefined),
  getGamePlanningTemplates: vi.fn(async () => []),
  getInteractiveStories: vi.fn(async () => ({ stories: [] })),
  getInteractiveTellers: vi.fn(async () => []),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}))

vi.mock('@/lib/api-client', () => ({
  APIError: class APIError extends Error {
    status: number
    constructor(message: string, status: number) {
      super(message)
      this.status = status
    }
  },
  listStoryPacks: api.listStoryPacks,
  getStoryPack: api.getStoryPack,
  saveStoryPackSettings: api.saveStoryPackSettings,
}))

vi.mock('@/features/interactive/api', () => interactiveApi)

vi.mock('@/features/interactive/stores/interactive-store', () => ({
  useInteractiveStore: (selector: (state: { currentStoryId: string }) => unknown) => selector({ currentStoryId: '' }),
}))

function renderHome(projectId: string) {
  return render(
    <TooltipProvider>
      <HomeView projectId={projectId} onStartStory={vi.fn()} onOpenStory={vi.fn()} />
    </TooltipProvider>,
  )
}

describe('HomeView saved stories', () => {
  beforeEach(() => {
    api.listStoryPacks.mockClear()
    interactiveApi.getInteractiveStories.mockClear()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  // The server refuses a story listing with no workspace context (409), and the
  // page cannot open a saved story before it knows which workspace it belongs to.
  // Asking anyway only fills the log with a refusal the player never caused.
  it('does not ask for saved stories while no workspace is open', async () => {
    renderHome('')

    await waitFor(() => {
      expect(api.listStoryPacks).toHaveBeenCalled()
    })
    expect(interactiveApi.getInteractiveStories).not.toHaveBeenCalled()
  })

  it('asks for saved stories once a workspace is open', async () => {
    renderHome('project-a')

    await waitFor(() => {
      expect(interactiveApi.getInteractiveStories).toHaveBeenCalledTimes(1)
    })
  })
})
