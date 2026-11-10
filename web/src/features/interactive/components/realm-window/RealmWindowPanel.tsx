import { useWorldPresentation } from '../../world-presentation-context'
import { useTranslation } from 'react-i18next'
import { X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import type { RealmInstance, RealmOption, RealmRewardOption } from '../../realm-types'
import { useRealm } from './use-realm'
import { RealmExploreBlock } from './RealmExploreBlock'
import { RealmCooperationBlock } from './RealmCooperationBlock'

interface RealmWindowPanelProps {
  storyId: string
  branchId: string
  /** True while the panel is on screen. It is what starts the read. */
  active: boolean
  /**
   * Collapses the panel. Closing is deliberately the only thing it does: opening
   * the entry must not move the character, so there is no action for it to call.
   */
  onClose: () => void
  className?: string
  /** Adds an editable intention to the main story; does not submit a turn. */
  onDraftAction?: (message: string) => void
  draftingDisabled?: boolean
}

/**
 * The realm as an in-story information panel: what this character already knows
 * about the realms they have entered, and which authored realm stands at their
 * door.
 *
 * It is read-only on purpose. Entering, walking a passage, taking a find and
 * leaving are all written in the main story's own input and settled by the
 * turn's world action, against the same rules this panel reads — so there is no
 * second path into a realm, and no button here can move the character while the
 * player is reading prose.
 *
 * Every room, passage and find on the panel is one the instance really has. It
 * never lists a hidden chamber, because the server projects only what this
 * character has discovered; it never names a passage the rules did not declare;
 * and it never shows a find as available once it has been taken.
 */
export function RealmWindowPanel({ storyId, branchId, active, onClose, className, onDraftAction, draftingDisabled }: RealmWindowPanelProps) {
  const { t } = useTranslation()
  const { terms } = useWorldPresentation()
  const entry = useRealm({ storyId, branchId, active })

  const data = entry.data
  const instances = data?.instances || []
  const available = data?.available || []

  return (
    <div data-testid="realm-panel" className={cn('flex h-full min-h-0 flex-col bg-[var(--nova-surface)] text-[var(--nova-text)]', className)}>
      <header className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2">
        <h2 className="min-w-0 flex-1 truncate text-[13px] font-medium">{t('realmWindow.title', terms)}</h2>
        <Button type="button" variant="ghost" size="icon-sm" onClick={onClose} aria-label={t('realmWindow.close', terms)} title={t('realmWindow.closeHint')}>
          <X className="h-4 w-4" />
        </Button>
      </header>

      {entry.error ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center">
          <p className="text-[12px] text-[var(--nova-danger)]">{t('realmWindow.loadFailed', terms)}</p>
          <p className="text-[11px] text-[var(--nova-text-faint)]">{entry.error}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void entry.reload()}>{t('realmWindow.retry')}</Button>
        </div>
      ) : entry.loading && !data ? (
        <div className="flex flex-1 items-center justify-center gap-2 text-[12px] text-[var(--nova-text-muted)]">
          <Spinner className="h-3.5 w-3.5" />
          {t('realmWindow.loading')}
        </div>
      ) : (
        <>
          <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-3 py-2">
            <PlaceBlock location={data?.location} />

            {instances.length === 0 && available.length === 0 ? (
              <p className="text-[11px] text-[var(--nova-text-faint)]">{t('realmWindow.unknownRealm', terms)}</p>
            ) : null}

            {(data?.cooperations || []).map((project) => (
              <RealmCooperationBlock key={`${storyId}:${branchId}:${project.project_id}:${project.instance_id}`} project={project} onDraftAction={onDraftAction} draftingDisabled={draftingDisabled} />
            ))}

            {instances.map((instance) => (
              <InstanceBlock key={instance.instance_id} instance={instance} clockSeconds={data?.clock_seconds} onDraftAction={onDraftAction} draftingDisabled={draftingDisabled} />
            ))}

            {available.length > 0 ? <AvailableBlock options={available} onDraftAction={onDraftAction} draftingDisabled={draftingDisabled} /> : null}
          </div>

          <footer className="shrink-0 border-t border-[var(--nova-border)] px-3 py-2">
            <p className="text-[11px] text-[var(--nova-text-faint)]">{t(onDraftAction ? 'realmWindow.explore.draftHint' : 'realmWindow.hint')}</p>
          </footer>
        </>
      )}
    </div>
  )
}

