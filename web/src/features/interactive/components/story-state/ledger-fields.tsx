import { useState } from 'react'
import { useWorldPresentation } from '../../world-presentation-context'
import type { TFunction } from 'i18next'
import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'
import type { ActorStateField } from '../../types'
import { realmLabel } from './realm-label'
import type { ClassifiedStateChange } from './changes'
import type { LedgerFieldItem } from './model'
import type { StateInjuryReport } from './injuries'
import type { MethodReport } from './methods'
import type { SkillReport } from './skills'
import { humanizeStateKey } from './model'
import { resolveNumberMeter } from './number-meter'
import { playerVisibleEntries, stateEntryLabel } from './state-entry-display'
import { FOUNDATION_FIELD_NAME, foundationDisplayText } from './foundation-display'

/** Long text beyond this length is clamped with an inline expand toggle. */
const BLOCK_CLAMP_LENGTH = 300
const OBJECT_ENTRY_LIMIT = 20
const OBJECT_ARRAY_LIMIT = 8
const MAX_OBJECT_DEPTH = 3

/**
 * LedgerFieldView renders one state field with the renderer resolved by
 * field-layout. All renderers share the same label typography and the
 * left change accent so a group grid stays visually aligned.
 *
 * `groupLabel` is the heading the row sits under. A caption that repeats it
 * would read as two rows for one thing — 功法 under 功法, 已学功法 under 技能 —
 * so the heading carries the word and the row drops its own caption. The change
 * marker stays, because that is about this turn rather than about what the
 * field is.
 */
export function LedgerFieldView({ item, groupLabel }: { item: LedgerFieldItem; groupLabel?: string }) {
  const { t, terms, family } = useWorldPresentation()
  // A pack's field name is already the word the player reads. The wound row is
  // one this interface invented, so its caption comes from the translation.
  const label = family === 'cultivation' && item.field?.name === '灵力' ? terms.energy
    : family === 'cultivation' && item.field?.name === '灵力上限' ? t('worldPresentation.energyMaximum', { energy: terms.energy })
    : item.labelKey ? t(item.labelKey) : item.label
  const { value, renderer, change, withheld } = item
  const showLabel = !groupLabel || label !== groupLabel
  if (withheld) {
    return (
      <section
        aria-label={label}
        data-ledger-field
        data-renderer="unknown"
        data-withheld="true"
        className="story-state-ledger__field"
      >
        <div className="mb-0.5 flex min-w-0 items-baseline justify-between gap-2">
          <FieldLabel label={label} />
        </div>
        <UnknownFieldBody />
      </section>
    )
  }
  return (
    <section
      aria-label={label}
      data-ledger-field
      data-renderer={renderer}
      data-change-tone={change ? change.tone : undefined}
      className="story-state-ledger__field"
    >
      {renderer === 'stat' ? (
        <StatFieldBody item={item} />
      ) : (
        <>
          {showLabel || change ? (
            <div className="mb-0.5 flex min-w-0 items-baseline justify-between gap-2">
              {showLabel ? <FieldLabel label={label} /> : <span />}
              <FieldChangeChip change={change} />
            </div>
          ) : null}
          {renderer === 'methods' ? <MethodFieldBody report={item.methods} /> : null}
          {renderer === 'skills' ? <SkillFieldBody report={item.skills} /> : null}
          {renderer === 'inline' ? <InlineFieldBody value={value} field={item.field} /> : null}
          {renderer === 'block' ? <BlockFieldBody value={value} field={item.field} /> : null}
          {renderer === 'list' ? <ListFieldBody value={value} /> : null}
          {renderer === 'object' ? <ObjectFieldBody value={value} /> : null}
          {renderer === 'injuries' ? <InjuryFieldBody report={item.injuries} /> : null}
        </>
      )}
    </section>
  )
}

/**
 * A field the player has not found out about reads as 未知, not as an empty
 * slot. §3.1 keeps that apart from "no value": an opponent whose dossier is
 * withheld is not an opponent who carries nothing.
 */
