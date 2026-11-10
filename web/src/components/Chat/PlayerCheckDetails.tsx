import { useTranslation } from 'react-i18next'
import type { PublicRuleRoll } from '@/lib/api-client/types'
import { builtinCheckLabelKey } from '@/features/interactive/builtin-display-labels'

export function PlayerCheckDetails({ evidence }: { evidence: PublicRuleRoll }) {
  const { t } = useTranslation()
  const prefix = 'snapshot.checkEvidence.rule.'
  const labelKey = evidence.label_key?.startsWith(prefix) ? builtinCheckLabelKey(evidence.label_key.slice(prefix.length), evidence.label) : undefined
  const signed = (value: number) => value > 0 ? `+${value}` : String(value)
  const outcome = ['success', 'critical_success', 'failure', 'critical_failure'].includes(evidence.outcome || '') ? evidence.outcome : 'other'
  return <details className="my-2 rounded-md border border-border text-xs" data-testid="player-check-details">
    <summary className="cursor-pointer px-3 py-2 font-medium">{t('snapshot.checkEvidence.title')}</summary>
    <div className="space-y-2 border-t px-3 py-2 text-muted-foreground">
      <p>{labelKey ? t(labelKey) : evidence.label || t('snapshot.ruleRoll.title')} · {t(`snapshot.checkEvidence.outcome.${outcome}`)}</p>
      <p>{t('snapshot.checkEvidence.rolls', { dice: evidence.dice || '', rolls: evidence.rolls?.join(', ') || t('snapshot.noRecord') })}</p>
      {evidence.total !== undefined ? <p>{t('snapshot.checkEvidence.total', { value: evidence.total })}</p> : null}
      {evidence.bonus_total !== undefined ? <p>{t('snapshot.checkEvidence.modifier', { value: signed(evidence.bonus_total) })}</p> : null}
      {evidence.state_changes?.map((change, index) => <p key={`${change.field_id}:${index}`}>{t('snapshot.checkEvidence.change', { field: change.field_id, value: signed(change.change) })}</p>)}
      <p className="text-[10px]">{t('snapshot.checkEvidence.saved')}</p>
    </div>
  </details>
}
