import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { animate, motion, useMotionValue, useReducedMotionConfig } from 'motion/react'
import { AlignLeft, AlertCircle, BookOpen, ChevronDown, ChevronUp, CircleCheck, Gauge, Globe2, LayoutDashboard, Loader2, Package, PanelRight, Sparkles, Tag, Users } from 'lucide-react'
import { useWorldPresentation } from '../../world-presentation-context'
import { useIsMobile } from '@/hooks/useIsMobile'
import { novaEase } from '@/features/motion/motion-tokens'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia } from '@/components/ui/empty'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { ActorStateField, Snapshot } from '../../types'
import { ChangesSummary } from './ChangesSummary'
import { ActorArchiveList } from './ActorArchiveList'
import type { StoryStateDisplayPreference } from './display-preference'
import { applyStoryStateLayout, readStoryStateLayouts, writeStoryStateTemplateLayout, type StoryStateLayouts, type StoryStateTemplateLayout } from './layout-preference'
import { LedgerFieldView } from './ledger-fields'
import {
  actorFieldEntries,
  actorName,
  actorTemplate,
  actorTraits,
  buildLedgerGroups,
  buildStoryStateModel,
  humanizeStateKey,
  isLeadActor,
  splitLedgerGroupsForPreview,
  type LedgerFieldEntry,
  type LedgerFieldGroup,
  type ActorStateEntry,
  type StoryStateChange,
} from './model'
import { StateDisplayPreferenceMenu } from './StateDisplayPreferenceMenu'
import { StateLayoutEditor } from './StateLayoutEditor'
import { isLegacyWorldOverview, playerFacingFieldLabelKey, playerFacingFieldName } from './field-layout'
import { adventureLedgerEntries } from './adventure-state'
import { namedStudyEntries } from './named-study-entries'
import { isSkillAbilityField, isSkillRecordField } from './skills'
import { isMethodRecordField, isMethodSpecialisationField } from './methods'
import { useAdventureDisplaySnapshot } from './use-adventure-display-snapshot'
import { OwnedEntryDetails, ownedEntryEndpoint } from '../entry-details/OwnedEntryDetails'
import type { TFunction } from 'i18next'

const WORLD_STATE_TAB = '__world_state__'

type StoryStatePanelMode = 'collapsed' | 'preview' | 'expanded'

const PANEL_MODE_BY_PREFERENCE: Record<StoryStateDisplayPreference, StoryStatePanelMode> = {
  preview: 'preview',
  expanded: 'expanded',
  collapsed: 'collapsed',
  'director-only': 'collapsed',
}

interface StoryStateLedgerProps {
  snapshot: Snapshot | null
  displayPreference: StoryStateDisplayPreference
  onDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  onOpenDirectorState?: () => void
}

interface StateLedgerPresentation {
  id: string
  name: string
  templateId: string
  groups: LedgerFieldGroup[]
  traits: ReturnType<typeof actorTraits>
}

/**
 * StoryStateLedger is the compact state panel pinned after the latest prose.
 * Fields lay out as bordered group sections on one page. Schema hints provide
 * the fallback grouping, while a story + template UI preference controls the
 * final section and field order. Preview mode shows the first two ordered
 * sections with a "show all" affordance; the turn's state delta surfaces once in the
 * summary row plus per-field change chips.
 */
