import type { LoreItem } from '@/lib/api'

/**
 * The types an author may assign from the library.
 *
 * `rule` is deliberately absent: world rules come from the world pack's own
 * world book and the bound gameplay configuration, and the library no longer
 * offers a rule list or a rule entry to retype. Entries that already carry the
 * type keep it — the label still resolves — but nothing new can be made into
 * one from here.
 */
export const TYPE_OPTIONS = [
  { value: 'character' },
  { value: 'world' },
  { value: 'location' },
  { value: 'faction' },
  { value: 'item' },
  { value: 'other' },
] as const

export const IMPORTANCE_OPTIONS = [
  { value: 'major' },
  { value: 'important' },
  { value: 'minor' },
] as const

export const LOAD_MODE_OPTIONS = [
  { value: 'resident' },
  { value: 'auto' },
  { value: 'manual' },
] as const

export const LORE_RESIDENT_TOTAL_WARNING_BYTES = 32 * 1024

/**
 * The selectable types for one entry.
 *
 * A `rule` entry that already exists keeps its own option so the select can
 * still show its current value — a Radix select whose value matches no item
 * renders blank and would silently retype the entry on the next save. It is
 * not offered for any other entry.
 */
export function typeOptionsFor(current: LoreItem['type'] | undefined) {
  if (current === 'rule') return [{ value: 'rule' as const }, ...TYPE_OPTIONS]
  return TYPE_OPTIONS
}

export function loreTypeLabel(
  type: LoreItem['type'],
  t: (key: string) => string,
) {
  const key = `lore.type.${type}`
  const label = t(key)
  return label === key ? t('lore.type.other') : label
}

export function loreImportanceLabel(
  importance: LoreItem['importance'],
  t: (key: string) => string,
) {
  const key = `lore.importance.${importance}`
  const label = t(key)
  return label === key ? t('lore.importance.important') : label
}

export function loreLoadModeLabel(
  loadMode: LoreItem['load_mode'] | undefined,
  t: (key: string) => string,
) {
  const key = `lore.loadMode.${loadMode || 'auto'}`
  const label = t(key)
  return label === key ? t('lore.loadMode.auto') : label
}

/**
 * What the current reading mode means, in the player's own terms.
 *
 * An entry with no explicit mode is shown as `auto` in the editor, so an
 * unset value must describe itself the same way instead of falling through to
 * a fourth sentence the player cannot select. A value this build does not
 * recognise keeps the neutral catalogue wording rather than guessing.
 */
export function loadModeDescription(
  loadMode: LoreItem['load_mode'] | undefined,
  t: (key: string) => string,
) {
  if (loadMode === 'resident') return t('settingPanel.lore.residentDesc')
  if (loadMode === 'manual') return t('settingPanel.lore.manualDesc')
  if (loadMode === 'auto' || loadMode === undefined) return t('settingPanel.lore.autoDesc')
  return t('settingPanel.lore.indexDesc')
}
