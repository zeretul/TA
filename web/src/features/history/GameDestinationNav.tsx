import { useTranslation } from 'react-i18next'
import { Compass, Landmark } from 'lucide-react'
import type { WorkspaceMode } from '@/stores/workspace-store'

/** These are destinations within Game, not a second global gameplay mode. */
export function GameDestinationNav({ current, onNavigate }: { current: 'interactive' | 'history'; onNavigate: (route: WorkspaceMode) => void }) {
  const { t } = useTranslation()
  return <nav aria-label={t('history.navigation')} className="flex shrink-0 flex-wrap items-center gap-1 border-b border-[var(--nova-border)] bg-[var(--nova-bg)] px-3 py-1.5 text-xs text-[var(--nova-text)]">
    {([{ id: 'interactive', label: 'history.adventure', icon: Compass }, { id: 'history', label: 'history.title', icon: Landmark }] as const).map(({ id, label, icon: Icon }) => <button type="button" key={id} aria-current={current === id ? 'page' : undefined} onClick={() => onNavigate(id)} className="flex items-center gap-1.5 rounded px-3 py-2 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] aria-[current=page]:bg-[var(--nova-active)] aria-[current=page]:text-[var(--nova-text)] focus-visible:outline-2 focus-visible:outline-offset-2"><Icon className="size-3.5" aria-hidden="true" />{t(label)}</button>)}
  </nav>
}