export function StoryStateLedger({ snapshot, displayPreference, onDisplayPreferenceChange, onOpenDirectorState }: StoryStateLedgerProps) {
  snapshot = useAdventureDisplaySnapshot(snapshot)
  const { t } = useWorldPresentation()
  const isMobile = useIsMobile()
  const { model, actorLedgers, worldLedger, allActors, actorTabs, leadActorId, hasWorldFacts, storyId } = useStoryStateLedgerData(snapshot)
  const [selectedTab, setSelectedTab] = useStoryStateTab(actorTabs, hasWorldFacts)
  const [layoutState, setLayoutState] = useState<{ storyId: string; layouts: StoryStateLayouts }>(() => ({ storyId, layouts: readStoryStateLayouts(storyId) }))
  const [layoutEditorOpen, setLayoutEditorOpen] = useState(false)
  const turnKey = `${snapshot?.story_id || ''}:${snapshot?.branch_id || ''}:${snapshot?.current_turn?.id || ''}`
  const [panelMode, setPanelMode] = useState<StoryStatePanelMode>(isMobile ? 'collapsed' : PANEL_MODE_BY_PREFERENCE[displayPreference])
  const layouts = layoutState.storyId === storyId ? layoutState.layouts : {}
  const selectedLedger = selectedTab === WORLD_STATE_TAB
    ? worldLedger
    : actorLedgers.find((ledger) => ledger.id === selectedTab)

  useEffect(() => {
    // Mobile summaries must not change the saved desktop display preference.
    setPanelMode(isMobile ? 'collapsed' : PANEL_MODE_BY_PREFERENCE[displayPreference])
  }, [displayPreference, turnKey, isMobile])

  useEffect(() => {
    setLayoutState({ storyId, layouts: readStoryStateLayouts(storyId) })
    setLayoutEditorOpen(false)
  }, [storyId])

  if (!model.hasState || displayPreference === 'director-only') return null

  const collapsed = panelMode === 'collapsed'

  return (
    <Collapsible
      open={!collapsed}
      onOpenChange={(nextOpen) => setPanelMode(nextOpen ? 'preview' : 'collapsed')}
      asChild
    >
      <section
        aria-label={t('storyStage.state.current')}
        data-state-panel-mode={panelMode}
        className="story-state-ledger mt-3 overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--story-state-canvas)]"
      >
        <header className="flex h-10 min-w-0 items-center gap-2 px-2.5">
          {isMobile ? (
            <CollapsibleTrigger asChild>
              <button type="button" className="flex min-w-0 flex-1 items-center gap-2 text-left" aria-label={collapsed ? t('storyStage.state.expand') : t('storyStage.state.collapse')}>
                <StatusIndicator status={snapshot?.current_turn?.state_status} />
                <span className="min-w-0 flex-1">
                  <span className="block text-sm font-semibold">{t('storyStage.state.current')}</span>
                  <span className="block truncate text-xs text-muted-foreground">{turnStatusLabel(snapshot, t)}</span>
                </span>
                {collapsed ? <ChevronDown className="size-4 shrink-0" /> : <ChevronUp className="size-4 shrink-0" />}
              </button>
            </CollapsibleTrigger>
          ) : <>
          <StatusIndicator status={snapshot?.current_turn?.state_status} />
          <div className="flex min-w-0 flex-1 items-baseline gap-2">
            <h2 className="shrink-0 text-[13px] font-semibold tracking-tight text-[var(--nova-text)]">{t('storyStage.state.current')}</h2>
            <p className="min-w-0 truncate text-[11px] text-[var(--nova-text-faint)]">{turnStatusLabel(snapshot, t)}</p>
          </div>
          </>}
          {isMobile ? (
            selectedLedger?.groups.length ? <Button type="button" variant="ghost" size="icon-sm" aria-label={t('storyStage.state.layout.customize')} onClick={() => setLayoutEditorOpen(true)}><LayoutDashboard /></Button> : null
          ) : <StateDisplayPreferenceMenu
            value={displayPreference}
            onChange={onDisplayPreferenceChange}
            onCustomizeLayout={selectedLedger?.groups.length ? () => setLayoutEditorOpen(true) : undefined}
            compact
          />}
          {onOpenDirectorState ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={onOpenDirectorState}
              aria-label={t('storyStage.state.openDirector')}
            >
              <PanelRight data-icon="inline-start" />
              <span className="story-state-ledger__director-label">{t('storyStage.state.openDirector')}</span>
            </Button>
          ) : null}
          {!isMobile ? <CollapsibleTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={collapsed ? t('storyStage.state.expand') : t('storyStage.state.collapse')}
            >
              {collapsed ? <ChevronDown data-icon="inline-start" /> : <ChevronUp data-icon="inline-start" />}
            </Button>
          </CollapsibleTrigger> : null}
        </header>

        {isMobile && collapsed && model.hasState ? <ChangesSummary changes={model.changes} actors={allActors} schema={snapshot?.actor_state_schema} turnCount={snapshot?.turn_count} /> : null}
        <CollapsibleContent forceMount>
          <StateReveal open={!collapsed}>
            {model.hasState ? (
              <ChangesSummary changes={model.changes} actors={allActors} schema={snapshot?.actor_state_schema} turnCount={snapshot?.turn_count} />
            ) : null}
            <StateEntityPanels
              snapshot={snapshot}
              leadActorId={leadActorId}
              actorLedgers={actorLedgers}
              actorTabs={actorTabs}
              worldLedger={worldLedger}
              showWorld={hasWorldFacts}
              selectedTab={selectedTab}
              layouts={layouts}
              panelMode={panelMode === 'expanded' ? 'expanded' : 'preview'}
              onSelectedTabChange={setSelectedTab}
              onPanelModeChange={setPanelMode}
            />
            <ActorArchiveList entries={model.archivedActors} />
          </StateReveal>
        </CollapsibleContent>
        {selectedLedger ? (
          <StateLayoutEditor
            open={layoutEditorOpen}
            title={selectedLedger.id === WORLD_STATE_TAB ? t('storyStage.state.world') : selectedLedger.name}
            groups={selectedLedger.groups}
            value={layouts[selectedLedger.templateId]}
            onOpenChange={setLayoutEditorOpen}
            onChange={(layout) => {
              const next = { ...layouts, [selectedLedger.templateId]: layout }
              setLayoutState({ storyId, layouts: next })
              writeStoryStateTemplateLayout(storyId, selectedLedger.templateId, layout)
            }}
            onReset={() => {
              const next = { ...layouts }
              delete next[selectedLedger.templateId]
              setLayoutState({ storyId, layouts: next })
              writeStoryStateTemplateLayout(storyId, selectedLedger.templateId, null)
            }}
          />
        ) : null}
      </section>
    </Collapsible>
  )
}

