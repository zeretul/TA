import { lazy, Suspense, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowRightLeft, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { LoadingState } from '@/components/common/LoadingState'
import { formatDateTime } from '@/i18n'
import type { StoryPackIdentityChoice } from '@/lib/api-client'
import { getGameSaves, type GameSave, type GameSaveCatalog } from '../game-saves'
import { useInteractiveStore } from '../stores/interactive-store'

const HomeView = lazy(() => import('@/components/Home/HomeView').then((m) => ({ default: m.HomeView })))
type Intent = 'new' | 'switch' | 'delete'
interface Props {
  projectId: string
  onStartStory: (packId: string, templateId?: string, identity?: StoryPackIdentityChoice) => void | Promise<void>
  onOpenSave?: (save: GameSave) => Promise<void>
  onDeleteSave?: (save: GameSave) => Promise<void>
}

/** Visible game actions share the existing world creation and Story mutations.
 * Catalog reads never switch worlds; writes are delegated to the App owner. */
export function GameSaveActions({ projectId, onStartStory, onOpenSave, onDeleteSave }: Props) {
  const { t } = useTranslation()
  const currentStoryId = useInteractiveStore((s) => s.currentStoryId)
  const [intent, setIntent] = useState<Intent | null>(null)
  const [catalog, setCatalog] = useState<GameSaveCatalog>({ saves: [], unavailable: [] })
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [revision, setRevision] = useState(0)
  const [target, setTarget] = useState<GameSave | null>(null)
  useEffect(() => {
    if (intent !== 'switch' && intent !== 'delete') return
    let cancelled = false
    setLoading(true); setError(''); setQuery('')
    getGameSaves().then((result) => { if (!cancelled) setCatalog(result) })
      .catch((cause) => {
        console.error('[game-saves] Failed to load saves', cause)
        if (!cancelled) { setCatalog({ saves: [], unavailable: [] }); setError(t('gameSaves.loadFailed')) }
      }).finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [intent, revision, t])
  const openSave = async (save: GameSave) => {
    if (!onOpenSave || busy) return
    setBusy(true); setError('')
    try { await onOpenSave(save); setIntent(null) }
    catch (cause) { console.error('[game-saves] Failed to open save', cause); setError(t('gameSaves.openFailed')) }
    finally { setBusy(false) }
  }
  const deleteSave = async () => {
    if (!target || !onDeleteSave) return false
    setBusy(true)
    try {
      await onDeleteSave(target)
      setCatalog((current) => ({ ...current, saves: current.saves.filter((s) => s.project_id !== target.project_id || s.story.id !== target.story.id) }))
    } catch (cause) {
      console.error('[game-saves] Failed to delete save', cause)
      throw new Error(t('gameSaves.deleteFailed'))
    } finally { setBusy(false) }
  }
  const needle = query.trim().toLocaleLowerCase()
  const saves = catalog.saves.filter((save) => [save.world_name, save.story.title, save.story.protagonist?.name].join(' ').toLocaleLowerCase().includes(needle))
  return <>
    <nav aria-label={t('gameSaves.actions')} data-testid="game-save-actions" className="grid shrink-0 grid-cols-3 gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 sm:flex sm:px-4">
      <Button variant="outline" size="sm" className="min-w-0 gap-1.5 px-2" onClick={() => setIntent('new')}><Plus className="size-3.5 shrink-0" />{t('gameSaves.new')}</Button>
      <Button variant="outline" size="sm" className="min-w-0 gap-1.5 px-2" onClick={() => setIntent('switch')}><ArrowRightLeft className="size-3.5 shrink-0" />{t('gameSaves.switch')}</Button>
      <Button variant="outline" size="sm" className="min-w-0 gap-1.5 px-2 text-[var(--nova-text-muted)]" onClick={() => setIntent('delete')}><Trash2 className="size-3.5 shrink-0" />{t('gameSaves.delete')}</Button>
    </nav>
    <Dialog open={intent !== null} onOpenChange={(open) => { if (!open && !busy) { setIntent(null); setTarget(null) } }}>
      <DialogContent showCloseButton={!busy} className={intent === 'new' ? 'border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)] flex h-[min(90dvh,52rem)] max-w-[min(calc(100vw-1rem),72rem)] flex-col overflow-hidden p-3 sm:p-5' : 'border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)] flex max-h-[85dvh] max-w-[min(calc(100vw-1rem),42rem)] flex-col overflow-hidden'} onEscapeKeyDown={(e) => { if (busy) e.preventDefault() }} onInteractOutside={(e) => { if (busy) e.preventDefault() }}>
        <DialogHeader className="shrink-0 pr-6">
          <DialogTitle>{t(intent === 'new' ? 'gameSaves.new' : intent === 'delete' ? 'gameSaves.delete' : 'gameSaves.switch')}</DialogTitle>
          <DialogDescription>{t(intent === 'new' ? 'gameSaves.newHint' : 'gameSaves.listHint')}</DialogDescription>
        </DialogHeader>
        {intent === 'new' ? <div className="flex min-h-0 flex-1 flex-col"><Suspense fallback={<LoadingState label={t('common.loading')} />}><HomeView newGameOnly projectId={projectId} onStartStory={async (...args) => { setBusy(true); try { await onStartStory(...args); setIntent(null) } finally { setBusy(false) } }} onOpenStory={async () => {}} /></Suspense></div> : <>
          <Input aria-label={t('gameSaves.search')} placeholder={t('gameSaves.search')} value={query} onChange={(e) => setQuery(e.target.value)} disabled={busy || loading} />
          {error && <div role="alert" className="shrink-0 text-sm text-destructive">{error}<Button variant="ghost" size="sm" onClick={() => setRevision((r) => r + 1)}><RefreshCw />{t('gameSaves.retry')}</Button></div>}
          {catalog.unavailable.length > 0 && <p role="status" className="text-xs text-[var(--nova-text-muted)]">{t('gameSaves.unavailable', { names: catalog.unavailable.join('、') })}</p>}
          <div className="min-h-0 flex-1 overflow-y-auto">
            {loading ? <LoadingState label={t('common.loading')} /> : saves.length === 0 ? <p className="py-8 text-center text-sm text-[var(--nova-text-muted)]">{t(needle ? 'gameSaves.noMatch' : 'gameSaves.empty')}</p> : <ul className="divide-y divide-[var(--nova-border)]">
              {saves.map((save) => <li key={`${save.project_id}:${save.story.id}`} className="flex min-w-0 items-center gap-3 py-3">
                <div className="min-w-0 flex-1">
                  <p className="break-words text-sm font-medium">{save.story.title}</p>
                  <p className="mt-1 break-words text-xs text-[var(--nova-text-muted)]">{save.world_name}{save.story.protagonist?.name ? ` · ${save.story.protagonist.name}` : ''}</p>
                  <p className="mt-1 text-xs text-[var(--nova-text-faint)]">{t('storyPicker.turnCount', { count: save.story.turn_count })}{save.story.updated_at ? ` · ${formatDateTime(save.story.updated_at)}` : ''}{save.project_id === projectId && save.story.id === currentStoryId ? ` · ${t('gameSaves.current')}` : ''}</p>
                </div>
                <Button variant={intent === 'delete' ? 'outline' : 'default'} size="sm" className="shrink-0" disabled={busy || (intent === 'delete' ? !onDeleteSave : !onOpenSave)} onClick={() => intent === 'delete' ? setTarget(save) : void openSave(save)}>{t(intent === 'delete' ? 'gameSaves.deleteOne' : 'gameSaves.continue')}</Button>
              </li>)}
            </ul>}
          </div>
        </>}
      </DialogContent>
    </Dialog>
    <ConfirmDialog open={target !== null} onOpenChange={(open) => { if (!open) setTarget(null) }} title={t('gameSaves.confirmTitle')} description={t('gameSaves.confirmBody', { title: target?.story.title, world: target?.world_name, name: target?.story.protagonist?.name || t('gameSaves.unnamed') })} confirmLabel={t('gameSaves.confirmDelete')} tone="danger" onConfirm={deleteSave} />
  </>
}
