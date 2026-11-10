import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, type ComponentProps, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'
import { ModeRouter } from './ModeRouter'

const toastMock = vi.hoisted(() => ({ warning: vi.fn() }))
const useDocumentReviewMock = vi.hoisted(() => vi.fn())
const loreLibraryFlushMock = vi.hoisted(() => vi.fn(async () => true))

function withAppProviders(ui: ReactNode) {
  return <TooltipProvider>{ui}</TooltipProvider>
}

vi.mock('@/lib/toast', () => ({ toast: toastMock }))

vi.mock('@/hooks/usePersistedUserSettings', () => ({
  usePersistedUserSettings: vi.fn(),
}))

vi.mock('@/features/interactive/components/InteractiveLayout', () => ({
  InteractiveLayout: () => <div data-testid="interactive-layout">story</div>,
}))

vi.mock('@/features/interactive/components/SettingPanel', () => ({
  SettingPanel: ({ mode, onFlushHandlerChange }: {
    mode?: string
    onFlushHandlerChange?: (handler: (() => Promise<boolean>) | null) => void
  }) => {
    useEffect(() => {
      onFlushHandlerChange?.(loreLibraryFlushMock)
      return () => onFlushHandlerChange?.(null)
    }, [onFlushHandlerChange])
    return <div data-testid={`setting-panel-${mode || 'default'}`}>{mode || 'default'}</div>
  },
}))

vi.mock('@/components/Home/HomeView', () => ({
  HomeView: () => <div data-testid="home-view">worlds</div>,
}))

vi.mock('@/features/settings/SettingsView', () => ({
  SettingsView: () => <div data-testid="settings-view">settings</div>,
}))

vi.mock('@/features/document-review/use-document-review', () => ({
  useDocumentReview: useDocumentReviewMock,
}))

vi.mock('@/features/interactive/api', () => ({
  getImagePresets: vi.fn(async () => []),
  getInteractiveStories: vi.fn(async () => ({ current_story_id: '', stories: [] })),
  getInteractiveTellers: vi.fn(async () => []),
}))

vi.mock('@/features/interactive/stores/interactive-store', () => ({
  useInteractiveStore: (selector: (state: Record<string, unknown>) => unknown) => selector({
    currentStoryId: '',
    currentBranchId: 'main',
    setSubmode: vi.fn(),
    setStories: vi.fn(),
    resetWorkspaceState: vi.fn(),
  }),
}))

vi.mock('./WorkbenchShell', () => ({
  WorkbenchShell: ({ onQuickSwitchBook, main }: {
    onQuickSwitchBook: (path: string) => Promise<boolean>
    main: ReactNode
  }) => (
    <>
      <button type="button" onClick={() => { void onQuickSwitchBook('/book-b') }}>
        quick switch
      </button>
      {main}
    </>
  ),
}))