function UnknownFieldBody() {
  const { t } = useWorldPresentation()
  return <span className="text-xs text-[var(--nova-text-faint)]">{t('storyStage.state.valueUnknown')}</span>
}

function FieldLabel({ label }: { label: string }) {
  return <h4 className="story-state-ledger__field-label">{label}</h4>
}

function StatFieldBody({ item }: { item: LedgerFieldItem }) {
  const { t, terms, family } = useWorldPresentation()
  const label = family === 'cultivation' && item.field?.name === '灵力' ? terms.energy
    : family === 'cultivation' && item.field?.name === '灵力上限' ? t('worldPresentation.energyMaximum', { energy: terms.energy })
    : item.labelKey ? t(item.labelKey) : item.label
  const field = item.field
  const value = typeof item.value === 'number' ? item.value : 0
  const min = field?.min ?? 0
  const meter = field ? resolveNumberMeter(field, value, item.resolveSibling) : null
  const max = meter?.max ?? field?.max ?? 100
  const signed = meter?.zeroPercent !== undefined
  const valueLabel = !meter || signed ? formatLedgerNumber(value) : `${formatLedgerNumber(value)} / ${formatLedgerNumber(max)}`
  return (
    <div>
      <div className="mb-1 flex min-w-0 items-baseline justify-between gap-2">
        <FieldLabel label={label} />
        <span className="flex shrink-0 items-baseline gap-1.5">
          <FieldChangeChip change={item.change} />
          <span className="font-mono text-[11px] font-semibold tabular-nums text-[var(--nova-text)]">{valueLabel}</span>
        </span>
      </div>
      {!meter ? null : signed ? (
        <div
          role="meter"
          aria-label={t('storyStage.state.metricProgress', {
            label,
            value: formatLedgerNumber(value),
            min: formatLedgerNumber(min),
            max: formatLedgerNumber(max),
          })}
          aria-valuemin={min}
          aria-valuemax={max}
          aria-valuenow={value}
          aria-valuetext={valueLabel}
          className="relative h-1.5 overflow-hidden rounded-full bg-[var(--nova-surface-3)]"
        >
          <span className="absolute inset-y-0 w-px bg-[var(--nova-text-faint)]" style={{ left: `${meter.zeroPercent}%` }} aria-hidden="true" />
          <span
            className={cn('absolute inset-y-0 rounded-full transition-[left,width] duration-300 motion-reduce:transition-none', meter.tone === 'negative' ? 'bg-[var(--story-state-negative)]' : 'bg-[var(--story-state-positive)]')}
            style={{ left: `${meter.startPercent}%`, width: `${meter.widthPercent}%` }}
            aria-hidden="true"
          />
        </div>
      ) : (
        <Progress
          value={meter.widthPercent}
          aria-label={t('storyStage.state.metricProgress', {
            label,
            value: formatLedgerNumber(value),
            min: formatLedgerNumber(min),
            max: formatLedgerNumber(max),
          })}
          aria-valuetext={valueLabel}
          className="story-state-ledger__metric-progress h-1.5"
        />
      )}
    </div>
  )
}

/**
 * InjuryFieldBody draws the one row that stands for a character's wounds.
 *
 * The row reads as a sentence, not as a parameter. A character with nothing
 * wrong says 无伤; one carrying a wound lists what the story wrote about it —
 * the name, what it stops them doing, and how it heals. The number the rules
 * hold the burden above never appears, because it means nothing outside the
 * recovery rules and the player is not the one applying them.
 */
