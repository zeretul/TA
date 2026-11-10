import { useEffect, useMemo, useState } from 'react'
import { getRuleSystems } from '@/features/interactive/api'
import type { RuleSystemModule } from '@/features/interactive/types'
import { useTranslation } from 'react-i18next'
import { builtinModuleDisplayName } from '@/features/interactive/builtin-display-labels'
import { Check, Minus, Plus } from 'lucide-react'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type {
  StoryPackModule,
  StoryPackSettings,
  StoryPackSummary,
} from '@/lib/api-client'
import { gamePlanningTemplateDescription, gamePlanningTemplateName, splitRecommendedPlanningTemplates } from '@/features/interactive/game-planning'
import { narrativeStyleName, recommendedNarrativeStyles } from '@/features/interactive/narrative-style'
import type { GamePlanningTemplate, Teller } from '@/features/interactive/types'
import { ReplyLengthSetting } from '@/features/interactive/components/director-console/ReplyLengthSetting'
import { NumberSettingInput } from '@/features/interactive/components/director-console/StoryTuningControls'

/**
 * The value a purpose select shows when it follows the main narrative.
 *
 * Radix selects cannot carry an empty value, so "inherit" is shown under a token
 * and translated back to the empty string the API stores.
 */
const STYLE_PURPOSE_INHERIT = '__inherit__'

type GroupID = 'planning' | 'style' | 'check' | 'content'

export interface WorldSettingsValue {
  ruleSystemId: string
  planningMode: string
  planningTemplateId: string
  styleMain: string
  styleDialogue: string
  styleCombat: string
  replyTargetChars: number
  /**
   * Retired from the page, kept only so a save does not erase text a world's
   * author already wrote. Writing direction now comes from the shared writing
   * preferences, so this value never reaches a model again.
   */
  narrativeRequirements: string
  writingPreferences: string
  difficultyShift: number
  rollModifier: number
  stateConsumptionMode: string
  visibilityMode: string
}

interface WorldSettingsEditorProps {
  pack: StoryPackSummary
  value: WorldSettingsValue
  onChange: (value: WorldSettingsValue) => void
  tellers: Teller[]
  planningTemplates: GamePlanningTemplate[]
  modules: StoryPackModule[]
  saving: boolean
  disabled?: boolean
}

/** The resolved settings a pack reports, in the page's editing shape. */
export function worldSettingsValue(settings: StoryPackSettings | undefined): WorldSettingsValue {
  const checks = settings?.check_settings ?? {}
  return {
    ruleSystemId: settings?.rule_system_id || '',
    planningMode: settings?.planning_mode === 'disabled' ? 'disabled' : 'enabled',
    planningTemplateId: settings?.planning_template_id || 'default',
    styleMain: settings?.style_purposes?.main || '',
    styleDialogue: settings?.style_purposes?.dialogue || '',
    styleCombat: settings?.style_purposes?.combat || '',
    replyTargetChars: settings?.reply_target_chars || 0,
    narrativeRequirements: settings?.narrative_requirements || '',
    writingPreferences: settings?.writing_preferences || '',
    difficultyShift: checks.difficulty_shift ?? 0,
    rollModifier: checks.roll_modifier ?? 0,
    stateConsumptionMode: checks.rule_state_consumption_mode || 'hybrid_auto',
    visibilityMode: checks.rule_visibility_mode || 'audit_only',
  }
}

/** The request body for saving; the server fills the application's fallbacks. */
export function storyPackSettingsFrom(value: WorldSettingsValue): StoryPackSettings {
  return {
    rule_system_id: value.ruleSystemId,
    planning_mode: value.planningMode,
    planning_template_id: value.planningTemplateId,
    style_purposes: {
      main: value.styleMain,
      dialogue: value.styleDialogue,
      combat: value.styleCombat,
    },
    reply_target_chars: value.replyTargetChars,
    narrative_requirements: value.narrativeRequirements,
    writing_preferences: value.writingPreferences,
    check_settings: {
      difficulty_shift: value.difficultyShift,
      roll_modifier: value.rollModifier,
      rule_state_consumption_mode: value.stateConsumptionMode,
      rule_visibility_mode: value.visibilityMode,
    },
  }
}

