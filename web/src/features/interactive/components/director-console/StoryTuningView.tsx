import { errorMessage } from '@/lib/error-diagnostics'
import { builtinModuleDisplayName } from '../../builtin-display-labels'
import { StorySpeechControls } from './StorySpeechControls'
import { useImageModelConfigured } from '@/features/settings/use-image-model-configured'
import { requestSettingsSection } from '@/features/onboarding/events'
import { Compass, Dices, ImagePlus, Palette, UserRound } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/lib/toast'
import { Switch } from '@/components/ui/switch'
import { getActorStates, getRuleSystems } from '../../api'
import { normalizeStoryCheckSettings } from '../../check-settings'
import { gamePlanningTemplateDescription, gamePlanningTemplateName, splitRecommendedPlanningTemplates } from '../../game-planning'
import { DEFAULT_IMAGE_PRESET_ID, normalizeImageIntervalTurns, normalizeStoryImageSettings } from '../../image-settings'
import { DEFAULT_NARRATIVE_STYLE_ID, isRecommendedNarrativeStyle, narrativeStyleName, recommendedNarrativeStyles } from '../../narrative-style'
import { MAX_INTERACTIVE_CHOICE_COUNT, MIN_INTERACTIVE_CHOICE_COUNT } from '../../opening'
import type {
  ActorStateModule,
  GamePlanningTemplate,
  ImagePreset,
  InteractiveStoryUpdateInput,
  RuleSystemModule,
  StoryCheckSettings,
  StoryDirectorModuleRefs,
  StorySummary,
  Teller,
} from '../../types'
import { StateDisplayPreferenceMenu } from '../story-state/StateDisplayPreferenceMenu'
import type { StoryStateDisplayPreference } from '../story-state/display-preference'
import { ReplyLengthSetting } from './ReplyLengthSetting'
import { ControlSection, NumberSettingInput, TuningLinkButton, TuningOptionDescription, TuningRecommendation, TuningRow, TuningSelect, type TuningSelectGroup, type TuningSelectOption } from './StoryTuningControls'
import { ModuleSelectRow } from './StoryTuningModuleRows'

/**
 * One style-purpose override row. It renders like the other module rows but is
 * driven by the story's own purpose config rather than by the module graph,
 * because the three purposes are an expression setting rather than a world
 * module a story resolves.
 */