/**
 * Full-width state projection for secondary surfaces such as the Story Console
 * dialog. It reuses the stage ledger's grouping, field renderers, saved layout,
 * and Actor/world navigation without repeating the narrow sidebar treatment.
 */
export function StoryStateDetails({ snapshot }: { snapshot: Snapshot | null }) {
  snapshot = useAdventureDisplaySnapshot(snapshot)
  const { t } = useWorldPresentation()
  const { model, actorLedgers, worldLedger, allActors, actorTabs, leadActorId, hasWorldFacts, storyId } = useStoryStateLedgerData(snapshot)
  const layouts = useMemo(() => readStoryStateLayouts(storyId), [storyId])
  const [selectedTab, setSelectedTab] = useStoryStateTab(actorTabs, hasWorldFacts)
  const [panelMode, setPanelMode] = useState<StoryStatePanelMode>('expanded')

  useEffect(() => {
    setPanelMode('expanded')
  }, [storyId])

  if (!model.hasState) return <StateSectionEmpty label={t('directorPanel.stateEmpty')} />

  return (
    <section className="story-state-ledger overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--story-state-canvas)]">
      {model.hasState ? (
        <ChangesSummary changes={model.changes} actors={allActors} schema={snapshot?.actor_state_schema} turnCount={snapshot?.turn_count} standalone />
      ) : null}
      <StateEntityPanels
        snapshot={snapshot}
        leadActorId={leadActorId}
        actorLedgers={actorLedgers}
        actorTabs={actorTabs}
        worldLedger={worldLedger}
        showWorld={hasWorldFacts}
        selectedTab={selectedTab}
        layouts={layouts}
        panelMode={panelMode === 'expanded' ? 'expanded' : 'preview'}
        onSelectedTabChange={setSelectedTab}
        onPanelModeChange={setPanelMode}
      />
      <ActorArchiveList entries={model.archivedActors} />
    </section>
  )
}