describe('ModeRouter navigation policy', () => {
  beforeEach(() => {
    toastMock.warning.mockReset()
    useDocumentReviewMock.mockReset()
    useDocumentReviewMock.mockReturnValue({
      visibleComments: [],
      addComment: vi.fn(),
      editComment: vi.fn(),
      removeComment: vi.fn(),
    })
    loreLibraryFlushMock.mockReset()
    loreLibraryFlushMock.mockResolvedValue(true)
    vi.mocked(usePersistedUserSettings).mockReturnValue({
      values: { interactive_story_teller_id: 'rhythm' },
      loading: false,
      isSaving: vi.fn(() => true),
      persist: vi.fn(async () => true),
      reload: vi.fn(async () => null),
      flushPending: vi.fn(async () => false),
    })
  })

  it('shows the playable story surface for the game route', async () => {
    render(withAppProviders(<ModeRouter {...modeRouterProps()} />))
    expect(await screen.findByTestId('interactive-layout')).toBeInTheDocument()
    expect(screen.queryByTestId('home-view')).not.toBeInTheDocument()
  })

  it('continues a workspace switch and warns when preference flush remains pending', async () => {
    const user = userEvent.setup()
    const onQuickSwitchBook = vi.fn(async () => true)
    render(withAppProviders(<ModeRouter {...modeRouterProps({ onQuickSwitchBook })} />))

    await user.click(screen.getByRole('button', { name: 'quick switch' }))

    await waitFor(() => expect(onQuickSwitchBook).toHaveBeenCalledWith('/book-b'))
    expect(toastMock.warning).toHaveBeenCalledWith('偏好设置暂未保存', {
      description: '本地修改已保留，将在下次自动保存时重试。',
    })
  })

  it('does not wait for a slow preference request before switching workspaces', async () => {
    const user = userEvent.setup()
    let resolveFlush!: (saved: boolean) => void
    const pendingFlush = new Promise<boolean>((resolve) => { resolveFlush = resolve })
    vi.mocked(usePersistedUserSettings).mockReturnValue({
      values: { interactive_story_teller_id: 'rhythm' },
      loading: false,
      isSaving: vi.fn(() => true),
      persist: vi.fn(async () => true),
      reload: vi.fn(async () => null),
      flushPending: vi.fn(() => pendingFlush),
    })
    const onQuickSwitchBook = vi.fn(async () => true)
    render(withAppProviders(<ModeRouter {...modeRouterProps({ onQuickSwitchBook })} />))

    await user.click(screen.getByRole('button', { name: 'quick switch' }))
    expect(onQuickSwitchBook).toHaveBeenCalledWith('/book-b')

    resolveFlush(false)
    await waitFor(() => expect(toastMock.warning).toHaveBeenCalled())
  })

  it('flushes the retained Lore library before switching workspaces', async () => {
    const user = userEvent.setup()
    const onQuickSwitchBook = vi.fn(async () => true)
    loreLibraryFlushMock.mockResolvedValue(false)
    render(withAppProviders(<ModeRouter {...modeRouterProps({ mode: 'lore', onQuickSwitchBook })} />))
    await screen.findByTestId('setting-panel-lore')

    await user.click(screen.getByRole('button', { name: 'quick switch' }))

    await waitFor(() => expect(loreLibraryFlushMock).toHaveBeenCalledTimes(1))
    expect(onQuickSwitchBook).not.toHaveBeenCalled()
  })

  it('renders the lore library for the lore route', async () => {
    render(withAppProviders(<ModeRouter {...modeRouterProps({ mode: 'lore' })} />))
    expect(await screen.findByTestId('setting-panel-lore')).toBeInTheDocument()
  })

  it('renders the world picker for the books route', async () => {
    render(withAppProviders(<ModeRouter {...modeRouterProps({ mode: 'books' })} />))
    expect(await screen.findByTestId('home-view')).toBeInTheDocument()
  })

  it('renders settings above the current route', async () => {
    render(withAppProviders(<ModeRouter {...modeRouterProps({ mode: 'interactive', settingsOpen: true })} />))
    expect(await screen.findByTestId('settings-view')).toBeInTheDocument()
  })
})

function modeRouterProps(
  overrides: Partial<ComponentProps<typeof ModeRouter>> = {},
): ComponentProps<typeof ModeRouter> {
  return {
    mode: 'interactive',
    currentBookName: 'Book A',
    workspace: '/book-a',
    projectId: 'project-book-a',
    summary: null,
    activityBarExpanded: true,
    rightPanel: null,
    settingsOpen: false,
    interactiveRightVisible: false,
    books: [],
    loreItems: [],
    onSetMode: vi.fn(),
    onToggleActivityBarExpanded: vi.fn(),
    onSetRightPanel: vi.fn(),
    onToggleSettings: vi.fn(),
    onCloseSettings: vi.fn(),
    onToggleInteractiveRightPanel: vi.fn(),
    onQuickSwitchBook: vi.fn(async () => true),
    onStartStory: vi.fn(),
    onOpenStory: vi.fn(),
    onSelectFile: vi.fn(),
    onOpenLoreTab: vi.fn(async () => true),
    onEditorFlushHandlerChange: vi.fn(),
    ...overrides,
  }
}
