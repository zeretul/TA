import { useWorldPresentation } from '../../world-presentation-context'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BookOpen, ChevronDown } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { requestJSON } from '@/lib/api-client'
import type { Snapshot } from '../../types'
import { EntryDetails } from './EntryDetails'
import { adventurePresentations, cultivationPresentations } from './presentations'
import type { AdventureDetailsResponse, CultivationDetailsResponse, EntryPresentation } from './types'

const groups = ['techniques', 'skills', 'craft', 'weapons', 'armor', 'pills', 'quest', 'manuals', 'tools', 'materials', 'assets', 'other'] as const
const collectionGroup = (entry: EntryPresentation) => entry.item_group ?? (entry.category === 'equipment' || entry.category === 'inventory' ? 'other' : entry.category)

export function ownedEntryEndpoint(snapshot: Snapshot): 'adventure-window' | 'cultivation-entry-details' | null {
  if (snapshot.actor_state_schema?.system.adventure_rules) return 'adventure-window'
  const fields = snapshot.actor_state_schema?.system.templates?.flatMap(template => template.fields || []) || []
  return fields.some(field => field.path?.startsWith('cultivation.') || ['境界', '已学功法', '功法'].includes(field.name)) ? 'cultivation-entry-details' : null
}

/** A separate read-only view of the player's own learned/held entries. This
 * never enriches or rewrites a canonical snapshot with mutable author data. */
export function OwnedEntryDetails({ snapshot }: { snapshot: Snapshot | null }) {
  const { i18n } = useTranslation()
  const { t, terms } = useWorldPresentation({ snapshot })
  const [open, setOpen] = useState(false)
  const [group, setGroup] = useState('all')
  const [attempt, setAttempt] = useState(0)
  const [result, setResult] = useState<{ key: string; entries: EntryPresentation[]; failed?: boolean } | null>(null)
  const route = snapshot && ownedEntryEndpoint(snapshot)
  const key = snapshot && route ? `${snapshot.story_id}:${snapshot.branch_id}:${snapshot.current_turn?.id || ''}:${snapshot.context_revision || 0}:${i18n.language}:${route}:${attempt}` : ''
  useEffect(() => {
    if (!open || !snapshot || !route) return
    let cancelled = false
    const path = `/api/interactive/stories/${encodeURIComponent(snapshot.story_id)}/${route}?branch=${encodeURIComponent(snapshot.branch_id)}`
    const read = route === 'adventure-window'
      ? requestJSON<AdventureDetailsResponse>(path).then(value => adventurePresentations(value, t))
      : requestJSON<CultivationDetailsResponse>(path).then(value => cultivationPresentations(value, t))
    void read.then(entries => { if (!cancelled) setResult({ key, entries }) }).catch(error => {
      console.error('[entry-details] Owned projection unavailable', error)
      if (!cancelled) setResult({ key, entries: [], failed: true })
    })
    return () => { cancelled = true }
  }, [open, key, route, snapshot?.story_id, snapshot?.branch_id, t])
  if (!snapshot || !route) return null
  const current = result?.key === key ? result : null
  const visibleGroups = groups.filter(name => current?.entries.some(entry => collectionGroup(entry) === name))
  const selected = visibleGroups.some(name => name === group) ? group : 'all'
  const label = (name: string) => name === 'techniques' ? terms.techniques : t(`entryDetails.collection.${name}`)
  return <Collapsible open={open} onOpenChange={setOpen} className="min-w-0 border-t px-2.5 py-2" data-testid="owned-entry-details">
    <CollapsibleTrigger asChild><button type="button" className="flex w-full min-w-0 items-center gap-2 rounded-md py-1 text-left text-xs font-medium focus-visible:outline-2 focus-visible:outline-ring">
      <BookOpen aria-hidden="true" className="size-3.5 shrink-0" /><span className="min-w-0 flex-1 break-words">{t('entryDetails.ownedTitle', terms)}</span><ChevronDown aria-hidden="true" className={`size-3.5 shrink-0 transition-transform ${open ? 'rotate-180' : ''}`} />
    </button></CollapsibleTrigger>
    <CollapsibleContent className="min-w-0 space-y-3 pt-2">
      {!current ? <p role="status" className="text-xs text-muted-foreground">{t('entryDetails.loading')}</p> : current.failed ? <div role="alert" className="space-y-2 text-xs"><p>{t('entryDetails.loadFailed')}</p><Button variant="outline" size="sm" onClick={() => setAttempt(value => value + 1)}>{t('entryDetails.retry')}</Button></div> : current.entries.length === 0 ? <p className="text-xs text-muted-foreground">{t('entryDetails.empty', terms)}</p> : (
        <Tabs value={selected} onValueChange={setGroup} className="min-w-0 gap-2">
          <div className="overflow-x-auto">
            <TabsList aria-label={t('entryDetails.collection.label')} className="w-max max-w-none justify-start">
              {['all', ...visibleGroups].map(name => <TabsTrigger key={name} value={name}>{label(name)}</TabsTrigger>)}
            </TabsList>
          </div>
          <TabsContent key={selected} value={selected}>
            <ul className="flex min-w-0 flex-wrap gap-1">
              {current.entries.filter(entry => selected === 'all' || collectionGroup(entry) === selected).map(entry => (
                <li className="min-w-0 max-w-full" key={`${key}:${entry.category}:${entry.id}`}><EntryDetails entry={entry} nameOnly /></li>
              ))}
            </ul>
          </TabsContent>
        </Tabs>
      )}
    </CollapsibleContent>
  </Collapsible>
}
