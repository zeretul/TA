import type { TFunction } from 'i18next'

/**
 * Renders a stage key as the wording the player knows.
 *
 * The rules report the key rather than a label, so the wording lives here and an
 * unrecognised rule key is reported as unknown without guessing a realm.
 * Custom worlds may provide free-text labels, which are preserved. These shared
 * translations keep each stage consistent in the story and in battle.
 */
export function realmLabel(realm: string | undefined, t: TFunction): string {
  const key = (realm || '').trim()
  if (!key) return ''
  if (key === 'mortal') return t('combatWindow.realm.mortal')
  const qi = /^qi_(\d+)$/.exec(key)
  if (qi) return t('combatWindow.realm.qi', { layer: qi[1] })
  const staged = /^(foundation|core|nascent|spirit|void|integration|mahayana)_(early|middle|late)$/.exec(key)
  if (staged) {
    return t(`combatWindow.realm.${staged[1]}`, {
      stage: t(`combatWindow.realmStage.${staged[2]}`),
    })
  }
  return /^(qi|foundation|core|nascent|spirit|void|integration|mahayana)_[a-z0-9_]+$/.test(key) ? t('combatWindow.realm.unknown') : key
}
