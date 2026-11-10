import { resolveWorldPresentation } from '../../world-presentation'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useReducedMotionConfig } from 'motion/react'
import type { AnimationPlaybackControls } from 'motion/react'
import { Bot, BookMarked, Globe, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { EmptyState } from '@/components/common/EmptyState'
import { LoadingState } from '@/components/common/LoadingState'
import { SectionedNavigation } from '@/components/navigation/sectioned-navigation'
import { ModelProfileSwitcher } from '@/components/Chat/ModelProfileSwitcher'
import { Textarea } from '@/components/ui/textarea'
import { SharedNarrativeRulesEditor } from '../setting-panel/CreatorEditor'
import type { ToolNavigationIntent } from '@/components/Chat/tool-navigation'
import { useConversationConfig } from '@/features/conversation-config/use-conversation-config'
import type { ConversationConfigBinding } from '@/features/conversation-config/types'
import { SettingsPageFrame } from '@/features/settings/SettingsPageFrame'
import { scrollSettingsSectionIntoView } from '@/features/settings/settings-section-scroll'
import { APIError, listStoryPacks, saveStoryPackSettings, type StoryPackSummary } from '@/lib/api-client'
import { toast } from '@/lib/toast'
import { errorMessage } from '@/lib/error-diagnostics'
import {
  WorldSettingsEditor,
  storyPackSettingsFrom,
  worldSettingsValue,
  type WorldSettingsValue,
} from '@/components/Home/WorldSettingsEditor'
import {
  getGamePlanningTemplates,
  getImagePresets,
  getInteractiveStories,
  getInteractiveSnapshot,
  getInteractiveTellers,
  updateInteractiveStory,
} from '../../api'
import { useInteractiveStore } from '../../stores/interactive-store'
import { useNarrativeStyleCatalog } from '../../use-narrative-style-catalog'
import { mergePreferredStory } from '../../story-index'
import type {
  GamePlanningTemplate,
  ImagePreset,
  InteractiveStoryUpdateInput,
  StorySummary,
  Snapshot,
  Teller,
} from '../../types'
import { ControlSection, TuningRow } from '../director-console/StoryTuningControls'
import { IntimacyModelSetting } from './IntimacyModelSetting'
import { StoryTuningView } from '../director-console/StoryTuningView'
import {
  readStoryStateDisplayPreference,
  writeStoryStateDisplayPreference,
  type StoryStateDisplayPreference,
} from '../story-state/display-preference'

type GameSettingsSectionId = 'world' | 'story' | 'narrative' | 'rules' | 'media' | 'creator'

const SECTION_IDS: GameSettingsSectionId[] = ['world', 'story', 'narrative', 'rules', 'media', 'creator']

/**
 * Where a resource a tool result links to is actually changed on this page.
 *
 * Every one of these catalogs used to open its own editor. They all live here
 * now, and a link that only switches to this page leaves the player scrolling
 * for the control it named, so each resource names the section that holds it.
 * Style, planning and the narration settings share one section; the check and
 * state systems share another; image presets sit with media. The authoring
 * catalogs with no player route (skills, agent profiles) have no section of
 * their own and belong to the assistant, which is the only surface that can
 * still change them.
 */
const RESOURCE_SECTIONS: Record<string, GameSettingsSectionId> = {
  style_reference: 'narrative',
  narrative_style: 'narrative',
  game_planning: 'narrative',
  event_package: 'narrative',
  rule_system: 'rules',
  state_system: 'rules',
  image_preset: 'media',
}

function sameWorldSettings(a: WorldSettingsValue, b: WorldSettingsValue): boolean {
  return a.ruleSystemId === b.ruleSystemId
    && a.planningMode === b.planningMode
    && a.planningTemplateId === b.planningTemplateId
    && a.styleMain === b.styleMain
    && a.styleDialogue === b.styleDialogue
    && a.styleCombat === b.styleCombat
    && a.replyTargetChars === b.replyTargetChars
    && a.narrativeRequirements === b.narrativeRequirements
    && a.writingPreferences === b.writingPreferences
    && a.difficultyShift === b.difficultyShift
    && a.rollModifier === b.rollModifier
    && a.stateConsumptionMode === b.stateConsumptionMode
    && a.visibilityMode === b.visibilityMode
}

interface GameSettingsViewProps {
  projectId: string
  visible: boolean
  onOpenBooks: () => void
  onOpenAssistant: () => void
  /**
   * A resource link from a tool result. It already moved this page in front, so
   * all that is left is to scroll to the section that holds that resource.
   */
  toolNavigationIntent?: ToolNavigationIntent | null
}

/**
 * The player's settings for the story they are playing.
 *
 * Everything here is scoped to one story (or, where a field is empty, to that
 * story's world pack default). Account, connection and appearance settings stay
 * in System settings; mixing the two is what made one story's model or rules
 * leak into another.
 */
export function GameSettingsView({ projectId, visible, onOpenBooks, onOpenAssistant, toolNavigationIntent }: GameSettingsViewProps) {
  const { t, i18n } = useTranslation()
  const locale = i18n.resolvedLanguage || i18n.language
  const reducedMotion = useReducedMotionConfig()
  const stories = useInteractiveStore((state) => state.stories)
  const playingStoryId = useInteractiveStore((state) => state.currentStoryId)
  const playingBranchId = useInteractiveStore((state) => state.currentBranchId)
  const playingSnapshot = useInteractiveStore((state) => state.snapshot)
  const [editingStoryId, setEditingStoryId] = useState('')
  const [loadedSnapshot, setLoadedSnapshot] = useState<{ projectId: string; snapshot: Snapshot } | null>(null)
  const [editingBranch, setEditingBranch] = useState<{ storyId: string; branchId: string } | null>(null)
  const requestEpoch = useRef(0)
  const catalogProject = useRef('')
  useEffect(() => {
    requestEpoch.current += 1
    catalogProject.current = ''
    setEditingStoryId('')
    setEditingBranch(null)
    setLoadedSnapshot(null)
    setWorldPackId('')
    setWorldSaving(false)
    return () => { requestEpoch.current += 1 }
  }, [projectId])
  const currentStoryId = stories.some((story) => story.id === editingStoryId) ? editingStoryId : playingStoryId
  const currentBranchId = currentStoryId === playingStoryId ? playingBranchId || 'main' : editingBranch?.storyId === currentStoryId ? editingBranch.branchId : ''
  const setStories = useInteractiveStore((state) => state.setStories)
  const [tellers, setTellers] = useState<Teller[]>([])
  const [planningTemplates, setPlanningTemplates] = useState<GamePlanningTemplate[]>([])
  const [imagePresets, setImagePresets] = useState<ImagePreset[]>([])
  const [loading, setLoading] = useState(true)
  const [loadFailed, setLoadFailed] = useState(false)
  const [stateDisplayPreference, setStateDisplayPreference] = useState<StoryStateDisplayPreference>(readStoryStateDisplayPreference)
  const [packs, setPacks] = useState<StoryPackSummary[]>([])
  const [worldPackId, setWorldPackId] = useState('')
  const [worldDraft, setWorldDraft] = useState<WorldSettingsValue | null>(null)
  const [worldSaved, setWorldSaved] = useState<WorldSettingsValue | null>(null)
  const [worldRevision, setWorldRevision] = useState('')
  const [worldSaving, setWorldSaving] = useState(false)
  const [worldConflict, setWorldConflict] = useState(false)
  const [worldNotice, setWorldNotice] = useState('')
  const [activeSection, setActiveSection] = useState<GameSettingsSectionId>('story')
  const contentRef = useRef<HTMLDivElement | null>(null)
  const sectionRefs = useRef<Partial<Record<GameSettingsSectionId, HTMLElement | null>>>({})
  const scrollTargetRef = useRef<GameSettingsSectionId | null>(null)
  const scrollAnimationRef = useRef<AnimationPlaybackControls | null>(null)

  const currentStory = stories.find((story) => story.id === currentStoryId)
  const matchingPlayingSnapshot = currentStoryId === playingStoryId
    && playingSnapshot?.story_id === currentStoryId && playingSnapshot.branch_id === currentBranchId ? playingSnapshot : null
  const currentSnapshot = matchingPlayingSnapshot || (loadedSnapshot?.projectId === projectId
    && loadedSnapshot.snapshot.story_id === currentStoryId
    && (!currentBranchId || loadedSnapshot.snapshot.branch_id === currentBranchId) ? loadedSnapshot.snapshot : null)
  useEffect(() => {
    if (!currentStoryId || currentSnapshot) return
    let cancelled = false
    void getInteractiveSnapshot(currentStoryId).then((result) => {
      if (!cancelled && result.story_id === currentStoryId) {
        setEditingBranch({ storyId: currentStoryId, branchId: result.branch_id })
        setLoadedSnapshot({ projectId, snapshot: result })
      }
    }).catch((error) => console.error('[game-settings] Failed to resolve editing story schema', error))
    return () => { cancelled = true }
  }, [currentStoryId, playingStoryId, currentBranchId, currentSnapshot, projectId])
  // The dual-cultivation activity is a cultivation world's own affair: a
  // detective story has no such thing, so it must not be offered a model for it.
  const dualCultivationAvailable = resolveWorldPresentation({ story: currentStory, snapshot: currentSnapshot }, locale, t).intimacyAvailable
  // The list endpoint omits the styles the recommended set absorbed, so a story
  // still running on one would show a raw id here. The catalog fills those in.
  const styleTellers = useNarrativeStyleCatalog(tellers, currentStory)

  const reloadStories = useCallback(async (preferredStory?: StorySummary) => {
    const epoch = requestEpoch.current
    const index = await getInteractiveStories()
    if (epoch !== requestEpoch.current) return
    setStories(mergePreferredStory(index.stories || [], preferredStory), useInteractiveStore.getState().currentStoryId || index.current_story_id)
  }, [setStories])

  useEffect(() => {
    let cancelled = false
    const refreshLabelsOnly = catalogProject.current !== '' && catalogProject.current === projectId
    if (!refreshLabelsOnly) setLoading(true)
    setLoadFailed(false)
    void Promise.all([
      ...(refreshLabelsOnly ? [] : [reloadStories()]),
      getInteractiveTellers().then((next) => { if (!cancelled) setTellers(next) }),
      getGamePlanningTemplates().then((next) => { if (!cancelled) setPlanningTemplates(next) }),
      getImagePresets().then((next) => { if (!cancelled) setImagePresets(next) }),
      // A machine with no readable world pack still has story-scoped settings to
      // show, so this failure stays local instead of blanking the whole page.
      listStoryPacks()
        .then((next) => {
          if (cancelled) return
          // A language switch refreshes display catalogs, not editable defaults.
          // Keep their baseline/revision even if the server has newer settings;
          // saving still detects that conflict without discarding a dirty draft.
          setPacks((current) => refreshLabelsOnly ? next.map((pack) => {
            const previous = current.find((entry) => entry.id === pack.id)
            return previous ? { ...pack, settings: previous.settings, settings_revision: previous.settings_revision, settings_customized: previous.settings_customized } : pack
          }) : next)
          catalogProject.current = projectId
        })
        .catch((error) => {
          console.error('[game-settings] Failed to read the world packs', error)
        }),
    ])
      .catch((error) => {
        if (cancelled) return
        console.error('[game-settings] Failed to load the story settings resources', error)
        setLoadFailed(true)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [locale, projectId, reloadStories])

  // This endpoint is project-scoped. Binding before the workspace has resolved its
  // project would send an unscoped request that the server rejects with 400, so the
  // config simply stays unbound until the project is known.
  const conversationBinding = useMemo<ConversationConfigBinding | undefined>(() => projectId && currentStoryId && currentBranchId ? {
    mode: 'interactive',
    project_id: projectId,
    story_id: currentStoryId,
    branch_id: currentBranchId,
  } : undefined, [currentBranchId, currentStoryId, projectId])
  const conversationConfig = useConversationConfig(conversationBinding)

  const handleStoryUpdate = useCallback(async (input: InteractiveStoryUpdateInput) => {
    const storyId = currentStoryId
    if (!storyId) return
    const epoch = requestEpoch.current
    const updated = await updateInteractiveStory(storyId, input)
    if (epoch !== requestEpoch.current) return
    setStories(mergePreferredStory(useInteractiveStore.getState().stories, updated), useInteractiveStore.getState().currentStoryId)
    await reloadStories(updated)
  }, [currentStoryId, reloadStories, setStories])

  const handlePlanningTemplateChange = useCallback(async (templateId: string) => {
    await handleStoryUpdate({ planning_template_id: templateId })
  }, [handleStoryUpdate])

  // Settings selection is local: inspecting another story must not switch play.
  const handleStorySelect = useCallback((storyId: string) => setEditingStoryId(storyId), [])

  const handleStateDisplayPreferenceChange = useCallback((value: StoryStateDisplayPreference) => {
    setStateDisplayPreference(value)
    writeStoryStateDisplayPreference(value)
  }, [])

  // The world whose defaults this page edits. It follows the story being played
  // until the player picks another one; picking one only retargets the editor,
  // so no story, branch or game session moves.
  const worldPack = useMemo(
    () => packs.find((pack) => pack.id === worldPackId),
    [packs, worldPackId],
  )
  // The story-scoped panel describes the story being played, so its suggestion
  // is that story's own world's. Falling back to the world being edited keeps a
  // story with no binding from showing no suggestion at all.
  const storyRecommendedPlanningTemplateId = useMemo(() => {
    const boundID = currentStory?.pack_binding?.pack_id
    const bound = boundID ? packs.find((pack) => pack.id === boundID) : undefined
    return bound?.recommended_planning_template_id ?? worldPack?.recommended_planning_template_id
  }, [currentStory, packs, worldPack])
  // The world's own rule system, read from the modules the pack already
  // declares rather than from a second recommendation field: a pack binds
  // exactly one rule system, so that binding is its suggestion.
  const storyRecommendedRuleSystemId = useMemo(() => {
    const boundID = currentStory?.pack_binding?.pack_id
    const bound = boundID ? packs.find((pack) => pack.id === boundID) : undefined
    const fromPack = (pack: typeof worldPack) =>
      pack?.modules?.find((module) => module.kind === 'rule_system')?.id
    return fromPack(bound) ?? fromPack(worldPack)
  }, [currentStory, packs, worldPack])
  useEffect(() => {
    if (packs.length === 0) return
    setWorldPackId((current) => {
      if (current && packs.some((pack) => pack.id === current)) return current
      const bound = currentStory?.pack_binding?.pack_id
      if (bound && packs.some((pack) => pack.id === bound)) return bound
      return packs[0].id
    })
  }, [currentStory, packs])

  // Reset only for a different editing target or settings block, never for a
  // translated display catalog object. Locale refreshes keep the block above.
  useEffect(() => {
    if (!worldPack) {
      setWorldDraft(null)
      setWorldSaved(null)
      return
    }
    const resolved = worldSettingsValue(worldPack.settings)
    setWorldDraft(resolved)
    setWorldSaved(resolved)
    setWorldRevision(worldPack.settings_revision || '')
    setWorldConflict(false)
    setWorldNotice('')
  }, [projectId, worldPack?.id, worldPack?.settings, worldPack?.settings_revision])

  const worldDirty = Boolean(worldDraft && worldSaved && !sameWorldSettings(worldDraft, worldSaved))

  const saveWorldDefaults = useCallback(async () => {
    if (!worldPack || !worldDraft) return
    const epoch = requestEpoch.current
    setWorldSaving(true)
    setWorldConflict(false)
    setWorldNotice('')
    try {
      const result = await saveStoryPackSettings(worldPack.id, storyPackSettingsFrom(worldDraft), worldRevision)
      if (epoch !== requestEpoch.current) return
      const resolved = worldSettingsValue(result.settings)
      setWorldDraft(resolved)
      setWorldSaved(resolved)
      setWorldRevision(result.revision)
      setPacks((current) => current.map((pack) => pack.id === worldPack.id
        ? { ...pack, settings: result.settings, settings_revision: result.revision, settings_customized: true }
        : pack))
      setWorldNotice(t('gameSettings.worldDefault.saved'))
      toast.success(t('gameSettings.worldDefault.saved'))
    } catch (error) {
      if (epoch !== requestEpoch.current) return
      if (error instanceof APIError && error.status === 409) {
        // Someone else saved first. The draft stays as it is so the player can
        // compare and decide, rather than losing what they just typed.
        setWorldConflict(true)
        setWorldNotice(t('gameSettings.worldDefault.conflict'))
        return
      }
      console.error('[game-settings] Failed to save the world defaults', error)
      setWorldNotice(errorMessage(error, t('gameSettings.worldDefault.saveFailed')))
    } finally {
      if (epoch === requestEpoch.current) setWorldSaving(false)
    }
  }, [t, worldDraft, worldPack, worldRevision])

  const discardWorldDefaults = useCallback(() => {
    setWorldDraft(worldSaved)
    setWorldConflict(false)
    setWorldNotice('')
  }, [worldSaved])

  // Re-reading is the only way out of a conflict: the player needs the block
  // that actually won before deciding what to keep.
  const reloadWorldDefaults = useCallback(async () => {
    if (!worldPack) return
    const epoch = requestEpoch.current
    setWorldSaving(true)
    setWorldConflict(false)
    setWorldNotice('')
    try {
      const next = await listStoryPacks()
      if (epoch !== requestEpoch.current) return
      setPacks(next)
      const reloaded = next.find((pack) => pack.id === worldPack.id)
      if (!reloaded) return
      const resolved = worldSettingsValue(reloaded.settings)
      setWorldDraft(resolved)
      setWorldSaved(resolved)
      setWorldRevision(reloaded.settings_revision || '')
    } catch (error) {
      if (epoch !== requestEpoch.current) return
      console.error('[game-settings] Failed to re-read the world defaults', error)
      setWorldNotice(errorMessage(error, t('gameSettings.worldDefault.saveFailed')))
    } finally {
      if (epoch === requestEpoch.current) setWorldSaving(false)
    }
  }, [t, worldPack])

  const cancelSectionScroll = useCallback(() => {
    scrollTargetRef.current = null
    scrollAnimationRef.current?.stop()
    scrollAnimationRef.current = null
  }, [])

  const jumpToSection = useCallback((id: GameSettingsSectionId) => {
    const container = contentRef.current
    const section = sectionRefs.current[id]
    setActiveSection(id)
    if (!container || !section) return
    scrollTargetRef.current = id
    scrollAnimationRef.current?.stop()
    scrollAnimationRef.current = scrollSettingsSectionIntoView(container, section, {
      reducedMotion: Boolean(reducedMotion),
      onComplete: () => {
        scrollAnimationRef.current = null
        scrollTargetRef.current = null
      },
    })
  }, [reducedMotion])

  useEffect(() => () => {
    scrollAnimationRef.current?.stop()
  }, [])

  // A resource link names a catalog, and the router already brought this page
  // forward. Without this the player lands at the top of the page and has to
  // find the control themselves; the nonce is what makes a second link to the
  // same resource scroll again.
  //
  // The route presentation is deferred, so this page can still be hidden on the
  // render the link arrives in. Scrolling a hidden container measures zero
  // heights, so the intent waits for `visible` and is only then consumed.
  const consumedResourceNonceRef = useRef(0)
  useEffect(() => {
    const intent = toolNavigationIntent
    if (!visible || loading) return
    if (!intent || intent.nonce === consumedResourceNonceRef.current || intent.target.kind !== 'config_resource') return
    const section = RESOURCE_SECTIONS[intent.target.resource]
    if (!section) return
    // Narration, rules and media are rendered only while a story is open. With no
    // story there is no control to scroll to, and the assistant is the one place
    // that can still read and change the catalog.
    const present = currentStory || section === 'world' || section === 'creator'
    consumedResourceNonceRef.current = intent.nonce
    jumpToSection(present ? section : 'world')
  }, [currentStory, jumpToSection, loading, toolNavigationIntent?.nonce, visible])

  const syncActiveSection = useCallback(() => {
    const container = contentRef.current
    if (!container || scrollTargetRef.current) return
    const threshold = container.getBoundingClientRect().top + 72
    let current: GameSettingsSectionId = SECTION_IDS[0]
    for (const id of SECTION_IDS) {
      const node = sectionRefs.current[id]
      if (!node) continue
      if (node.getBoundingClientRect().top <= threshold) current = id
    }
    setActiveSection((previous) => (previous === current ? previous : current))
  }, [])

  const sectionTitle = (id: GameSettingsSectionId) => t(`gameSettings.section.${id}`)

  const navPanel = (
    <SectionedNavigation
      groups={[
        {
          id: 'current',
          title: t('gameSettings.group.current'),
          items: currentStory
            ? [{ id: 'world' as const, title: sectionTitle('world') }, { id: 'story' as const, title: sectionTitle('story') }]
            : [{ id: 'world' as const, title: sectionTitle('world') }],
        },
        ...(currentStory ? [{
          id: 'play',
          title: t('gameSettings.group.play'),
          items: [
            { id: 'narrative' as const, title: sectionTitle('narrative') },
            { id: 'rules' as const, title: sectionTitle('rules') },
            { id: 'media' as const, title: sectionTitle('media') },
          ],
        }] : []),
        { id: 'creator', title: t('gameSettings.group.play'), items: [{ id: 'creator' as const, title: t('settingPanel.workspaceNarrative.title') }] },
      ]}
      activeId={activeSection}
      onSelect={jumpToSection}
    />
  )

  const storySelector = stories.length > 1 ? (
    <Select value={currentStoryId} onValueChange={handleStorySelect}>
      <SelectTrigger size="sm" aria-label={t('gameSettings.story.select')} className="col-start-2 w-full min-w-0 max-w-full bg-background text-xs @[36rem]/settings-header:col-start-auto @[36rem]/settings-header:w-64">
        <SelectValue />
      </SelectTrigger>
      <SelectContent position="popper">
        <SelectGroup>
          {stories.map((story) => <SelectItem key={story.id} value={story.id}>{story.title}</SelectItem>)}
        </SelectGroup>
      </SelectContent>
    </Select>
  ) : null

  // Picking a world only retargets this editor: no story is created, no branch
  // moves and the story being played stays the one the stage has open.
  const worldSelector = packs.length > 1 ? (
    <Select value={worldPackId} onValueChange={setWorldPackId} disabled={worldSaving}>
      <SelectTrigger size="sm" aria-label={t('gameSettings.worldDefault.select')} className="col-start-2 w-full min-w-0 max-w-full bg-background text-xs @[36rem]/settings-header:col-start-auto @[36rem]/settings-header:w-64">
        <SelectValue />
      </SelectTrigger>
      <SelectContent position="popper">
        <SelectGroup>
          {packs.map((pack) => <SelectItem key={pack.id} value={pack.id}>{pack.name || pack.title || pack.id}</SelectItem>)}
        </SelectGroup>
      </SelectContent>
    </Select>
  ) : null

  const worldDefaultsSection = (
    <GameSettingsSection id="world" title={sectionTitle('world')} refs={sectionRefs} group={t('gameSettings.group.current')}>
      <section className="@container/settings-header overflow-hidden rounded-xl border border-border bg-card">
        <div className="grid grid-cols-[auto_minmax(0,1fr)] items-start gap-2 px-3 py-2.5 @[36rem]/settings-header:grid-cols-[auto_minmax(0,1fr)_auto] @[36rem]/settings-header:items-center">
          <Globe className="mt-0.5 size-4 shrink-0 text-[var(--director-brass)] @[36rem]/settings-header:mt-0" aria-hidden="true" />
          <div className="min-w-0">
            <p className="truncate text-xs font-semibold text-foreground">
              {worldPack ? (worldPack.name || worldPack.title || worldPack.id) : t('gameSettings.worldDefault.noWorld')}
            </p>
            <p className="mt-0.5 text-[10px] leading-4 text-muted-foreground">{t('gameSettings.worldDefault.hint')}</p>
          </div>
          {worldSelector}
        </div>
        {worldPack && worldDraft ? (
          <>
            <div className="border-t border-border px-3 pt-1">
              <p className="py-2 text-xs font-medium break-words">{t('gameSettings.target.world', { name: worldPack.name || worldPack.title || worldPack.id })}</p>
              <WorldSettingsEditor
                pack={worldPack}
                value={worldDraft}
                onChange={setWorldDraft}
                tellers={styleTellers}
                planningTemplates={planningTemplates}
                modules={worldPack.modules || []}
                saving={worldSaving}
                disabled={worldConflict}
              />
            </div>
            <div className="flex flex-wrap items-center gap-2 border-t border-border px-3 py-2">
              <p className="w-full break-words text-xs text-muted-foreground">{t('gameSettings.target.world', { name: worldPack.name || worldPack.title || worldPack.id })}</p>
              <Button
                type="button"
                size="xs"
                disabled={!worldDirty || worldSaving || worldConflict}
                onClick={() => { void saveWorldDefaults() }}
              >
                {worldSaving ? t('gameSettings.worldDefault.saving') : t('gameSettings.worldDefault.save')}
              </Button>
              <Button
                type="button"
                size="xs"
                variant="outline"
                disabled={!worldDirty || worldSaving}
                onClick={discardWorldDefaults}
              >
                {t('gameSettings.worldDefault.discard')}
              </Button>
              {worldConflict ? (
                <Button type="button" size="xs" variant="outline" disabled={worldSaving} onClick={() => { void reloadWorldDefaults() }}>
                  {t('gameSettings.worldDefault.reload')}
                </Button>
              ) : null}
              {worldNotice && activeSection !== 'creator' ? (
                <span
                  role="status"
                  className={`min-w-0 text-[10px] leading-4 ${worldConflict ? 'text-[var(--nova-danger)]' : 'text-muted-foreground'}`}
                >
                  {worldNotice}
                </span>
              ) : null}
            </div>
          </>
        ) : null}
      </section>
    </GameSettingsSection>
  )

  if (loading) {
    return (
      <SettingsPageFrame visible={visible} title={t('gameSettings.title')} className="nova-game-settings-view">
        <LoadingState label={t('common.loading')} className="min-h-0 flex-1" />
      </SettingsPageFrame>
    )
  }

  if (!currentStory && packs.length === 0) {
    return (
      <SettingsPageFrame
        visible={visible}
        title={t('gameSettings.title')}
        className="nova-game-settings-view"
        error={loadFailed ? t('gameSettings.loadFailed') : undefined}
        errorTitle={t('gameSettings.title')}
      >
        <div className="flex min-h-0 flex-1 items-center justify-center px-6 py-10">
          <EmptyState
            icon={SlidersHorizontal}
            variant="panel"
            title={t('gameSettings.empty.title')}
            description={t('gameSettings.empty.description')}
            action={{ label: t('gameSettings.empty.action'), onClick: onOpenBooks }}
          />
        </div>
      </SettingsPageFrame>
    )
  }

  return (
    <SettingsPageFrame
      visible={visible}
      title={t('gameSettings.title')}
      className="nova-game-settings-view"
      error={loadFailed ? t('gameSettings.loadFailed') : undefined}
      errorTitle={t('gameSettings.title')}
      navigation={navPanel}
      navigationLayoutKey="nova-game-settings-navigation-layout"
    >
      <div
        ref={contentRef}
        data-nova-game-settings-content="true"
        onScroll={syncActiveSection}
        onWheelCapture={cancelSectionScroll}
        onPointerDownCapture={cancelSectionScroll}
        onKeyDownCapture={cancelSectionScroll}
        onTouchStartCapture={cancelSectionScroll}
        className="h-full min-h-0 overflow-y-auto overscroll-contain px-4 py-5 sm:px-6"
      >
        <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-4">
          {worldDefaultsSection}

          {currentStory ? (
          <GameSettingsSection id="story" title={sectionTitle('story')} refs={sectionRefs} group={t('gameSettings.group.current')}>
            <section className="@container/settings-header overflow-hidden rounded-xl border border-border bg-card">
              <div className="grid grid-cols-[auto_minmax(0,1fr)] items-start gap-2 px-3 py-2.5 @[36rem]/settings-header:grid-cols-[auto_minmax(0,1fr)_auto] @[36rem]/settings-header:items-center">
                <BookMarked className="mt-0.5 size-4 shrink-0 text-[var(--director-brass)] @[36rem]/settings-header:mt-0" aria-hidden="true" />
                <div className="min-w-0">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate text-xs font-semibold text-foreground">{currentStory.title}</span>
                    <Badge variant="secondary" className="h-4 shrink-0 px-1.5 text-[9px] font-medium">
                      {t('gameSettings.story.currentBadge')}
                    </Badge>
                  </div>
                  <p className="mt-0.5 truncate text-[10px] text-muted-foreground">
                    {t('gameSettings.story.branch', { branch: currentBranchId || t('common.loading') })}
                  </p>
                </div>
                {storySelector}
              </div>
              <p className="border-t border-border px-3 py-2.5 text-[10px] leading-4 text-muted-foreground">
                {t('gameSettings.story.scopeHint')}
              </p>
            </section>
          </GameSettingsSection>
          ) : (
          <GameSettingsSection id="story" title={sectionTitle('story')} refs={sectionRefs} group={t('gameSettings.group.current')}>
            <section className="overflow-hidden rounded-xl border border-border bg-card">
              <div className="px-3 py-3">
                <p className="text-xs font-semibold text-foreground">{t('gameSettings.empty.title')}</p>
                <p className="mt-1 text-[10px] leading-4 text-muted-foreground">{t('gameSettings.story.noStoryHint')}</p>
                <Button type="button" size="xs" variant="outline" className="mt-2" onClick={onOpenBooks}>
                  {t('gameSettings.empty.action')}
                </Button>
              </div>
            </section>
          </GameSettingsSection>
          )}

          {currentStory ? (
          <>
          <GameSettingsSection id="narrative" title={sectionTitle('narrative')} refs={sectionRefs} group={t('gameSettings.group.play')}>
            <p className="rounded-md bg-muted px-3 py-2 text-xs break-words">{t('gameSettings.target.story', { name: currentStory.title })}</p>
            <ControlSection icon={<Bot className="size-4" />} title={t('gameSettings.model.title')}>
              <TuningRow title={t('gameSettings.model.row')} description={t('gameSettings.model.hint')}>
                <ModelProfileSwitcher
                  agentKey="interactive_story"
                  conversationConfig={conversationConfig}
                  disabled={!conversationConfig.initialized || conversationConfig.saving}
                />
              </TuningRow>
              {/* §4.1 keeps this binding separate from the story's own model, and
                  T27 keeps the choice out of the game windows, so it lives here
                  beside that model. Only a world that actually has the activity
                  gets the row. */}
              {dualCultivationAvailable ? <IntimacyModelSetting story={currentStory} onUpdate={handleStoryUpdate} /> : null}
            </ControlSection>
            <StoryTuningView
              projectId={projectId}
              story={currentStory}
              planningTemplates={planningTemplates}
              recommendedPlanningTemplateId={storyRecommendedPlanningTemplateId}
              tellers={styleTellers}
              imagePresets={imagePresets}
              stateDisplayPreference={stateDisplayPreference}
              onStateDisplayPreferenceChange={handleStateDisplayPreferenceChange}
              onPlanningTemplateChange={handlePlanningTemplateChange}
              onUpdate={handleStoryUpdate}
              onOpenPresets={onOpenAssistant}
              group="narrative"
              className="flex flex-col gap-2"
            />
          </GameSettingsSection>

          <GameSettingsSection id="rules" title={sectionTitle('rules')} refs={sectionRefs} group={t('gameSettings.group.play')}>
            <p className="rounded-md bg-muted px-3 py-2 text-xs break-words">{t('gameSettings.target.story', { name: currentStory.title })}</p>
            <StoryTuningView
              projectId={projectId}
              story={currentStory}
              planningTemplates={planningTemplates}
              recommendedRuleSystemId={storyRecommendedRuleSystemId}
              tellers={styleTellers}
              imagePresets={imagePresets}
              stateDisplayPreference={stateDisplayPreference}
              onStateDisplayPreferenceChange={handleStateDisplayPreferenceChange}
              onPlanningTemplateChange={handlePlanningTemplateChange}
              onUpdate={handleStoryUpdate}
              group="rules"
              className="flex flex-col gap-2"
            />
          </GameSettingsSection>

          <GameSettingsSection id="media" title={sectionTitle('media')} refs={sectionRefs} group={t('gameSettings.group.play')}>
            <p className="rounded-md bg-muted px-3 py-2 text-xs break-words">{t('gameSettings.target.story', { name: currentStory.title })}</p>
            <StoryTuningView
              projectId={projectId}
              story={currentStory}
              planningTemplates={planningTemplates}
              tellers={styleTellers}
              imagePresets={imagePresets}
              stateDisplayPreference={stateDisplayPreference}
              onStateDisplayPreferenceChange={handleStateDisplayPreferenceChange}
              onPlanningTemplateChange={handlePlanningTemplateChange}
              onUpdate={handleStoryUpdate}
              group="media"
              className="flex flex-col gap-2"
            />
          </GameSettingsSection>
          </>
          ) : null}

          <GameSettingsSection id="creator" title={t('settingPanel.workspaceNarrative.title')} refs={sectionRefs} group={t('gameSettings.group.play')}>
            {visible ? <SharedNarrativeRulesEditor /> : null}
            <div className="mt-3 flex min-w-0 flex-col gap-2 rounded-xl border border-border p-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <label htmlFor="world-writing-preferences" className="text-sm font-medium">{t('settingPanel.sharedNarrative.worldPreferences')}</label>
                {worldPack ? <span className="text-xs">{t('settingPanel.sharedNarrative.worldTarget', { name: worldPack.name })}</span> : null}
                {packs.length > 1 ? <Button size="sm" type="button" variant="ghost" disabled={worldSaving}
                  onClick={() => jumpToSection('world')}>{t('settingPanel.sharedNarrative.chooseWorld')}</Button> : null}
              </div>
              <p className="text-xs text-muted-foreground">{t('settingPanel.sharedNarrative.worldHint')}</p>
              <Textarea id="world-writing-preferences" className="min-h-36 resize-y"
                value={worldDraft?.writingPreferences ?? ''} disabled={!worldDraft || worldSaving || worldConflict}
                onChange={event => setWorldDraft(current => current ? { ...current, writingPreferences: event.target.value } : current)} />
              <div className="flex flex-wrap items-center gap-3">
                <Button size="sm" type="button" disabled={!worldDirty || worldSaving || worldConflict}
                  onClick={() => void saveWorldDefaults()}>{t('settingPanel.sharedNarrative.saveWorld')}</Button>
                {worldNotice && activeSection === 'creator' ? <span role="status" className="text-xs text-muted-foreground">{worldNotice}</span> : null}
              </div>
            </div>
          </GameSettingsSection>
        </div>
      </div>
    </SettingsPageFrame>
  )
}

function GameSettingsSection({
  id,
  title,
  group,
  refs,
  children,
}: {
  id: GameSettingsSectionId
  title: string
  group: string
  refs: React.MutableRefObject<Partial<Record<GameSettingsSectionId, HTMLElement | null>>>
  children: ReactNode
}) {
  return (
    <section
      id={`game-settings-${id}`}
      data-nova-game-settings-section={id}
      ref={(node) => { refs.current[id] = node }}
      className="scroll-mt-4"
      aria-label={title}
    >
      <h2 className="mb-2 px-1 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{group}</h2>
      <div className="flex flex-col gap-2">{children}</div>
    </section>
  )
}
