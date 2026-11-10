import { useTranslation } from 'react-i18next'
import type { KnownAffiliation } from './known-affiliations'
import { ProvenanceDetails } from './ProvenanceDetails'

export function AffiliationList({ affiliations, onOpen }: { affiliations: KnownAffiliation[]; onOpen: (id: string) => void }) {
  const { t } = useTranslation()
  return <section className="flex min-w-0 flex-col gap-2 border-t border-[var(--nova-border)] pt-2">
    <h3 className="text-xs font-medium">{t('loreOverview.row.affiliations')}</h3>
    {affiliations.length ? affiliations.map((item, index) => <div key={`${item.factionId || item.name}:${index}`} className="min-w-0 rounded border border-[var(--nova-border)] p-2 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        {item.factionId ? <button type="button" className="break-words text-[var(--nova-accent)] hover:underline" onClick={() => onOpen(item.factionId!)}>{item.name}</button> : <span className="break-words">{item.name}</span>}
        <span>{t(`loreOverview.membership.${item.status}`)}</span>
        {item.role ? <span className="break-words text-muted-foreground">{item.role}</span> : null}
      </div>
      {item.factionId ? <p className="mt-1 break-all text-[11px] text-muted-foreground">{t('loreOverview.membership.identity')}：{item.factionId}</p> : null}
      {item.statementKey || item.statement ? <p className="mt-1 break-words text-muted-foreground">{item.statementKey ? t(item.statementKey) : item.statement}</p> : null}
      <ProvenanceDetails value={item.provenance} />
    </div>) : <p className="text-xs text-muted-foreground">{t('loreOverview.notKnown')}</p>}
  </section>
}
