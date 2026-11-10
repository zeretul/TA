import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { RealmCooperation } from '../../realm-types'

const phaseKeys: Record<RealmCooperation['phase'], string> = {
  none: 'realmWindow.cooperation.phase.none',
  agreed: 'realmWindow.cooperation.phase.agreed',
  worked: 'realmWindow.cooperation.phase.worked',
  used: 'realmWindow.cooperation.phase.used',
  reported: 'realmWindow.cooperation.phase.reported',
}
const blockedKeys: Record<string, string> = {
  not_here: 'realmWindow.cooperation.blocked.notHere',
  not_present: 'realmWindow.cooperation.blocked.notPresent',
  cannot_act: 'realmWindow.cooperation.blocked.cannotAct',
  partner_unavailable: 'realmWindow.cooperation.blocked.partnerUnavailable',
  missing_material: 'realmWindow.cooperation.blocked.missingMaterial',
  partner_energy: 'realmWindow.cooperation.blocked.partnerEnergy',
  not_agreed: 'realmWindow.cooperation.blocked.notAgreed',
  already_worked: 'realmWindow.cooperation.blocked.alreadyWorked',
  ingress_taken: 'realmWindow.cooperation.blocked.ingressTaken',
  setback: 'realmWindow.cooperation.blocked.setback',
  inside_realm: 'realmWindow.cooperation.blocked.insideRealm',
}