function useStoryStateLedgerData(snapshot: Snapshot | null) {
  const { t } = useWorldPresentation()
  const model = useMemo(() => buildStoryStateModel(snapshot), [snapshot])
  const leadActorId = model.actors.find(([id]) => id === 'protagonist')?.[0]
    ?? model.actors.find(([id, actor]) => isLeadActor(id, actor))?.[0]
  const actorLedgers = useMemo(
    () => model.actors.map(([actorId, actor]) => buildActorLedger(actorId, actor, snapshot, model.changes, t, actorId === leadActorId)),
    [model.actors, model.changes, snapshot, t, leadActorId],
  )
  const worldLedger = useMemo(() => buildWorldLedger(model.worldFacts, model.changes, snapshot?.actor_state_schema), [model.changes, model.worldFacts, snapshot?.actor_state_schema])
  const allActors = useMemo<ActorStateEntry[]>(() => [
    ...model.actors,
    ...model.archivedActors.map((entry): ActorStateEntry => [entry.actorId, { name: entry.name, template_id: entry.templateId }]),
  ], [model.actors, model.archivedActors])
  const actorTabs = useMemo(() => actorLedgers.map(({ id, name }) => ({ id, name })), [actorLedgers])

  return {
    model,
    actorLedgers,
    worldLedger,
    allActors,
    actorTabs,
    leadActorId,
    hasWorldFacts: model.worldFacts.length > 0,
    storyId: snapshot?.story_id || '',
  }
}

function useStoryStateTab(actorTabs: Array<{ id: string; name: string }>, showWorld: boolean) {
  const [selectedTab, setSelectedTab] = useState(actorTabs[0]?.id || WORLD_STATE_TAB)

  useEffect(() => {
    if (selectedTab === WORLD_STATE_TAB && showWorld) return
    if (actorTabs.some(({ id }) => id === selectedTab)) return
    setSelectedTab(actorTabs[0]?.id || WORLD_STATE_TAB)
  }, [actorTabs, selectedTab, showWorld])

  return [selectedTab, setSelectedTab] as const
}

function StateEntityPanels({
  snapshot,
  leadActorId,
  actorLedgers,
  actorTabs,
  worldLedger,
  showWorld,
  selectedTab,
  layouts,
  panelMode,
  onSelectedTabChange,
  onPanelModeChange,
}: {
  snapshot: Snapshot | null
  leadActorId?: string
  actorLedgers: StateLedgerPresentation[]
  actorTabs: Array<{ id: string; name: string }>
  worldLedger: StateLedgerPresentation
  showWorld: boolean
  selectedTab: string
  layouts: StoryStateLayouts
  panelMode: 'preview' | 'expanded'
  onSelectedTabChange: (tab: string) => void
  onPanelModeChange: (mode: StoryStatePanelMode) => void
}) {
  const reducedMotion = useReducedMotionConfig()
  const viewportRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const height = useMotionValue<number | 'auto'>('auto')

  useLayoutEffect(() => {
    if (height.get() === 'auto' || !contentRef.current) return
    // Animate real layout height so the chat footer can follow each frame.
    // Return to intrinsic sizing afterward to avoid nesting height animations
    // when sections expand, content changes, or the viewport resizes.
    const controls = animate(height, contentRef.current.getBoundingClientRect().height, {
      type: 'tween', duration: reducedMotion ? 0 : 0.16, ease: novaEase,
    })
    let cancelled = false
    void controls.then(() => { if (!cancelled) height.set('auto') })
    return () => { cancelled = true; controls.stop() }
  }, [selectedTab, reducedMotion, height])

  const selectTab = (tab: string) => {
    if (tab === selectedTab) return
    if (viewportRef.current) height.set(viewportRef.current.getBoundingClientRect().height)
    onSelectedTabChange(tab)
  }
  if (actorLedgers.length === 0 && !showWorld) return null

  return (
    <Tabs value={selectedTab} onValueChange={selectTab} className="gap-0">
      <StateEntityTabs actors={actorTabs} showWorld={showWorld} />
      <motion.div ref={viewportRef} style={{ height, overflow: 'hidden' }}>
        <div ref={contentRef}>
          {actorLedgers.map((ledger) => (
            <TabsContent key={ledger.id} value={ledger.id} forceMount hidden={selectedTab !== ledger.id} className="mt-0">
              <motion.div
                initial={false}
                animate={{ opacity: selectedTab === ledger.id ? 1 : 0 }}
                transition={{ duration: reducedMotion ? 0 : 0.14, ease: novaEase }}
              >
                <ActorLedgerBody
                  ledger={ledger}
                  layout={layouts[ledger.templateId]}
                  panelMode={panelMode}
                  onPanelModeChange={onPanelModeChange}
                />
                {ledger.id === leadActorId ? <OwnedEntryDetails snapshot={snapshot} /> : null}
              </motion.div>
            </TabsContent>
          ))}
          {showWorld ? (
            <TabsContent value={WORLD_STATE_TAB} forceMount hidden={selectedTab !== WORLD_STATE_TAB} className="mt-0">
              <motion.div
                initial={false}
                animate={{ opacity: selectedTab === WORLD_STATE_TAB ? 1 : 0 }}
                transition={{ duration: reducedMotion ? 0 : 0.14, ease: novaEase }}
              >
                <WorldLedgerBody
                  ledger={worldLedger}
                  layout={layouts[worldLedger.templateId]}
                  panelMode={panelMode}
                  onPanelModeChange={onPanelModeChange}
                />
              </motion.div>
            </TabsContent>
          ) : null}
        </div>
      </motion.div>
    </Tabs>
  )
}