function InjuryFieldBody({ report }: { report?: StateInjuryReport }) {
  const { t } = useWorldPresentation()
  if (!report) return null
  if (!report.wounded) {
    return <span className="text-xs text-[var(--nova-text-faint)]">{t('storyStage.state.injury.none')}</span>
  }
  if (report.lines.length === 0) {
    // A burden floor above zero with nothing written about it. The wound is
    // real — the rules hold the burden there — but the story has not said what
    // it is yet, and inventing a name for it would be worse than saying so.
    return <span className="text-xs text-[var(--nova-text)]">{t('storyStage.state.injury.unspecified')}</span>
  }
  return (
    <ul className="story-state-ledger__item-list">
      {report.lines.map((line, index) => (
        <li key={index} className="story-state-ledger__item-row">{line}</li>
      ))}
    </ul>
  )
}

/**
 * SkillFieldBody draws the one list that holds everything a character can do.
 *
 * §T20 merges the rules' technique record with the pack's free-form record of
 * knacks into one 技能 list, and asks that the two stay distinguishable. The tag
 * is that distinction: 招式 is a registered definition a cast is priced from,
 * 本领 is something the story says the character can do with no definition
 * behind it. The list never turns one into the other — a knack gains no combat
 * numbers, and a technique loses none.
 */
function SkillFieldBody({ report }: { report?: SkillReport }) {
  const { t } = useWorldPresentation()
  if (!report) return null
  if (report.withheld) {
    return <span className="text-xs text-[var(--nova-text)]">{t('storyStage.state.valueUnknown')}</span>
  }
  if (report.lines.length === 0) {
    return <span className="text-xs text-[var(--nova-text-faint)]">{t('storyStage.state.skill.none')}</span>
  }
  return (
    <ul className="story-state-ledger__item-list">
      {report.lines.map((line, index) => (
        <li key={`${line.kind}:${line.name}:${index}`} className="story-state-ledger__item-row">
          <span className="story-state-ledger__method-name">{line.name}</span>
          <span className={cn('story-state-ledger__method-kind', `story-state-ledger__skill-kind--${line.kind}`)}>
            {t(`storyStage.state.skill.kind.${line.kind}`)}
          </span>
          {line.kind === 'learning' && line.learningMinutes !== undefined ? <span className="story-state-ledger__method-note">{t('storyStage.state.studyMinutes', { minutes: line.learningMinutes })}</span> : null}
          {line.description ? <span className="story-state-ledger__method-note">{line.description}</span> : null}
        </li>
      ))}
    </ul>
  )
}

/**
 * MethodFieldBody draws the one row that stands for a character's methods.
 *
 * The design keeps 功法 as its own section rather than folding it into 技能: a
 * method is what a character cultivates, not another thing they can perform.
 * What the player needs from it is the direction — which method is being
 * cultivated right now (主修), which ones are held but not circulating (兼修),
 * and which are still being learned. The definition ID the settlement prices
 * the method from is rule bookkeeping and stays out of the row.
 */
function MethodFieldBody({ report }: { report?: MethodReport }) {
  const { t } = useWorldPresentation()
  if (!report) return null
  if (report.lines.length === 0) {
    return <span className="text-xs text-[var(--nova-text-faint)]">{t('storyStage.state.method.none')}</span>
  }
  return (
    <ul className="story-state-ledger__item-list">
      {report.lines.map((line, index) => (
        <li key={index} className="story-state-ledger__item-row">
          <span className="story-state-ledger__method-name">{line.name}</span>
          <span className={cn('story-state-ledger__method-kind', `story-state-ledger__method-kind--${line.kind}`)}>
            {t(`storyStage.state.method.kind.${line.kind}`)}
          </span>
          {line.kind === 'learning' && line.learningMinutes !== undefined ? <span className="story-state-ledger__method-note">{t('storyStage.state.studyMinutes', { minutes: line.learningMinutes })}</span> : null}
          {line.description ? <span className="story-state-ledger__method-note">{line.description}</span> : null}
        </li>
      ))}
    </ul>
  )
}