/**
 * The world's new-game defaults, grouped the way the design page groups them.
 *
 * Every control writes into the page's draft, never to the server: the page has
 * one save action, so a half-edited block is never stored and a running story is
 * never touched. Each group is a whole-row header carrying its current value, so
 * a collapsed group still says what it holds.
 */
export function WorldSettingsEditor({
  pack,
  value,
  onChange,
  tellers,
  planningTemplates,
  modules,
  saving,
  disabled = false,
}: WorldSettingsEditorProps) {
  const { t } = useTranslation()
  const [ruleSystems, setRuleSystems] = useState<RuleSystemModule[]>([])
  useEffect(() => {
    let cancelled = false
    void getRuleSystems().then((items) => { if (!cancelled) setRuleSystems(items) })
      .catch((error) => console.error('[world-settings] Failed to load rule systems', error))
    return () => { cancelled = true }
  }, [])
  // Only style starts open; the rest read as summaries until asked for.
  const [openGroups, setOpenGroups] = useState<Record<GroupID, boolean>>({
    planning: false,
    style: true,
    check: false,
    content: false,
  })

  const styleOptions = useMemo(
    () => recommendedNarrativeStyles(tellers).map((teller) => ({ id: teller.id, label: narrativeStyleName(teller, t) })),
    [t, tellers],
  )
  const styleOptionsFor = (id: string) => {
    const saved = tellers.find(teller => teller.id === id)
    return saved && !styleOptions.some(option => option.id === id)
      ? [...styleOptions, { id, label: narrativeStyleName(saved, t) }] : styleOptions
  }
  const planningOptions = useMemo<ValueOption[]>(
    () => planningTemplates.map((template) => ({
      id: template.id,
      label: gamePlanningTemplateName(template, t),
      description: gamePlanningTemplateDescription(template, t) || undefined,
    })),
    [planningTemplates, t],
  )
  // The world's own suggestion heads the list under its own heading, so the
  // author's recommendation is distinguishable from the alternatives rather
  // than being one more entry in an undifferentiated run.
  const planningGroups = useMemo(() => {
    const split = splitRecommendedPlanningTemplates(planningTemplates, pack.recommended_planning_template_id)
    if (!split.recommendedID) return undefined
    const toOption = (template: GamePlanningTemplate): ValueOption => ({
      id: template.id,
      label: gamePlanningTemplateName(template, t),
      description: gamePlanningTemplateDescription(template, t) || undefined,
    })
    return [
      { label: t('worldPack.settings.planning.recommendedGroup'), options: split.recommended.map(toOption) },
      { label: t('worldPack.settings.planning.othersGroup'), options: split.others.map(toOption) },
    ]
  }, [pack.recommended_planning_template_id, planningTemplates, t])
  const styleMainName = styleOptions.find((option) => option.id === value.styleMain)?.label || value.styleMain
  const planningTemplateName = planningOptions.find((option) => option.id === value.planningTemplateId)?.label
    || value.planningTemplateId
  // Only named when the template really exists on this machine; a world that
  // recommends something this build does not ship suggests nothing rather than
  // naming an id the author cannot choose.
  const recommendedPlanning = splitRecommendedPlanningTemplates(
    planningTemplates,
    pack.recommended_planning_template_id,
  ).recommended[0]
  const recommendedPlanningName = recommendedPlanning ? gamePlanningTemplateName(recommendedPlanning, t) : ''

  const blocked = disabled || saving
  const set = (patch: Partial<WorldSettingsValue>) => onChange({ ...value, ...patch })

  const ruleModule = modules.find((module) => module.kind === 'rule_system')
  const stateID = modules.find((module) => module.kind === 'actor_state')?.id
  const ruleOptions = ruleSystems.filter((item) => !item.actor_state_id || item.actor_state_id === stateID)
    .map((item) => ({ id: item.id, label: builtinModuleDisplayName('rule_system', item, t) }))
  const recommendedLabel = builtinModuleDisplayName('rule_system', ruleModule, t)
  const selectedRuleID = value.ruleSystemId || ruleModule?.id || '__recommended__'
  if (!ruleOptions.some((item) => item.id === selectedRuleID)) {
    ruleOptions.unshift({ id: selectedRuleID, label: selectedRuleID === ruleModule?.id ? recommendedLabel : selectedRuleID })
  }
  const stateModule = modules.find((module) => module.kind === 'actor_state')
  const eventModules = modules.filter((module) => module.kind === 'event_package')
  const toggle = (group: GroupID) => setOpenGroups((current) => ({ ...current, [group]: !current[group] }))

  return (
    <div className="min-w-0">
      <SettingsGroup
        id="planning"
        title={t('worldPack.settings.groups.planning')}
        summary={value.planningMode === 'disabled'
          ? t('worldPack.settings.planning.off')
          : t('worldPack.settings.planning.on', { template: planningTemplateName })}
        open={openGroups.planning}
        onToggle={() => toggle('planning')}
      >
        <SettingRow label={t('worldPack.settings.planning.enable')} hint={t('worldPack.settings.planning.enableHint')}>
          <Switch
            size="sm"
            checked={value.planningMode !== 'disabled'}
            disabled={blocked}
            aria-label={t('worldPack.settings.planning.enable')}
            onCheckedChange={(enabled) => set({ planningMode: enabled ? 'enabled' : 'disabled' })}
          />
        </SettingRow>
        <SettingRow label={t('worldPack.settings.planning.template')} hint={t('worldPack.settings.planning.templateHint')}>
          {planningTemplates.length === 0 ? (
            <span className="text-xs text-[var(--nova-text-faint)]">{t('worldPack.settings.planning.loadingTemplates')}</span>
          ) : (
            <div className="flex min-w-0 flex-col gap-1">
              <ValueSelect
                value={value.planningTemplateId}
                options={planningOptions}
                groups={planningGroups}
                label={t('worldPack.settings.planning.template')}
                disabled={blocked || value.planningMode === 'disabled'}
                onChange={(planningTemplateId) => set({ planningTemplateId })}
              />
              {/* The world's own suggestion stays readable after the author
                  picks something else, which is what makes it a suggestion
                  rather than a silent default. */}
              {recommendedPlanningName ? (
                <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">
                  <span className="font-medium text-[var(--nova-text)]">{t('worldPack.settings.planning.recommendedBy')}</span>
                  <span className="mx-1">·</span>
                  <span>{recommendedPlanningName}</span>
                </p>
              ) : null}
            </div>
          )}
        </SettingRow>
      </SettingsGroup>

      <SettingsGroup
        id="style"
        title={t('worldPack.settings.groups.style')}
        summary={styleMainName}
        open={openGroups.style}
        onToggle={() => toggle('style')}
      >
        <SettingRow label={t('worldPack.settings.style.main')}>
          <ValueSelect
            value={value.styleMain}
            options={styleOptionsFor(value.styleMain)}
            label={t('worldPack.settings.style.main')}
            disabled={blocked}
            onChange={(styleMain) => set({ styleMain })}
          />
        </SettingRow>
        <SettingRow label={t('worldPack.settings.style.dialogue')} hint={t('worldPack.settings.style.dialogueHint')}>
          <ValueSelect
            value={value.styleDialogue || STYLE_PURPOSE_INHERIT}
            options={[{ id: STYLE_PURPOSE_INHERIT, label: t('worldPack.settings.style.inherit') }, ...styleOptionsFor(value.styleDialogue)]}
            label={t('worldPack.settings.style.dialogue')}
            disabled={blocked}
            onChange={(next) => set({ styleDialogue: next === STYLE_PURPOSE_INHERIT ? '' : next })}
          />
        </SettingRow>
        <SettingRow label={t('worldPack.settings.style.combat')} hint={t('worldPack.settings.style.combatHint')}>
          <ValueSelect
            value={value.styleCombat || STYLE_PURPOSE_INHERIT}
            options={[{ id: STYLE_PURPOSE_INHERIT, label: t('worldPack.settings.style.inherit') }, ...styleOptionsFor(value.styleCombat)]}
            label={t('worldPack.settings.style.combat')}
            disabled={blocked}
            onChange={(next) => set({ styleCombat: next === STYLE_PURPOSE_INHERIT ? '' : next })}
          />
        </SettingRow>
        <SettingRow label={t('worldPack.settings.style.replyLength')} hint={t('worldPack.settings.style.replyLengthHint')}>
          <ReplyLengthSetting
            value={value.replyTargetChars || 2500}
            label={t('worldPack.settings.style.replyLength')}
            disabled={blocked}
            onCommit={(replyTargetChars) => set({ replyTargetChars })}
          />
        </SettingRow>
      </SettingsGroup>

      <SettingsGroup
        id="check"
        title={t('worldPack.settings.groups.check')}
        summary={[ruleOptions.find((item) => item.id === selectedRuleID)?.label, builtinModuleDisplayName('actor_state', stateModule, t)].filter(Boolean).join(' · ')}
        open={openGroups.check}
        onToggle={() => toggle('check')}
      >
        <SettingRow label={t('worldPack.settings.check.rule')} hint={`${t('directorPanel.tuning.check.recommendedBy')} · ${recommendedLabel}`}>
          <ValueSelect value={selectedRuleID} options={ruleOptions}
            groups={ruleModule ? [
              { label: t('directorPanel.tuning.check.recommendedGroup'), options: ruleOptions.filter((item) => item.id === ruleModule.id) },
              { label: t('directorPanel.tuning.check.othersGroup'), options: ruleOptions.filter((item) => item.id !== ruleModule.id) },
            ] : undefined}
            label={t('worldPack.settings.check.rule')} disabled={blocked}
            onChange={(next) => set({ ruleSystemId: next === ruleModule?.id || next === '__recommended__' ? '' : next })} />
        </SettingRow>
        <SettingRow label={t('worldPack.settings.check.state')} hint={t('worldPack.settings.check.fixed')}>
          <FixedValue>{builtinModuleDisplayName('actor_state', stateModule, t) || t('worldPack.settings.check.state')}</FixedValue>
        </SettingRow>
        <SettingRow label={t('worldPack.settings.check.difficulty')} hint={t('worldPack.settings.check.hint')}>
          <ValueSelect
            value={String(value.difficultyShift)}
            options={[-2, -1, 0, 1, 2].map((shift) => ({
              id: String(shift),
              label: t(`directorPanel.tuning.check.difficulty.${shift}`),
            }))}
            label={t('worldPack.settings.check.difficulty')}
            disabled={blocked}
            onChange={(next) => set({ difficultyShift: Number(next) })}
          />
        </SettingRow>
        <SettingRow label={t('worldPack.settings.check.rollModifier')}>
          <NumberSettingInput
            value={value.rollModifier}
            min={-20}
            max={20}
            label={t('worldPack.settings.check.rollModifier')}
            disabled={blocked}
            onCommit={(rollModifier) => set({ rollModifier })}
          />
        </SettingRow>
        <SettingRow label={t('worldPack.settings.check.stateConsumption')}>
          <ValueSelect
            value={value.stateConsumptionMode}
            options={['hybrid_auto', 'suggestions_only'].map((mode) => ({
              id: mode,
              label: t(`directorPanel.tuning.check.stateConsumption.${mode}`),
            }))}
            label={t('worldPack.settings.check.stateConsumption')}
            disabled={blocked}
            onChange={(stateConsumptionMode) => set({ stateConsumptionMode })}
          />
        </SettingRow>
        <SettingRow label={t('worldPack.settings.check.visibility')}>
          <ValueSelect
            value={value.visibilityMode}
            options={['audit_only', 'public_roll'].map((mode) => ({
              id: mode,
              label: t(`directorPanel.tuning.check.visibility.${mode}`),
            }))}
            label={t('worldPack.settings.check.visibility')}
            disabled={blocked}
            onChange={(visibilityMode) => set({ visibilityMode })}
          />
        </SettingRow>
      </SettingsGroup>

      <SettingsGroup
        id="content"
        title={t('worldPack.settings.groups.content')}
        summary={t('worldPack.settings.content.summary')}
        open={openGroups.content}
        onToggle={() => toggle('content')}
      >
        <div className="flex min-w-0 flex-col gap-1.5 py-2 text-xs text-[var(--nova-text-muted)]">
          <p>
            {t('worldPack.settings.content.events')}
            {' · '}
            {eventModules.length > 0
              ? eventModules.map((module) => module.name || module.id).join(' · ')
              : t('worldPack.settings.content.summary')}
          </p>
          <p>{t('worldPack.overview.publicEntries', { count: pack.public_world_count ?? pack.world_count })}</p>
          <p>{t('worldPack.settings.content.people', { count: pack.character_count })}</p>
          <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">
            {t('worldPack.settings.content.hint')}
          </p>
          {pack.invalid || (pack.missing ?? 0) > 0 ? (
            <p className="text-[11px] text-[var(--nova-danger)]">{t('worldPack.settings.content.missing')}</p>
          ) : null}
        </div>
      </SettingsGroup>
    </div>
  )
}

