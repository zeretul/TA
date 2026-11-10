import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { fetchProjectSettings, fetchSettings, refreshProjectSettings, refreshSettings } from '@/features/settings/api'
import { subscribeSettingsTarget } from '@/features/settings/query'
import { useStartupUpdateCheck } from '@/features/settings/use-startup-update-check'
import { applyFontSettings, fontSettingsFromEffective } from '@/features/settings/font-variables'
import { getProjectLoreItems, switchWorkspace, type LoreItem } from '@/lib/api'
import { withErrorLogID } from '@/lib/api-client'
import { CommandPalette } from '@/components/common/command-palette'
import { useWorkspace } from '@/hooks/useWorkspace'
import { useWorkspaceHotkeys } from '@/hooks/use-workspace-hotkeys'
import { useWorkspaceStore, workspaceModeRequiresBook, type RightPanel, type WorkspaceMode } from '@/stores/workspace-store'
import { useInteractiveStore } from '@/features/interactive/stores/interactive-store'
import { toast } from '@/lib/toast'
import { setConfiguredLocale } from '@/i18n'
import { NovaMotionProvider, normalizeMotionIntensity } from '@/features/motion/motion-preferences'
import { applyClipPlaybackPreference } from '@/features/interactive/components/character-window/clip-playback-preference'
import { ModeRouter } from '@/components/workbench/ModeRouter'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import { SETTINGS_SECTION_EVENT } from '@/features/onboarding/events'
import {
  AUTOSAVE_CONFLICT_PRESERVED_EVENT,
  type AutosaveConflictPreservedDetail,
} from '@/lib/autosave/rebase-with-recovery'
import { LORE_UPDATED_EVENT, type LoreUpdatedDetail } from '@/features/lore/events'
import type { GameSave } from '@/features/interactive/game-saves'
import { deleteInteractiveStory, getInteractiveStories, selectInteractiveStory } from '@/features/interactive/api'
import type { StorySummary } from '@/features/interactive/types'
import { startStoryPack, type StoryPackIdentityChoice } from '@/lib/api-client'

const ACTIVITY_BAR_EXPANDED_KEY = 'ta.layout.activityBarExpanded'
const INTERACTIVE_RIGHT_VISIBLE_KEY = 'ta.layout.interactiveRightVisible'
const SETTINGS_OPEN_KEY = 'ta.layout.settingsOpen'