function StatusIndicator({ status }: { status?: 'pending' | 'ready' | 'failed' }) {
  const { t } = useWorldPresentation()
  if (status === 'pending') {
    return (
      <span
        aria-label={t('storyStage.state.syncingShort')}
        className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-[var(--story-state-pending-soft)] text-[var(--story-state-pending)]"
      >
        <Loader2 aria-hidden="true" className="size-3.5 animate-spin motion-reduce:animate-none" />
      </span>
    )
  }
  if (status === 'failed') {
    return (
      <span
        aria-label={t('storyStage.state.failedShort')}
        className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-[var(--story-state-negative-soft)] text-[var(--story-state-negative)]"
      >
        <AlertCircle aria-hidden="true" className="size-3.5" />
      </span>
    )
  }
  return (
    <span
      aria-label={t('storyStage.state.readyShort')}
      className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-[var(--story-state-positive-soft)] text-[var(--story-state-positive)]"
    >
      <CircleCheck aria-hidden="true" className="size-3.5" />
    </span>
  )
}

function StateEntityTabs({ actors, showWorld }: { actors: Array<{ id: string; name: string }>; showWorld: boolean }) {
  const { t } = useWorldPresentation()
  if (actors.length <= 1 && !showWorld) return null
  return (
    <div className="story-state-ledger__tabs-scroll overflow-x-auto overflow-y-hidden px-2.5 pb-1.5">
      <TabsList
        aria-label={t('storyStage.state.tabs')}
        className="story-state-ledger__tabs-list w-max max-w-none justify-start"
      >
        {actors.map((actor) => (
          <TabsTrigger
            key={actor.id}
            value={actor.id}
            className="min-w-20 max-w-40 flex-none"
          >
            <span className="truncate">{actor.name}</span>
          </TabsTrigger>
        ))}
        {showWorld ? (
          <TabsTrigger
            value={WORLD_STATE_TAB}
            className="min-w-20 flex-none"
          >
            <Globe2 data-icon="inline-start" />
            <span>{t('storyStage.state.world')}</span>
          </TabsTrigger>
        ) : null}
      </TabsList>
    </div>
  )
}

const cultivationPossessionFields = new Set(['主法器', '护具', '布阵', '攻击法宝', '防御法宝', '辅助法宝', '重要物品'])

