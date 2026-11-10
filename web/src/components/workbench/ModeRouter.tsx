import { lazy, memo, useCallback, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/lib/toast'
import { WRITING_COMPOSER_SETTING_DEFAULTS } from '@/components/Chat/AgentPanel'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import { useInteractiveStore } from '@/features/interactive/stores/interactive-store'
import { useStoryIndexRecovery } from '@/features/interactive/use-story-index-recovery'
import type { AssistantHandoff } from '@/features/interactive/assistant-handoff'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'
import { isLoreItemsPath } from '@/lib/workspace-path'
import { useDocumentReview } from '@/features/document-review/use-document-review'
import { WorkbenchShell } from './WorkbenchShell'
import { useWorkbenchRouteHost, WorkbenchRouteLayer } from './WorkbenchRouteHost'
import type { ModeRouterProps } from './ModeRouter.types'
import { SharedWorkbenchRoutes } from './SharedWorkbenchRoutes'
import { GameChooser } from '@/features/history/GameChooser'
import { GameDestinationNav } from '@/features/history/GameDestinationNav'
import { GameSaveActions } from '@/features/interactive/components/GameSaveActions'
import { ToolNavigationProvider, type ToolNavigationIntent, type ToolNavigationTarget } from '@/components/Chat/tool-navigation'

const HistoryGame = memo(lazy(() => import('@/features/history/HistoryGame').then((module) => ({ default: module.HistoryGame }))))
const InteractiveLayout = memo(lazy(() => import('@/features/interactive/components/InteractiveLayout').then((module) => ({ default: module.InteractiveLayout }))))
const SettingPanel = memo(lazy(() => import('@/features/interactive/components/SettingPanel').then((module) => ({ default: module.SettingPanel }))))
const GameAssistantView = memo(lazy(() => import('@/features/interactive/components/game-settings/GameAssistantView').then((module) => ({ default: module.GameAssistantView }))))
const GameSettingsView = memo(lazy(() => import('@/features/interactive/components/game-settings/GameSettingsView').then((module) => ({ default: module.GameSettingsView }))))

export function ModeRouter(props: ModeRouterProps) {
  const { t } = useTranslation()
  const {
    mode,
    currentBookName,
    workspace,
    projectId,
    summary,
    activityBarExpanded,
    rightPanel,
    settingsOpen,
    interactiveRightVisible,
    books,
    loreItems,
    onSetMode,
    onToggleActivityBarExpanded,
    onSetRightPanel,
    onToggleSettings,
    onCloseSettings,
    onToggleInteractiveRightPanel,
    onQuickSwitchBook,
    onStartStory,
    onOpenStory,
    onOpenSave,
    onDeleteSave,
    onSelectFile,
    onOpenLoreTab,
    onEditorFlushHandlerChange,
  } = props

  const setInteractiveSubmode = useInteractiveStore((state) => state.setSubmode)
  const currentStoryId = useInteractiveStore((state) => state.currentStoryId)
  const currentBranchId = useInteractiveStore((state) => state.currentBranchId)
  const [toolNavigationIntent, setToolNavigationIntent] = useState<ToolNavigationIntent | null>(null)
  const [assistantHandoff, setAssistantHandoff] = useState<AssistantHandoff | null>(null)
  const toolNavigationNonceRef = useRef(0)
  const loreLibraryFlushHandlerRef = useRef<EditorFlushHandler | null>(null)
  // The router is the lifecycle owner: the settings lane survives panel close/unmount.
  const composerSettings = usePersistedUserSettings({ workspace, defaults: WRITING_COMPOSER_SETTING_DEFAULTS })
  const flushComposerSettings = composerSettings.flushPending

  const flushComposerSettingsBestEffort = useCallback(() => {
    void flushComposerSettings().then((saved) => {
      if (saved) return
      toast.warning(t('common.autosave.preferencesPending'), {
        description: t('common.autosave.preferencesPendingDetail'),
      })
    }).catch((error) => {
      console.warn('[ModeRouter.tsx] preference autosave flush failed during navigation; pending edits remain owned', { error })
      toast.warning(t('common.autosave.preferencesPending'), {
        description: t('common.autosave.preferencesPendingDetail'),
      })
    })
  }, [flushComposerSettings, t])

  const flushLoreLibraryDraft = useCallback(async (): Promise<boolean> => {
    const handler = loreLibraryFlushHandlerRef.current
    return handler ? handler() : true
  }, [])
  const handleLoreLibraryFlushHandlerChange = useCallback((handler: EditorFlushHandler | null) => {
    loreLibraryFlushHandlerRef.current = handler
    onEditorFlushHandlerChange(handler)
  }, [onEditorFlushHandlerChange])

  const quickSwitchBook = useCallback(async (path: string): Promise<boolean> => {
    flushComposerSettingsBestEffort()
    if (!(await flushLoreLibraryDraft())) return false
    return onQuickSwitchBook(path)
  }, [flushComposerSettingsBestEffort, flushLoreLibraryDraft, onQuickSwitchBook])

  const loreEmpty = Boolean(workspace) && loreItems.length === 0
  const selectWorkspacePath = useCallback((path: string) => {
    if (isLoreItemsPath(path)) return onOpenLoreTab()
    return onSelectFile(path)
  }, [onOpenLoreTab, onSelectFile])

  const openToolNavigationTarget = useCallback((target: ToolNavigationTarget) => {
    toolNavigationNonceRef.current += 1
    if (target.kind === 'workspace_file') {
      void selectWorkspacePath(target.path)
      return
    }
    setToolNavigationIntent({ target, nonce: toolNavigationNonceRef.current })
    if (target.kind === 'lore_item') {
      onSetMode('lore')
      return
    }

    // Every one of these resources is consumed by the game and is edited in
    // Game settings. The authoring catalogs with no player route (skills, agent
    // profiles) land there too, because that page is the only place the content
    // the game actually reads can still be changed.
    switch (target.resource) {
      case 'skill':
      case 'agent_profile':
        onSetMode('assistant')
        return
      case 'style_reference':
      case 'narrative_style':
      case 'game_planning':
      case 'event_package':
      case 'rule_system':
      case 'state_system':
      case 'image_preset':
        onSetMode('game-settings')
        return
    }
  }, [onSetMode, selectWorkspacePath])
  const toolNavigation = useMemo(() => ({ workspace, open: openToolNavigationTarget }), [openToolNavigationTarget, workspace])

  // One independent assistant serves both the lore library and Game settings; an entry
  // there hands over what it was showing and this route opens on it.
  const openAssistant = useCallback((handoff: AssistantHandoff) => {
    setAssistantHandoff(handoff)
    onSetMode('assistant')
  }, [onSetMode])
  const consumeAssistantHandoff = useCallback((key: string) => {
    setAssistantHandoff((current) => (current?.key === key ? null : current))
  }, [])

  const requestLoreInit = useCallback(() => {
    onSetMode('lore')
  }, [onSetMode])
  const showAgent = useCallback(() => onSetRightPanel('ai'), [onSetRightPanel])
  const documentReview = useDocumentReview({
    projectId,
    agentVisible: rightPanel === 'ai',
    onShowAgent: showAgent,
  })
  const documentReviewController = useMemo(() => ({
    comments: documentReview.visibleComments,
    onCreate: documentReview.addComment,
    onUpdate: documentReview.editComment,
    onDelete: documentReview.removeComment,
  }), [documentReview.addComment, documentReview.editComment, documentReview.removeComment, documentReview.visibleComments])
  const routeHost = useWorkbenchRouteHost({ mode, rightPanel, settingsOpen })
  const presentedMainRoute = routeHost.route
  // The game surface keeps the story index current while it is mounted, and Game
  // settings reads the same index itself. The lore library does neither, so a
  // direct reload onto it has to restore the current story here.
  useStoryIndexRecovery(workspace, (mode === 'lore' || mode === 'assistant') && !routeHost.isMounted('interactive'))

  const main = (
    <main className="relative h-full min-w-0 overflow-hidden bg-[var(--nova-bg)]">
      {routeHost.isMounted('interactive') && (
        <WorkbenchRouteLayer visible={presentedMainRoute === 'interactive'} loadingLabel={t('router.loading')}>
          <GameDestinationNav current="interactive" onNavigate={onSetMode} />
          <GameSaveActions projectId={projectId} onStartStory={onStartStory} onOpenSave={onOpenSave} onDeleteSave={onDeleteSave} />
          <InteractiveLayout
            projectId={projectId}
            workspace={workspace}
            active={presentedMainRoute === 'interactive'}
            recentNarrativeStyleID={composerSettings.values.interactive_story_teller_id}
            narrativeStyleLoading={composerSettings.loading}
            loreEmpty={loreEmpty}
            loreItems={loreItems}
            onRequestLoreInit={requestLoreInit}
            onOpenPresets={() => onSetMode('game-settings')}
            onOpenWorldPacks={() => onSetMode('books')}
            rightPanelVisible={interactiveRightVisible}
            onToggleRightPanel={onToggleInteractiveRightPanel}
          />
        </WorkbenchRouteLayer>
      )}

      {routeHost.isMounted('games') && (
        <WorkbenchRouteLayer visible={presentedMainRoute === 'games'} loadingLabel={t('router.loading')}><GameChooser onNavigate={onSetMode} /></WorkbenchRouteLayer>
      )}
      {routeHost.isMounted('history') && (
        <WorkbenchRouteLayer visible={presentedMainRoute === 'history'} loadingLabel={t('router.loading')}>
          <GameDestinationNav current="history" onNavigate={onSetMode} />
          <div className="min-h-0 flex-1"><HistoryGame active={presentedMainRoute === 'history'} /></div>
        </WorkbenchRouteLayer>
      )}

      {routeHost.isMounted('lore') && (
        <WorkbenchRouteLayer visible={presentedMainRoute === 'lore'} loadingLabel={t('router.loading')}>
          <SettingPanel
            mode="lore"
            projectId={projectId}
            storyId={currentStoryId}
            branchId={currentBranchId}
            documentReview={documentReviewController}
            onFlushHandlerChange={handleLoreLibraryFlushHandlerChange}
            toolNavigationIntent={toolNavigationIntent}
            onOpenAssistant={openAssistant}
          />
        </WorkbenchRouteLayer>
      )}
      {routeHost.isMounted('game-settings') && (
        <WorkbenchRouteLayer visible={presentedMainRoute === 'game-settings'} loadingLabel={t('router.loading')}>
          <GameSettingsView
            projectId={projectId}
            visible={presentedMainRoute === 'game-settings'}
            onOpenBooks={() => onSetMode('books')}
            onOpenAssistant={() => onSetMode('assistant')}
            toolNavigationIntent={toolNavigationIntent}
          />
        </WorkbenchRouteLayer>
      )}

      {routeHost.isMounted('assistant') && (
        <WorkbenchRouteLayer visible={presentedMainRoute === 'assistant'} loadingLabel={t('router.loading')}>
          <GameAssistantView projectId={projectId} visible={presentedMainRoute === 'assistant'}
            assistantHandoff={assistantHandoff} onAssistantHandoffConsumed={consumeAssistantHandoff} />
        </WorkbenchRouteLayer>
      )}
      <SharedWorkbenchRoutes
        route={presentedMainRoute}
        isMounted={routeHost.isMounted}
        loadingLabel={t('router.loading')}
        home={{ projectId, onStartStory, onOpenStory, onOpenGameSettings: () => onSetMode('game-settings') }}
      />
    </main>
  )

  return (
    <ToolNavigationProvider value={toolNavigation}>
      <WorkbenchShell
        mode={mode}
        presentedLayout={routeHost.layout}
        currentBookName={currentBookName}
        workspace={workspace}
        books={books}
        summary={summary}
        activityBarExpanded={activityBarExpanded}
        settingsOpen={settingsOpen}
        main={main}
        onSetMode={onSetMode}
        onToggleActivityBarExpanded={onToggleActivityBarExpanded}
        onSetInteractiveSubmode={setInteractiveSubmode}
        onToggleSettings={onToggleSettings}
        onCloseSettings={onCloseSettings}
        onQuickSwitchBook={quickSwitchBook}
      />
    </ToolNavigationProvider>
  )
}