/** Where the character stands, in the scene's own committed wording. */
function PlaceBlock({ location }: { location?: string }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-between rounded border border-[var(--nova-border)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
      <span>{t('realmWindow.currentPlace')}</span>
      <span className="min-w-0 truncate text-[var(--nova-text)]">{location || t('realmWindow.unknownPlace')}</span>
    </div>
  )
}

/**
 * One realm this branch has entered, as this character knows it.
 *
 * The room they stand in, the rooms and passages they have discovered, the goals
 * they were given and the finds they have taken are all reported. Nothing here
 * is an action: the prose decides when the character walks a passage or picks
 * something up, and the turn settles it.
 */
function InstanceBlock({ instance, clockSeconds, onDraftAction, draftingDisabled }: {
  instance: RealmInstance
  clockSeconds?: number
  onDraftAction?: (message: string) => void
  draftingDisabled?: boolean
}) {
  const { t } = useTranslation()
  const { terms } = useWorldPresentation()
  const view = instance.view
  const name = instance.name_key ? t(instance.name_key, { defaultValue: instance.name }) : instance.name
  const areaName = (area: typeof view.areas[number]) => area.name_key ? t(area.name_key, { defaultValue: area.name }) : area.name
  const current = view.current_area
  const areaNames = new Map(view.areas.map((area) => [area.id, areaName(area)]))
  const completed = new Set(view.completed_objectives)
  const rewards = instance.rewards || []
  const granted = instance.granted || []

  return (
    <section className="flex flex-col gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5">
      <div className="flex items-baseline gap-2">
        <span className="min-w-0 flex-1 truncate text-[12px] text-[var(--nova-text)]">{name}</span>
        <span className="shrink-0 text-[11px] text-[var(--nova-text-muted)]">
          {instance.presence === 'inside' ? t('realmWindow.inside', terms) : t('realmWindow.left')}
        </span>
      </div>
      {current ? (
        <>
          <span className="text-[12px] text-[var(--nova-text)]">{areaName(current)}</span>
          {current.description ? (
            <span className="text-[11px] text-[var(--nova-text-muted)]">{current.description_key ? t(current.description_key, { defaultValue: current.description }) : current.description}</span>
          ) : null}
        </>
      ) : null}

      {instance.explore ? (
        <RealmExploreBlock explore={instance.explore} realmName={name} inside={instance.presence === 'inside'} clockSeconds={clockSeconds} onDraftAction={onDraftAction} draftingDisabled={draftingDisabled} />
      ) : null}

      {instance.explore && instance.presence === 'inside' && (instance.ways || []).length > 0 ? (
        <div className="my-2 flex flex-col gap-1.5">
          <h3 className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.explore.currentWays')}</h3>
          {(instance.ways || []).map((way) => {
            const destination = way.destination_name_key ? t(way.destination_name_key, { defaultValue: way.destination_name }) : way.destination_name
            return onDraftAction ? <Button key={way.connection_id} type="button" variant="outline" size="sm" className="h-auto min-h-8 justify-start whitespace-normal text-left" disabled={draftingDisabled}
              onClick={() => onDraftAction(t('realmWindow.explore.draftMove', { realm: name, destination }))}>{t('realmWindow.explore.stageMove', { destination })}</Button>
              : <p key={way.connection_id} className="text-[12px]">{destination}</p>
          })}
        </div>
      ) : null}

      {view.confirmed_facts.some((fact) => fact.publicly_displayable) ? (
        <div className="mt-2 flex flex-col gap-1">
          <h3 className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.explore.knownFacts')}</h3>
          {view.confirmed_facts.filter((fact) => fact.publicly_displayable).map((fact) => (
            <p key={fact.id} className="break-words text-[12px] leading-relaxed">{fact.text_key ? t(fact.text_key, { defaultValue: fact.text }) : fact.text}</p>
          ))}
        </div>
      ) : null}

      {view.objectives.length > 0 ? (
        <div className="flex flex-col gap-0.5">
          <span className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.objectives')}</span>
          {view.objectives.map((objective) => (
            <p key={objective.id} className="text-[11px] text-[var(--nova-text)]">
              {completed.has(objective.id) ? '✓ ' : '· '}
              {objective.description_key ? t(objective.description_key, { defaultValue: objective.description }) : objective.description}
            </p>
          ))}
        </div>
      ) : null}

      <div className="flex flex-col gap-0.5">
        <span className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.knownAreas')}</span>
        {view.areas.length === 0 ? (
          <p className="text-[11px] text-[var(--nova-text-faint)]">{t('realmWindow.noPassages')}</p>
        ) : (
          <p className="text-[11px] text-[var(--nova-text)]">
            {view.areas.map((area) => areaNames.get(area.id) || area.id).join(' · ')}
          </p>
        )}
      </div>

      {view.connections.length > 0 ? (
        <div className="flex flex-col gap-0.5">
          <span className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.passages')}</span>
          {view.connections.map((connection) => (
            <p key={connection.id} className="text-[11px] text-[var(--nova-text)]">
              {areaNames.get(connection.from_area_id) || connection.from_area_id}
              {' → '}
              {areaNames.get(connection.to_area_id) || connection.to_area_id}
              {connection.bidirectional ? '' : ` (${t('realmWindow.oneWay')})`}
            </p>
          ))}
        </div>
      ) : null}

      {rewards.length > 0 ? (
        <div className="flex flex-col gap-0.5">
          <span className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.findings')}</span>
          {rewards.map((reward: RealmRewardOption) => (
            <p key={reward.content_ref_id} className="text-[11px] text-[var(--nova-text)]">
              {reward.name_key ? t(reward.name_key, { defaultValue: reward.name || t('realmWindow.explore.unknownResource') }) : reward.name || t('realmWindow.explore.unknownResource')} × {reward.quantity}
            </p>
          ))}
        </div>
      ) : null}

      {onDraftAction && instance.explore ? (
        <Button type="button" variant="outline" size="sm" className="my-2 h-auto min-h-8 whitespace-normal" disabled={draftingDisabled}
          onClick={() => onDraftAction(t(instance.presence === 'inside' ? 'realmWindow.explore.draftExit' : 'realmWindow.explore.draftReenter', { realm: name }))}>
          {t(instance.presence === 'inside' ? 'realmWindow.explore.stageExit' : 'realmWindow.explore.stageReenter')}
        </Button>
      ) : null}

      {granted.length > 0 ? (
        <div className="flex flex-col gap-0.5">
          <span className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.taken')}</span>
          {granted.map((reward) => (
            <p key={reward.content_ref_id} className="text-[11px] text-[var(--nova-text-faint)]">
              {t('realmWindow.alreadyTaken', { item: reward.name_key ? t(reward.name_key, { defaultValue: reward.name || t('realmWindow.explore.unknownResource') }) : reward.name || t('realmWindow.explore.unknownResource') })}
            </p>
          ))}
        </div>
      ) : null}
    </section>
  )
}