function SettingsGroup({
  id,
  title,
  summary,
  open,
  onToggle,
  children,
}: {
  id: string
  title: string
  summary: string
  open: boolean
  onToggle: () => void
  children: React.ReactNode
}) {
  const Icon = open ? Minus : Plus
  return (
    <section data-settings-group={id} className="min-w-0 border-b border-[var(--nova-border)] first:border-t">
      {/* The whole header row is the control, so a long name and its value stay
          one target rather than two. */}
      <button
        type="button"
        aria-expanded={open}
        onClick={onToggle}
        className="flex min-h-11 w-full min-w-0 items-center gap-3 px-0.5 py-2.5 text-left outline-none transition-colors hover:bg-[var(--nova-hover)] focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]/50"
      >
        <span className="min-w-0 flex-1 truncate text-sm font-medium text-[var(--nova-text)]">{title}</span>
        <span className="hidden min-w-0 max-w-[45%] truncate text-xs text-[var(--nova-text-faint)] sm:block">{summary}</span>
        <Icon className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" />
      </button>
      {open ? <div className="min-w-0 pb-3 pl-0.5">{children}</div> : null}
    </section>
  )
}

function SettingRow({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="grid min-w-0 grid-cols-1 items-start gap-1.5 py-2 sm:grid-cols-[minmax(6rem,9rem)_minmax(0,1fr)] sm:gap-4">
      <div className="min-w-0 text-xs text-[var(--nova-text-muted)] sm:pt-1.5">{label}</div>
      <div className="flex min-w-0 flex-col gap-1">
        <div className="flex min-w-0 flex-wrap items-center gap-2">{children}</div>
        {hint ? <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{hint}</p> : null}
      </div>
    </div>
  )
}

