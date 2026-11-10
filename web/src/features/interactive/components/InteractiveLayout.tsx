import { WorldPresentationProvider } from '../world-presentation-context'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { toast } from '@/lib/toast'
import { errorMessage } from '@/lib/error-diagnostics'
import { useShallow } from 'zustand/react/shallow'
import type { LoreItem } from '@/lib/api'
import { createInteractiveBranch, deleteInteractiveBranch, deleteInteractiveStory, getGamePlanningTemplates, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, selectInteractiveStory, switchInteractiveBranch, updateInteractiveStory } from '../api'
import { branchPlanSnapshotAfterUpdate } from '../branch-plan-snapshot'
import { useInteractiveStore } from '../stores/interactive-store'
import { BranchTimeline } from './BranchTimeline'
import { DirectorPanel } from './DirectorPanel'
import { StoryPicker } from './StoryPicker'
import { StoryStage } from './StoryStage'
import { CreateBranchDialog } from './branching/CreateBranchDialog'
import type { BranchCreationSource } from './branching/model'
import {
  readStoryStateDisplayPreference,
  writeStoryStateDisplayPreference,
  type StoryStateDisplayPreference,
} from './story-state/display-preference'
import { novaEase, panelPresence } from '@/features/motion/motion-tokens'
import { useIsMobile } from '@/hooks/useIsMobile'
import { StoryWorkspace } from './StoryWorkspace'
import type { InteractiveTurnPersistedEvent, Snapshot, StoryAssistantAdoptResult, StorySummary } from '../types'
import type { StoryCreateInput } from '../opening'
import { DEFAULT_NARRATIVE_STYLE_ID, resolveNarrativeStyle } from '../narrative-style'
import { useNarrativeStyleCatalog } from '../use-narrative-style-catalog'
import { mergePreferredStory } from '../story-index'
import { LoadingState } from '@/components/common/LoadingState'

interface InteractiveLayoutProps {
  projectId?: string
  workspace?: string
  active?: boolean
  recentNarrativeStyleID?: string
  narrativeStyleLoading?: boolean
  loreEmpty?: boolean
  loreItems?: LoreItem[]
  onRequestLoreInit?: () => void
  onOpenPresets?: () => void
  onOpenWorldPacks?: () => void
  rightPanelVisible?: boolean
  onToggleRightPanel?: () => void
}

const SNAPSHOT_POLL_INTERVAL_MS = 1000