/** The authored realms that stand at this character's door. */
function AvailableBlock({ options, onDraftAction, draftingDisabled }: {
  options: RealmOption[]
  onDraftAction?: (message: string) => void
  draftingDisabled?: boolean
}) {
  const { t } = useTranslation()
  const { terms } = useWorldPresentation()
  return (
    <section className="flex flex-col gap-1">
      <span className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.available', terms)}</span>
      {options.map((option) => (
        <div key={option.realm_id} className="flex flex-col gap-0.5 rounded border border-[var(--nova-border)] px-2 py-1 text-[11px]">
          <span className="truncate text-[var(--nova-text)]">{option.name_key ? t(option.name_key, { defaultValue: option.name }) : option.name}</span>
          <span className="text-[var(--nova-text-muted)]">{option.description_key ? t(option.description_key, { defaultValue: option.theme }) : option.theme}</span>
          {onDraftAction ? <Button type="button" variant="outline" size="sm" className="mt-1 h-auto min-h-8 whitespace-normal" disabled={draftingDisabled} onClick={() => onDraftAction(t('realmWindow.explore.draftEnter', { realm: option.name_key ? t(option.name_key, { defaultValue: option.name }) : option.name }))}>{t('realmWindow.explore.stageEnter')}</Button> : null}
        </div>
      ))}
    </section>
  )
}
