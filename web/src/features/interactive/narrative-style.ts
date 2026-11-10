import type { TFunction } from 'i18next'
import { getResolvedLocale } from '@/i18n'
import type { Teller } from './types'

export const DEFAULT_NARRATIVE_STYLE_ID = 'source-drama'

/**
 * The six styles the picker recommends, in the design's own order.
 *
 * The backend still resolves every id a story may have saved — including the six
 * the recommended set absorbed — but offering twelve names for six styles is
 * exactly what the merge removed. The picker therefore filters to these, while
 * the preset directory keeps showing whatever exists so an old selection stays
 * findable and restorable.
 */
export const RECOMMENDED_NARRATIVE_STYLE_IDS = [
  'source-drama',
  'straight-comedy',
  'slice-banter',
  'strange-tales',
  'hotblooded-combat',
  'cinematic-shots',
] as const

const RECOMMENDED = new Set<string>(RECOMMENDED_NARRATIVE_STYLE_IDS)

/** True when the id belongs to the recommended six rather than to an absorbed one. */
export function isRecommendedNarrativeStyle(id: string): boolean {
  return RECOMMENDED.has(id)
}

const BUILTIN_NARRATIVE_STYLE_IDS = new Set(['rhythm', 'classic', 'screenwriter', 'grimdark'])

/**
 * The styles a picker offers.
 *
 * A story that saved an absorbed id still has to show that id, or the select
 * would render a value it has no option for and silently look unset. So the
 * current selection is always kept, and only the rest of the list is filtered.
 */
export function recommendedNarrativeStyles(tellers: Teller[], currentID?: string): Teller[] {
  return tellers.filter((teller) => RECOMMENDED.has(teller.id) || teller.id === currentID)
}

/** Resolve an available selection without depending on backend list ordering. */
export function resolveNarrativeStyle(tellers: Teller[], requestedID: string | undefined): Teller | undefined {
  return tellers.find((teller) => teller.id === requestedID)
    ?? tellers.find((teller) => teller.id === DEFAULT_NARRATIVE_STYLE_ID)
    ?? tellers.find((teller) => RECOMMENDED.has(teller.id))
    ?? tellers[0]
}

export function narrativeStyleName(teller: Teller, t: TFunction): string {
  if (isUntouchedBuiltin(teller)) return t(`narrativeStyle.builtin.${teller.id}.name`)
  return presetText(teller.name_en, teller.name) || teller.id
}

export function narrativeStyleDescription(teller: Teller, t: TFunction): string {
  if (isUntouchedBuiltin(teller)) return t(`narrativeStyle.builtin.${teller.id}.description`)
  return presetText(teller.description_en, teller.description)
}

/**
 * Picks between a preset's own two display strings.
 *
 * The migrated styles carry their zh/en name and description on the preset
 * definition rather than as locale keys, so a new style is localized by
 * declaring both strings once instead of adding entries to two locale files.
 * Both fields are the preset's own identity and an edit to its prompt does not
 * change them, so an overridden builtin keeps whichever name it declares. A
 * user-created preset has no English field and is shown as written. The four
 * pre-migration styles keep their locale keys, which is why this only applies
 * where the English field exists.
 */
function presetText(english: string | undefined, chinese: string | undefined): string {
  const zh = (chinese || '').trim()
  const en = (english || '').trim()
  if (!en) return zh
  return getResolvedLocale() === 'en-US' ? en : zh
}

function isUntouchedBuiltin(teller: Teller): boolean {
  return BUILTIN_NARRATIVE_STYLE_IDS.has(teller.id) && !teller.custom && !teller.builtin_overridden
}
