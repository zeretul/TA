import type { TFunction } from 'i18next'

/**
 * The stored thinking level as the player chose it.
 *
 * The selector offers these same four levels and words them in the interface
 * language, so the display line has to read from the same keys: printing
 * `high` would show a configuration value where a choice belongs. A level the
 * interface does not word — an older saved value, or one set by hand — is named
 * generically rather than echoed in English, and the stored value goes to the
 * log where a bug report can still find it.
 */
export function thinkingLevelLabel(level: string, t: TFunction): string {
  const stored = level.trim()
  const key = `intimacyWindow.thinking.${stored}`
  const translated = t(key)
  if (translated !== key) return translated
  console.warn('[intimacy-window] unworded thinking level', stored)
  return t('intimacyWindow.thinking.other')
}