function App() {
  useStartupUpdateCheck()
  const { t } = useTranslation()
  const { setTheme } = useTheme()
  const [activityBarExpanded, setActivityBarExpanded] = useState(() => readLayoutBoolean(ACTIVITY_BAR_EXPANDED_KEY, true))
  const [interactiveRightVisible, setInteractiveRightVisible] = useState(() => readLayoutBoolean(INTERACTIVE_RIGHT_VISIBLE_KEY, true))
  const [settingsOpen, setSettingsOpen] = useState(() => readLayoutBoolean(SETTINGS_OPEN_KEY, false))
  useEffect(() => {
    const openSettings = () => setSettingsOpen(true)
    window.addEventListener(SETTINGS_SECTION_EVENT, openSettings)
    return () => window.removeEventListener(SETTINGS_SECTION_EVENT, openSettings)
  }, [])
  const [motionIntensity, setMotionIntensity] = useState('system')
  const [loreItems, setLoreItems] = useState<LoreItem[]>([])
  const editorFlushHandlerRef = useRef<EditorFlushHandler | null>(null)

  const rightPanel = useWorkspaceStore((state) => state.rightPanel)
  const commandOpen = useWorkspaceStore((state) => state.commandOpen)
  const mode = useWorkspaceStore((state) => state.mode)
  const setRightPanel = useWorkspaceStore((state) => state.setRightPanel)
  const setCommandOpen = useWorkspaceStore((state) => state.setCommandOpen)
  const setMode = useWorkspaceStore((state) => state.setMode)
  const toggleActivityBarExpanded = useCallback(() => setActivityBarExpanded((value) => !value), [])
  const toggleSettings = useCallback(() => setSettingsOpen((open) => !open), [])
  const closeSettings = useCallback(() => setSettingsOpen(false), [])
  const toggleInteractiveRightPanel = useCallback(() => setInteractiveRightVisible((value) => !value), [])

  const {
    selectedFile, workspace, projectId, workspaceLoaded, summary, books,
    selectFile, clearSelectedFile, refreshAll,
  } = useWorkspace()
  const settingsWorkspaceRef = useRef<string | null>(null)

  const handleEditorFlushHandlerChange = useCallback((handler: EditorFlushHandler | null) => {
    editorFlushHandlerRef.current = handler
  }, [])

  const flushEditorDraft = useCallback(async () => {
    const handler = editorFlushHandlerRef.current
    if (!handler) return true
    try {
      return await handler()
    } catch (error) {
      console.error('[App.tsx] failed to flush the editor draft before navigation', error)
      toast.error(withErrorLogID(t('editor.saveFailed'), error))
      return false
    }
  }, [t])

  const refreshLoreItems = useCallback(async () => {
    if (!projectId) {
      setLoreItems([])
      return
    }
    try {
      setLoreItems(await getProjectLoreItems(projectId))
    } catch (e) {
      console.warn('[App.tsx] failed to load lore items', e)
      setLoreItems([])
    }
  }, [projectId])

  useEffect(() => {
    void refreshLoreItems()
    const onLoreUpdated = (event: Event) => {
      const detail = (event as CustomEvent<LoreUpdatedDetail>).detail
      if (detail?.projectId === projectId) void refreshLoreItems()
    }
    window.addEventListener(LORE_UPDATED_EVENT, onLoreUpdated)
    return () => window.removeEventListener(LORE_UPDATED_EVENT, onLoreUpdated)
  }, [refreshLoreItems])

  const currentBookName = workspaceLoaded
    ? summary?.title?.trim() ||
      books.find((book) => book.path === workspace)?.name?.trim() ||
      workspace.replace(/\/+$/, '').split('/').pop() ||
      t('workbench.noBook')
    : t('common.loading')

  useEffect(() => {
    let cancelled = false
    const target = projectId ? { kind: 'project' as const, projectId } : { kind: 'global' as const }
    const applySettings = (data: Awaited<ReturnType<typeof fetchSettings>>) => {
      if (cancelled) return
      const effective = data.effective
      setConfiguredLocale(effective?.language)
      setTheme(normalizeAppTheme(effective?.theme))
      setMotionIntensity(normalizeMotionIntensity(effective?.motion_intensity))
      applyClipPlaybackPreference(effective?.clips_enabled)
      applyFontSettings(fontSettingsFromEffective(effective))
    }
    const reload = (fresh = false) => {
      const request = projectId
        ? (fresh ? refreshProjectSettings(projectId) : fetchProjectSettings(projectId))
        : (fresh ? refreshSettings() : fetchSettings())
      request
        .then(applySettings)
        .catch((e) => console.warn('[App.tsx] failed to load interface settings', e))
    }
    const workspaceChanged = workspaceLoaded
      && settingsWorkspaceRef.current !== null
      && settingsWorkspaceRef.current !== projectId
    if (workspaceLoaded) settingsWorkspaceRef.current = projectId
    reload(workspaceChanged)
    const unsubscribe = subscribeSettingsTarget(target, applySettings)
    return () => {
      cancelled = true
      unsubscribe()
    }
  }, [projectId, setTheme, workspaceLoaded])

  useEffect(() => {
    const onConflictPreserved = (event: Event) => {
      const detail = (event as CustomEvent<AutosaveConflictPreservedDetail>).detail
      if (!detail?.id) return
      toast.warning(t('common.autosave.conflictPreserved'), {
        description: t('common.autosave.conflictPreservedDetail', { id: detail.id }),
      })
    }
    window.addEventListener(AUTOSAVE_CONFLICT_PRESERVED_EVENT, onConflictPreserved)
    return () => window.removeEventListener(AUTOSAVE_CONFLICT_PRESERVED_EVENT, onConflictPreserved)
  }, [t])

  useEffect(() => { window.localStorage.setItem(ACTIVITY_BAR_EXPANDED_KEY, String(activityBarExpanded)) }, [activityBarExpanded])
  useEffect(() => { window.localStorage.setItem(INTERACTIVE_RIGHT_VISIBLE_KEY, String(interactiveRightVisible)) }, [interactiveRightVisible])
  useEffect(() => { window.localStorage.setItem(SETTINGS_OPEN_KEY, String(settingsOpen)) }, [settingsOpen])

  useEffect(() => {
    if (workspace || !workspaceLoaded) return
    clearSelectedFile()
    // The independent history game and game chooser do not need an adventure Book.
    if (workspaceModeRequiresBook(mode)) setMode(mode === 'interactive' ? 'games' : 'books')
  }, [clearSelectedFile, mode, setMode, workspace, workspaceLoaded])

  const handleQuickWorkspaceSwitch = useCallback(async (newPath: string): Promise<boolean> => {
    if (!newPath || newPath === workspace) return true
    if (!(await flushEditorDraft())) return false
    try {
      const result = await switchWorkspace(newPath)
      const nextWorkspace = result.workspace || newPath
      console.info('[App.tsx] title-bar Book switch completed', { from: workspace, to: nextWorkspace })
      await refreshAll()
      return true
    } catch (error) {
      console.error('[App.tsx] title-bar Book switch failed', { from: workspace, to: newPath, error })
      toast.error(t('workbench.bookSwitcher.switchError'), {
        description: error instanceof Error ? error.message : String(error),
      })
      return false
    }
  }, [flushEditorDraft, refreshAll, t, workspace])

  const handleSelectFile = useCallback(async (path: string) => {
    if (selectedFile === path) return true
    if (!(await flushEditorDraft())) return false
    const result = await selectFile(path)
    return result === 'selected'
  }, [flushEditorDraft, selectFile, selectedFile])

  const handleOpenLoreTab = useCallback(async () => {
    if (!(await flushEditorDraft())) return false
    clearSelectedFile()
    return true
  }, [clearSelectedFile, flushEditorDraft])

  const handleSetMode = useCallback((nextMode: WorkspaceMode) => {
    setSettingsOpen(false)
    setMode(nextMode)
  }, [setMode])
  const handleSetRightPanel = useCallback((panel: RightPanel) => {
    setSettingsOpen(false)
    setRightPanel(panel)
  }, [setRightPanel])

  // Starting a world from the world page is one backend operation: it creates
  // the Project, writes the pack's world book, and creates the story bound to
  // that pack revision. CreateBook already makes the new Project the open one,
  // so only a stale foreground needs an explicit switch.
  const handleStartStoryFromPack = useCallback(async (packId: string, templateId?: string, identity?: StoryPackIdentityChoice) => {
    if (!(await flushEditorDraft())) return
    const started = await startStoryPack(packId, templateId, identity)
    // Record the handoff before the workspace switch, because switching
    // resets the interactive store and the new stage has to know that this
    // story's opening is still owed.
    useInteractiveStore.getState().setPendingOpeningStoryId(started.story.id)
    if (started.workspace && started.workspace !== workspace) {
      await switchWorkspace(started.workspace)
    }
    await refreshAll()
    console.info('[App.tsx] started a world from a pack', {
      packId: started.pack_id,
      templateId: templateId ?? '',
      created: Boolean(identity),
      projectId: started.project_id,
      storyId: started.story.id,
    })
    useInteractiveStore.getState().setCurrentStoryId(started.story.id)
    useInteractiveStore.getState().setSubmode('story')
    handleSetMode('interactive')
  }, [flushEditorDraft, handleSetMode, refreshAll, workspace])

  // The world page lists the stories of the open Project, so opening one only
  // has to make the selection durable and show the game surface.
  const handleOpenStoryFromWorldPage = useCallback(async (story: StorySummary) => {
    if (!(await flushEditorDraft())) return
    useInteractiveStore.getState().setCurrentStoryId(story.id)
    await selectInteractiveStory(story.id)
    useInteractiveStore.getState().setSubmode('story')
    handleSetMode('interactive')
  }, [flushEditorDraft, handleSetMode])

  const handleOpenGameSave = useCallback(async (save: GameSave) => {
    if (!(await flushEditorDraft())) throw new Error(t('gameSaves.cancelled'))
    if (save.workspace !== workspace) {
      await switchWorkspace(save.workspace)
      await refreshAll()
    }
    await selectInteractiveStory(save.story.id)
    useInteractiveStore.getState().setCurrentStoryId(save.story.id)
    useInteractiveStore.getState().setSubmode('story')
    handleSetMode('interactive')
  }, [flushEditorDraft, handleSetMode, refreshAll, t, workspace])

  const handleDeleteGameSave = useCallback(async (save: GameSave) => {
    if (!(await flushEditorDraft())) throw new Error(t('gameSaves.cancelled'))
    // The existing Story deletion drains its runs and removes bindings and
    // attachments. Reuse that boundary; never unlink another world's journal.
    let switched = false
    try {
      if (save.workspace !== workspace) {
        await switchWorkspace(save.workspace)
        switched = true
      }
      await deleteInteractiveStory(save.story.id)
    } finally {
      if (switched) await switchWorkspace(workspace)
      await refreshAll()
      const index = await getInteractiveStories()
      useInteractiveStore.getState().setStories(index.stories || [], index.current_story_id)
    }
  }, [flushEditorDraft, refreshAll, t, workspace])

  const toggleRightPanel = useCallback(() => {
    if (mode === 'interactive') {
      setInteractiveRightVisible((value) => !value)
      return
    }
    handleSetRightPanel(rightPanel ? null : 'ai')
  }, [handleSetRightPanel, mode, rightPanel])

  useWorkspaceHotkeys({
    onOpenCommand: () => setCommandOpen(true),
    onToggleRightPanel: toggleRightPanel,
  })

  return (
    <NovaMotionProvider intensity={motionIntensity}>
      <ModeRouter
        mode={mode}
        currentBookName={currentBookName}
        workspace={workspace}
        projectId={projectId}
        summary={summary}
        activityBarExpanded={activityBarExpanded}
        rightPanel={rightPanel}
        settingsOpen={settingsOpen}
        interactiveRightVisible={interactiveRightVisible}
        books={books}
        loreItems={loreItems}
        onSetMode={handleSetMode}
        onToggleActivityBarExpanded={toggleActivityBarExpanded}
        onSetRightPanel={handleSetRightPanel}
        onToggleSettings={toggleSettings}
        onCloseSettings={closeSettings}
        onToggleInteractiveRightPanel={toggleInteractiveRightPanel}
        onQuickSwitchBook={handleQuickWorkspaceSwitch}
        onStartStory={handleStartStoryFromPack}
        onOpenStory={handleOpenStoryFromWorldPage}
        onOpenSave={handleOpenGameSave}
        onDeleteSave={handleDeleteGameSave}
        onSelectFile={handleSelectFile}
        onOpenLoreTab={handleOpenLoreTab}
        onEditorFlushHandlerChange={handleEditorFlushHandlerChange}
      />
      <CommandPalette
        open={commandOpen}
        onOpenChange={setCommandOpen}
        onToggleRightPanel={toggleRightPanel}
      />
    </NovaMotionProvider>
  )
}

function readLayoutBoolean(key: string, fallback: boolean) {
  if (typeof window === 'undefined') return fallback
  const value = window.localStorage.getItem(key)
  if (value === null) return fallback
  return value === 'true'
}

function normalizeAppTheme(theme?: string) {
  if (theme === 'light' || theme === 'dark' || theme === 'system') return theme
  return 'dark'
}

export default App
