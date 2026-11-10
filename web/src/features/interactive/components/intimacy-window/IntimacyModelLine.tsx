import { useTranslation } from 'react-i18next'
import type { IntimacyModelState } from '../../types'
import { thinkingLevelLabel } from './thinking-level-label'

interface IntimacyModelLineProps {
  model: IntimacyModelState
}

/**
 * Which model this activity is speaking through, said plainly and nothing more.
 *
 * §4.1 forbids falling back to the main story's model, so an unconfigured
 * activity says so instead of quietly borrowing a route. The choice itself is
 * not made here: T27 keeps runtime and model menus out of game windows, so the
 * binding is picked in the game settings' narrative section and this line only
 * reports what that binding resolved to.
 */
export function IntimacyModelLine({ model }: IntimacyModelLineProps) {
  const { t } = useTranslation()

  if (!model.configured) {
    return (
      <div data-testid="intimacy-window-model" className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1.5 text-[11px] text-[var(--nova-text-muted)]">
        <p>{t('intimacyWindow.model.unconfigured')}</p>
        <p className="text-[10px] text-[var(--nova-text-faint)]">{t('intimacyWindow.model.unconfiguredHint')}</p>
      </div>
    )
  }

  return (
    <div data-testid="intimacy-window-model" className="flex shrink-0 items-baseline gap-2 border-b border-[var(--nova-border)] px-3 py-1.5 text-[11px] text-[var(--nova-text-muted)]">
      <span className="min-w-0 flex-1 truncate" title={t('intimacyWindow.model.title')}>
        {t('intimacyWindow.model.line', { model: model.model || model.profile_id || '', provider: model.provider || '' })}
        {model.thinking_level ? ` · ${t('intimacyWindow.model.thinking', { level: thinkingLevelLabel(model.thinking_level, t) })}` : ''}
      </span>
    </div>
  )
}
