import { History, Activity } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { DirectorConsoleTab } from './types'

interface DirectorConsoleTabsProps {
  activeTab: DirectorConsoleTab
  onChange: (tab: DirectorConsoleTab) => void
}

/** What just changed, or what has happened so far. Settings live in game settings. */
export function DirectorConsoleTabs({ activeTab, onChange }: DirectorConsoleTabsProps) {
  const { t } = useTranslation()
  const items: Array<{ id: DirectorConsoleTab; label: string; icon: React.ReactNode }> = [
    { id: 'recent', label: t('directorPanel.handbookTab.recent'), icon: <Activity className="director-console-tab__icon size-3.5" /> },
    { id: 'history', label: t('directorPanel.handbookTab.history'), icon: <History className="director-console-tab__icon size-3.5" /> },
  ]

  return (
    <Tabs value={activeTab} onValueChange={(value) => onChange(value as DirectorConsoleTab)} className="shrink-0 gap-0 border-b border-[var(--nova-border)] bg-[var(--director-canvas)]">
      <TabsList variant="line" aria-label={t('directorPanel.handbookTabs')} className="h-10 w-full gap-0 rounded-none px-2">
        {items.map((item) => (
          <TabsTrigger
            key={item.id}
            value={item.id}
            aria-label={item.label}
            className="h-full min-w-0 flex-1 gap-1 rounded-none px-1 text-xs font-medium text-[var(--nova-text-faint)] after:bottom-0 after:bg-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] data-[state=active]:bg-transparent data-[state=active]:text-[var(--nova-text)]"
          >
            {item.icon}
            <span className="director-console-tab__label min-w-0 truncate">{item.label}</span>
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  )
}
