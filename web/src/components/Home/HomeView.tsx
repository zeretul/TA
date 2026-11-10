import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { originCopy, visibleCreationOrigins } from '@/lib/pack-origin-copy'
import {
  AlertTriangle,
  ArrowRight,
  BookOpen,
  Globe2,
  Layers,
  LibraryBig,
  Loader2,
  Plus,
  RefreshCw,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { toast } from '@/lib/toast'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { formatDateTime } from '@/i18n'
import { cn } from '@/lib/utils'
import { getStoryPack, listStoryPacks, type StoryPackIdentityChoice, type StoryPackSummary } from '@/lib/api-client'
import {
  deleteInteractiveStory,
  getGamePlanningTemplates,
  getInteractiveStories,
  getInteractiveTellers,
} from '@/features/interactive/api'
import { gamePlanningTemplateName } from '@/features/interactive/game-planning'
import { narrativeStyleName } from '@/features/interactive/narrative-style'
import type { GamePlanningTemplate, StorySummary, Teller } from '@/features/interactive/types'
import { useInteractiveStore } from '@/features/interactive/stores/interactive-store'
import { CharacterCreationDialog } from './CharacterCreationDialog'
import { WorldCreationDialog } from './WorldCreationDialog'
import { worldSettingsValue } from './WorldSettingsEditor'

interface HomeViewProps {
  /** The open Project; a story row that matches it is the one already loaded. */
  projectId: string
  /** Starts a story in a new world from this pack, optionally as a created character. */
  onStartStory: (packId: string, templateId?: string, identity?: StoryPackIdentityChoice) => void | Promise<void>
  /** Opens an existing story and its world. */
  onOpenStory: (story: StorySummary) => void | Promise<void>
  /** Sends the player to game settings, the one page that edits a world's defaults. */
  onOpenGameSettings?: () => void
  /** Drops the cached pack catalog so a just-installed pack shows up. */
  refreshKey?: number
  /** Creation picker hides saved-story navigation and settings destinations. */
  newGameOnly?: boolean
}

type Tab = 'overview' | 'settings' | 'stories'

/**
 * The world page: choose a world, read it, see what a new game in it starts
 * from, load it, and either create a character or continue a story already
 * saved in it.
 *
 * The page keeps three actions apart on purpose. Selecting a world only shows
 * its detail. Loading resolves the pack and its modules on the server but creates
 * nothing. Starting is the single write: it makes the Project and the story once.
 *
 * Editing a world's defaults is not one of those actions. There is one editor,
 * in game settings, because a second editable copy of the same block is how two
 * pages drift into disagreeing about what a world's defaults are. This page
 * shows the resolved values read-only and links to that editor.
 */
export function HomeView({ projectId, onStartStory, onOpenStory, onOpenGameSettings, refreshKey = 0, newGameOnly = false }: HomeViewProps) {
  const { t } = useTranslation()
  const [packs, setPacks] = useState<StoryPackSummary[]>([])
  const [packsLoading, setPacksLoading] = useState(true)
  const [packsError, setPacksError] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const [tab, setTab] = useState<Tab>('overview')
  // With no world prepared, a player who already has saves still needs a way to
  // reach them; the list is shown in the detail pane rather than a new page.
  const [showAllStories, setShowAllStories] = useState(false)

  const [tellers, setTellers] = useState<Teller[]>([])
  const [planningTemplates, setPlanningTemplates] = useState<GamePlanningTemplate[]>([])

  const [stories, setStories] = useState<StorySummary[]>([])
  const [storiesLoading, setStoriesLoading] = useState(true)
  const [storiesError, setStoriesError] = useState('')

  // Loading is per world and remembered, so returning to a loaded world does not
  // ask the player to load it again. It is still only a prepared state.
  const [loadedIds, setLoadedIds] = useState<Set<string>>(() => new Set())
  const [loadingPackId, setLoadingPackId] = useState('')
  const [loadError, setLoadError] = useState('')
  const [showProblem, setShowProblem] = useState(false)
  const [startingPackId, setStartingPackId] = useState('')
  const [creationOpen, setCreationOpen] = useState(false)
  // Creating a world is a separate act from starting one: it writes a new pack
  // and never a Project or a story.
  const [worldCreationOpen, setWorldCreationOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<StorySummary | null>(null)
  const [openingStoryId, setOpeningStoryId] = useState('')

  // Identifies the newest saved-story request, so a slow answer from a world the
  // player has already left is dropped instead of filling the new list.
  const storyRequestRef = useRef(0)
  const currentStoryId = useInteractiveStore((state) => state.currentStoryId)

  const selectedPack = useMemo(
    () => packs.find((pack) => pack.id === selectedId) || null,
    [packs, selectedId],
  )
  const loaded = selectedPack ? loadedIds.has(selectedPack.id) : false
  const loading = Boolean(selectedPack) && loadingPackId === selectedPack?.id
  const starting = Boolean(selectedPack) && startingPackId === selectedPack?.id
  const blocked = Boolean(selectedPack?.invalid) || (selectedPack?.missing ?? 0) > 0
  const canCreate = Boolean(selectedPack?.creation)

  // The defaults this world hands to a new game, read for the summary. The
  // editor lives in game settings; this only names what it would start from.
  const defaults = useMemo(() => worldSettingsValue(selectedPack?.settings), [selectedPack])
  const defaultsSummary = useMemo(() => {
    const template = planningTemplates.find((item) => item.id === defaults.planningTemplateId)
    const style = tellers.find((item) => item.id === defaults.styleMain)
    return {
      planning: defaults.planningMode === 'disabled'
        ? t('worldPack.defaults.planningOff')
        : template
          ? gamePlanningTemplateName(template, t)
          : defaults.planningTemplateId,
      style: style ? narrativeStyleName(style, t) : defaults.styleMain,
      length: defaults.replyTargetChars,
    }
  }, [defaults, planningTemplates, t, tellers])

  const packStories = useMemo(
    () => stories.filter((story) => story.pack_binding?.pack_id === selectedId),
    [stories, selectedId],
  )
  const styleNames = useMemo(() => {
    const map = new Map<string, string>()
    for (const teller of tellers) map.set(teller.id, narrativeStyleName(teller, t))
    return map
  }, [t, tellers])

  const loadPacks = useCallback(async () => {
    setPacksLoading(true)
    setPacksError('')
    try {
      setPacks(await listStoryPacks())
    } catch (reason) {
      console.error('[world-pack] Failed to list world packs', reason)
      setPacksError(reason instanceof Error ? reason.message : String(reason))
    } finally {
      setPacksLoading(false)
    }
  }, [])

  const loadStories = useCallback(async () => {
    // Saved stories belong to a workspace. With no workspace open the request
    // cannot succeed — the server answers 409 because it has no context to
    // answer from — so the page shows its own empty state instead of asking a
    // question that is certain to fail. The server-side check stays: a request
    // that really arrives without context is still refused.
    if (!projectId) {
      setStories([])
      setStoriesError('')
      setStoriesLoading(false)
      return
    }
    const request = ++storyRequestRef.current
    setStoriesLoading(true)
    setStoriesError('')
    try {
      const index = await getInteractiveStories()
      // A response that arrives after the player switched worlds describes the
      // previous one, so it is dropped rather than painted over the new list.
      if (request !== storyRequestRef.current) return
      setStories(index.stories || [])
    } catch (reason) {
      if (request !== storyRequestRef.current) return
      console.error('[world-pack] Failed to list saved stories', reason)
      setStoriesError(reason instanceof Error ? reason.message : String(reason))
    } finally {
      if (request === storyRequestRef.current) setStoriesLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    void loadPacks()
  }, [loadPacks, refreshKey])

  useEffect(() => {
    void loadStories()
  }, [loadStories, refreshKey, projectId])

  // The style and planning catalogs are shared with the game surface; they name
  // the world's defaults and the style a saved story was told in.
  useEffect(() => {
    let cancelled = false
    Promise.all([getInteractiveTellers(), getGamePlanningTemplates()])
      .then(([nextTellers, nextTemplates]) => {
        if (cancelled) return
        setTellers(nextTellers)
        setPlanningTemplates(nextTemplates)
      })
      .catch((reason) => {
        // The page still works: selects fall back to ids and the player can
        // retry by refreshing the catalog.
        console.error('[world-pack] Failed to read style and planning catalogs', reason)
      })
    return () => { cancelled = true }
  }, [refreshKey])

  // With a single prepared world the detail should be open, not an empty pane.
  useEffect(() => {
    if (selectedId || packs.length === 0) return
    setSelectedId(packs[0].id)
  }, [packs, selectedId])

  const selectPack = (packId: string) => {
    if (packId === selectedId) return
    setSelectedId(packId)
    setTab('overview')
  }

  const loadPack = async () => {
    if (!selectedPack || loadingPackId || blocked) return
    setLoadingPackId(selectedPack.id)
    setLoadError('')
    setShowProblem(false)
    try {
      await getStoryPack(selectedPack.id)
      setLoadedIds((current) => {
        const next = new Set(current)
        next.add(selectedPack.id)
        return next
      })
      toast.success(t('worldPack.loadedToast', { name: selectedPack.name }))
    } catch (reason) {
      console.error('[world-pack] Failed to load the world pack', { packId: selectedPack.id, reason })
      setLoadError(reason instanceof Error ? reason.message : String(reason))
      setShowProblem(true)
    } finally {
      setLoadingPackId('')
    }
  }

  const requestLoad = () => {
    void loadPack()
  }

  const startStory = async (packId: string, choice?: StoryPackIdentityChoice) => {
    if (startingPackId) return
    setStartingPackId(packId)
    try {
      await onStartStory(packId, undefined, choice)
      setCreationOpen(false)
    } finally {
      setStartingPackId('')
    }
  }

  const startDirect = async () => {
    if (!selectedPack) return
    try {
      await startStory(selectedPack.id)
    } catch (reason) {
      console.error('[world-pack] Failed to start the world', { packId: selectedPack.id, reason })
      toast.error(t('worldPack.startFailed'), {
        description: reason instanceof Error ? reason.message : String(reason),
      })
    }
  }

  const openStory = async (story: StorySummary) => {
    if (openingStoryId) return
    setOpeningStoryId(story.id)
    try {
      await onOpenStory(story)
    } catch (reason) {
      console.error('[world-pack] Failed to open the story', { storyId: story.id, reason })
      toast.error(t('worldPack.openFailed'), {
        description: reason instanceof Error ? reason.message : String(reason),
      })
    } finally {
      setOpeningStoryId('')
    }
  }

  const deleteStory = async () => {
    if (!deleteTarget) return
    try {
      await deleteInteractiveStory(deleteTarget.id)
      setStories((current) => current.filter((story) => story.id !== deleteTarget.id))
    } catch (reason) {
      console.error('[world-pack] Failed to delete the story', { storyId: deleteTarget.id, reason })
      toast.error(t('worldPack.deleteFailed'), {
        description: reason instanceof Error ? reason.message : String(reason),
      })
    }
  }

  const renderFooter = () => {
    const primary = (() => {
      if (!selectedPack) return null
      if (loading) {
        return (
          <Button type="button" size="sm" disabled>
            <Loader2 data-icon="inline-start" className="animate-spin" />
            {t('worldPack.loadingPack')}
          </Button>
        )
      }
      if (loaded) {
        if (canCreate) {
          return (
            <Button
              type="button"
              size="sm"
              disabled={starting}
              onClick={() => setCreationOpen(true)}
            >
              {t('worldPack.createCharacter')}
              <ArrowRight data-icon="inline-end" />
            </Button>
          )
        }
        return (
          <Button type="button" size="sm" disabled={starting} onClick={() => void startDirect()}>
            {starting ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Sparkles data-icon="inline-start" />}
            {t('worldPack.startNow')}
          </Button>
        )
      }
      if (loadError) {
        return (
          <Button type="button" size="sm" disabled={blocked} onClick={() => void loadPack()}>
            {t('worldPack.retryLoad')}
          </Button>
        )
      }
      return (
        <Button type="button" size="sm" disabled={blocked} onClick={requestLoad}>
          {t('worldPack.load')}
          <ArrowRight data-icon="inline-end" />
        </Button>
      )
    })()

    const status = !selectedPack
      ? ''
      : loadError
        ? t('worldPack.loadFailedHint')
        : loaded
          ? t('worldPack.loadedNotice', { name: selectedPack.name })
          : t('worldPack.readyToEnter')
    const hint = !selectedPack
      ? ''
      : loaded
        ? t('worldPack.loadedHint')
        : t('worldPack.loadHint')

    return (
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          {status ? <p className="text-xs font-medium text-[var(--nova-text)]">{status}</p> : null}
          {hint ? <p className="mt-0.5 text-[11px] leading-5 text-[var(--nova-text-faint)]">{hint}</p> : null}
          {loadError ? (
            <p className="mt-0.5 break-words text-[11px] leading-5 text-[var(--nova-danger)]">{loadError}</p>
          ) : null}
          {blocked && selectedPack ? (
            <p className="mt-0.5 text-[11px] leading-5 text-[var(--nova-danger)]">
              {selectedPack.invalid
                ? t('worldPack.invalid', { error: selectedPack.error || t('worldPack.unknownPack') })
                : t('worldPack.missingModules', { count: selectedPack.missing ?? 0 })}
            </p>
          ) : null}
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {loadError || blocked ? (
            <Button type="button" variant="ghost" size="sm" onClick={() => setShowProblem((current) => !current)}>
              {t('worldPack.viewProblem')}
            </Button>
          ) : null}
          {!newGameOnly && <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={packStories.length === 0}
            onClick={() => setTab('stories')}
          >
            {t('worldPack.viewStories')}
          </Button>}
          {primary}
        </div>
      </div>
    )
  }

  const renderStoryRow = (story: StorySummary) => {
    const active = Boolean(currentStoryId) && story.id === currentStoryId
    const opening = openingStoryId === story.id
    const branches = story.branches
    const style = styleNames.get(story.story_teller_id) || ''
    const meta = style
      ? t('worldPack.saves.metaNoVersion', { branches, style })
      : t('worldPack.saves.metaPlain', { branches })
    const player = story.protagonist?.name
    return (
      <li
        key={story.id}
        className={cn(
          'flex min-w-0 flex-col gap-1 border-b border-[var(--nova-border)] px-1 py-2.5 sm:flex-row sm:items-center sm:justify-between sm:gap-3',
          active && 'bg-[var(--nova-hover)]',
        )}
      >
        <button
          type="button"
          className="min-w-0 flex-1 text-left outline-none focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]/50"
          disabled={Boolean(openingStoryId)}
          onClick={() => void openStory(story)}
        >
          <span className="flex min-w-0 flex-wrap items-center gap-1.5 text-sm font-medium text-[var(--nova-text)]">
            <span className="min-w-0 truncate">{story.title || t('home.unnamedBook')}</span>
            {active ? (
              <span className="shrink-0 rounded border border-[var(--nova-border)] px-1 text-[10px] font-normal text-[var(--nova-text-muted)]">
                {t('worldPack.saves.current')}
              </span>
            ) : null}
          </span>
          <span className="mt-0.5 block min-w-0 truncate text-[11px] text-[var(--nova-text-muted)]">
            {player
              ? t('worldPack.saves.updatedAt', { name: player, time: formatDateTime(story.updated_at) })
              : formatDateTime(story.updated_at)}
          </span>
          <span className="mt-0.5 block min-w-0 truncate text-[10px] text-[var(--nova-text-faint)]">{meta}</span>
        </button>
        <div className="flex shrink-0 items-center gap-1">
          <Button
            type="button"
            size="xs"
            variant="outline"
            disabled={Boolean(openingStoryId)}
            onClick={() => void openStory(story)}
          >
            {opening ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <ArrowRight data-icon="inline-start" />}
            {t('worldPack.continue')}
          </Button>
          <TooltipIconButton
            label={t('worldPack.delete')}
            className="nova-nav-item text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-danger)]"
            onClick={() => setDeleteTarget(story)}
          >
            <Trash2 className="h-3.5 w-3.5" />
          </TooltipIconButton>
        </div>
      </li>
    )
  }

  return (
    <FeaturePageShell
      icon={LibraryBig}
      title={t('home.title')}
      subtitle={t('worldPack.subtitle')}
      className="nova-sidebar min-w-0 text-[var(--nova-text)]"
      actions={(
        <>
          <Button type="button" variant="ghost" size="xs" onClick={() => setWorldCreationOpen(true)}>
            <Plus data-icon="inline-start" />
            <span className="hidden sm:inline">{t('worldPack.createWorld')}</span>
          </Button>
          <Button type="button" variant="ghost" size="xs" onClick={() => { void loadPacks(); void loadStories() }}>
            <RefreshCw data-icon="inline-start" />
            <span className="hidden sm:inline">{t('worldPack.refresh')}</span>
          </Button>
        </>
      )}
    >
      <ScrollArea className="min-h-0 flex-1">
        <div className="mx-auto grid w-full min-w-0 max-w-full grid-cols-1 gap-4 px-3 py-4 sm:px-5 lg:max-w-6xl lg:grid-cols-[220px_minmax(0,1fr)] lg:items-start">
          <aside className="min-w-0 lg:sticky lg:top-0" aria-label={t('worldPack.prepared')}>
            <div className="mb-2 flex items-center gap-2 px-1 text-[11px] font-medium uppercase text-[var(--nova-text-faint)]">
              <Layers className="h-3.5 w-3.5" aria-hidden="true" />
              {t('worldPack.prepared')}
            </div>

            {packsLoading ? (
              <div className="flex items-center gap-2 px-1 py-4 text-xs text-[var(--nova-text-muted)]">
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                {t('worldPack.loading')}
              </div>
            ) : packsError ? (
              <div role="alert" className="rounded-[var(--nova-radius)] border border-[var(--nova-danger)]/30 px-3 py-3 text-xs text-[var(--nova-danger)]">
                <p className="font-medium">{t('worldPack.loadFailed')}</p>
                <p className="mt-0.5 break-words">{packsError}</p>
                <Button type="button" size="xs" variant="ghost" className="mt-2" onClick={() => void loadPacks()}>
                  {t('common.retry')}
                </Button>
              </div>
            ) : packs.length === 0 ? (
              <div className="flex min-w-0 flex-col gap-2">
                <EmptyState
                  variant="compact"
                  icon={LibraryBig}
                  title={t('worldPack.empty')}
                  description={t('worldPack.emptyDescription')}
                  action={{ label: t('worldPack.createWorld'), onClick: () => setWorldCreationOpen(true) }}
                  className="text-[var(--nova-text-faint)]"
                />
                <Button type="button" variant="ghost" size="xs" onClick={() => void loadPacks()}>
                  <RefreshCw data-icon="inline-start" />
                  {t('worldPack.refresh')}
                </Button>
                {!newGameOnly && stories.length > 0 ? (
                  <Button type="button" variant="ghost" size="xs" onClick={() => setShowAllStories(true)}>
                    {t('worldPack.openStories')}
                  </Button>
                ) : null}
              </div>
            ) : (
              <ul className="flex max-h-64 flex-col gap-1 overflow-y-auto lg:max-h-none lg:overflow-visible">
                {packs.map((pack) => {
                  const active = pack.id === selectedId
                  const count = stories.filter((story) => story.pack_binding?.pack_id === pack.id).length
                  return (
                    <li key={pack.id} className="min-w-0">
                      <button
                        type="button"
                        aria-current={active ? 'true' : undefined}
                        aria-label={t('worldPack.selectWorld', { name: pack.name })}
                        onClick={() => selectPack(pack.id)}
                        className={cn(
                          'relative flex w-full min-w-0 flex-col gap-0.5 rounded-[var(--nova-radius)] border px-2.5 py-2 text-left outline-none transition-colors focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]/50',
                          active
                            ? 'border-[var(--nova-accent)]/45 bg-[var(--nova-hover)]'
                            : 'border-transparent hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)]',
                        )}
                      >
                        <span className="flex min-w-0 items-center gap-1.5">
                          <span className="min-w-0 flex-1 break-words text-sm font-medium text-[var(--nova-text)]" title={pack.name}>
                            {pack.name}
                          </span>
                          {pack.invalid ? (
                            <AlertTriangle className="h-3 w-3 shrink-0 text-[var(--nova-danger)]" aria-hidden="true" />
                          ) : null}
                          {count > 0 ? (
                            <span className="shrink-0 text-[10px] tabular-nums text-[var(--nova-text-faint)]">{count}</span>
                          ) : null}
                        </span>
                        {pack.description ? (
                          <span className="break-words text-[11px] leading-5 text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">
                            {pack.description}
                          </span>
                        ) : null}
                        <span className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[10px] text-[var(--nova-text-faint)]">
                          {(pack.tags ?? []).slice(0, 2).map((tag) => <span key={tag}>{tag}</span>)}
                          <span>{pack.builtin ? t('worldPack.sourceBuiltin') : t('worldPack.sourceCustom')}</span>
                        </span>
                      </button>
                    </li>
                  )
                })}
              </ul>
            )}
            <p className="mt-2 hidden px-1 text-[10px] leading-4 text-[var(--nova-text-faint)] lg:block">
              {t('worldPack.listNote')}
            </p>
          </aside>

          <section
            className="flex min-w-0 flex-col overflow-clip rounded-2xl border border-[var(--nova-border)] bg-[var(--nova-surface)]"
            aria-label={t('worldPack.detailLabel')}
          >
            {!selectedPack ? (
              showAllStories ? (
                <div className="flex min-w-0 flex-col gap-2 px-4 py-4 sm:px-6">
                  <div className="flex min-w-0 items-center justify-between gap-2">
                    <h2 className="text-sm font-semibold text-[var(--nova-text)]">{t('worldPack.saves.title')}</h2>
                    <Button type="button" variant="ghost" size="xs" onClick={() => setShowAllStories(false)}>
                      {t('common.close')}
                    </Button>
                  </div>
                  {storiesLoading ? (
                    <div className="flex items-center gap-2 py-4 text-xs text-[var(--nova-text-muted)]">
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      {t('common.loading')}
                    </div>
                  ) : storiesError ? (
                    <div role="alert" className="rounded-[var(--nova-radius)] border border-[var(--nova-danger)]/30 px-3 py-3 text-xs text-[var(--nova-danger)]">
                      <p className="break-words">{t('worldPack.saves.loadFailed')}：{storiesError}</p>
                      <Button type="button" size="xs" variant="ghost" className="mt-2" onClick={() => void loadStories()}>
                        {t('common.retry')}
                      </Button>
                    </div>
                  ) : stories.length === 0 ? (
                    <EmptyState
                      variant="dashed"
                      icon={BookOpen}
                      title={t('worldPack.saves.empty')}
                      description={t('worldPack.saves.emptyHint')}
                      className="bg-transparent text-[var(--nova-text-faint)]"
                    />
                  ) : (
                    <ul className="flex min-w-0 flex-col border-t border-[var(--nova-border)]">
                      {stories.map(renderStoryRow)}
                    </ul>
                  )}
                </div>
              ) : (
                <div className="flex min-h-64 items-center justify-center px-4 py-10">
                  <EmptyState
                    variant="compact"
                    icon={Globe2}
                    title={packsLoading ? t('worldPack.loading') : packsError ? t('worldPack.loadFailed') : t('worldPack.empty')}
                    description={packsLoading ? undefined : packsError ? undefined : t('worldPack.emptyDescription')}
                    action={packsLoading || packsError
                      ? undefined
                      : { label: t('worldPack.createWorld'), onClick: () => setWorldCreationOpen(true) }}
                    className="text-[var(--nova-text-faint)]"
                  />
                </div>
              )
            ) : (
              <>
                <header className="relative min-w-0 overflow-hidden border-b border-[var(--nova-border)] px-4 pb-4 pt-5 sm:px-6">
                  <div
                    aria-hidden="true"
                    className="pointer-events-none absolute inset-0 bg-[radial-gradient(120%_120%_at_0%_0%,var(--nova-surface-3),transparent_65%)]"
                  />
                  <div className="relative min-w-0">
                    <div className="flex min-w-0 flex-wrap items-center gap-1.5 text-[10px] text-[var(--nova-text-faint)]">
                      {(selectedPack.tags ?? []).slice(0, 3).map((tag) => (
                        <span key={tag} className="rounded border border-[var(--nova-border)] px-1.5 py-0.5">{tag}</span>
                      ))}
                      <span className="rounded border border-[var(--nova-border)] px-1.5 py-0.5">
                        {selectedPack.builtin ? t('worldPack.sourceBuiltin') : t('worldPack.sourceCustom')}
                      </span>
                    </div>
                    <h1 className="mt-2 min-w-0 break-words text-2xl font-semibold leading-tight text-[var(--nova-text)] [font-family:var(--preset-display-font)]">
                      {selectedPack.name}
                    </h1>
                    <div className="mt-2.5 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-[var(--nova-text-faint)]">
                      <span className="flex items-center gap-1">
                        <BookOpen className="h-3 w-3" aria-hidden="true" />
                        {t('worldPack.overview.publicEntries', { count: selectedPack.public_world_count ?? selectedPack.world_count })}
                      </span>
                      <span>{t('worldPack.settings.content.people', { count: selectedPack.character_count })}</span>
                      {canCreate ? <span>{t('worldPack.customOrigin')}</span> : null}
                      <span className={loaded ? 'text-[var(--nova-text-muted)]' : undefined}>
                        {loaded ? t('worldPack.statusLoaded') : t('worldPack.statusUnloaded')}
                      </span>
                    </div>
                  </div>
                </header>

                <Tabs
                  value={tab}
                  onValueChange={(next) => setTab(next as Tab)}
                  className="min-w-0 gap-0"
                >
                  <TabsList
                    variant="line"
                    className="h-11 w-full justify-start gap-5 overflow-x-auto rounded-none border-b border-[var(--nova-border)] bg-transparent px-3 sm:px-5"
                  >
                    <TabsTrigger value="overview" className="flex-none px-0 text-xs">
                      {t('worldPack.tabOverview')}
                    </TabsTrigger>
                    {!newGameOnly && <TabsTrigger value="settings" className="flex-none px-0 text-xs">
                      {t('worldPack.tabSettings')}
                    </TabsTrigger>}
                    {!newGameOnly && <TabsTrigger value="stories" className="flex-none px-0 text-xs">
                      {t('worldPack.tabStories')}
                      {packStories.length > 0 ? (
                        <span className="text-[10px] tabular-nums text-[var(--nova-text-faint)]">{packStories.length}</span>
                      ) : null}
                    </TabsTrigger>}
                  </TabsList>

                  <TabsContent value="overview" className="min-w-0 px-4 py-4 sm:px-6">
                    <div className="flex min-w-0 flex-col gap-5">
                      <section className="min-w-0">
                        <h2 className="text-sm font-semibold text-[var(--nova-text)]">{t('worldPack.overview.playTitle')}</h2>
                        <p className="mt-1.5 max-w-2xl text-xs leading-6 text-[var(--nova-text-muted)]">
                          {selectedPack.description || t('worldPack.overview.noIntroduction')}
                        </p>
                      </section>

                      <section className="min-w-0">
                        <h2 className="text-sm font-semibold text-[var(--nova-text)]">{t('worldPack.overview.startTitle')}</h2>
                        {canCreate && visibleCreationOrigins(selectedPack.creation).length > 0 ? (
                          <ul className="mt-2 grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-3">
                            {visibleCreationOrigins(selectedPack.creation).map((origin) => (
                              <li
                                key={origin.id}
                                className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-2.5 py-2"
                              >
                                <p className="min-w-0 break-words text-xs font-medium text-[var(--nova-text)]">
                                  {origin.name}
                                  {origin.opening_tier ? <span className="ml-2 text-[10px] text-[var(--nova-text-faint)]">{t(origin.opening_tier === 'strong' ? 'worldPack.overview.strongOrigin' : 'worldPack.overview.ordinaryOrigin')}</span> : null}
                                </p>
                                {origin.description ? (
                                  <p className="mt-0.5 break-words text-[11px] leading-5 text-[var(--nova-text-faint)] [overflow-wrap:anywhere]">
                                    {originCopy(origin.description, t(origin.affiliation_kind === 'family' ? 'creation.familyPlaceholderName' : 'creation.affiliationPlaceholderName'))}
                                  </p>
                                ) : null}
                              </li>
                            ))}
                          </ul>
                        ) : (
                          <p className="mt-1.5 max-w-2xl text-xs leading-6 text-[var(--nova-text-muted)]">
                            {t('worldPack.overview.startBody')}
                          </p>
                        )}
                      </section>

                      <section className="min-w-0">
                        <h2 className="text-sm font-semibold text-[var(--nova-text)]">
                          {t(canCreate ? 'worldPack.overview.beforeTitle' : 'worldPack.overview.beforeTitleFixed')}
                        </h2>
                        <p className="mt-1.5 max-w-2xl text-xs leading-6 text-[var(--nova-text-muted)]">
                          {t(canCreate ? 'worldPack.overview.beforeBody' : 'worldPack.overview.beforeBodyFixed')}
                        </p>
                      </section>

                    </div>
                  </TabsContent>

                  <TabsContent value="settings" className="min-w-0 px-4 py-4 sm:px-6">
                    <div className="flex min-w-0 flex-col gap-3">
                      <div className="min-w-0">
                        <h2 className="text-sm font-semibold text-[var(--nova-text)]">{t('worldPack.defaults.title')}</h2>
                        <p className="mt-0.5 max-w-2xl text-[11px] leading-5 text-[var(--nova-text-muted)]">
                          {t('worldPack.defaults.description')}
                        </p>
                      </div>

                      <dl className="flex min-w-0 flex-col border-t border-[var(--nova-border)]">
                        <div className="flex min-w-0 items-baseline justify-between gap-3 border-b border-[var(--nova-border)] py-2">
                          <dt className="shrink-0 text-[11px] text-[var(--nova-text-muted)]">{t('worldPack.defaults.planning')}</dt>
                          <dd className="min-w-0 break-words text-right text-xs text-[var(--nova-text)]">{defaultsSummary.planning}</dd>
                        </div>
                        <div className="flex min-w-0 items-baseline justify-between gap-3 border-b border-[var(--nova-border)] py-2">
                          <dt className="shrink-0 text-[11px] text-[var(--nova-text-muted)]">{t('worldPack.defaults.style')}</dt>
                          <dd className="min-w-0 break-words text-right text-xs text-[var(--nova-text)]">{defaultsSummary.style}</dd>
                        </div>
                        <div className="flex min-w-0 items-baseline justify-between gap-3 border-b border-[var(--nova-border)] py-2">
                          <dt className="shrink-0 text-[11px] text-[var(--nova-text-muted)]">{t('worldPack.defaults.length')}</dt>
                          <dd className="min-w-0 text-right text-xs tabular-nums text-[var(--nova-text)]">{defaultsSummary.length}</dd>
                        </div>
                      </dl>

                      <div className="flex min-w-0 flex-col gap-1">
                        <Button
                          type="button"
                          size="xs"
                          variant="outline"
                          className="self-start"
                          disabled={!onOpenGameSettings}
                          onClick={() => onOpenGameSettings?.()}
                        >
                          {t('worldPack.defaults.openGameSettings')}
                          <ArrowRight className="h-3 w-3" aria-hidden="true" />
                        </Button>
                        <p className="max-w-2xl text-[11px] leading-5 text-[var(--nova-text-faint)]">
                          {t('worldPack.defaults.openGameSettingsHint')}
                        </p>
                      </div>
                    </div>
                  </TabsContent>

                  <TabsContent value="stories" className="min-w-0 px-4 py-4 sm:px-6">
                    <div className="flex min-w-0 flex-col gap-2">
                      <h2 className="text-sm font-semibold text-[var(--nova-text)]">{t('worldPack.saves.title')}</h2>
                      <p className="text-[11px] leading-5 text-[var(--nova-text-muted)]">{t('worldPack.saves.description')}</p>

                      {storiesLoading ? (
                        <div className="flex items-center gap-2 py-4 text-xs text-[var(--nova-text-muted)]">
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          {t('common.loading')}
                        </div>
                      ) : storiesError ? (
                        <div role="alert" className="rounded-[var(--nova-radius)] border border-[var(--nova-danger)]/30 px-3 py-3 text-xs text-[var(--nova-danger)]">
                          <p className="break-words">{t('worldPack.saves.loadFailed')}：{storiesError}</p>
                          <Button type="button" size="xs" variant="ghost" className="mt-2" onClick={() => void loadStories()}>
                            {t('common.retry')}
                          </Button>
                        </div>
                      ) : packStories.length === 0 ? (
                        <EmptyState
                          variant="dashed"
                          icon={BookOpen}
                          title={t('worldPack.saves.empty')}
                          description={t('worldPack.saves.emptyHint')}
                          action={loaded
                            ? {
                                label: canCreate ? t('worldPack.createCharacter') : t('worldPack.startNow'),
                                onClick: () => {
                                  if (canCreate) setCreationOpen(true)
                                  else void startDirect()
                                },
                              }
                            : { label: t('worldPack.load'), onClick: requestLoad }}
                          className="bg-transparent text-[var(--nova-text-faint)]"
                        />
                      ) : (
                        <ul className="flex min-w-0 flex-col border-t border-[var(--nova-border)]">
                          {packStories.map(renderStoryRow)}
                        </ul>
                      )}
                    </div>
                  </TabsContent>
                </Tabs>

                {showProblem && (loadError || blocked) ? (
                  <div role="alert" className="mx-4 mb-3 mt-3 flex min-w-0 items-start gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger)]/30 px-2.5 py-2 text-[11px] text-[var(--nova-danger)] sm:mx-6">
                    <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                    <span className="min-w-0 break-words">
                      {loadError
                        || (selectedPack.invalid
                          ? t('worldPack.invalid', { error: selectedPack.error || t('worldPack.unknownPack') })
                          : t('worldPack.missingModules', { count: selectedPack.missing ?? 0 }))}
                    </span>
                  </div>
                ) : null}

                <footer className="sticky bottom-0 z-10 mt-auto shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3 sm:px-6">
                  {renderFooter()}
                </footer>
              </>
            )}
          </section>
        </div>
      </ScrollArea>

      <CharacterCreationDialog
        projectId={projectId}
        open={creationOpen}
        pack={selectedPack}
        onOpenChange={setCreationOpen}
        onStart={(choice) => {
          if (!selectedPack) return
          return startStory(selectedPack.id, choice)
        }}
      />

      <WorldCreationDialog
        open={worldCreationOpen}
        onOpenChange={setWorldCreationOpen}
        onSaved={(summary) => {
          // The saved world joins the list in place, and is selected, so the
          // player lands on the world they just wrote rather than on the list.
          setPacks((current) => {
            const rest = current.filter((pack) => pack.id !== summary.id)
            return [...rest, summary]
          })
          setSelectedId(summary.id)
          setTab('overview')
          void loadPacks()
        }}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}
        title={t('worldPack.deleteTitle')}
        description={t('worldPack.deleteDescription', { name: deleteTarget?.title || t('home.unnamedBook') })}
        confirmLabel={t('worldPack.deleteConfirm')}
        tone="danger"
        onConfirm={deleteStory}
      />
    </FeaturePageShell>
  )
}
