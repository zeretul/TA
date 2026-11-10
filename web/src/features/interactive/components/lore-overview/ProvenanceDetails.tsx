import { useTranslation } from 'react-i18next'
import type { KnownProvenance } from './known-provenance'

export function ProvenanceDetails({ value }: { value?: KnownProvenance }) {
  const { t } = useTranslation()
  const unknown = t('loreOverview.notRecorded')
  const source = value?.sourceKey ? t(value.sourceKey) : value?.source
  return <dl className="mt-1 grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-2 gap-y-1 text-[11px] text-muted-foreground">
    <dt>{t('loreOverview.provenance.learnedAt')}</dt><dd className="break-words">{value?.learnedAt || unknown}</dd>
    <dt>{t('loreOverview.provenance.source')}</dt><dd className="break-words">{source || unknown}</dd>
    {value?.nature ? <><dt>{t('loreOverview.provenance.nature')}</dt><dd>{t(`loreOverview.provenance.nature.${value.nature}`)}</dd></> : null}
    <dt>{t('loreOverview.provenance.confidence')}</dt><dd>{value?.confidence ? t(`loreOverview.provenance.confidence.${value.confidence}`) : unknown}</dd>
    <dt>{t('loreOverview.provenance.evidence')}</dt><dd className="min-w-0 break-words">
      {value?.evidence?.length || value?.references?.length ? <ul className="space-y-1">
        {value.evidence?.map((item, index) => <li key={`text:${index}`}>{item}</li>)}
        {value.references?.map(item => <li key={`${item.labelKey}:${item.id}`}>{t(item.labelKey)}：{item.id}</li>)}
      </ul> : unknown}
    </dd>
  </dl>
}
