import { AffiliationList } from './AffiliationList'
import { ProvenanceDetails } from './ProvenanceDetails'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { PointerEvent as ReactPointerEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, ChevronRight, Search, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import type { Snapshot } from '../../types'
import { PersonAvatar, PersonPortrait, personAvatarSource } from '../person-avatar'
import { LedgerFieldView } from '../story-state/ledger-fields'
import type { LedgerFieldGroup } from '../story-state/model'
import {
  buildLoreOverview,
  filterLoreEntries,
  LORE_TABS,
  type LoreEntry,
  type LoreRow,
  type LoreTabKey,
} from './model'
import {
  MAX_LORE_OVERVIEW_WIDTH,
  MIN_LORE_OVERVIEW_WIDTH,
  MIN_STORY_BODY_WIDTH,
  readLoreOverviewTab,
  readLoreOverviewWidth,
  writeLoreOverviewTab,
  writeLoreOverviewWidth,
} from './preferences'
import { useLoreOverview } from './use-lore-overview'
import { RelationshipList, RelationshipView } from './RelationshipView'

interface LoreOverviewPanelProps {
  storyId: string
  branchId: string
  /** True while the panel is on screen. It is what starts the reads. */
  active: boolean
  /** The player-known snapshot the rest of the stage already reads. */
  snapshot: Snapshot | null
  /** Collapses the panel. It settles nothing and advances nothing. */
  onClose: () => void
  /** The story's opening situation, so the world tab can name it. */
  origin?: string
  worldName?: string
  /**
   * The project this story's world belongs to. A person's world-book picture is
   * stored as a project-relative path, so it is addressed through this project
   * rather than turned into a URL by the read.
   */
  projectId?: string
  /**
   * False inside the narrow-screen sheet. There the panel is the whole layer,
   * so it takes the sheet's width and offers no drag edge of its own.
   */
  resizable?: boolean
  className?: string
  /**
   * A person to open on arrival, from somewhere else in the game that already
   * named them — a chat window offering to show whose conversation this is. It
   * opens their entry on the people tab; it is still a read, and the player can
   * leave it for any other tab.
   */
  focusEntryId?: string
}

/**
 * 资料总览: what this character knows, in six reading categories.
 *
 * It is a view and nothing else. There is no accept, hand in, collect, travel,
 * talk or fight control here, because those are the player's intentions and go
 * through the main story's own input and the formal path. Opening, searching,
 * switching tabs and closing are all reads: no world time passes, no turn is
 * generated, no model is called, and the draft in the composer is untouched.
 *
 * Every list is built after the backend has already withheld what the character
 * does not know, so a search term or a count can never reveal that a secret
 * exists.
 */
export function LoreOverviewPanel({
  storyId, branchId, active, snapshot, onClose, origin, worldName, projectId, resizable = true, className, focusEntryId,
}: LoreOverviewPanelProps) {
  const { t } = useTranslation()
  // A turn that commits while the panel is open moves the story on, so the reads
  // follow the committed head. It is still a read: no turn is asked for here.
  const read = useLoreOverview({
    storyId, branchId, active, revision: `${snapshot?.context_revision ?? ''}:${snapshot?.current_turn?.id || ''}`,
  })
  const [tab, setTab] = useState<LoreTabKey>('player')
  const [query, setQuery] = useState('')
  const [peopleView, setPeopleView] = useState<'directory' | 'relations'>('directory')
  // Which entry is open, per tab. The panel keeps one at a time so a jump
  // replaces the detail rather than stacking a second layer over it.
  const [openId, setOpenId] = useState('')
  const [returnTab, setReturnTab] = useState<LoreTabKey | null>(null)
  const [width, setWidth] = useState(() => readLoreOverviewWidth())
  // The room the panel may take. It is measured from the column the panel sits
  // in, so a window that shrinks after the reader chose a width pulls the panel
  // back in rather than crushing the prose.
  const [ceiling, setCeiling] = useState(MAX_LORE_OVERVIEW_WIDTH)
  const dragRef = useRef<{ pointerId: number; startX: number; startWidth: number } | null>(null)
  const rootRef = useRef<HTMLDivElement | null>(null)

  // §2 says Esc closes the layer that has focus, which only means anything if
  // opening the panel hands it the focus. Without this the panel would open
  // behind the reader's cursor and an Esc would go nowhere.
  useEffect(() => {
    rootRef.current?.focus()
  }, [])

  useEffect(() => {
    if (!resizable) return
    const column = rootRef.current?.closest('[data-story-stage-content]')
    if (!column || typeof ResizeObserver === 'undefined') return
    const measure = () => {
      const available = column.getBoundingClientRect().width
      if (available <= 0) return
      setCeiling(Math.min(MAX_LORE_OVERVIEW_WIDTH, Math.max(MIN_LORE_OVERVIEW_WIDTH, available - MIN_STORY_BODY_WIDTH)))
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(column)
    return () => observer.disconnect()
  }, [resizable])

  // The last tab read in this story and branch, so returning puts the reader
  // back where they were. A tab saved by another story is not reused.
  useEffect(() => {
    const saved = readLoreOverviewTab(storyId, branchId)
    setTab(isLoreTabKey(saved) ? saved : 'player')
    setOpenId('')
    setReturnTab(null)
    setPeopleView('directory')
    setQuery('')
  }, [storyId, branchId])

  // Someone else in the game already named this person, so the panel arrives on
  // them instead of making the reader find them again in a list of a hundred.
  useEffect(() => {
    if (!focusEntryId) return
    setTab('people')
    setOpenId(focusEntryId)
    setReturnTab(null)
    setQuery('')
  }, [focusEntryId])

  const model = useMemo(() => buildLoreOverview({
    snapshot: read.data?.snapshot ?? null,
    characters: read.data?.characters || null,
    journey: read.data?.journey || null,
    quests: read.data?.quests || null,
    events: read.data?.events || null,
    origin,
    worldName,
  }), [origin, read.data, snapshot, worldName, storyId, branchId])

  const entries = model.byTab[tab]
  const visible = useMemo(() => filterLoreEntries(entries, query), [entries, query])
  const open = openId ? model.entries.find((entry) => entry.id === openId) || null : null

  const selectTab = (next: string) => {
    if (!isLoreTabKey(next)) return
    setTab(next)
    setOpenId('')
    setReturnTab(null)
    setQuery('')
    writeLoreOverviewTab(storyId, branchId, next)
  }

  const openEntry = (entry: LoreEntry) => {
    setOpenId(entry.id)
    setReturnTab(tab)
    setTab(entry.tab)
  }

  const backToList = () => {
    if (returnTab) setTab(returnTab)
    setOpenId('')
    setReturnTab(null)
  }

  // The panel widens by dragging its own left edge. The bound is not a taste:
  // §2 requires the prose to stay readable, so the drag stops while there is
  // still room for the story beside the panel, and a narrow window never lets
  // the panel eat the column it is meant to sit next to.
  const startResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    dragRef.current = { pointerId: event.pointerId, startX: event.clientX, startWidth: width }
    event.currentTarget.setPointerCapture?.(event.pointerId)
    event.preventDefault()
  }, [width])

  const moveResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    setWidth(Math.min(ceiling, Math.max(MIN_LORE_OVERVIEW_WIDTH, drag.startWidth - (event.clientX - drag.startX))))
  }, [ceiling])

  const endResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    event.currentTarget.releasePointerCapture?.(event.pointerId)
    dragRef.current = null
    setWidth((current) => {
      writeLoreOverviewWidth(current)
      return current
    })
  }, [])

  return (
    <div
      ref={rootRef}
      tabIndex={-1}
      data-testid="lore-overview-panel"
      // The stored width is the reader's preference and survives a narrow
      // window; what is drawn is that width held under the ceiling this column
      // can afford, so the prose keeps its reading room instead of being
      // squeezed to a sliver.
      style={resizable ? { width: `min(${Math.min(width, ceiling)}px, 92vw)` } : undefined}
      // §2: Esc closes only the layer that has focus. The detail is a layer
      // inside the panel, so Esc there goes back to the list, and Esc from the
      // list closes the panel. It is handled on the panel itself, so an Esc
      // pressed elsewhere in the stage is left alone.
      onKeyDown={(event) => {
        if (event.key !== 'Escape') return
        event.stopPropagation()
        if (open) {
          backToList()
          return
        }
        onClose()
      }}
      className={cn('relative flex h-full min-h-0 flex-col bg-[var(--nova-surface)] text-[var(--nova-text)]', className)}
    >
      {resizable ? (
        <div
          role="separator"
          aria-orientation="vertical"
          aria-label={t('loreOverview.resize')}
          tabIndex={0}
          data-lore-overview-resize
          onPointerDown={startResize}
          onPointerMove={moveResize}
          onPointerUp={endResize}
          onPointerCancel={endResize}
          onKeyDown={(event) => {
            const step = event.shiftKey ? 64 : 16
            if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
            event.preventDefault()
            setWidth((current) => {
              const next = Math.min(ceiling, Math.max(MIN_LORE_OVERVIEW_WIDTH,
                current + (event.key === 'ArrowLeft' ? step : -step)))
              writeLoreOverviewWidth(next)
              return next
            })
          }}
          className="absolute inset-y-3 left-0 z-20 w-2 cursor-ew-resize touch-none"
        >
          <span aria-hidden className="absolute inset-y-0 left-0 w-1 rounded-full bg-[var(--nova-border)] opacity-0 transition-opacity hover:opacity-100" />
        </div>
      ) : null}
      <header className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2">
        {open ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={backToList}
            aria-label={t('loreOverview.back')}
            title={t('loreOverview.back')}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
        ) : null}
        <h2 className="min-w-0 flex-1 truncate text-[13px] font-medium">
          {open ? <EntryTitle entry={open} /> : t('loreOverview.title')}
        </h2>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={onClose}
          aria-label={t('loreOverview.close')}
          title={t('loreOverview.closeHint')}
        >
          <X className="h-4 w-4" />
        </Button>
      </header>

      {read.error ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center">
          <p className="text-[12px] text-[var(--nova-danger)]">{t('loreOverview.loadFailed')}</p>
          <p className="text-[11px] text-[var(--nova-text-faint)]">{read.error === 'loreOverview.changedDuringRead' ? t(read.error) : read.error}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void read.reload()}>
            {t('loreOverview.retry')}
          </Button>
        </div>
      ) : read.loading && !read.data ? (
        <div className="flex flex-1 items-center justify-center gap-2 text-[12px] text-[var(--nova-text-muted)]">
          <Spinner className="h-3.5 w-3.5" />
          {t('loreOverview.loading')}
        </div>
      ) : (
        <>
          <Tabs value={tab} onValueChange={selectTab} className="flex min-h-0 flex-1 flex-col gap-0">
            <TabsList variant="line" className="w-full shrink-0 justify-start gap-0 overflow-x-auto rounded-none border-b border-[var(--nova-border)] px-1">
              {LORE_TABS.map((key) => (
                <TabsTrigger key={key} value={key} className="shrink-0 px-2 text-[11px]">
                  {t(`loreOverview.tab.${key}`)}
                </TabsTrigger>
              ))}
            </TabsList>

            <div className="flex min-h-0 flex-1 flex-col">
              {open ? (
                <EntryDetail
                  entry={open}
                  projectId={projectId}
                  onOpenLink={(entryId, entryTab) => {
                    setReturnTab((current) => current ?? tab)
                    setOpenId(entryId)
                    setTab(entryTab)
                  }}
                  onBack={backToList}
                />
              ) : (
                <>
                  {tab === 'people' ? <div className="flex shrink-0 flex-wrap gap-1 px-3 pt-2">
                    <Button size="sm" variant={peopleView === 'directory' ? 'secondary' : 'ghost'} aria-pressed={peopleView === 'directory'} onClick={() => setPeopleView('directory')}>{t('loreOverview.peopleDirectory')}</Button>
                    <Button size="sm" variant={peopleView === 'relations' ? 'secondary' : 'ghost'} aria-pressed={peopleView === 'relations'} onClick={() => setPeopleView('relations')}>{t('loreOverview.relationships.title')}</Button>
                  </div> : null}
                  {tab === 'people' && peopleView === 'relations' ? <div className="min-h-0 flex-1 overflow-y-auto p-3">
                    <RelationshipView key={`${storyId}:${branchId}`} people={model.knownPeople.people} relations={model.knownPeople.relations} onOpenPerson={id => {
                      const entry = model.entries.find(item => item.id === (id === 'protagonist' ? 'actor:protagonist' : id))
                      if (entry) openEntry(entry)
                    }} />
                  </div> : <>
                  <div className="relative shrink-0 px-3 py-2">
                    <Search aria-hidden className="pointer-events-none absolute left-5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--nova-text-faint)]" />
                    <Input
                      value={query}
                      onChange={(event) => setQuery(event.target.value)}
                      placeholder={t('loreOverview.searchPlaceholder')}
                      aria-label={t('loreOverview.searchPlaceholder')}
                      className="h-7 pl-7 text-[11px]"
                    />
                  </div>
                  <div className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto px-3 pb-2">
                    {visible.length === 0 ? (
                      <p className="py-3 text-[11px] text-[var(--nova-text-faint)]">
                        {entries.length === 0 ? t('loreOverview.empty') : t('loreOverview.noMatch')}
                      </p>
                    ) : (
                      visible.map((entry) => (
                        <EntryRow key={entry.id} entry={entry} projectId={projectId} onOpen={() => openEntry(entry)} />
                      ))
                    )}
                  </div>
                  </>}
                </>
              )}
            </div>
          </Tabs>

          <footer className="shrink-0 border-t border-[var(--nova-border)] px-3 py-2">
            <p className="text-[11px] text-[var(--nova-text-faint)]">{t('loreOverview.hint')}</p>
          </footer>
        </>
      )}
    </div>
  )
}