function buildActorLedger(actorId: string, actor: Record<string, unknown>, snapshot: Snapshot | null, changes: StoryStateChange[], t: TFunction, isLead: boolean): StateLedgerPresentation {
  const template = actorTemplate(actor, snapshot?.actor_state_schema)
  const entries: LedgerFieldEntry[] = actorFieldEntries(actor, template?.fields).map(({ field, value, withheld }) => ({
    id: field.id || field.path || field.name,
    label: playerFacingFieldName(field, field.name),
    labelKey: playerFacingFieldLabelKey(field),
    field,
    value: withheld ? null : value ?? field.default ?? null,
    withheld,
  }))
  const owned = isLead && snapshot ? ownedEntryEndpoint(snapshot) : null
  const named = namedStudyEntries(entries, snapshot?.state, actorId, t('storyStage.state.unnamedStudyRecord'), { craftMethods: snapshot?.actor_state_schema?.system.craft_methods, name: key => t(key) })
  const rows = adventureLedgerEntries(named, snapshot?.actor_state_schema?.system.adventure_rules, t, snapshot?.clock?.seconds || 0)
  const hasRelationship = !isLead && rows.some(row => row.field?.name === '与主角关系' && !row.withheld
    && typeof row.value === 'string' && row.value.trim() !== '' && row.value.trim() !== '未知')
  const visible = rows.filter(row => {
    if (hasRelationship && row.field?.name === '关系' && (row.withheld || row.value === '未知' || row.value == null)) return false
    if (owned === 'cultivation-entry-details' && row.field && cultivationPossessionFields.has(row.field.name)) {
      return row.withheld || (row.field.name === '重要物品' && !isRecordValue(row.value))
    }
    if (owned === 'cultivation-entry-details' && !row.withheld) {
      if ((isSkillAbilityField(row.field) || isSkillRecordField(row.field) || isMethodRecordField(row.field)) && isRecordValue(row.value)) return false
      if (isMethodSpecialisationField(row.field) && (row.value === '家族家传功法，强攻，良好。' || row.value === '叠锋诀，强攻，良好。')) return false
    }
    return owned !== 'adventure-window' || !/^adventure:(item|equipment|asset|skill):/.test(row.id)
  })
  const rawTemplateId = typeof actor.template_id === 'string' ? actor.template_id.trim() : ''
  return {
    id: actorId,
    name: actorName(actorId, actor),
    templateId: template?.id || rawTemplateId || `actor:${actorId}`,
    groups: buildLedgerGroups(visible, changes.filter((change) => change.actorId === actorId)),
    traits: actorTraits(actor),
  }
}

function buildWorldLedger(facts: Array<[string, unknown]>, changes: StoryStateChange[], schema: Snapshot['actor_state_schema']): StateLedgerPresentation {
  // The world's rows are declared by the pack exactly like a character's are —
  // 当前时间 is grouped under 当前场景 and 世界局势 under 世界状态 — and the panel
  // needs that declaration to place a row. Without it every row fell through to
  // the renderer heuristic, which put 商贾 (a shop's cash) beside 在场角色 and
  // pushed 当前事件 into 详情. The names are unique across the two bookkeeping
  // templates, so one lookup by name serves both.
  const fieldsByName = new Map<string, ActorStateField>()
  for (const template of schema?.system.templates ?? []) {
    for (const field of template.fields ?? []) {
      const name = typeof field.name === 'string' ? field.name.trim() : ''
      if (name && !fieldsByName.has(name)) fieldsByName.set(name, field)
    }
  }
  const withField = (name: string): ActorStateField | undefined => fieldsByName.get(name)
    ?? (name === '世界局势' || isLegacyWorldOverview(undefined, name) ? { name, type: 'string' } : undefined)
  // Record-valued facts (e.g. the story-context object) are exploded one
  // level so each nested value routes to its own renderer and group instead
  // of flattening into one unreadable mega-row.
  const entries: LedgerFieldEntry[] = facts.flatMap(([key, value]) => {
    if (isRecordValue(value)) {
      return Object.entries(value).map(([nestedKey, nestedValue]) => ({
        id: `${key}.${nestedKey}`,
        label: humanizeStateKey(nestedKey),
        labelKey: playerFacingFieldLabelKey(withField(nestedKey)),
        field: withField(nestedKey),
        value: nestedValue,
      }))
    }
    return [{ id: key, label: humanizeStateKey(key), labelKey: playerFacingFieldLabelKey(withField(key)), field: withField(key), value }]
  })
  return {
    id: WORLD_STATE_TAB,
    name: 'world',
    templateId: WORLD_STATE_TAB,
    groups: buildLedgerGroups(
      entries.some(entry => (entry.field?.path === 'world.situation' || entry.label === '世界局势')
        && typeof entry.value === 'string' && entry.value.trim())
        ? entries.filter(entry => !isLegacyWorldOverview(entry.field, entry.label))
        : entries,
      changes.filter(change => !change.actorId),
    ),
    traits: [],
  }
}

