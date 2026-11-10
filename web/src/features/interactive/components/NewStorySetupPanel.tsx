import { Play, Settings2, SlidersHorizontal } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import type { LoreItem } from '@/lib/api'
import { applyStoryPack } from '@/lib/api-client'
import { hasLoreProtagonistTag } from '@/features/lore/tags'
import type { ConversationConfigController, ConversationConfigSnapshot } from '@/features/conversation-config/types'
import { normalizeThinkingLevel, type ThinkingLevel } from '@/features/settings/thinking-levels'
import { requestSettingsSection } from '@/features/onboarding/events'
import { normalizeStoryCheckSettings } from '../check-settings'
import { useStoryModelReadiness } from '../model-readiness'
import { DEFAULT_IMAGE_PRESET_ID, normalizeStoryImageSettings } from '../image-settings'
import { DEFAULT_NARRATIVE_STYLE_ID, resolveNarrativeStyle } from '../narrative-style'
import { DEFAULT_INTERACTIVE_CHOICE_COUNT, DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS, truncateStoryOpeningText, type StoryCreateInput } from '../opening'
import type { GamePlanningTemplate, StoryDirectorModuleRefs, StoryCheckSettings, StoryImageSettings, StoryOpeningConfig, StoryPackBinding, StoryProtagonist, StoryStateSchemaMode, StoryStylePurposes, StorySummary, Teller } from '../types'
import { StoryOpeningSelector } from './story-setup/StoryOpeningSelector'
import { StoryPackSelector } from './story-setup/StoryPackSelector'
import { StoryProtagonistSelector } from './story-setup/StoryProtagonistSelector'

/**
 * The values a start sends for one story. Most of them are the story's own
 * settings and are edited on the game settings page; this panel only carries
 * them through so a start uses what the player already chose.
 */
interface StorySetupSettings {
  customAgentId: string
  modelProfileId: string
  thinkingLevel: ThinkingLevel
  planningEnabled: boolean
  moduleRefs: StoryDirectorModuleRefs
  replyTargetChars: number
  choiceCount: number
  imageSettings: StoryImageSettings
  checkSettings: StoryCheckSettings
  stateSchemaMode: StoryStateSchemaMode
  stylePurposes: StoryStylePurposes
}

interface NewStorySetupPanelProps {
  projectId: string
  tellers: Teller[]
  planningTemplates: GamePlanningTemplate[]
  loreItems?: LoreItem[]
  recentNarrativeStyleID?: string
  narrativeStyleLoading?: boolean
  conversationConfig: ConversationConfigController
  story?: StorySummary
  onRequestLoreInit?: () => void
  onOpenPresets?: () => void
  onCreate: (input: StoryCreateInput) => void | Promise<void>
}