function isLoreTabKey(value: string | null): value is LoreTabKey {
  return Boolean(value) && (LORE_TABS as string[]).includes(value as string)
}

function EntryTitle({ entry }: { entry: LoreEntry }) {
  const { t } = useTranslation()
  return <>{entry.titleKey ? t(entry.titleKey) : entry.title}</>
}

/**
 * One entry as a directory row. The lore library's 本局新建 page lists the same
 * entries the overview does, so it draws them with this rather than a second
 * renderer that could disagree with it.
 */
export function EntryRow({ entry, projectId, onOpen }: { entry: LoreEntry; projectId?: string; onOpen: () => void }) {
  const { t } = useTranslation()
  const subtitle = entry.subtitleKey ? t(entry.subtitleKey) : entry.subtitle
  const title = entry.titleKey ? t(entry.titleKey) : entry.title
  // Only a person carries a picture. The other tabs name a road or a matter,
  // and an initial in a circle would say nothing a reader asked for.
  const portrait = entry.tab === 'people' ? (
    <PersonAvatar avatar={entry.avatar} lorePath={entry.avatarLorePath} projectId={projectId} name={title} size="sm" />
  ) : null
  return (
    <button
      type="button"
      onClick={onOpen}
      data-lore-entry={entry.id}
      className="flex min-w-0 items-start gap-2 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-left transition-colors hover:bg-[var(--nova-hover)]"
    >
      {portrait}
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="min-w-0 truncate text-[12px] text-[var(--nova-text)]">
          {title}
        </span>
        {subtitle ? <span className="min-w-0 truncate text-[11px] text-[var(--nova-text-muted)]">{subtitle}</span> : null}
        {entry.statusKey ? (
          <span className="min-w-0 truncate text-[11px] text-[var(--nova-text-faint)]">{t(entry.statusKey)}</span>
        ) : null}
        {entry.lastKnownLocation ? <span className="line-clamp-2 text-[11px] text-[var(--nova-text-faint)]">{t('loreOverview.row.lastKnownLocation')}：{entry.lastKnownLocation}</span> : null}
        {entry.affiliations?.length ? <span className="line-clamp-2 text-[11px] text-[var(--nova-text-muted)]">{t('loreOverview.row.affiliations')}：{entry.affiliations.join(' · ')}</span> : null}
        {entry.tab === 'factions' ? <span className="text-[11px] text-[var(--nova-text-faint)]">{t('loreOverview.knownMembersCount', { count: entry.members?.length || 0 })}</span> : null}
        {entry.summary ? (
          <span className="line-clamp-2 text-[11px] text-[var(--nova-text-faint)]">{entry.summary}</span>
        ) : null}
      </span>
      <ChevronRight aria-hidden className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
    </button>
  )
}