function FixedValue({ children }: { children: React.ReactNode }) {
  return (
    <span className="flex min-w-0 items-center gap-1.5 text-xs text-[var(--nova-text-muted)]">
      <Check className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" />
      <span className="min-w-0 truncate">{children}</span>
    </span>
  )
}

function ValueSelect({
  value,
  options,
  groups,
  label,
  disabled,
  onChange,
}: {
  value: string
  options: ValueOption[]
  groups?: { label: string; options: ValueOption[] }[]
  label: string
  disabled?: boolean
  onChange: (value: string) => void
}) {
  const all = groups ? groups.flatMap((group) => group.options) : options
  const description = all.find((option) => option.id === value)?.description
  const rendered = groups ?? [{ label: '', options }]
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <Select value={value} disabled={disabled} onValueChange={onChange}>
        <SelectTrigger
          size="sm"
          aria-label={label}
          className="w-full min-w-0 max-w-full bg-[var(--nova-surface-2)] text-xs text-[var(--nova-text)] sm:w-[min(20rem,100%)]"
        >
          <SelectValue>{all.find((option) => option.id === value)?.label || value}</SelectValue>
        </SelectTrigger>
        <SelectContent position="popper">
          {rendered.map((group, index) => (
            <SelectGroup key={group.label || index}>
              {group.label ? <SelectLabel>{group.label}</SelectLabel> : null}
              {group.options.map((option) => (
                <SelectItem key={option.id} value={option.id}>
                  {/* Two lines in the list, because the one-line name says
                      nothing about what choosing it changes. */}
                  <span className="flex min-w-0 flex-col items-start gap-0.5">
                    <span className="truncate">{option.label}</span>
                    {option.description ? (
                      <span className="whitespace-normal text-[10px] leading-4 text-[var(--nova-text-faint)]">{option.description}</span>
                    ) : null}
                  </span>
                </SelectItem>
              ))}
            </SelectGroup>
          ))}
        </SelectContent>
      </Select>
      {/* Repeated beside the closed control: a phone has no hover to read a
          tooltip from, and the chosen line is the one that matters. */}
      {description ? (
        <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{description}</p>
      ) : null}
    </div>
  )
}

interface ValueOption {
  id: string
  label: string
  description?: string
}
