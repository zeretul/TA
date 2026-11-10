import { Package, Sparkles } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { listStoryPacks, type StoryPackDraft, type StoryPackSummary } from '@/lib/api-client'
import { cn } from '@/lib/utils'

interface StoryPackSelectorProps {
  projectId: string
  onApply: (packId: string) => Promise<StoryPackDraft>
}

/**
 * Loads a script pack into the story being set up. The pack fills the fields
 * below rather than starting the story itself, so the user keeps the ordinary
 * "review then start" flow and can still adjust anything the pack proposed.
 */
export function StoryPackSelector({ projectId, onApply }: StoryPackSelectorProps) {
  const { t } = useTranslation()
  const [packs, setPacks] = useState<StoryPackSummary[]>([])
  const [selectedId, setSelectedId] = useState('')
  const [loading, setLoading] = useState(true)
  const [applying, setApplying] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    listStoryPacks()
      .then((items) => {
        if (cancelled) return
        setPacks(items)
        setSelectedId(items.find((item) => !item.invalid)?.id || '')
      })
      .catch((reason) => {
        if (cancelled) return
        console.error('[story-pack] Failed to list script packs', { projectId, reason })
        setError(t('storyPicker.pack.failed'))
      })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [projectId, t])

  const apply = useCallback(async () => {
    if (!selectedId || applying) return
    setApplying(true)
    setError('')
    setMessage('')
    try {
      const draft = await onApply(selectedId)
      setMessage(draft.world_added > 0
        ? t('storyPicker.pack.applied', { name: draft.pack_name, added: draft.world_added })
        : t('storyPicker.pack.appliedNoWorld', { name: draft.pack_name }))
    } catch (reason) {
      console.error('[story-pack] Failed to apply script pack', { projectId, packId: selectedId, reason })
      setError(reason instanceof Error ? reason.message : t('storyPicker.pack.failed'))
    } finally {
      setApplying(false)
    }
  }, [applying, onApply, projectId, selectedId, t])

  if (loading) return null
  const selected = packs.find((item) => item.id === selectedId)

  // No pack is available. This used to render nothing, which left the setup
  // screen looking as if packs were simply not part of the product and hid a
  // failed listing behind an empty page. The state is stated instead: the user
  // is told no pack is selected and that the fields below still work.
  if (packs.length === 0) {
    return (
      <section aria-labelledby="story-pack-title" className="overflow-hidden rounded-xl border border-border bg-card">
        <div className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1 px-4 pb-3 pt-4 sm:px-5">
          <h3 id="story-pack-title" className="text-sm font-semibold text-foreground">{t('storyPicker.pack.title')}</h3>
          <p className="text-xs text-muted-foreground">{t('storyPicker.pack.description')}</p>
        </div>
        <div className="border-t border-border px-4 py-3 sm:px-5">
          <p className="text-xs font-medium text-foreground">{t('storyPicker.pack.noneSelected')}</p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{error || t('storyPicker.pack.emptyHint')}</p>
        </div>
      </section>
    )
  }

  return (
    <section aria-labelledby="story-pack-title" className="overflow-hidden rounded-xl border border-border bg-card">
      <div className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1 px-4 pb-3 pt-4 sm:px-5">
        <h3 id="story-pack-title" className="text-sm font-semibold text-foreground">{t('storyPicker.pack.title')}</h3>
        <p className="text-xs text-muted-foreground">{t('storyPicker.pack.description')}</p>
      </div>
      <div role="listbox" aria-label={t('storyPicker.pack.placeholder')} className="grid max-h-56 gap-2 overflow-y-auto border-t border-border px-3 py-3 sm:px-4">
        {packs.map((pack) => {
          const active = pack.id === selectedId
          return (
            <button
              key={pack.id}
              type="button"
              role="option"
              aria-selected={active}
              aria-label={pack.name}
              disabled={pack.invalid}
              className={cn(
                'rounded-lg border p-3 text-left transition-colors disabled:opacity-60',
                active ? 'border-primary/55 bg-primary/5' : 'border-border bg-background hover:bg-muted/50',
              )}
              onClick={() => { setSelectedId(pack.id); setMessage(''); setError('') }}
            >
              <span className="flex min-w-0 items-center gap-2 text-xs font-medium text-foreground">
                <Package className="size-3.5 shrink-0 text-primary" />
                <span className="min-w-0 flex-1 truncate">{pack.name}</span>
                {pack.builtin ? <span className="shrink-0 rounded border border-border px-1 text-[10px] text-muted-foreground">{t('storyPicker.pack.builtin')}</span> : null}
              </span>
              {pack.description ? <span className="mt-1.5 line-clamp-2 block text-xs leading-5 text-muted-foreground">{pack.description}</span> : null}
              <span className="mt-1 block text-[11px] text-muted-foreground">
                {pack.invalid
                  ? t('storyPicker.pack.invalid', { error: pack.error || '' })
                  : t('storyPicker.pack.worldCount', { count: pack.world_count })}
              </span>
            </button>
          )
        })}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border px-4 py-3 sm:px-5">
        <span className="min-w-0 flex-1 text-xs text-muted-foreground">{message || error}</span>
        <Button type="button" size="sm" variant="outline" disabled={!selected || applying} onClick={() => void apply()}>
          {applying ? <Spinner /> : <Sparkles data-icon="inline-start" />}
          {applying ? t('storyPicker.pack.applying') : t('storyPicker.pack.apply')}
        </Button>
      </div>
    </section>
  )
}
