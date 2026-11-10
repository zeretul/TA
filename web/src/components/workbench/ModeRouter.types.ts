import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import type {
  BookRecord,
  LoreItem,
  WorkspaceSummary,
} from '@/lib/api'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
import type { GameSave } from '@/features/interactive/game-saves'
import type { StoryPackIdentityChoice } from '@/lib/api-client'
import type { StorySummary } from '@/features/interactive/types'

/** Public composition contract for the workbench route host. */
export interface ModeRouterProps {
  mode: WorkspaceMode
  currentBookName: string
  workspace: string
  projectId: string
  summary: WorkspaceSummary | null
  activityBarExpanded: boolean
  rightPanel: RightPanel
  settingsOpen: boolean
  interactiveRightVisible: boolean
  books: BookRecord[]
  loreItems: LoreItem[]
  onSetMode: (mode: WorkspaceMode) => void
  onToggleActivityBarExpanded: () => void
  onSetRightPanel: (panel: RightPanel) => void
  onToggleSettings: () => void
  onCloseSettings: () => void
  onToggleInteractiveRightPanel: () => void
  onQuickSwitchBook: (path: string) => Promise<boolean>
  /** Starts a story in a new world from a prepared world pack. */
  onStartStory: (packId: string, templateId?: string, identity?: StoryPackIdentityChoice) => void | Promise<void>
  onOpenSave?: (save: GameSave) => Promise<void>
  onDeleteSave?: (save: GameSave) => Promise<void>
  /** Opens an already saved story and its world. */
  onOpenStory: (story: StorySummary) => void | Promise<void>
  onSelectFile: (path: string) => boolean | void | Promise<boolean | void>
  onOpenLoreTab: () => Promise<boolean>
  onEditorFlushHandlerChange: (handler: EditorFlushHandler | null) => void
}
