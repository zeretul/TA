import { Compass, Landmark, ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { WorkspaceMode } from '@/stores/workspace-store'

export function GameChooser({ onNavigate }: { onNavigate: (route: WorkspaceMode) => void }) {
  const { t } = useTranslation()
  return <section className="h-full overflow-y-auto bg-[var(--nova-bg)] px-6 py-12 text-[var(--nova-text)]"><div className="mx-auto max-w-4xl"><p className="mb-3 text-xs uppercase tracking-widest text-[var(--nova-text-muted)]">TA</p><h1 className="text-3xl font-medium">{t('history.choose.title')}</h1><p className="mt-3 text-sm leading-7 text-[var(--nova-text-muted)]">{t('history.choose.subtitle')}</p><div className="mt-10 grid gap-5 md:grid-cols-2">{([{route:'books', title:'history.adventure', description:'history.choose.adventure', action:'history.choose.openAdventure',icon:Compass},{route:'history',title:'history.title',description:'history.choose.history',action:'history.choose.openHistory',icon:Landmark}] as const).map(({route,title,description,action,icon:Icon})=><article key={route} className="flex min-w-0 flex-col rounded-xl border border-[var(--nova-border)] p-7"><Icon className="mb-5 size-8 text-[var(--nova-text-muted)]" aria-hidden="true"/><h2 className="text-xl">{t(title)}</h2><p className="my-5 flex-1 text-sm leading-7 text-[var(--nova-text-muted)]">{t(description)}</p><Button variant="outline" className="w-fit" onClick={()=>onNavigate(route)}>{t(action)}<ArrowRight/></Button></article>)}</div></div></section>
}
