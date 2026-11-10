import { useTranslation } from 'react-i18next'
import type { HistoryState } from './types'

/** Localizes authored labels; player text and model dialogue keep their saved wording. */
export function useHistoryLabels(state?: HistoryState | null) {
  const { t, i18n } = useTranslation()
  const localized = (key: string, fallback: string) => i18n.exists(key) ? t(key) : fallback
  const en=(i18n.resolvedLanguage || i18n.language).startsWith("en")
  return {
    en,
    text: (value?: {zh:string;en:string}) => value ? (en ? value.en : value.zh) : "",
    t,
    number: (value: number) => new Intl.NumberFormat(i18n.resolvedLanguage || i18n.language).format(value),
    season: (year: number, quarter: number) => t('history.season', { year, season: t(`history.season.${quarter}`) }),
    region: (id?: string) => id ? localized(`history.region.${id}`, state?.regions?.find((item) => item.id === id)?.name || id) : '—',
    minister: (id?: string) => id && state?.world ? (state.ministers.find(item=>item.id===id)?.[en ? "name_en" : "name"] || state.world.entities.find(item=>item.id===id)?.name[en ? "en" : "zh"] || id) : id ? localized(`history.minister.${id}`, state?.ministers?.find((item) => item.id === id)?.name || id) : t('history.order.noOwner'),
    office: (id: string) => state?.world ? (state.ministers.find(item=>item.id===id)?.[en ? "office_en" : "office"] || "") : localized(`history.office.${id}`, state?.ministers?.find((item) => item.id === id)?.office || ''),
    kind: (value: string) => localized(`history.kind.${value}`, value),
    status: (value: string) => localized(`history.status.${value}`, value),
    reason: (value?: string) => value ? localized(`history.reason.${value}`, value) : '',
  }
}
