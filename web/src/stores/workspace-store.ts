import { create } from 'zustand'

export type RightPanel = 'ai' | null
type BottomPanel = 'versions' | 'problems' | null
/**
 * `game-settings` is the single place a player configures the current world and
 * story. `presets` is gone from the player vocabulary: the preset directory is
 * an internal template source, and leaving the route in this union would let a
 * stored preference reopen the old 45-item editor.
 */
export type WorkspaceRoute = 'interactive' | 'history' | 'games' | 'lore' | 'game-settings' | 'assistant' | 'books'
/** Historical internal name retained while the persisted key stays stable. */
export type WorkspaceMode = WorkspaceRoute

/**
 * Shared top-level destinations alongside the game. The historical `mode` name
 * remains in persisted state, but navigation no longer has a global mode switch.
 */
const SHARED_WORKSPACE_MODES = ['lore', 'game-settings', 'assistant', 'books'] as const

export type SharedWorkspaceMode = (typeof SHARED_WORKSPACE_MODES)[number]

/** Reports whether a mode belongs to the shared top-level menu. */
export function isSharedWorkspaceMode(mode: WorkspaceMode): mode is SharedWorkspaceMode {
  return (SHARED_WORKSPACE_MODES as readonly WorkspaceMode[]).includes(mode)
}

/**
 * Reports whether a destination needs an active Book. Game settings does not:
 * with no story it shows an empty state that links to the world pack page
 * instead of bouncing the player out of the page they asked for.
 */
export function workspaceModeRequiresBook(mode: WorkspaceMode): boolean {
  return mode === 'interactive' || mode === 'lore'
}

const MODE_STORAGE_KEY = 'nova:mode'
const RIGHT_PANEL_STORAGE_KEY = 'nova:right-panel'

/** Game-only build: a fresh install opens the playable story surface. */
const DEFAULT_WORKSPACE_MODE: WorkspaceMode = 'interactive'

function readInitialMode(): WorkspaceMode {
  if (typeof window === 'undefined') return DEFAULT_WORKSPACE_MODE
  if (new URLSearchParams(window.location.search).get('game') === 'history') return 'history'
  const stored = window.localStorage.getItem(MODE_STORAGE_KEY)
  return isPlayerWorkspaceMode(stored) ? stored : DEFAULT_WORKSPACE_MODE
}

/** Player destinations. A stored preference naming a retired surface must not restore it. */
const PLAYER_WORKSPACE_MODES: WorkspaceMode[] = ['interactive', 'history', 'games', 'lore', 'game-settings', 'assistant', 'books']

function isPlayerWorkspaceMode(value: unknown): value is WorkspaceMode {
  return typeof value === 'string' && (PLAYER_WORKSPACE_MODES as string[]).includes(value)
}

function readInitialRightPanel(): RightPanel {
  if (typeof window === 'undefined') return 'ai'
  const stored = window.localStorage.getItem(RIGHT_PANEL_STORAGE_KEY)
  if (stored === null) return 'ai'
  return isRightPanel(stored) ? stored : 'ai'
}

function isRightPanel(value: unknown): value is RightPanel {
  return value === 'ai' || value === null
}

function persistMode(mode: WorkspaceMode) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(MODE_STORAGE_KEY, mode)
}

function persistRightPanel(panel: RightPanel) {
  if (typeof window === 'undefined') return
  if (panel === null) {
    window.localStorage.removeItem(RIGHT_PANEL_STORAGE_KEY)
    return
  }
  window.localStorage.setItem(RIGHT_PANEL_STORAGE_KEY, panel)
}

type WorkspaceStore = {
  mode: WorkspaceMode
  selectedProjectId?: string
  selectedChapterId?: string
  rightPanel: RightPanel
  bottomPanel: BottomPanel
  commandOpen: boolean
  setMode: (mode: WorkspaceMode) => void
  setSelectedProjectId: (id?: string) => void
  setSelectedChapterId: (id?: string) => void
  setRightPanel: (panel: RightPanel) => void
  setBottomPanel: (panel: BottomPanel) => void
  setCommandOpen: (open: boolean) => void
}

/** 工作区 UI 状态 Store，仅保存本地界面状态，不存放服务端数据。 */
export const useWorkspaceStore = create<WorkspaceStore>((set) => ({
  mode: readInitialMode(),
  selectedProjectId: undefined,
  selectedChapterId: undefined,
  rightPanel: readInitialRightPanel(),
  bottomPanel: null,
  commandOpen: false,
  setMode: (mode) => {
    persistMode(mode)
    set({ mode })
  },
  setSelectedProjectId: (id) => set({ selectedProjectId: id }),
  setSelectedChapterId: (id) => set({ selectedChapterId: id }),
  setRightPanel: (panel) => {
    persistRightPanel(panel)
    set({ rightPanel: panel })
  },
  setBottomPanel: (panel) => set({ bottomPanel: panel }),
  setCommandOpen: (open) => set({ commandOpen: open }),
}))