/**
 * A display-only projection deliberately withholds every roster image source.
 * Story-managed portraits use that contract for auto_show:false. Do not infer
 * permission from a name, world or URL pattern: require a separate image open.
 * The parent keys this state by actor and media identity, including scoped URLs.
 */
function EntryPortrait({ entry, projectId, name }: { entry: LoreEntry; projectId?: string; name: string }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const displayOnly = Boolean(entry.avatarDisplay && !personAvatarSource(entry.avatar, entry.avatarLorePath, projectId))
  const portrait = (
    <PersonPortrait
      avatar={entry.avatarDisplay || entry.avatar}
      lorePath={entry.avatarLorePath}
      projectId={projectId}
      name={name}
      caption={entry.imageCaption}
    />
  )
  if (!displayOnly) return portrait
  return (
    <div className="flex shrink-0 flex-col items-start gap-1.5">
      <button
        type="button"
        data-testid="lore-overview-portrait-toggle"
        aria-expanded={open}
        onClick={() => setOpen(value => !value)}
        className="text-left text-[11px] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
      >
        {open ? t('characterWindow.portraitHide') : t('characterWindow.portraitShow', { name })}
      </button>
      {open ? portrait : null}
    </div>
  )
}

/**
 * One entry's full detail. The lore library's 本局新建 page shows the same
 * fields the overview shows, from the same model, so a fact reads the same in
 * both places.
 */