export function NewStorySetupPanel({
  projectId,
  tellers,
  planningTemplates,
  loreItems = [],
  recentNarrativeStyleID = DEFAULT_NARRATIVE_STYLE_ID,
  narrativeStyleLoading = false,
  conversationConfig,
  story,
  onRequestLoreInit,
  onOpenPresets,
  onCreate,
}: NewStorySetupPanelProps) {
  const { t } = useTranslation()
  const initialTemplate = planningTemplates.find((item) => item.id === story?.planning_template_id) || planningTemplates[0]
  const recentTeller = resolveNarrativeStyle(tellers, recentNarrativeStyleID)
  const initialProtagonist = story?.protagonist || defaultStoryProtagonist(loreItems)
  const [planningTemplateId, setPlanningTemplateId] = useState(initialTemplate?.id || 'default')
  const [protagonist, setProtagonist] = useState<StoryProtagonist>(initialProtagonist)
  const [opening, setOpening] = useState<StoryOpeningConfig>(() => story?.opening || { mode: 'ai' })
  const [packOrigin, setPackOrigin] = useState('')
  const [packBinding, setPackBinding] = useState<StoryPackBinding | undefined>(story?.pack_binding)
  // The modules the bound pack names. Kept beside the binding so a start sends
  // the pack's own rule/state/event modules even if the panel's defaults are
  // older than the pack revision.
  const [packModules, setPackModules] = useState<StoryDirectorModuleRefs | undefined>(story?.module_refs)
  const [settings, setSettings] = useState<StorySetupSettings>(() => initialSettings(story, recentTeller?.id, conversationConfig.snapshot))
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const initialProtagonistRef = useRef<StoryProtagonist>(initialProtagonist)
  const protagonistSelectionTouchedRef = useRef(false)
  const narrativeStyleSelectionLockedRef = useRef(Boolean(story))
  const advancedSummary = t('storyPicker.setup.advanced.summary', {
    planning: settings.planningEnabled ? t('storyPicker.setup.advanced.planningOn') : t('storyPicker.setup.advanced.planningOff'),
    checks: settings.moduleRefs.rule_system_disabled ? t('storyPicker.setup.advanced.checksOff') : t('storyPicker.setup.advanced.checksOn'),
    images: settings.imageSettings.mode === 'interval' ? t('storyPicker.setup.advanced.imagesOn') : t('storyPicker.setup.advanced.imagesOff'),
  })
  const runtimeConfigLoading = conversationConfig.loading || (!conversationConfig.error && !settings.modelProfileId)
  const runtimeConfigReady = conversationConfig.initialized && Boolean(settings.modelProfileId)
  const modelReadiness = useStoryModelReadiness()
  const startButtonLoading = creating || narrativeStyleLoading || runtimeConfigLoading
  let startButtonLabel = t('storyPicker.setup.start')
  if (narrativeStyleLoading || runtimeConfigLoading) startButtonLabel = t('common.loading')
  if (creating) startButtonLabel = t('storyPicker.setup.starting')

  useEffect(() => {
    if (story || narrativeStyleSelectionLockedRef.current || !recentTeller) return
    setSettings((current) => ({
      ...current,
      moduleRefs: { ...current.moduleRefs, narrative_style_id: recentTeller.id },
      // The panel's single style control is the main narrative, so keep the
      // stored purpose in step with it. Dialogue and combat are left alone.
      stylePurposes: { ...current.stylePurposes, main: recentTeller.id },
    }))
  }, [recentTeller, story])

  useEffect(() => {
    const snapshot = conversationConfig.snapshot
    if (!snapshot) return
    setSettings((current) => ({
      ...current,
      modelProfileId: snapshot.profile_id || 'default',
      thinkingLevel: normalizeThinkingLevel(snapshot.thinking_level) || 'default',
    }))
  }, [conversationConfig.snapshot])

  useEffect(() => {
    if (protagonistSelectionTouchedRef.current || protagonist.mode !== 'default') return
    const tagged = defaultStoryProtagonist(loreItems)
    if (tagged.mode === 'lore') setProtagonist(tagged)
  }, [loreItems, protagonist.mode])

  const changeProtagonist = (next: StoryProtagonist) => {
    protagonistSelectionTouchedRef.current = true
    setProtagonist(next)
  }

  /**
   * Fills the panel from a script pack. The pack contributes initial values
   * only; every field stays editable and the story is still started through the
   * ordinary submit below, so a pack adds no second creation path.
   *
   * The pack's settings are defaults, not locks: applying a world copies them
   * into the panel, where the player may still change them for this game. That
   * is why planning is read from the pack rather than forced off, and why a
   * world that names its own styles and creative direction actually starts with
   * them instead of showing a selection that never reaches the story.
   */
  const applyPack = async (packId: string) => {
    const draft = await applyStoryPack(projectId, packId)
    const request = draft.request
    protagonistSelectionTouchedRef.current = true
    setProtagonist(request.protagonist)
    setOpening(request.opening)
    setPackOrigin(request.origin || '')
    // Keep the pack's binding: it is what scopes this story's world book and
    // freezes the pack revision, so it must reach creation rather than being
    // recomputed from whatever the pack file says later.
    setPackBinding(request.pack_binding)
    setPackModules(request.module_refs)
    if (request.planning_template_id) setPlanningTemplateId(request.planning_template_id)
    setSettings((current) => ({
      ...current,
      moduleRefs: {
        ...current.moduleRefs,
        ...request.module_refs,
        ...(request.story_teller_id ? { narrative_style_id: request.story_teller_id } : {}),
      },
      replyTargetChars: request.reply_target_chars || current.replyTargetChars,
      choiceCount: request.choice_count || current.choiceCount,
      planningEnabled: request.planning_mode ? request.planning_mode === 'enabled' : current.planningEnabled,
      stylePurposes: request.style_purposes
        ? { ...request.style_purposes, main: request.style_purposes.main || request.story_teller_id || current.stylePurposes.main }
        : { ...current.stylePurposes, main: request.story_teller_id || current.stylePurposes.main },
      checkSettings: request.check_settings
        ? normalizeStoryCheckSettings(request.check_settings)
        : current.checkSettings,
    }))
    return draft
  }

  const submit = async () => {
    if (creating) return
    setError('')
    if (!conversationConfig.initialized || !settings.modelProfileId) {
      setError(t('storyPicker.setup.model.loadFailed'))
      return
    }
    const validationError = validateDraft(protagonist, opening, loreItems, t)
    if (validationError) {
      setError(validationError)
      return
    }
    setCreating(true)
    try {
      const tellerID = resolveNarrativeStyle(tellers, settings.moduleRefs.narrative_style_id || recentNarrativeStyleID)?.id || DEFAULT_NARRATIVE_STYLE_ID
      // A story started from a world pack runs on that pack's modules. The
      // panel shows them read-only, but the request is where it matters: the
      // pack revision and the modules it names are one binding, so a start
      // must not quietly keep an older default here.
      const moduleRefs = packBinding
        ? { ...settings.moduleRefs, ...(packModules || {}) }
        : {
            ...settings.moduleRefs,
            actor_state_id: settings.moduleRefs.actor_state_id || 'default',
            actor_state_disabled: settings.stateSchemaMode === 'generate',
          }
      const protagonistInput = protagonistForSubmit(protagonist)
      const includeProtagonist = !story || !sameProtagonist(protagonist, initialProtagonistRef.current)
      // The main narrative style is the one control this panel shows, so it wins
      // over a pack's main entry. Dialogue and combat stay as the world set them
      // and inherit the main narrative when empty.
      const stylePurposes = {
        main: tellerID,
        dialogue: settings.stylePurposes.dialogue || '',
        combat: settings.stylePurposes.combat || '',
      }
      await onCreate({
        title: story?.title_source === 'pending' ? '' : story?.title || '',
        ...(!story && settings.customAgentId ? { custom_agent_id: settings.customAgentId } : {}),
        profile_id: settings.modelProfileId,
        thinking_level: settings.thinkingLevel,
        origin: story?.origin || packOrigin,
        ...(includeProtagonist ? { protagonist: protagonistInput } : {}),
        story_teller_id: tellerID,
        style_purposes: stylePurposes,
        planning_template_id: planningTemplateId,
        planning_mode: settings.planningEnabled ? 'enabled' : 'disabled',
        module_refs: moduleRefs,
        reply_target_chars: settings.replyTargetChars,
        choice_count: settings.choiceCount,
        opening: openingForSubmit(opening),
        image_settings: { ...settings.imageSettings, preset_id: settings.imageSettings.preset_id || moduleRefs.image_preset_id || DEFAULT_IMAGE_PRESET_ID },
        check_settings: settings.checkSettings,
        state_schema_policy: { mode: packBinding ? 'fixed_template' : settings.stateSchemaMode },
        ...(!story && packBinding ? { pack_binding: packBinding } : {}),
      })
    } catch (reason) {
      console.error('[story-setup] Failed to save and start story', reason)
      setError(reason instanceof Error ? reason.message : t('storyPicker.createFailed'))
      setCreating(false)
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-6 pt-5 sm:px-7 lg:px-10">
        <section className="mx-auto w-full max-w-5xl" aria-labelledby="new-story-title">
          <header className="mb-4">
            <h2 id="new-story-title" className="text-xl font-semibold tracking-[-0.02em] text-foreground sm:text-2xl">{story ? t('storyPicker.setup.resumeTitle') : t('storyPicker.setup.title')}</h2>
            <p className="mt-1 text-sm text-muted-foreground">{t('storyPicker.setup.description')}</p>
          </header>

          <div className="flex flex-col gap-4">
            {modelReadiness === false ? (
              <section role="status" aria-labelledby="story-model-missing-title" className="rounded-xl border border-border bg-card px-4 py-3 sm:px-5">
                <h3 id="story-model-missing-title" className="text-sm font-semibold text-foreground">{t('storyPicker.setup.model.missingTitle')}</h3>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('storyPicker.setup.model.notConfigured')}</p>
                <Button type="button" variant="outline" size="sm" className="mt-2" onClick={() => requestSettingsSection('model')}>
                  <Settings2 data-icon="inline-start" />
                  {t('storyPicker.setup.model.configure')}
                </Button>
              </section>
            ) : null}
            {!story && !packBinding ? <StoryPackSelector projectId={projectId} onApply={applyPack} /> : null}
            {!story && packBinding ? (
              <section aria-labelledby="story-pack-bound-title" className="rounded-xl border border-border bg-card px-4 py-3 sm:px-5">
                <h3 id="story-pack-bound-title" className="text-sm font-semibold text-foreground">
                  {t('storyPicker.pack.boundTitle', { name: packBinding.pack_name || packBinding.pack_id })}
                </h3>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('storyPicker.pack.boundHint')}</p>
              </section>
            ) : null}
            <StoryProtagonistSelector projectId={projectId} value={protagonist} loreItems={loreItems} onChange={changeProtagonist} onRequestLoreInit={onRequestLoreInit} />
            <StoryOpeningSelector value={opening} onChange={setOpening} />

            {/* The story already exists by the time this panel shows, so its model,
                rules, checks and images are ordinary story settings. They live on
                the game settings page and are edited there; this panel only says
                what they currently are and offers the way to change them. */}
            <section aria-labelledby="story-setup-settings-title" className="flex flex-wrap items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 sm:px-5">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-border bg-background text-primary"><SlidersHorizontal className="size-4" /></span>
              <span className="min-w-0 flex-1">
                <span id="story-setup-settings-title" className="block text-sm font-medium text-foreground">{t('storyPicker.setup.settings.title')}</span>
                <span className="mt-0.5 block truncate text-xs text-muted-foreground">{advancedSummary}</span>
              </span>
              <Button type="button" variant="outline" size="sm" onClick={onOpenPresets} disabled={!onOpenPresets}>
                <Settings2 data-icon="inline-start" />
                {t('storyPicker.setup.settings.open')}
              </Button>
            </section>
          </div>

          {error ? <div role="alert" className="mt-4 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">{error}</div> : null}
        </section>
      </div>
      <footer data-testid="story-setup-footer" className="shrink-0 border-t border-border bg-[var(--nova-surface-2)] px-4 py-3 sm:px-7 lg:px-10">
        <div className="mx-auto flex w-full max-w-5xl items-center justify-end gap-2">
          <Button
            type="button"
            disabled={creating || narrativeStyleLoading || runtimeConfigLoading || conversationConfig.saving || !runtimeConfigReady}
            onClick={() => void submit()}
          >
            {startButtonLoading ? <Spinner /> : <Play data-icon="inline-start" />}
            {startButtonLabel}
          </Button>
        </div>
      </footer>
    </div>
  )
}

