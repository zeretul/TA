import { useMemo } from 'react'
import { Activity } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import type { Snapshot } from '../../types'
import { ChangesSummary } from '../story-state/ChangesSummary'
import { buildStoryStateModel, type ActorStateEntry } from '../story-state/model'
import { StateDetailsDialog } from './StateDetailsDialog'

interface OverviewViewProps {
  snapshot: Snapshot | null
  stateError?: string
}

/**
 * What just changed in this story. The branch plan is deliberately absent: it is
 * the story's own outline of what has not happened yet, and showing it here
 * would put a future the player has not lived through on the same page as what
 * actually did.
 */
export function OverviewView({ snapshot, stateError }: OverviewViewProps) {
  const { t } = useTranslation()
  const model = useMemo(() => buildStoryStateModel(snapshot), [snapshot])
  const actors = useMemo<ActorStateEntry[]>(() => [
    ...model.actors,
    ...model.archivedActors.map((entry): ActorStateEntry => [entry.actorId, { name: entry.name, template_id: entry.templateId }]),
  ], [model.actors, model.archivedActors])
  const error = snapshot?.current_turn?.state_error || stateError

  return (
    <div className="director-console__scroll h-full min-h-0 overflow-y-auto px-3 py-3">
      <div className="flex flex-col gap-3">
        {error ? <InlineErrorNotice message={error} /> : null}
        <section className="story-state-ledger overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--director-panel)]">
          {model.hasState ? (
            <ChangesSummary changes={model.changes} actors={actors} schema={snapshot?.actor_state_schema} turnCount={snapshot?.turn_count} standalone />
          ) : (
            <div className="flex items-center gap-2 px-3 py-2.5 text-[10px] text-[var(--nova-text-faint)]">
              <Activity className="size-3.5" />
              <span>{t('directorPanel.stateDeltaEmpty')}</span>
            </div>
          )}
        </section>
        <StateDetailsDialog snapshot={snapshot} stateError={stateError} />
      </div>
    </div>
  )
}