export function EntryDetail({ entry, projectId, onOpenLink, onBack, onCollect, picture, settings }: {
  entry: LoreEntry
  projectId?: string
  onOpenLink: (entryId: string, tab: LoreTabKey) => void
  onBack: () => void
  // Collecting is offered by the author library, which is where a story's own
  // entity can be filed into the world it was played in. The read-only overview
  // passes nothing, so the same detail view carries no write affordance there.
  onCollect?: () => void
  // The material control for this entry, when the page has one to offer. Only
  // the author library passes it: the read-only overview has no media library
  // and must not draw a button that could not work.
  picture?: ReactNode
  // What the player filed this entity under and how much of it the model reads.
  // Same rule as picture: only the author library offers it, because the
  // overview is a reading surface and must not look editable.
  settings?: ReactNode
}) {
  const { t } = useTranslation()
  const subtitle = entry.subtitleKey ? t(entry.subtitleKey) : entry.subtitle
  const title = entry.titleKey ? t(entry.titleKey) : entry.title
  // §4: a person's detail opens on their picture beside their name and what the
  // player knows of them. Desktop puts them side by side, a narrow panel stacks
  // them, and the picture keeps its own proportions rather than being cropped to
  // a shape the world never gave it.
  const isPerson = entry.tab === 'people'
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-3 py-2">
      {isPerson ? (
        <div className="flex flex-col items-start gap-2 sm:flex-row sm:items-center">
          <EntryPortrait
            key={JSON.stringify([entry.id, entry.identitySource, entry.avatar, entry.avatarDisplay, entry.avatarLorePath, entry.imageCaption, projectId])}
            entry={entry}
            projectId={projectId}
            name={title}
          />
          <div className="flex min-w-0 flex-col gap-0.5">
            <p className="min-w-0 truncate text-[13px] text-[var(--nova-text)]">{title}</p>
            {subtitle ? <p className="text-[11px] text-[var(--nova-text-muted)]">{subtitle}</p> : null}
          </div>
        </div>
      ) : subtitle ? (
        <p className="text-[11px] text-[var(--nova-text-muted)]">{subtitle}</p>
      ) : null}
      {entry.statusKey ? <p className="text-[11px] text-[var(--nova-text-faint)]">{t(entry.statusKey)}</p> : null}
      {entry.summary ? <p className="text-[11px] text-[var(--nova-text-faint)]">{entry.summary}</p> : null}

      {entry.groups?.map((group) => <GroupGrid key={group.key} group={group} />)}

      {entry.rows && entry.rows.length > 0 ? (
        <div className="flex flex-col gap-1.5">
          {entry.rows.map((row) => <RowView key={row.id} row={row} />)}
        </div>
      ) : null}

      {entry.memberships ? <AffiliationList affiliations={entry.memberships} onOpen={id => onOpenLink(`world:势力记录:${id}`, 'factions')} /> : null}
      {entry.factionId ? <p className="break-all text-[11px] text-muted-foreground">{t('loreOverview.membership.identity')}：{entry.factionId}</p> : null}
      {isPerson ? <><ProvenanceDetails value={entry.locationProvenance} /><p className="text-[11px] text-muted-foreground">{t('loreOverview.locationHint')}</p></> : null}
      {entry.tab === 'factions' ? <section className="flex flex-col gap-1.5 border-t border-[var(--nova-border)] pt-2">
        <h3 className="text-xs font-medium">{t('loreOverview.knownMembers')}</h3>
        <p className="text-[11px] text-muted-foreground">{t('loreOverview.locationHint')}</p>
        {entry.members?.length ? entry.members.map(person => <div key={person.id} className="min-w-0 rounded border border-[var(--nova-border)] p-2 text-xs">
          <button type="button" className="break-words text-left text-[var(--nova-accent)] hover:underline" onClick={() => onOpenLink(person.id, 'people')}>{person.name}</button>
          <span className="mt-1 block break-words text-muted-foreground">{t(person.seenNow ? 'loreOverview.row.observedLocation' : 'loreOverview.row.lastKnownLocation')}：{person.lastKnownLocation || t('loreOverview.notKnown')}</span>
          <ProvenanceDetails value={person.locationProvenance} />
        </div>) : <p className="text-xs text-muted-foreground">{t('loreOverview.noKnownMembers')}</p>}
      </section> : null}
      {entry.tab === 'factions' ? (['pendingMembers', 'pastMembers', 'unresolvedMembers'] as const).map(group => entry[group]?.length ? <section key={group} className="flex flex-col gap-1.5 border-t border-[var(--nova-border)] pt-2">
        <h3 className="text-xs font-medium">{t(`loreOverview.${group}`)}</h3>
        {entry[group]!.map(person => <button type="button" key={person.id} className="break-words text-left text-xs text-[var(--nova-accent)] hover:underline" onClick={() => onOpenLink(person.id, 'people')}>{person.name}</button>)}
      </section> : null) : null}
      {entry.relations ? <section className="flex min-w-0 flex-col gap-1.5 border-t border-[var(--nova-border)] pt-2">
        <h3 className="text-xs font-medium">{t('loreOverview.relationships.title')}</h3>
        <RelationshipList people={entry.relationPeople || []} relations={entry.relations} onOpenPerson={id => onOpenLink(id === 'protagonist' ? 'actor:protagonist' : id, id === 'protagonist' ? 'player' : 'people')} />
      </section> : null}

      {picture ? <div className="border-t border-[var(--nova-border)] pt-2">{picture}</div> : null}

      {settings ? <div className="border-t border-[var(--nova-border)] pt-2">{settings}</div> : null}

      {entry.links && entry.links.length > 0 ? (
        <div className="flex flex-wrap items-baseline gap-1.5 border-t border-[var(--nova-border)] pt-2">
          <span className="text-[11px] text-[var(--nova-text-muted)]">{t('loreOverview.related')}</span>
          {entry.links.map((link) => (
            <button
              key={`${link.tab}:${link.entryId}`}
              type="button"
              className="text-[11px] text-[var(--nova-accent)] underline-offset-2 hover:underline"
              onClick={() => onOpenLink(link.entryId, link.tab)}
            >
              {link.label}
            </button>
          ))}
        </div>
      ) : null}

      <div className="mt-1 flex flex-wrap items-center gap-1.5">
        <Button type="button" variant="ghost" size="sm" className="self-start" onClick={onBack}>
          {t('loreOverview.backToList')}
        </Button>
        {onCollect ? (
          <Button type="button" variant="outline" size="sm" className="self-start" onClick={onCollect}>
            {t('settingPanel.library.collectToWorld')}
          </Button>
        ) : null}
      </div>
    </div>
  )
}