export function InteractiveLayout({ projectId = '', workspace, active = true, recentNarrativeStyleID = DEFAULT_NARRATIVE_STYLE_ID, narrativeStyleLoading = false, loreEmpty = false, loreItems = [], onRequestLoreInit, onOpenPresets, onOpenWorldPacks, rightPanelVisible = true, onToggleRightPanel }: InteractiveLayoutProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const {
    stories,
    tellers,
    planningTemplates,
    branches,
    snapshot,
    currentStoryId,
    currentBranchId,
    submode,
    setStories,
    setTellers,
    setPlanningTemplates,
    setBranches,
    setSnapshot,
    applyTurnPersisted,
    setCurrentStoryId,
    setCurrentBranchId,
    setSubmode,
    resetWorkspaceState,
  } = useInteractiveStore(useShallow((state) => ({
    stories: state.stories,
    tellers: state.tellers,
    planningTemplates: state.planningTemplates,
    branches: state.branches,
    snapshot: state.snapshot,
    currentStoryId: state.currentStoryId,
    currentBranchId: state.currentBranchId,
    submode: state.submode,
    setStories: state.setStories,
    setTellers: state.setTellers,
    setPlanningTemplates: state.setPlanningTemplates,
    setBranches: state.setBranches,
    setSnapshot: state.setSnapshot,
    applyTurnPersisted: state.applyTurnPersisted,
    setCurrentStoryId: state.setCurrentStoryId,
    setCurrentBranchId: state.setCurrentBranchId,
    setSubmode: state.setSubmode,
    resetWorkspaceState: state.resetWorkspaceState,
  })))
  const currentStory = stories.find((story) => story.id === currentStoryId)
  const styleTellers = useNarrativeStyleCatalog(tellers, currentStory)
  const currentTeller = resolveNarrativeStyle(styleTellers, currentStory?.story_teller_id)
  const styleSceneSuggestions = Array.from(new Set((currentTeller?.style_rules || []).map((rule) => rule.scene.trim()).filter((scene) => scene && !isGlobalStyleSceneName(scene))))
  const currentBranchSnapshot = snapshot?.story_id === currentStoryId && snapshot.branch_id === currentBranchId ? snapshot : null
  const storyIndexRequestSeqRef = useRef(0)
  const workspaceIndexReadyRef = useRef(false)
  const snapshotStoryIdRef = useRef('')
  const snapshotRequestSeqRef = useRef(0)
  const storySelectionQueueRef = useRef<Promise<void>>(Promise.resolve())
  const lastStableSnapshotRef = useRef<Snapshot | null>(null)
  const [snapshotLoading, setSnapshotLoading] = useState(false)
  const [snapshotLoadFailed, setSnapshotLoadFailed] = useState(false)
  const [storyIndexLoading, setStoryIndexLoading] = useState(true)
  const [mobileSnapshotOpen, setMobileSnapshotOpen] = useState(false)
  const [storyStateDisplayPreference, setStoryStateDisplayPreference] = useState(readStoryStateDisplayPreference)
  const [branchCreationSource, setBranchCreationSource] = useState<BranchCreationSource | null>(null)
  // The stage owns the prose scroller, so the handbook asks it to jump rather
  // than scrolling anything itself. The stage registers its jump on mount.
  const openTurnRef = useRef<((turnId: string) => void) | null>(null)
  const registerOpenTurn = useCallback((openTurn: ((turnId: string) => void) | null) => {
    openTurnRef.current = openTurn
  }, [])
  const handleOpenTurn = useCallback((turnId: string) => {
    if (isMobile) setMobileSnapshotOpen(false)
    openTurnRef.current?.(turnId)
  }, [isMobile])

  if (currentBranchSnapshot) {
    lastStableSnapshotRef.current = currentBranchSnapshot
  }
  const fallbackSnapshot = lastStableSnapshotRef.current?.story_id === currentStoryId ? lastStableSnapshotRef.current : null
  const snapshotPending = !snapshotLoadFailed && Boolean(currentStoryId) && !currentBranchSnapshot && (snapshotLoading || !snapshot || snapshot.story_id !== currentStoryId || snapshot.branch_id !== currentBranchId)
  const displaySnapshot = currentBranchSnapshot ?? (snapshotPending ? fallbackSnapshot : null)

  useEffect(() => {
    snapshotStoryIdRef.current = snapshot?.story_id || ''
  }, [snapshot?.story_id])

  useEffect(() => {
    setBranchCreationSource(null)
  }, [currentStoryId])

  const reloadStories = useCallback(async (preferredStory?: StorySummary) => {
    const requestSeq = storyIndexRequestSeqRef.current + 1
    storyIndexRequestSeqRef.current = requestSeq
    const index = await getInteractiveStories()
    if (requestSeq !== storyIndexRequestSeqRef.current) return
    workspaceIndexReadyRef.current = true
    setStories(mergePreferredStory(index.stories || [], preferredStory), preferredStory?.id || index.current_story_id)
  }, [setStories])

  const reloadSnapshot = useCallback(
    async (branchOverride?: string, storyOverride?: string, options?: { silent?: boolean; includeBranches?: boolean }) => {
      const silent = options?.silent === true
      const includeBranches = options?.includeBranches !== false
      const storyId = storyOverride || currentStoryId
      // Workspace reset and activation can share a render whose closure still
      // names the previous Story. Do not issue that read against the new root.
      if (!workspaceIndexReadyRef.current || (storyId && useInteractiveStore.getState().currentStoryId !== storyId)) return
      const requestSeq = snapshotRequestSeqRef.current + 1
      snapshotRequestSeqRef.current = requestSeq
      if (!storyId) {
        if (!silent) {
          setSnapshotLoading(false)
          setSnapshot(null)
        }
        return
      }
      if (!silent) {
        setSnapshotLoading(true)
        setSnapshotLoadFailed(false)
      }
      const branchId = branchOverride ?? (snapshotStoryIdRef.current === storyId || currentBranchId !== 'main' ? currentBranchId : '')
      try {
        const [nextSnapshot, nextBranches] = await Promise.all([
          getInteractiveSnapshot(storyId, branchId),
          includeBranches ? getInteractiveBranches(storyId) : Promise.resolve(null),
        ])
        if (requestSeq !== snapshotRequestSeqRef.current || useInteractiveStore.getState().currentStoryId !== storyId) return
        setSnapshot(nextSnapshot)
        if (nextBranches) setBranches(nextBranches)
        return nextSnapshot
      } catch (error) {
        if (requestSeq !== snapshotRequestSeqRef.current || useInteractiveStore.getState().currentStoryId !== storyId) return
        console.error('[interactive-layout] Failed to refresh interactive snapshot', error)
        if (!silent) setSnapshotLoadFailed(true)
        if (silent) return
        throw error
      } finally {
        if (!silent && requestSeq === snapshotRequestSeqRef.current) setSnapshotLoading(false)
      }
    },
    [currentBranchId, currentStoryId, setBranches, setSnapshot],
  )

  useEffect(() => {
    let cancelled = false
    workspaceIndexReadyRef.current = false
    storyIndexRequestSeqRef.current += 1
    snapshotRequestSeqRef.current += 1
    snapshotStoryIdRef.current = ''
    lastStableSnapshotRef.current = null
    setSnapshotLoading(false)
    setSnapshotLoadFailed(false)
    const cancelWorkspaceReads = () => {
      cancelled = true
      workspaceIndexReadyRef.current = false
      storyIndexRequestSeqRef.current += 1
      snapshotRequestSeqRef.current += 1
    }
    if (workspace !== undefined) {
      resetWorkspaceState()
      if (!workspace) {
        setStoryIndexLoading(false)
        return cancelWorkspaceReads
      }
    }
    setStoryIndexLoading(true)
    void Promise.all([
      reloadStories(),
      getInteractiveTellers().then((next) => { if (!cancelled) setTellers(next) }),
      getGamePlanningTemplates().then((next) => { if (!cancelled) setPlanningTemplates(next) }),
    ])
      .catch((error) => {
        if (!cancelled) console.error('[InteractiveLayout.tsx] failed to load the interactive workspace index', { workspace, error })
      })
      .finally(() => {
        if (!cancelled) setStoryIndexLoading(false)
      })
    return cancelWorkspaceReads
  }, [projectId, reloadStories, resetWorkspaceState, setPlanningTemplates, setTellers, workspace])

  useEffect(() => {
    if (!active || storyIndexLoading) return
    // Current-scope failures remain visible, but this automatic read must own
    // its rejection instead of leaking it into the global unhandled handler.
    void reloadSnapshot().catch((error) => toast.error(errorMessage(error)))
  }, [active, currentStoryId, reloadSnapshot, storyIndexLoading])

  useEffect(() => {
    const branchID = snapshot?.branch_id
    const storyID = snapshot?.story_id
    const statePending = snapshot?.current_turn?.state_status === 'pending'
    if (!active || !storyID || !branchID || !statePending) return
    let cancelled = false
    let timer: number | null = null
    const clearTimer = () => {
      if (timer === null) return
      window.clearTimeout(timer)
      timer = null
    }
    const schedule = () => {
      clearTimer()
      if (cancelled || document.visibilityState !== 'visible') return
      timer = window.setTimeout(() => {
        timer = null
        void reloadSnapshot(branchID, storyID, { silent: true, includeBranches: false }).finally(schedule)
      }, SNAPSHOT_POLL_INTERVAL_MS)
    }
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') schedule()
      else clearTimer()
    }
    schedule()
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => {
      cancelled = true
      clearTimer()
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [active, reloadSnapshot, snapshot?.branch_id, snapshot?.current_turn?.id, snapshot?.current_turn?.state_status, snapshot?.story_id])

  useEffect(() => {
    if (!active || !isMobile || submode !== 'story') setMobileSnapshotOpen(false)
  }, [active, isMobile, submode])

  const handleStorySelect = useCallback((storyId: string) => {
    setMobileSnapshotOpen(false)
    if (!storyId || storyId === useInteractiveStore.getState().currentStoryId) return
    setCurrentStoryId(storyId)
    const persisted = storySelectionQueueRef.current
      .catch(() => undefined)
      .then(() => selectInteractiveStory(storyId))
    storySelectionQueueRef.current = persisted
    void persisted.catch((error) => {
      console.error('[interactive-layout] Failed to persist the active story', { storyId, error })
    })
  }, [setCurrentStoryId])

  const handleDeleteStories = async (storyIds: string[]) => {
    const uniqueStoryIds = Array.from(new Set(storyIds.filter(Boolean)))
    if (uniqueStoryIds.length === 0) return

    console.info('[interactive-layout] Deleting stories', { count: uniqueStoryIds.length, storyIds: uniqueStoryIds })
    const results = await Promise.allSettled(uniqueStoryIds.map((storyId) => deleteInteractiveStory(storyId)))
    await reloadStories()
    const failed = results.flatMap((result, index) => result.status === 'rejected' ? [{ storyId: uniqueStoryIds[index], reason: result.reason }] : [])
    if (failed.length > 0) {
      console.error('[interactive-layout] Failed to delete stories', { requested: uniqueStoryIds.length, failed })
      const reason = failed[0].reason
      throw reason instanceof Error ? reason : new Error(String(reason))
    }
    console.info('[interactive-layout] Stories deleted', { count: uniqueStoryIds.length })
  }

  const handleRenameStory = async (storyId: string, title: string) => {
    console.info('[interactive-layout] Renaming story', { storyId })
    const updated = await updateInteractiveStory(storyId, { title })
    setStories(mergePreferredStory(useInteractiveStore.getState().stories, updated), updated.id)
    await reloadStories(updated)
    console.info('[interactive-layout] Story renamed', { storyId })
  }

  const handleStorySetupUpdate = async (input: StoryCreateInput) => {
    if (!currentStoryId) return
    await updateInteractiveStory(currentStoryId, {
      title: input.title,
      origin: input.origin,
      protagonist: input.protagonist,
      story_teller_id: input.story_teller_id,
      planning_template_id: input.planning_template_id,
      planning_mode: input.planning_mode,
      module_refs: input.module_refs,
      reply_target_chars: input.reply_target_chars,
      choice_count: input.choice_count,
      image_settings: input.image_settings,
      check_settings: input.check_settings,
      opening: input.opening,
      state_schema_policy: input.state_schema_policy,
    })
    await reloadStories()
    await reloadSnapshot(undefined, currentStoryId, { silent: true })
  }

  // The assistant's adoption already wrote the plan, so fold that exact plan
  // into the snapshot instead of re-fetching: a refetch would race the branch
  // switch and could apply a plan for a branch the player just left.
  const handleBranchPlanAdvanced = useCallback((result: StoryAssistantAdoptResult) => {
    const currentState = useInteractiveStore.getState()
    const nextSnapshot = branchPlanSnapshotAfterUpdate({
      currentStoryId: currentState.currentStoryId,
      currentBranchId: currentState.currentBranchId,
      snapshot: currentState.snapshot,
      updatedStoryId: result.story_id,
      updatedBranchId: result.branch_id,
      result,
    })
    if (!nextSnapshot) return
    snapshotRequestSeqRef.current += 1
    setSnapshotLoading(false)
    setSnapshot(nextSnapshot)
  }, [setSnapshot])

  const handleStoryStateDisplayPreferenceChange = useCallback((value: StoryStateDisplayPreference) => {
    setStoryStateDisplayPreference(value)
    writeStoryStateDisplayPreference(value)
  }, [])

  const openDirectorState = useCallback(() => {
    if (isMobile) {
      setMobileSnapshotOpen(true)
      return
    }
    if (!rightPanelVisible) onToggleRightPanel?.()
  }, [isMobile, onToggleRightPanel, rightPanelVisible])

  const openBranchTimeline = useCallback(() => {
    setMobileSnapshotOpen(false)
    setSubmode('timeline')
  }, [setSubmode])

  const handleTurnPersisted = useCallback((event: InteractiveTurnPersistedEvent) => {
    const nextSnapshot = applyTurnPersisted(event) || undefined
    const persistedStory = useInteractiveStore.getState().stories.find((story) => story.id === event.story_id)
    if (persistedStory?.title_source === 'pending') {
      void reloadStories().catch((error) => {
        console.error('[interactive-layout] Failed to refresh the generated story title', { storyId: event.story_id, error })
      })
    }
    return nextSnapshot
  }, [applyTurnPersisted, reloadStories])

  const handleStoryStageDone = useCallback((options?: { silent?: boolean }) => {
    return reloadSnapshot(undefined, undefined, options)
  }, [reloadSnapshot])

  const handleSwitchBranch = async (branchId: string) => {
    const storyId = currentStoryId || useInteractiveStore.getState().currentStoryId || snapshot?.story_id
    if (!storyId) return
    await switchInteractiveBranch(storyId, branchId)
    setCurrentBranchId(branchId)
    await reloadSnapshot(branchId, storyId)
  }

  const handleCreateBranch = async (turnId: string, title: string, customAgentId?: string) => {
    const storyId = currentStoryId || useInteractiveStore.getState().currentStoryId
    if (!storyId) throw new Error(t('branchTimeline.createUnavailable'))
    const branch = await createInteractiveBranch(storyId, {
      parent_event_id: turnId,
      title,
      ...(customAgentId !== undefined ? { custom_agent_id: customAgentId } : {}),
    })
    setCurrentBranchId(branch.id)
    await reloadSnapshot(branch.id, storyId)
    toast.success(t('branchTimeline.createdAndSwitched', { name: branch.title || title }))
  }

  const handleDeleteBranch = async (branchId: string) => {
    if (!currentStoryId) return
    await deleteInteractiveBranch(currentStoryId, branchId)
    if (branchId === currentBranchId) {
      setCurrentBranchId('main')
    }
    await reloadSnapshot(branchId === currentBranchId ? 'main' : undefined)
    await reloadStories()
  }

  if (storyIndexLoading) {
    return (
      <LoadingState
        label={t('interactiveLayout.loading')}
        className="h-full min-h-0 bg-[var(--nova-bg)]"
      />
    )
  }

  const contentKey = submode
  const directorPanelVisible = isMobile ? mobileSnapshotOpen : rightPanelVisible
  const storyStage = (
    <StoryStage
      active={active}
      projectId={projectId}
      workspace={workspace}
      styleSceneSuggestions={styleSceneSuggestions}
      stories={stories}
      story={currentStory}
      tellers={styleTellers}
      planningTemplates={planningTemplates}
      recentNarrativeStyleID={recentNarrativeStyleID}
      narrativeStyleLoading={narrativeStyleLoading}
      storyId={currentStoryId}
      branchId={currentBranchId}
      snapshot={displaySnapshot}
      snapshotLoading={snapshotPending}
      loreEmpty={loreEmpty}
      loreItems={loreItems}
      directorPanelVisible={directorPanelVisible}
      stateDisplayPreference={storyStateDisplayPreference}
      onStorySelect={handleStorySelect}
      onStorySetupUpdate={handleStorySetupUpdate}
      onStoryDelete={handleDeleteStories}
      onStoryRename={handleRenameStory}
      onRequestLoreInit={onRequestLoreInit}
      onOpenDirectorConfig={() => {
        onOpenPresets?.()
        setMobileSnapshotOpen(false)
      }}
      onOpenWorldPacks={onOpenWorldPacks}
      onToggleDirectorPanel={isMobile ? () => setMobileSnapshotOpen((open) => !open) : onToggleRightPanel}
      onOpenDirectorState={openDirectorState}
      onRegisterOpenTurn={registerOpenTurn}
      onRequestCreateBranch={setBranchCreationSource}
      onStateDisplayPreferenceChange={handleStoryStateDisplayPreferenceChange}
      onTurnPersisted={handleTurnPersisted}
      onBranchPlanAdvanced={handleBranchPlanAdvanced}
      onDone={handleStoryStageDone}
    />
  )
  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div data-testid="interactive-shell" className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-bg)]">
        <div className="flex min-h-0 flex-1">
          <div className="flex min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            <motion.div key={contentKey} variants={panelPresence} initial="initial" animate="animate" transition={{ duration: 0.2, ease: novaEase }} className="flex min-h-0 flex-1 flex-col">
              {submode === 'timeline' ? (
                <BranchTimeline projectId={projectId} snapshot={displaySnapshot} branches={branches} currentBranchId={currentBranchId} onSwitchBranch={handleSwitchBranch} onCreateBranch={handleCreateBranch} onDeleteBranch={handleDeleteBranch} fill variant="workspace" onBackToStory={() => setSubmode('story')} headerControls={<StoryPicker stories={stories} currentStoryId={currentStoryId} onSelect={handleStorySelect} onDeleteStories={handleDeleteStories} onRenameStory={handleRenameStory} />} />
              ) : (
                <StoryWorkspace
                  rightPanelVisible={rightPanelVisible}
                  mobileConsoleOpen={mobileSnapshotOpen}
                  onMobileConsoleOpenChange={setMobileSnapshotOpen}
                  story={storyStage}
                  console={<WorldPresentationProvider story={currentStory} snapshot={displaySnapshot}><DirectorPanel
                      storyId={currentStoryId}
                      story={currentStory}
                      branchId={currentBranchId}
                      branches={branches}
                      snapshot={displaySnapshot}
                      onSwitchBranch={handleSwitchBranch}
                      onOpenBranchTimeline={openBranchTimeline}
                      onOpenTurn={handleOpenTurn}
                    /></WorldPresentationProvider>}
                />
              )}
            </motion.div>
          </div>
        </div>
      </div>
      <CreateBranchDialog
        projectId={projectId}
        source={branchCreationSource}
        onClose={() => setBranchCreationSource(null)}
        onCreate={(source, title, customAgentId) => handleCreateBranch(source.turnId, title, customAgentId)}
      />
    </div>
  )
}

function isGlobalStyleSceneName(scene: string) {
  const normalized = scene.trim().toLowerCase()
  return normalized === '全局' || normalized === 'global'
}