function initialSettings(
  story: StorySummary | undefined,
  recentTellerID: string | undefined,
  runtimeConfig: ConversationConfigSnapshot | null,
): StorySetupSettings {
  const moduleRefs = {
    narrative_style_id: DEFAULT_NARRATIVE_STYLE_ID,
    event_package_ids: ['default'],
    rule_system_id: 'default',
    actor_state_id: 'default',
    image_preset_id: DEFAULT_IMAGE_PRESET_ID,
    ...(story?.module_refs || {}),
  }
  if (!story && recentTellerID) moduleRefs.narrative_style_id = recentTellerID
  const imageSettings = normalizeStoryImageSettings(story?.image_settings || { mode: 'manual', interval_turns: 3, preset_id: moduleRefs.image_preset_id || DEFAULT_IMAGE_PRESET_ID })
  return {
    customAgentId: '',
    modelProfileId: runtimeConfig ? runtimeConfig.profile_id || 'default' : '',
    thinkingLevel: normalizeThinkingLevel(runtimeConfig?.thinking_level) || 'default',
    planningEnabled: story ? story.planning_mode === 'enabled' : true,
    moduleRefs,
    replyTargetChars: story?.reply_target_chars || DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS,
    choiceCount: story?.choice_count || DEFAULT_INTERACTIVE_CHOICE_COUNT,
    imageSettings,
    checkSettings: normalizeStoryCheckSettings(story?.check_settings),
    stateSchemaMode: story?.state_schema_policy?.mode || 'adapt_template',
    stylePurposes: {
      main: story?.style_purposes?.main || moduleRefs.narrative_style_id || DEFAULT_NARRATIVE_STYLE_ID,
      dialogue: story?.style_purposes?.dialogue || '',
      combat: story?.style_purposes?.combat || '',
    },
  }
}

