import type { StoryCreateInput } from '../../opening'
import type { GamePlanningTemplate, InteractiveTurnPersistedEvent, Snapshot, StoryAssistantAdoptResult, StorySummary, Teller } from '../../types'
import type { LoreItem } from '@/lib/api'
import type { BranchCreationSource } from '../branching/model'
import type { StoryStateDisplayPreference } from '../story-state/display-preference'

/** Stable integration surface between the interactive workspace and the story stage. */
export interface StoryStageProps {
  active?: boolean
  projectId: string
  workspace?: string
  styleSceneSuggestions?: string[]
  stories?: StorySummary[]
  story?: StorySummary
  tellers?: Teller[]
	planningTemplates?: GamePlanningTemplate[]
  recentNarrativeStyleID?: string
  narrativeStyleLoading?: boolean
  storyId: string
  branchId: string
  snapshot: Snapshot | null
  snapshotLoading?: boolean
  loreEmpty?: boolean
  loreItems?: LoreItem[]
  directorPanelVisible?: boolean
  stateDisplayPreference?: StoryStateDisplayPreference
  onStorySelect?: (storyId: string) => void
  onStorySetupUpdate?: (input: StoryCreateInput) => void | Promise<void>
  onStoryDelete?: (storyIds: string[]) => void | Promise<void>
  onStoryRename?: (storyId: string, title: string) => void | Promise<void>
  onRequestLoreInit?: () => void
  onOpenDirectorConfig?: () => void
  /** Creation lives on the world page, so an ended branch sends the player there. */
  onOpenWorldPacks?: () => void
  onToggleDirectorPanel?: () => void
  onOpenDirectorState?: () => void
  /**
   * Lends the stage's own "jump to this turn" to the handbook beside it, so a
   * history entry opens the turn through the same path the turn navigator uses
   * instead of a second scrolling mechanism. Registered on mount and cleared on
   * unmount; the caller holds the latest function in a ref.
   */
  onRegisterOpenTurn?: (openTurn: ((turnId: string) => void) | null) => void
  onRequestCreateBranch?: (source: BranchCreationSource) => void
  onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void
  onTurnPersisted?: (event: InteractiveTurnPersistedEvent, options?: { replayed: boolean }) => Snapshot | void
  /** The assistant recorded a direction; fold the returned plan into what is shown. */
  onBranchPlanAdvanced?: (result: StoryAssistantAdoptResult) => void
  onDone: (options?: { silent?: boolean }) => void | Promise<Snapshot | void>
}