function ActorLedgerBody({ ledger, layout, panelMode, onPanelModeChange }: { ledger: StateLedgerPresentation; layout?: StoryStateTemplateLayout; panelMode: 'preview' | 'expanded'; onPanelModeChange: (mode: StoryStatePanelMode) => void }) {
  const { t } = useWorldPresentation()
  const groups = applyStoryStateLayout(ledger.groups, layout)

  return (
    <div>
      {ledger.traits.length > 0 ? <ActorTraits traits={ledger.traits} /> : null}
      {groups.length > 0
        ? <LedgerSections groups={groups} mode={panelMode} onModeChange={onPanelModeChange} />
        : <StateSectionEmpty label={t('storyStage.state.actorEmpty')} />}
    </div>
  )
}

function WorldLedgerBody({ ledger, layout, panelMode, onPanelModeChange }: { ledger: StateLedgerPresentation; layout?: StoryStateTemplateLayout; panelMode: 'preview' | 'expanded'; onPanelModeChange: (mode: StoryStatePanelMode) => void }) {
  const { t } = useWorldPresentation()
  const groups = applyStoryStateLayout(ledger.groups, layout)
  if (groups.length === 0) return <StateSectionEmpty label={t('storyStage.state.worldEmpty')} />
  return <LedgerSections groups={groups} mode={panelMode} onModeChange={onPanelModeChange} />
}

/**
 * LedgerSections lays groups out as visually distinct blocks on one page. In
 * preview mode only the first two ordered sections show, with a mode toggle that
 * reveals the rest without any height-clamped tricks.
 */
function LedgerSections({ groups, mode, onModeChange }: { groups: LedgerFieldGroup[]; mode: 'preview' | 'expanded'; onModeChange: (mode: StoryStatePanelMode) => void }) {
  const { t } = useWorldPresentation()
  const { preview, hidden } = useMemo(() => splitLedgerGroupsForPreview(groups), [groups])
  const expanded = mode === 'expanded'
  // Keep the already-visible preview sections anchored in place. Sections
  // revealed by the user's action append after them even when their schema
  // order originally placed them above the preview set.
  const decorated = groups.length > 1
  return (
    <div className="story-state-ledger__sections">
      {preview.map((group) => (
        <LedgerSectionBlock key={group.key} group={group} decorated={decorated} />
      ))}
      {hidden.length > 0 ? (
        // Offset the extra flex gap while closed; include group spacing in the animated height.
        <StateReveal open={expanded} className="-my-1">
          <div className="flex flex-col gap-2 py-1">
            {hidden.map((group) => <LedgerSectionBlock key={group.key} group={group} decorated={decorated} />)}
          </div>
        </StateReveal>
      ) : null}
      {!expanded && hidden.length > 0 ? (
        <button
          type="button"
          className="story-state-ledger__mode-toggle"
          onClick={() => onModeChange('expanded')}
        >
          <ChevronDown aria-hidden="true" className="size-3.5" />
          {t('storyStage.state.expandAll', { count: hidden.length })}
        </button>
      ) : null}
      {expanded && hidden.length > 0 ? (
        <button
          type="button"
          className="story-state-ledger__mode-toggle"
          onClick={() => onModeChange('preview')}
        >
          <ChevronUp aria-hidden="true" className="size-3.5" />
          {t('storyStage.state.collapseToPreview')}
        </button>
      ) : null}
    </div>
  )
}