function StylePurposeRow({
  label,
  purpose,
  value,
  options,
  busy,
  disabled,
  onChange,
}: {
  label: string
  purpose: string
  value: string
  options: TuningSelectOption[]
  busy: boolean
  disabled: boolean
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  return (
    <TuningRow
      title={label}
      description={purpose === 'dialogue' ? t('directorPanel.tuning.agent.stylePurpose.hint') : undefined}
      source={value === STYLE_PURPOSE_INHERIT ? undefined : 'story'}
      busy={busy}
      disabled={disabled}
    >
      <TuningSelect
        value={value}
        options={options}
        label={label}
        disabled={disabled}
        onChange={onChange}
      />
    </TuningRow>
  )
}

type ModuleIDKey = 'narrative_style_id' | 'rule_system_id' | 'actor_state_id' | 'image_preset_id'
type ModuleDisabledKey = 'narrative_style_disabled' | 'rule_system_disabled' | 'actor_state_disabled' | 'image_preset_disabled'

/**
 * Which part of the story's settings one instance renders.
 *
 * Game settings lays the same controls out in four player-facing groups, so the
 * sections have to be mountable separately. Leaving this unset renders every
 * section, which is what the tests and any single-surface host expect.
 */
export type StoryTuningGroup = 'narrative' | 'rules' | 'media'

export interface StoryTuningViewProps {
  projectId?: string
  story?: StorySummary
  planningTemplates: GamePlanningTemplate[]
  /**
   * The world's own planning suggestion, from the world pack rather than from
   * this story's saved choice. It is shown beside the picker so a player who
   * already chose something else can still see what the world proposes, and it
   * is never written into the story: a recommendation is the author's, and the
   * player's own choice is the one that is saved.
   */
  recommendedPlanningTemplateId?: string
  /**
   * The rule system the world itself binds, from the world pack rather than
   * from this story's saved choice. Shown as a suggestion and used to order the
   * list; it is never written into the story.
   */
  recommendedRuleSystemId?: string
  tellers: Teller[]
  imagePresets: ImagePreset[]
  stateDisplayPreference: StoryStateDisplayPreference
  onStateDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  onPlanningTemplateChange?: (templateId: string) => void | Promise<void>
  onUpdate?: (input: InteractiveStoryUpdateInput) => void | Promise<void>
  onOpenPresets?: () => void
  group?: StoryTuningGroup
  className?: string
}

const DEFAULT_MODULE_REFS: StoryDirectorModuleRefs = {
  narrative_style_id: DEFAULT_NARRATIVE_STYLE_ID,
  event_package_ids: ['default'],
  rule_system_id: 'default',
  actor_state_id: 'default',
  image_preset_id: DEFAULT_IMAGE_PRESET_ID,
}

/**
 * The picker's value for "this purpose follows the main narrative".
 *
 * A select item cannot carry an empty value, so the choice is shown under a
 * token and translated to the empty string the API expects when saved. The
 * distinction matters: "" is the real choice to inherit, while omitting the
 * field means "leave this purpose alone".
 */
const STYLE_PURPOSE_INHERIT = '__inherit__'

export function StoryTuningView(props: StoryTuningViewProps) {
  // Numeric drafts and pending saves belong to one editing target, including
  // when two targets happen to have the same saved numeric values.
  return <StoryTuningTarget key={`${props.projectId ?? ''}:${props.story?.id ?? ''}`} {...props} />
}

function StoryTuningTarget({
  projectId,
  story,
  planningTemplates,
  recommendedPlanningTemplateId,
  recommendedRuleSystemId,
  tellers,
  imagePresets,
  stateDisplayPreference,
  onStateDisplayPreferenceChange,
  onPlanningTemplateChange,
  onUpdate,
  onOpenPresets,
  group,
  className,
}: StoryTuningViewProps) {
  const { t } = useTranslation()
  const imageConfigured = useImageModelConfigured(projectId)
  const [savingKey, setSavingKey] = useState('')
  const [ruleSystems, setRuleSystems] = useState<RuleSystemModule[]>([])
  const [actorStates, setActorStates] = useState<ActorStateModule[]>([])

  useEffect(() => {
    let cancelled = false
    void Promise.all([getRuleSystems(), getActorStates()])
      .then(([rules, states]) => {
        if (cancelled) return
        setRuleSystems(rules)
        setActorStates(states)
      })
      .catch((error) => console.error('[story-console] Failed to load story-owned tuning resources', error))
    return () => {
      cancelled = true
    }
  }, [])

  const refs = { ...DEFAULT_MODULE_REFS, ...(story?.module_refs || {}) }
  // An unset purpose is stored as empty and shown as inheriting, so the picker
  // value is the token rather than the raw (possibly undefined) id.
  const stylePurposes = {
    dialogue: story?.style_purposes?.dialogue || STYLE_PURPOSE_INHERIT,
    combat: story?.style_purposes?.combat || STYLE_PURPOSE_INHERIT,
  }
  const imageSettings = normalizeStoryImageSettings(story?.image_settings)
  const checkSettings = normalizeStoryCheckSettings(story?.check_settings)
  const structureLocked = (story?.turn_count || 0) > 0
  const disabled = !story || !onUpdate || Boolean(savingKey)

  const save = async (key: string, input: InteractiveStoryUpdateInput) => {
    if (!story || !onUpdate || savingKey) return
    setSavingKey(key)
    try {
      await onUpdate(input)
    } catch (error) {
      console.error('[story-console] Failed to update story tuning', { storyId: story.id, key, error })
      toast.error(errorMessage(error, t('directorPanel.tuning.saveFailed')))
    } finally {
      setSavingKey('')
    }
  }

  const saveStylePurpose = async (key: string, purpose: 'dialogue' | 'combat', pickerValue: string) => {
    // The picker's inherit token becomes the empty string the API reads as
    // "follow the main narrative". Sending the token itself would name a style
    // that does not exist, and omitting the field would leave the purpose as it
    // is rather than clearing it.
    const value = pickerValue === STYLE_PURPOSE_INHERIT ? '' : pickerValue
    await save(key, { style_purposes: { [purpose]: value } })
  }

  const selectPlanningTemplate = async (templateId: string) => {
    if (!story || !onPlanningTemplateChange || savingKey || templateId === story.planning_template_id) return
    setSavingKey('planning-template')
    try {
      await onPlanningTemplateChange(templateId)
    } catch (error) {
      console.error('[story-console] Failed to update planning template', { storyId: story.id, templateId, error })
      toast.error(errorMessage(error, t('directorPanel.tuning.saveFailed')))
    } finally {
      setSavingKey('')
    }
  }

  const updateModule = (
    key: string,
    idKey: ModuleIDKey,
    disabledKey: ModuleDisabledKey,
    value: string,
    extra?: (nextID: string) => InteractiveStoryUpdateInput,
    rebuildState = false,
  ) => {
    const nextRefs = cloneRefs(refs)
    nextRefs[idKey] = value
    nextRefs[disabledKey] = false
    const input: InteractiveStoryUpdateInput = { module_refs: nextRefs, ...(extra?.(value) || {}) }
    if (rebuildState && !structureLocked && story?.state_schema_policy) input.state_schema_policy = story.state_schema_policy
    void save(key, input)
  }

  const setRuleChecksEnabled = (enabled: boolean) => {
    const nextRefs = cloneRefs(refs)
    nextRefs.rule_system_disabled = !enabled
    if (enabled && !nextRefs.rule_system_id) nextRefs.rule_system_id = ruleSystems[0]?.id || 'default'
    void save('checks-enabled', { module_refs: nextRefs })
  }

  // Every option carries its own one-line description: the list shows it under
  // the name, and the row below the picker repeats it for the current choice so
  // a phone — which has no hover — still reads what the chosen shape means.
  const planningOptions = useMemo<TuningSelectOption[]>(
    () => planningTemplates.map((item) => ({
      id: item.id,
      label: gamePlanningTemplateName(item, t),
      description: gamePlanningTemplateDescription(item, t) || undefined,
    })),
    [planningTemplates, t],
  )
  // The world's suggestion leads the list under its own heading. Grouping is
  // presentation only: the recommendation is not written into the story, so a
  // story that already chose something else keeps that choice and still sees
  // what the world proposed.
  const planningGroups = useMemo<TuningSelectGroup[]>(() => {
    const split = splitRecommendedPlanningTemplates(planningTemplates, recommendedPlanningTemplateId)
    if (!split.recommendedID) return []
    const toOption = (item: GamePlanningTemplate): TuningSelectOption => ({
      id: item.id,
      label: gamePlanningTemplateName(item, t),
      description: gamePlanningTemplateDescription(item, t) || undefined,
    })
    return [
      { label: t('directorPanel.tuning.planning.recommendedGroup'), options: split.recommended.map(toOption) },
      { label: t('directorPanel.tuning.planning.othersGroup'), options: split.others.map(toOption) },
    ]
  }, [planningTemplates, recommendedPlanningTemplateId, t])
  const recommendedPlanning = useMemo(() => {
    const split = splitRecommendedPlanningTemplates(planningTemplates, recommendedPlanningTemplateId)
    const item = split.recommended[0]
    if (!item) return undefined
    return { name: gamePlanningTemplateName(item, t), description: gamePlanningTemplateDescription(item, t) }
  }, [planningTemplates, recommendedPlanningTemplateId, t])
  const narrativeOptions = useMemo<TuningSelectOption[]>(
    () => recommendedNarrativeStyles(tellers, story?.story_teller_id)
      .map((item) => ({ id: item.id, label: narrativeStyleName(item, t) })),
    [story?.story_teller_id, t, tellers],
  )
  // The two override purposes offer "follow the main narrative" as a real
  // option, sent as the empty string. Omitting the field would mean "leave it
  // alone", which is a different choice, so the sentinel has to be explicit.
  // A purpose still pointing at a merged-away id keeps its own option for the
  // same reason the main select does: a value with no option looks unset.
  const purposeOptions = useMemo<TuningSelectOption[]>(() => {
    const selected = [stylePurposes.dialogue, stylePurposes.combat]
    const absorbed = tellers.filter((item) => selected.includes(item.id) && !isRecommendedNarrativeStyle(item.id))
    return [
      { id: STYLE_PURPOSE_INHERIT, label: t('directorPanel.tuning.agent.stylePurpose.inherit') },
      ...narrativeOptions,
      ...absorbed.map((item) => ({ id: item.id, label: narrativeStyleName(item, t) })),
    ]
  }, [narrativeOptions, stylePurposes.combat, stylePurposes.dialogue, t, tellers])
  const imageOptions = useMemo<TuningSelectOption[]>(() => imagePresets.map(moduleOption), [imagePresets])
  // A rule system may declare the state system its checks read from. Offering
  // one on a story running a different state system would let the player build
  // a pairing whose checks can never fire, so the list is limited to what the
  // current state system can actually run. A rule system that declares nothing
  // reads no state and runs anywhere.
  const ruleInScope = useMemo(() => {
    const stateID = refs.actor_state_disabled ? '' : String(refs.actor_state_id || '')
    return (module: RuleSystemModule): boolean => {
      const required = module.actor_state_id
      if (!required) return true
      if (!stateID) return false
      return required === stateID
    }
  }, [refs.actor_state_disabled, refs.actor_state_id])
  // The current value keeps its option even when it falls out of scope: a story
  // may already run a pairing a preset no longer supports, and a value with no
  // option renders as unset, which would look like the player had never chosen.
  const ruleScopedOptions = useMemo<TuningSelectOption[]>(() => {
    const currentID = String(refs.rule_system_id || '')
    const inScope = ruleSystems.filter(ruleInScope)
    const options = inScope.map(module => ({ id: module.id, label: builtinModuleDisplayName('rule_system', module, t) }))
    if (currentID && !options.some((option) => option.id === currentID)) {
      const selected = ruleSystems.find((module) => module.id === currentID)
      options.push(selected ? { id: selected.id, label: builtinModuleDisplayName('rule_system', selected, t) } : { id: currentID, label: currentID })
    }
    return options
  }, [refs.rule_system_id, ruleInScope, ruleSystems, t])
  // The world's own rule system leads the list under its own heading, the same
  // way the planning template does. Grouping is presentation only: the
  // suggestion is the world's, and the player's choice is the one saved.
  const ruleGroups = useMemo<TuningSelectGroup[]>(() => {
    if (!recommendedRuleSystemId) return []
    const recommended = ruleScopedOptions.filter((option) => option.id === recommendedRuleSystemId)
    const others = ruleScopedOptions.filter((option) => option.id !== recommendedRuleSystemId)
    if (!recommended.length) return []
    return [
      { label: t('directorPanel.tuning.check.recommendedGroup'), options: recommended },
      { label: t('directorPanel.tuning.check.othersGroup'), options: others },
    ]
  }, [recommendedRuleSystemId, ruleScopedOptions, t])
  const recommendedRule = useMemo(
    () => ruleScopedOptions.find((option) => option.id === recommendedRuleSystemId),
    [recommendedRuleSystemId, ruleScopedOptions],
  )
  const stateOptions = useMemo<TuningSelectOption[]>(() => actorStates.map(module => ({ id: module.id, label: builtinModuleDisplayName('actor_state', module, t) })), [actorStates, t])
  const ruleEnabled = !refs.rule_system_disabled
  // No group means "render everything", which is what a single-surface host and
  // the tests expect; a named group renders only its own sections.
  const visible = (name: StoryTuningGroup) => !group || group === name

  return (
    <div className={className || 'director-console__scroll h-full min-h-0 overflow-y-auto px-2.5 py-2.5'}>
      <div className="flex flex-col gap-2">
        {visible('narrative') && <>
        <ControlSection icon={<Palette className="size-4" />} title={t('directorPanel.tuning.style.title')}>
          <ModuleSelectRow
            label={t('directorPanel.tuning.agent.narrativeStyle')}
            // The story's own style field is what generation reads. The module
            // graph also carries a style key for backward compatibility, but a
            // story created from a pack has no style in its graph, so reading
            // that copy would show the default while the story runs on something
            // else.
            value={String(story?.story_teller_id || '')}
            moduleDisabled={Boolean(refs.narrative_style_disabled)}
            options={narrativeOptions}
            busy={savingKey === 'narrative'}
            disabled={disabled}
            onChange={(value) => updateModule(
              'narrative',
              'narrative_style_id',
              'narrative_style_disabled',
              value,
              (nextID) => ({ story_teller_id: nextID }),
            )}
          />
          <StylePurposeRow
            label={t('directorPanel.tuning.agent.stylePurpose.dialogue')}
            purpose="dialogue"
            value={stylePurposes.dialogue}
            options={purposeOptions}
            busy={savingKey === 'style-dialogue'}
            disabled={disabled}
            onChange={(value) => void saveStylePurpose('style-dialogue', 'dialogue', value)}
          />
          <StylePurposeRow
            label={t('directorPanel.tuning.agent.stylePurpose.combat')}
            purpose="combat"
            value={stylePurposes.combat}
            options={purposeOptions}
            busy={savingKey === 'style-combat'}
            disabled={disabled}
            onChange={(value) => void saveStylePurpose('style-combat', 'combat', value)}
          />
          <TuningRow title={t('directorPanel.tuning.agent.replyLength')} busy={savingKey === 'reply-length'}>
            <ReplyLengthSetting
              value={story?.reply_target_chars || 2000}
              label={t('directorPanel.tuning.agent.replyLength')}
              disabled={disabled}
              onCommit={(reply_target_chars) => void save('reply-length', { reply_target_chars })}
            />
          </TuningRow>
          <TuningRow title={t('directorPanel.tuning.agent.choiceCount')} busy={savingKey === 'choice-count'}>
            <NumberSettingInput
              value={story?.choice_count || 5}
              min={MIN_INTERACTIVE_CHOICE_COUNT}
              max={MAX_INTERACTIVE_CHOICE_COUNT}
              label={t('directorPanel.tuning.agent.choiceCount')}
              disabled={disabled}
              onCommit={(choice_count) => void save('choice-count', { choice_count })}
            />
          </TuningRow>
        </ControlSection>

        <ControlSection
          icon={<Compass className="size-4" />}
          title={t('directorPanel.tuning.planning.title')}
          action={<TuningLinkButton label={t('directorPanel.tuning.editPlanning')} onClick={onOpenPresets} />}
        >
          <TuningRow title={t('directorPanel.tuning.agent.planningTemplate')} busy={savingKey === 'planning-template'}>
            <div className="flex min-w-0 flex-col items-end">
              <TuningSelect
                value={story?.planning_template_id || 'default'}
                options={planningOptions}
                groups={planningGroups.length ? planningGroups : undefined}
                label={t('directorPanel.tuning.agent.planningTemplate')}
                disabled={!story || !onPlanningTemplateChange || Boolean(savingKey)}
                onChange={(value) => void selectPlanningTemplate(value)}
              />
              <TuningOptionDescription
                options={planningOptions}
                value={story?.planning_template_id || 'default'}
              />
              {recommendedPlanning ? (
                <TuningRecommendation
                  label={t('directorPanel.tuning.planning.recommendedBy')}
                  name={recommendedPlanning.name}
                  description={recommendedPlanning.description}
                />
              ) : null}
            </div>
          </TuningRow>
          <TuningRow title={t('directorPanel.tuning.agent.planning')} description={t('directorPanel.tuning.agent.planningHint')} busy={savingKey === 'planning'}>
            <Switch
              checked={story?.planning_mode === 'enabled'}
              disabled={disabled}
              aria-label={t('directorPanel.tuning.agent.planning')}
              onCheckedChange={(enabled) => void save('planning', { planning_mode: enabled ? 'enabled' : 'disabled' })}
            />
          </TuningRow>
        </ControlSection>
        </>}

        {visible('rules') && <>
        <ControlSection icon={<Dices className="size-4" />} title={t('directorPanel.tuning.check.title')}>
          <TuningRow title={t('directorPanel.tuning.check.enabled')} busy={savingKey === 'checks-enabled'}>
            <Switch
              checked={ruleEnabled}
              disabled={disabled}
              aria-label={t('directorPanel.tuning.check.enabled')}
              onCheckedChange={setRuleChecksEnabled}
            />
          </TuningRow>
          <ModuleSelectRow
            label={t('directorPanel.tuning.check.system')}
            value={String(refs.rule_system_id || '')}
            moduleDisabled={!ruleEnabled}
            options={ruleScopedOptions}
            groups={ruleGroups.length ? ruleGroups : undefined}
            busy={savingKey === 'rule-system'}
            disabled={disabled || !ruleEnabled}
            onChange={(value) => updateModule(
              'rule-system',
              'rule_system_id',
              'rule_system_disabled',
              value,
            )}
          />
          {recommendedRule ? (
            <TuningRow title={t('directorPanel.tuning.check.recommendedBy')}>
              <span className="text-xs text-foreground">{recommendedRule.label}</span>
            </TuningRow>
          ) : null}
          <TuningRow title={t('directorPanel.tuning.check.difficulty')} busy={savingKey === 'difficulty'}>
            <TuningSelect
              value={String(checkSettings.difficulty_shift)}
              options={difficultyOptions(t)}
              label={t('directorPanel.tuning.check.difficulty')}
              disabled={disabled || !ruleEnabled}
              onChange={(value) => void save('difficulty', {
                check_settings: { ...checkSettings, difficulty_shift: Number(value) },
              })}
            />
          </TuningRow>
          <TuningRow
            title={t('directorPanel.tuning.check.rollModifier')}
            description={t('directorPanel.tuning.check.formula', { modifier: formatSigned(checkSettings.roll_modifier) })}
            busy={savingKey === 'roll-modifier'}
          >
            <NumberSettingInput
              value={checkSettings.roll_modifier}
              min={-20}
              max={20}
              label={t('directorPanel.tuning.check.rollModifier')}
              disabled={disabled || !ruleEnabled}
              onCommit={(roll_modifier) => void save('roll-modifier', {
                check_settings: { ...checkSettings, roll_modifier },
              })}
            />
          </TuningRow>
          <TuningRow
            title={t('directorPanel.tuning.check.stateConsumption')}
            busy={savingKey === 'state-consumption'}
          >
            <TuningSelect
              value={checkSettings.rule_state_consumption_mode || 'hybrid_auto'}
              options={ruleStateConsumptionOptions(t)}
              label={t('directorPanel.tuning.check.stateConsumption')}
              disabled={disabled || !ruleEnabled}
              onChange={(value) => void save('state-consumption', {
                check_settings: {
                  ...checkSettings,
                  rule_state_consumption_mode: value as StoryCheckSettings['rule_state_consumption_mode'],
                },
              })}
            />
          </TuningRow>
        </ControlSection>
        </>}

        {visible('media') && <>
        <ControlSection icon={<ImagePlus className="size-4" />} title={t('directorPanel.tuning.image.title')}
          action={<TuningLinkButton label={t('directorPanel.tuning.image.configure')} onClick={() => requestSettingsSection('image')} />}>
          {imageConfigured && <>
            <TuningRow title={t('directorPanel.tuning.image.automatic')} busy={savingKey === 'image-mode'}>
              <Switch
                checked={imageSettings.mode === 'interval'}
                disabled={disabled}
                aria-label={t('directorPanel.tuning.image.automatic')}
                onCheckedChange={(automatic) => void save('image-mode', {
                  image_settings: { ...imageSettings, mode: automatic ? 'interval' : 'manual' },
                })}
              />
            </TuningRow>
            <TuningRow title={t('directorPanel.tuning.image.interval')} busy={savingKey === 'image-interval'}>
              <NumberSettingInput
                value={imageSettings.interval_turns}
                min={1}
                max={50}
                label={t('directorPanel.tuning.image.interval')}
                disabled={disabled || imageSettings.mode !== 'interval'}
                onCommit={(interval_turns) => void save('image-interval', {
                  image_settings: {
                    ...imageSettings,
                    interval_turns: normalizeImageIntervalTurns(interval_turns),
                  },
                })}
              />
            </TuningRow>
            <ModuleSelectRow
              label={t('directorPanel.tuning.image.preset')}
              value={imageSettings.preset_id || String(refs.image_preset_id || '')}
              moduleDisabled={Boolean(refs.image_preset_disabled)}
              options={imageOptions}
              busy={savingKey === 'image-preset'}
              disabled={disabled}
              onChange={(value) => updateModule(
                'image-preset',
                'image_preset_id',
                'image_preset_disabled',
                value,
                (nextID) => ({ image_settings: { ...imageSettings, preset_id: nextID } }),
              )}
            />
          </>}
        </ControlSection>

        <StorySpeechControls story={story} disabled={disabled} onChange={speech_settings => { void save('speech_settings', { speech_settings }) }} />
        </>}

        {visible('rules') && <>
        <ControlSection icon={<UserRound className="size-4" />} title={t('directorPanel.tuning.state.title')}>
          <ModuleSelectRow
            label={t('directorPanel.tuning.state.system')}
            value={String(refs.actor_state_id || '')}
            moduleDisabled={Boolean(refs.actor_state_disabled)}
            options={stateOptions}
            busy={savingKey === 'state-system'}
            disabled={disabled || structureLocked}
            locked={structureLocked}
            onChange={(value) => updateModule(
              'state-system',
              'actor_state_id',
              'actor_state_disabled',
              value,
            )}
          />
          <TuningRow title={t('directorPanel.tuning.state.display')}>
            <StateDisplayPreferenceMenu
              value={stateDisplayPreference}
              onChange={onStateDisplayPreferenceChange}
            />
          </TuningRow>
        </ControlSection>
        </>}
      </div>
    </div>
  )
}

function cloneRefs(refs: StoryDirectorModuleRefs): StoryDirectorModuleRefs {
  return { ...refs, event_package_ids: [...(refs.event_package_ids || [])] }
}

function moduleOption(item: { id: string; name: string }): TuningSelectOption {
  return { id: item.id, label: item.name || item.id }
}

function formatSigned(value: number): string {
  return value > 0 ? `+${value}` : String(value)
}

function difficultyOptions(t: ReturnType<typeof useTranslation>['t']): TuningSelectOption[] {
  return [-2, -1, 0, 1, 2].map((value) => ({
    id: String(value),
    label: t(`directorPanel.tuning.check.difficulty.${value}`),
  }))
}

function ruleStateConsumptionOptions(t: ReturnType<typeof useTranslation>['t']): TuningSelectOption[] {
  return ['hybrid_auto', 'suggestions_only'].map((mode) => ({
    id: mode,
    label: t(`directorPanel.tuning.check.stateConsumption.${mode}`),
  }))
}