function InlineFieldBody({ value, field }: { value: unknown; field?: ActorStateField }) {
  const { t } = useWorldPresentation()
  if (value === null || value === undefined || value === '') {
    return <span className="text-xs text-[var(--nova-text-faint)]">—</span>
  }
  if (typeof value === 'boolean') {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs text-[var(--nova-text)]">
        <span className={cn('size-1.5 rounded-full', value ? 'bg-[var(--nova-success)]' : 'bg-[var(--nova-text-faint)]')} />
        {value ? t('directorPanel.stateValue.yes') : t('directorPanel.stateValue.no')}
      </span>
    )
  }
  if (typeof value === 'number') {
    return <span className="font-mono text-xs tabular-nums text-[var(--nova-text)]">{formatLedgerNumber(value)}</span>
  }
  return <span className="break-words text-xs leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{formatFieldValue(value, field, t)}</span>
}

/**
 * Renders a stored value in the wording a player reads.
 *
 * A field that declares `value_format: stage` holds a rules key rather than
 * prose, so "mortal" reads as 凡人 and "qi_6" as 炼气六层. The journal keeps the
 * key; only the display changes, and a key this interface does not recognise is
 * shown as itself rather than guessed into a realm.
 *
 * The aptitude line is written once at creation and then settled, so a
 * character made before the wording changed still carries the old phrasing.
 * It is rewritten here rather than in storage: the cultivation rules read the
 * tier word out of the stored value, and this must not touch it.
 */
function formatFieldValue(value: unknown, field: ActorStateField | undefined, t: TFunction): string {
  const text = String(value)
  if (field?.value_format === 'stage') return realmLabel(text, t) || text
  if (field?.name === FOUNDATION_FIELD_NAME) return foundationDisplayText(text)
  return text
}

function BlockFieldBody({ value, field }: { value: unknown; field?: ActorStateField }) {
  const { t } = useWorldPresentation()
  const [expanded, setExpanded] = useState(false)
  const text = typeof value === 'string' ? formatFieldValue(value, field, t) : value == null ? '' : String(value)
  if (!text.trim()) return <span className="text-xs text-[var(--nova-text-faint)]">—</span>
  const clamped = text.length > BLOCK_CLAMP_LENGTH && !expanded
  return (
    <div>
      <p className={cn('story-state-ledger__block-text', clamped && 'story-state-ledger__block-text--clamped')}>{text}</p>
      {text.length > BLOCK_CLAMP_LENGTH ? (
        <button
          type="button"
          className="mt-0.5 text-[11px] text-[var(--nova-text-faint)] transition-colors hover:text-[var(--nova-text)]"
          onClick={() => setExpanded((next) => !next)}
        >
          {expanded ? t('storyStage.state.collapseField') : t('storyStage.state.expandField')}
        </button>
      ) : null}
    </div>
  )
}

function ListFieldBody({ value }: { value: unknown }) {
  const { t } = useWorldPresentation()
  if (!Array.isArray(value) || value.length === 0) {
    return <span className="text-xs text-[var(--nova-text-faint)]">{t('directorPanel.stateValue.empty')}</span>
  }
  return (
    <ul className="story-state-ledger__item-list">
      {value.map((item, index) => (
        <li key={index} className="story-state-ledger__item-row">
          {item === null
            ? '—'
            : isRecord(item)
              ? <ObjectEntryList value={item} />
              : String(item)}
        </li>
      ))}
    </ul>
  )
}

function ObjectFieldBody({ value }: { value: unknown }) {
  const { t } = useWorldPresentation()
  if (Array.isArray(value)) {
    const items = value.filter((item) => isRecord(item))
    if (items.length === 0) return <span className="text-xs text-[var(--nova-text-faint)]">{t('directorPanel.stateValue.empty')}</span>
    return (
      <div className="flex flex-col gap-2">
        {items.slice(0, 8).map((item, index) => (
          <ObjectEntryList key={index} value={item} />
        ))}
      </div>
    )
  }
  if (!isRecord(value) || Object.keys(value).length === 0) {
    return <span className="text-xs text-[var(--nova-text-faint)]">{t('directorPanel.stateValue.empty')}</span>
  }
  return <ObjectEntryList value={value} />
}