/**
 * Animate intrinsic content with grid tracks so opening and closing share the same
 * path without temporarily expanding to measure an auto height in the virtualized list.
 */
function StateReveal({ open, children, className }: { open: boolean; children: ReactNode; className?: string }) {
  const reducedMotion = useReducedMotionConfig()
  return (
    <motion.div
      initial={false}
      animate={{ gridTemplateRows: open ? '1fr' : '0fr', opacity: open ? 1 : 0 }}
      transition={{ type: 'tween', duration: reducedMotion ? 0 : 0.16, ease: novaEase }}
      aria-hidden={!open}
      inert={!open}
      className={className}
      style={{ display: 'grid' }}
    >
      <div className="min-h-0 overflow-hidden">{children}</div>
    </motion.div>
  )
}

function LedgerSectionBlock({ group, decorated }: { group: LedgerFieldGroup; decorated: boolean }) {
  const { t } = useWorldPresentation()
  const label = group.labelKey ? t(group.labelKey) : group.custom ? group.key : t(`storyStage.state.group.${group.key}`)
  return (
    <section aria-label={label} data-decorated={decorated || undefined} className="story-state-ledger__section">
      {decorated ? (
        <header className="story-state-ledger__section-header">
          <LedgerGroupIcon group={group} />
          <h3 className="story-state-ledger__section-title">{label}</h3>
        </header>
      ) : null}
      <LedgerGroupGrid group={group} groupLabel={label} />
    </section>
  )
}

function LedgerGroupIcon({ group }: { group: LedgerFieldGroup }) {
  const className = 'story-state-ledger__section-icon'
  if (group.custom) return <Tag aria-hidden="true" className={className} />
  switch (group.key) {
    case 'overview':
      return <Gauge aria-hidden="true" className={className} />
    case 'holdings':
      return <Package aria-hidden="true" className={className} />
    case 'skills':
      return <Sparkles aria-hidden="true" className={className} />
    case 'methods':
      return <BookOpen aria-hidden="true" className={className} />
    case 'relations':
      return <Users aria-hidden="true" className={className} />
    case 'details':
      return <AlignLeft aria-hidden="true" className={className} />
    default:
      return <Tag aria-hidden="true" className={className} />
  }
}

function LedgerGroupGrid({ group, groupLabel }: { group: LedgerFieldGroup; groupLabel?: string }) {
  return (
    <div className="story-state-ledger__grid" data-group={group.custom ? 'custom' : group.key}>
      {group.fields.map((item) => <LedgerFieldView key={item.id} item={item} groupLabel={groupLabel} />)}
    </div>
  )
}

function ActorTraits({ traits }: { traits: ReturnType<typeof actorTraits> }) {
  return (
    <div className="flex min-w-0 flex-wrap gap-1 border-b border-[var(--nova-border-soft)] px-2.5 py-1.5">
      {traits.map((trait) => (
        <Badge
          key={`${trait.pool_id}:${trait.trait_id}`}
          variant="secondary"
          className="max-w-32 truncate"
        >
          {trait.name}
        </Badge>
      ))}
    </div>
  )
}

function StateSectionEmpty({ label }: { label: string }) {
  return (
    <Empty className="min-h-20">
      <EmptyHeader>
        <EmptyMedia variant="icon"><Sparkles /></EmptyMedia>
        <EmptyDescription>{label}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}

function turnStatusLabel(snapshot: Snapshot | null, t: ReturnType<typeof useWorldPresentation>['t']) {
  const turnId = snapshot?.current_turn?.id
  const turns = snapshot?.turns || []
  const matchedIndex = turnId ? turns.findIndex((turn) => turn.id === turnId) : -1
  const turn = matchedIndex >= 0 ? matchedIndex + 1 : Math.max(turns.length, turnId ? 1 : 0)
  if (snapshot?.current_turn?.state_status === 'pending') return t('storyStage.state.syncing', { turn })
  if (snapshot?.current_turn?.state_status === 'failed') return t('storyStage.state.failed', { turn })
  return t('storyStage.state.updatedTurn', { turn })
}

function isRecordValue(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