/** No mutation API: asking, sharing, working and reporting remain editable intentions. */
export function RealmCooperationBlock({ project, onDraftAction, draftingDisabled }: {
  project: RealmCooperation
  onDraftAction?: (message: string) => void
  draftingDisabled?: boolean
}) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<string[]>([])
  const name = t(project.name_key)
  const partner = project.partner_name
  const clue = project.clue_text_key ? t(project.clue_text_key, { defaultValue: project.clue_text || '' }) : project.clue_text || ''
  const reportFacts = project.report_facts || []
  const reportScope = `${project.project_id}:${project.instance_id}:${reportFacts.map(fact => `${fact.kind}:${fact.source_ref}`).join('|')}`
  // Never carry an earlier selection into another instance or a new set of outcomes.
  useEffect(() => setSelected([]), [reportScope])
  const factText = (fact: typeof reportFacts[number]) => t(fact.description_key, { value: fact.value_key ? t(fact.value_key) : '' })
  const factKey = (fact: typeof reportFacts[number]) => `${fact.kind}:${fact.source_ref}`
  const chosen = reportFacts.filter(fact => selected.includes(factKey(fact)))
  const duration = project.duration_seconds % 60 === 0
    ? t('realmWindow.explore.minutes', { count: project.duration_seconds / 60 })
    : t('realmWindow.explore.seconds', { count: project.duration_seconds })
  const canPlan = Boolean(onDraftAction) && !draftingDisabled
  const workFinished = project.phase === 'worked' || project.phase === 'used' || project.phase === 'reported'
  const buttonClass = 'h-auto min-h-9 justify-start whitespace-normal text-left text-[12px]'

  return (
    <section data-testid="realm-cooperation" className="flex min-w-0 flex-col gap-2 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
      <div className="flex flex-wrap items-baseline justify-between gap-1">
        <h3 className="break-words text-[13px] font-medium">{name}</h3>
        <span className="text-[11px] text-[var(--nova-text-muted)]">{partner}</span>
      </div>
      <p className="text-[12px] leading-relaxed">{t(project.description_key)}</p>
      <p className="text-[12px] font-medium">{t(phaseKeys[project.phase])}</p>
      {clue ? <p className="break-words text-[12px] leading-relaxed">{t('realmWindow.cooperation.clue', { clue })}</p> : null}
      {project.location_name ? <p className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.cooperation.meetingPlace', { place: project.location_name })}</p> : null}
      <dl className="space-y-2 text-[12px] leading-relaxed">
        <div><dt className="font-medium">{t('realmWindow.cooperation.yourPart')}</dt><dd className="text-[var(--nova-text-muted)]">{t(project.player_role_key)}</dd></div>
        <div><dt className="font-medium">{t('realmWindow.cooperation.partnerPart', { partner })}</dt><dd className="text-[var(--nova-text-muted)]">{t(project.partner_role_key)}</dd></div>
      </dl>
      <div className="space-y-1 rounded border border-[var(--nova-border)] p-2 text-[11px] text-[var(--nova-text-muted)]">
        <p>{t('realmWindow.cooperation.time', { duration })}</p>
        <p>{project.blocked_reason === 'partner_unavailable' && project.partner_energy_label_key && project.partner_energy_cost === 0
          ? t('realmWindow.cooperation.unverifiedPartnerCost')
          : project.partner_energy_cost > 0
          ? t('realmWindow.cooperation.partnerCost', { partner, energy: project.partner_energy_label_key ? t(project.partner_energy_label_key) : t('realmWindow.cooperation.energy'), count: project.partner_energy_cost })
          : t('realmWindow.cooperation.noPartnerCost')}</p>
        {project.player_costs.length ? project.player_costs.map((cost, index) => {
          const resource = cost.name_key ? t(cost.name_key, { defaultValue: cost.name || t('realmWindow.explore.unknownResource') }) : cost.name || t('realmWindow.explore.unknownResource')
          const costKey = cost.kind === 'consume_item' ? 'realmWindow.explore.consume' : cost.kind === 'require_item' ? 'realmWindow.explore.require' : 'realmWindow.explore.skill'
          return <p key={`${cost.kind}:${cost.ref_id}:${index}`}>{t('realmWindow.cooperation.yourCost', { cost: t(costKey, { name: resource, count: cost.quantity }) })}</p>
        }) : <p>{t('realmWindow.cooperation.noPlayerCost')}</p>}
      </div>
      {project.blocked_reason ? <p className="text-[12px] text-[var(--nova-text-muted)]">{t(blockedKeys[project.blocked_reason] || 'realmWindow.cooperation.blocked.unavailable')}</p> : null}
      {onDraftAction && !workFinished ? (
        <div className="flex flex-col gap-2">
          <Button type="button" variant="outline" size="sm" className={buttonClass} disabled={!canPlan || !project.can_ask}
            onClick={() => onDraftAction(t('realmWindow.cooperation.draftAsk', { partner, project: name }))}>{t('realmWindow.cooperation.ask')}</Button>
          {project.phase === 'agreed' ? (
            <Button type="button" variant="outline" size="sm" className={buttonClass} disabled={!canPlan || !project.can_work}
              onClick={() => onDraftAction(t('realmWindow.cooperation.draftWork', { partner, project: name }))}>{t('realmWindow.cooperation.work')}</Button>
          ) : (
            <Button type="button" variant="outline" size="sm" className={buttonClass} disabled={!canPlan || !project.can_work_if_agreed || !clue}
              onClick={() => onDraftAction(t('realmWindow.cooperation.draftShareWork', { partner, project: name, clue }))}>{t('realmWindow.cooperation.shareWork')}</Button>
          )}
        </div>
      ) : null}
      {reportFacts.length > 0 ? (
        <fieldset className="min-w-0 space-y-2 border-t border-[var(--nova-border)] pt-2">
          <legend className="text-[12px] font-medium">{t('realmWindow.cooperation.reportTitle')}</legend>
          <p className="text-[11px] text-[var(--nova-text-muted)]">{t('realmWindow.cooperation.reportHint')}</p>
          {reportFacts.map(fact => (
            <label key={`${fact.kind}:${fact.source_ref}`} className="flex min-h-9 cursor-pointer items-start gap-2 rounded p-1 text-[12px] leading-relaxed hover:bg-[var(--nova-hover)]">
              <input type="checkbox" className="mt-1 h-4 w-4 shrink-0 accent-[var(--nova-accent)]" checked={selected.includes(factKey(fact))} disabled={!canPlan || !project.can_report}
                onChange={event => setSelected(current => event.target.checked ? [...current, factKey(fact)] : current.filter(ref => ref !== factKey(fact)))} />
              <span className="min-w-0 break-words">{factText(fact)}</span>
            </label>
          ))}
          {onDraftAction ? <Button type="button" variant="outline" size="sm" className={buttonClass} disabled={!canPlan || !project.can_report || chosen.length === 0}
            onClick={() => onDraftAction(t('realmWindow.cooperation.draftReport', { partner, project: name, facts: chosen.map(factText).join(t('realmWindow.cooperation.factSeparator')) }))}>{t('realmWindow.cooperation.report')}</Button> : null}
        </fieldset>
      ) : null}
      <p className="text-[11px] leading-relaxed text-[var(--nova-text-muted)]">{t('realmWindow.cooperation.solo')}</p>
    </section>
  )
}