function validateDraft(protagonist: StoryProtagonist, opening: StoryOpeningConfig, loreItems: LoreItem[], t: ReturnType<typeof useTranslation>['t']): string {
  if (protagonist.mode === 'custom' && !protagonist.name?.trim()) return t('storyPicker.setup.protagonist.custom.nameRequired')
  if (protagonist.mode === 'lore' && !protagonist.source_lore_item_id?.trim()) return t('storyPicker.setup.protagonist.lore.required')
  if (protagonist.mode === 'default' && !loreItems.some((item) => item.enabled && item.type === 'character')) return t('storyPicker.setup.protagonist.lore.emptyRequired')
  if (opening.mode === 'custom' && !opening.custom_text?.trim()) return t('storyPicker.setup.opening.customRequired')
  return ''
}

function defaultStoryProtagonist(loreItems: LoreItem[]): StoryProtagonist {
  const tagged = loreItems.find((item) => item.enabled && item.type === 'character' && hasLoreProtagonistTag(item.tags))
  if (!tagged) return { mode: 'default' }
  return {
    mode: 'lore',
    name: tagged.name,
    profile: tagged.content || tagged.brief_description,
    source_lore_item_id: tagged.id,
    source_lore_updated_at: tagged.updated_at,
  }
}

function protagonistForSubmit(protagonist: StoryProtagonist): StoryProtagonist {
  if (protagonist.mode === 'custom') return { mode: 'custom', name: protagonist.name?.trim(), profile: protagonist.profile?.trim() }
  if (protagonist.mode === 'lore') return { mode: 'lore', source_lore_item_id: protagonist.source_lore_item_id?.trim() }
  return { mode: 'default' }
}

function openingForSubmit(opening: StoryOpeningConfig): StoryOpeningConfig {
  if (opening.mode === 'preset') {
    // A story from before the preset library was removed still opens from its
    // stored snapshot. Handing it back unchanged keeps that; the player's own
    // choice of AI or custom replaces it.
    return { mode: 'preset', preset_id: opening.preset_id?.trim(), preset_text: truncateStoryOpeningText(opening.preset_text || '') }
  }
  if (opening.mode === 'custom') {
    const customText = truncateStoryOpeningText(opening.custom_text || '')
    return customText ? { mode: 'custom', custom_text: customText } : { mode: 'ai' }
  }
  return { mode: 'ai' }
}

function sameProtagonist(left: StoryProtagonist, right: StoryProtagonist): boolean {
  return JSON.stringify(protagonistForSubmit(left)) === JSON.stringify(protagonistForSubmit(right))
}
