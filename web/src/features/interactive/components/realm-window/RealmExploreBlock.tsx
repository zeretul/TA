import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { RealmExplore, RealmExploreCost } from '../../realm-types'

const blockedKeys: Record<string, string> = {
  used: 'realmWindow.explore.blocked.used',
  setback: 'realmWindow.explore.blocked.setback',
  choice_made: 'realmWindow.explore.blocked.choice',
  damaged: 'realmWindow.explore.blocked.damaged',
  depleted: 'realmWindow.explore.blocked.depleted',
  return_pending: 'realmWindow.explore.blocked.return',
  capability: 'realmWindow.explore.blocked.capability',
  cannot_act: 'realmWindow.explore.blocked.cannotAct',
  cooperation_required: 'realmWindow.explore.blocked.cooperation',
}

/** A projected choice is a draft, never a parallel settlement endpoint. */
export function RealmExploreBlock({ explore, realmName, inside, clockSeconds, onDraftAction, draftingDisabled }: {
  explore: RealmExplore
  realmName: string
  inside: boolean
  clockSeconds?: number
  onDraftAction?: (message: string) => void
  draftingDisabled?: boolean
}) {
  const { t } = useTranslation()
  const label = t(explore.pressure_label_key, { defaultValue: explore.pressure_label })
  const duration = (seconds: number) => seconds % 60 === 0
    ? t('realmWindow.explore.minutes', { count: seconds / 60 })
    : t('realmWindow.explore.seconds', { count: seconds })
  const costText = (cost: RealmExploreCost) => {
    // An unknown reference is not a player-facing name or evidence of ownership.
    const name = cost.name_key ? t(cost.name_key, { defaultValue: cost.name || t('realmWindow.explore.unknownResource') })
      : cost.name || t('realmWindow.explore.unknownResource')
    const key = cost.kind === 'consume_item' ? 'realmWindow.explore.consume'
      : cost.kind === 'require_item' ? 'realmWindow.explore.require' : 'realmWindow.explore.skill'
    const base = t(key, { name, count: cost.quantity })
    return cost.kind === 'use_skill' && cost.energy_cost != null
      ? `${base} · ${t('realmWindow.explore.energy', { count: cost.energy_cost })}` : base
  }
  const remaining = clockSeconds == null || explore.return_due_seconds == null ? null
    : Math.max(0, explore.return_due_seconds - clockSeconds)
  // Recovery should not be buried below a screenful of exhausted choices.
  // Keep the authored order within each group, and retain every blocked reason.
  const approaches = [...explore.approaches].sort((a, b) => Number(b.available) - Number(a.available))

  return (
    <div className="mt-2 flex min-w-0 flex-col gap-3 border-t border-[var(--nova-border)] pt-2" data-testid="realm-explore">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 text-[12px]">
        <h3 className="font-medium">{t('realmWindow.explore.title')}</h3>
        <p>{label} <strong className="tabular-nums">{explore.pressure} / {explore.pressure_limit}</strong></p>
      </div>
      <p className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.explore.pressureHint')}</p>
      {explore.setback ? <p className="text-[12px] text-[var(--nova-danger)]">{t('realmWindow.explore.setback')}</p> : null}
      {Boolean(explore.return_due_seconds) ? (
        <p className="text-[12px] text-[var(--nova-text-muted)]">
          {explore.setback ? t('realmWindow.explore.returnLost') : explore.return_opened
            ? t('realmWindow.explore.returnOpened') : explore.return_available ? t('realmWindow.explore.returnReady') : remaining == null
            ? t('realmWindow.explore.returnPending')
            : t('realmWindow.explore.returnWait', { duration: duration(remaining) })}
        </p>
      ) : null}
      {!inside ? <p className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.explore.outside')}</p> : null}
      {approaches.map((approach) => {
        const name = t(approach.name_key, { defaultValue: approach.name })
        return (
          <section key={approach.id} className="flex min-w-0 flex-col gap-1.5 rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2">
            <h4 className="break-words text-[12px] font-medium">{name}</h4>
            <p className="break-words text-[12px] leading-relaxed text-[var(--nova-text-muted)]">{t(approach.description_key, { defaultValue: approach.description })}</p>
            <p className="text-[11px] text-[var(--nova-text-muted)]">
              {t('realmWindow.explore.timeCost', { duration: duration(approach.duration_seconds) })}
              {' · '}{t('realmWindow.explore.pressureChange', { count: approach.pressure_delta })}
            </p>
            {(approach.costs || []).length > 0 ? (
              <ul className="space-y-0.5 text-[11px] text-[var(--nova-text-muted)]">
                {approach.costs.map((cost, index) => <li key={`${cost.kind}-${cost.ref_id}-${index}`}>{costText(cost)}</li>)}
              </ul>
            ) : <p className="text-[11px] text-[var(--nova-text-faint)]">{t('realmWindow.explore.noCost')}</p>}
            {approach.causes_setback && !explore.setback ? <p className="text-[12px] font-medium text-[var(--nova-danger)]">{t('realmWindow.explore.willSetback')}</p> : null}
            {!approach.available ? <p className="text-[11px] text-[var(--nova-text-muted)]">{t(blockedKeys[approach.blocked_reason || ''] || 'realmWindow.explore.blocked.unavailable')}</p> : null}
            {onDraftAction ? (
              <Button type="button" size="sm" variant="outline" className="mt-1 h-auto min-h-8 justify-start whitespace-normal text-left text-[12px]" disabled={!inside || !approach.available || draftingDisabled}
                onClick={() => onDraftAction(t('realmWindow.explore.draftApproach', { realm: realmName, approach: name }))}>
                {t('realmWindow.explore.stage', { action: name })}
              </Button>
            ) : null}
          </section>
        )
      })}
    </div>
  )
}