function GroupGrid({ group }: { group: LedgerFieldGroup }) {
  const { t } = useTranslation()
  const label = group.custom ? group.key : t(`storyStage.state.group.${group.key}`)
  return (
    <div className="story-state-ledger__grid" data-group={group.custom ? 'custom' : group.key}>
      {group.fields.map((item) => <LedgerFieldView key={item.id} item={item} groupLabel={label} />)}
    </div>
  )
}

function RowView({ row }: { row: LoreRow }) {
  const { t } = useTranslation()
  const label = row.labelKey ? t(row.labelKey) : row.label
  if (row.withheld) {
    return (
      <div className="flex flex-col gap-0.5">
        <span className="text-[11px] text-[var(--nova-text-faint)]">{label}</span>
        <span className="text-[11px] text-[var(--nova-text-faint)]">{t('storyStage.state.valueUnknown')}</span>
      </div>
    )
  }
  // An absent value is stated as not yet known rather than drawn as a blank.
  // §4 keeps "no record" apart from "zero": an empty deadline is not a deadline
  // of nothing, and a row that says 未约定期限 says something different again.
  const text = row.valueKey
    ? `${t(row.valueKey)}${row.valueCount === undefined ? '' : ` × ${row.valueCount}`}${row.valueSuffix || ''}`
    : rawText(row.value)
  if (row.block) {
    return (
      <div className="flex flex-col gap-0.5">
        <span className="text-[11px] text-[var(--nova-text-faint)]">{label}</span>
        <p className="story-state-ledger__block-text">{text || t('loreOverview.notKnown')}</p>
      </div>
    )
  }
  return (
    <div className="flex items-baseline justify-between gap-3">
      <span className="shrink-0 text-[11px] text-[var(--nova-text-faint)]">{label}</span>
      <span className="min-w-0 truncate text-right text-[11px] text-[var(--nova-text)]">
        {text || t('loreOverview.notKnown')}
      </span>
    </div>
  )
}

function rawText(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  return ''
}