function ObjectEntryList({ value }: { value: Record<string, unknown> }) {
  return <NestedObjectList value={value} depth={0} />
}

function ObjectValue({ value, depth = 0 }: { value: unknown; depth?: number }) {
  if (value === null || value === undefined || value === '') return <span className="text-[var(--nova-text-faint)]">—</span>
  if (typeof value === 'boolean') return <span>{value ? '✓' : '—'}</span>
  if (typeof value === 'number') return <span className="font-mono tabular-nums">{formatLedgerNumber(value)}</span>
  if (typeof value === 'string') return <span className="break-words [overflow-wrap:anywhere]">{value}</span>
  if (Array.isArray(value)) {
    if (value.length === 0) return <span className="text-[var(--nova-text-faint)]">—</span>
    return (
      <ul className="story-state-ledger__object-value-list">
        {value.slice(0, OBJECT_ARRAY_LIMIT).map((item, index) => (
          <li key={index} className="story-state-ledger__object-value-item">
            <ObjectValue value={item} depth={depth + 1} />
          </li>
        ))}
      </ul>
    )
  }
  if (isRecord(value)) {
    if (depth >= MAX_OBJECT_DEPTH) return <span className="text-[var(--nova-text-faint)]">…</span>
    return <NestedObjectList value={value} depth={depth} />
  }
  return <span>{String(value)}</span>
}

function NestedObjectList({ value, depth }: { value: Record<string, unknown>; depth: number }) {
  const { t } = useWorldPresentation()
  const entries = playerVisibleEntries(value)
  if (entries.length === 0) return <span className="text-[var(--nova-text-faint)]">—</span>
  return (
    <ul className={cn('story-state-ledger__nested-object-list', depth > 0 && 'story-state-ledger__nested-object-list--nested')} data-depth={depth}>
      {entries.slice(0, OBJECT_ENTRY_LIMIT).map(([key, item]) => {
        const structured = Array.isArray(item) || isRecord(item)
        const label = humanizeStateKey(stateEntryLabel(key, item, t))
        return (
          <li key={key} className={cn('story-state-ledger__nested-object-item', structured && 'story-state-ledger__nested-object-item--branch')}>
            <span className="story-state-ledger__nested-object-key">{label}:</span>
            <div className="story-state-ledger__nested-object-value">
              <ObjectValue value={item} depth={depth + 1} />
            </div>
          </li>
        )
      })}
    </ul>
  )
}

/** FieldChangeChip renders the compact per-field turn-change marker. */
function FieldChangeChip({ change }: { change: ClassifiedStateChange | null }) {
  const { t } = useWorldPresentation()
  if (!change) return null
  if (change.kind === 'delta' && change.delta !== null) {
    return (
      <span className={cn('story-state-ledger__change-chip', `story-state-ledger__change-chip--${change.tone}`)}>
        {change.delta >= 0 ? `+${formatLedgerNumber(change.delta)}` : formatLedgerNumber(change.delta)}
      </span>
    )
  }
  if (change.kind === 'added' || change.kind === 'removed') {
    const sign = change.kind === 'added' ? '+' : '−'
    const text = change.text ? truncateEnd(change.text, 16) : t('storyStage.state.change.oneItem')
    return (
      <span className={cn('story-state-ledger__change-chip', `story-state-ledger__change-chip--${change.tone}`)}>
        {sign}{text}
      </span>
    )
  }
  return (
    <span className="story-state-ledger__change-chip story-state-ledger__change-chip--neutral">
      {change.kind === 'cleared' ? t('storyStage.state.change.cleared') : t('storyStage.state.change.updated')}
    </span>
  )
}

function formatLedgerNumber(value: number) {
  return Number.isInteger(value) ? String(value) : String(Number(value.toFixed(2)))
}

function truncateEnd(text: string, max: number) {
  return text.length > max ? `${text.slice(0, max)}…` : text
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
