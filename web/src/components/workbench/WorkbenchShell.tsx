import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { arrayMove } from '@dnd-kit/sortable'
import { Bot, BookOpen, Database, Gamepad2, Search, Settings, SlidersHorizontal } from 'lucide-react'
import { WorkspaceLayout } from '@/components/layout/workspace-layout'
import { WorkspaceMobileLayout, type MobileNavItem } from '@/components/layout/workspace-mobile-layout'
import { createStablePortalHost, StablePortalSlot } from '@/components/layout/stable-portal-slot'
import { MobileWorkspaceHeaderProvider, MobileWorkspaceHeaderOutlet } from '@/components/layout/mobile-workspace-header'
import { closeMobilePanes } from '@/components/layout/mobile-pane-events'
import { Button } from '@/components/ui/button'
import { SidebarProvider } from '@/components/ui/sidebar'
import { useIsMobile } from '@/hooks/useIsMobile'
import type { BookRecord, WorkspaceSummary } from '@/lib/api'
import { useWorkspaceStore, type WorkspaceMode } from '@/stores/workspace-store'
import type { InteractiveSubmode } from '@/features/interactive/types'
import { BookSwitcher } from './BookSwitcher'
import { WorkbenchAppSidebar } from './WorkbenchAppSidebar'
import {
  defaultActivityOrderForScope,
  isActivityItemID,
  mergeVisibleActivityOrder,
  readStoredActivityOrders,
  readStoredHiddenActivityIDs,
  sortActivityItems,
  storeActivityOrder,
  storeHiddenActivityIDs,
  type ActivityItem,
  type ActivityItemID,
  type ActivityOrderScope,
} from './workbench-activity-order'

export type WorkbenchPresentedLayout = 'interactive' | 'full'

interface WorkbenchShellProps {
  mode: WorkspaceMode
  presentedLayout: WorkbenchPresentedLayout
  currentBookName: string
  workspace: string
  books: BookRecord[]
  summary: WorkspaceSummary | null
  activityBarExpanded: boolean
  settingsOpen: boolean
  main: ReactNode
  onSetMode: (mode: WorkspaceMode) => void
  onToggleActivityBarExpanded: () => void
  onSetInteractiveSubmode: (mode: InteractiveSubmode) => void
  onToggleSettings: () => void
  onCloseSettings: () => void
  onQuickSwitchBook: (path: string) => Promise<boolean>
}

type PrimaryNavigationId = ActivityItemID | 'settings'
// User-level width preference; it should survive reloads and development hot updates.
const ACTIVITY_BAR_WIDTH_STORAGE_KEY = 'ta.layout.activityBarWidth'
const ACTIVITY_BAR_COLLAPSED_WIDTH = 48
const ACTIVITY_BAR_MIN_WIDTH = 192
const ACTIVITY_BAR_LEGACY_DEFAULT_WIDTH = 152
const ACTIVITY_BAR_DEFAULT_WIDTH = 224
const ACTIVITY_BAR_MAX_WIDTH = 288
const ACTIVITY_BAR_WIDTH_KEYBOARD_STEP = 8

export function WorkbenchShell({
  mode,
  presentedLayout,
  currentBookName,
  workspace,
  books,
  summary,
  activityBarExpanded,
  settingsOpen,
  main,
  onSetMode,
  onToggleActivityBarExpanded,
  onSetInteractiveSubmode,
  onToggleSettings,
  onCloseSettings,
  onQuickSwitchBook,
}: WorkbenchShellProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const setCommandOpen = useWorkspaceStore((state) => state.setCommandOpen)
  const [activityOrders, setActivityOrders] = useState<Record<ActivityOrderScope, ActivityItemID[]>>(readStoredActivityOrders)
  const [hiddenActivityIDs, setHiddenActivityIDs] = useState<ActivityItemID[]>(readStoredHiddenActivityIDs)
  const [activityBarWidth, setActivityBarWidth] = useState(readStoredActivityBarWidth)
  const [optimisticNavigationId, setOptimisticNavigationId] = useState<PrimaryNavigationId | null>(null)
  const navigationFrameRef = useRef<number | null>(null)
  const [mainContentHost] = useState(() => {
    const host = createStablePortalHost('h-full min-h-0 w-full min-w-0 overflow-hidden')
    if (host) host.dataset.novaWorkbenchMainHost = 'true'
    return host
  })
  const selectPrimaryNavigation = useCallback((id: PrimaryNavigationId, action: () => void) => {
    setOptimisticNavigationId(id)
    if (navigationFrameRef.current !== null) window.cancelAnimationFrame(navigationFrameRef.current)

    // Let the optimistic selected state reach one paint before the workspace performs the
    // potentially expensive route update. Two frames guarantee a paint boundary in browsers.
    navigationFrameRef.current = window.requestAnimationFrame(() => {
      navigationFrameRef.current = window.requestAnimationFrame(() => {
        navigationFrameRef.current = null
        try {
          action()
        } finally {
          setOptimisticNavigationId(null)
        }
      })
    })
  }, [])

  useEffect(() => () => {
    if (navigationFrameRef.current !== null) window.cancelAnimationFrame(navigationFrameRef.current)
  }, [])

  const interactiveModeActive = (mode === 'interactive' || mode === 'history' || mode === 'games') && !settingsOpen
  const loreActive = mode === 'lore' && !settingsOpen
  const adventureContextVisible = mode !== 'history' && mode !== 'games'
  const activityOrderScope: ActivityOrderScope = 'workspace'
  const activityOrder = activityOrders[activityOrderScope]

  const closeSettingsIfOpen = () => {
    if (settingsOpen) onCloseSettings()
  }

  const openGame = () => {
    closeSettingsIfOpen()
    onSetInteractiveSubmode('story')
    onSetMode(mode === 'history' ? 'history' : workspace ? 'interactive' : 'games')
  }

  const openRoute = (nextMode: WorkspaceMode) => {
    closeSettingsIfOpen()
    onSetMode(nextMode)
  }

  const manageBooks = () => {
    openRoute('books')
  }

  const playerActivityItems: ActivityItem[] = [
    {
      id: 'story',
      label: t('workbench.activity.game'),
      onClick: openGame,
      active: interactiveModeActive,
      icon: <Gamepad2 className="h-4 w-4" />,
    },
    {
      id: 'lore',
      label: t('workbench.activity.lore'),
      onClick: () => openRoute('lore'),
      active: loreActive,
      icon: <Database className="h-4 w-4" />,
    },
    {
      id: 'game-settings',
      label: t('workbench.activity.game-settings'),
      onClick: () => openRoute('game-settings'),
      active: mode === 'game-settings' && !settingsOpen,
      icon: <SlidersHorizontal className="h-4 w-4" />,
    },
    {
      id: 'assistant',
      label: t('gameSettings.assistant.title'),
      onClick: () => openRoute('assistant'),
      active: mode === 'assistant' && !settingsOpen,
      icon: <Bot className="h-4 w-4" />,
    },
    {
      id: 'books',
      label: t('workbench.activity.books'),
      onClick: () => openRoute('books'),
      active: mode === 'books' && !settingsOpen,
      icon: <BookOpen className="h-4 w-4" />,
    },
  ]

  const allActivityItems = useMemo(
    () => sortActivityItems(playerActivityItems, activityOrder, defaultActivityOrderForScope(activityOrderScope)),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [activityOrder, activityOrderScope, interactiveModeActive, loreActive, mode, workspace, settingsOpen, t, onCloseSettings, onSetMode, onSetInteractiveSubmode],
  )
  const effectiveHiddenActivityIDs = useMemo(() => {
    const hasVisibleItem = allActivityItems.some((item) => !hiddenActivityIDs.includes(item.id))
    if (hasVisibleItem || allActivityItems.length === 0) return hiddenActivityIDs
    return hiddenActivityIDs.filter((id) => id !== allActivityItems[0].id)
  }, [allActivityItems, hiddenActivityIDs])
  const activityItems = useMemo(
    () => {
      const hiddenIDs = new Set(effectiveHiddenActivityIDs)
      return allActivityItems.filter((item) => !hiddenIDs.has(item.id))
    },
    [allActivityItems, effectiveHiddenActivityIDs],
  )

  useEffect(() => {
    if (effectiveHiddenActivityIDs.length === hiddenActivityIDs.length) return
    setHiddenActivityIDs(effectiveHiddenActivityIDs)
    storeHiddenActivityIDs(effectiveHiddenActivityIDs)
  }, [effectiveHiddenActivityIDs, hiddenActivityIDs])

  const handleActivityReorder = (activeId: string, overId: string) => {
    if (!isActivityItemID(activeId) || !isActivityItemID(overId)) return
    const visibleIds = activityItems.map((item) => item.id)
    const oldIndex = visibleIds.indexOf(activeId)
    const newIndex = visibleIds.indexOf(overId)
    if (oldIndex === -1 || newIndex === -1) return

    const nextVisibleIds = arrayMove(visibleIds, oldIndex, newIndex)
    const nextOrder = mergeVisibleActivityOrder(nextVisibleIds, activityOrder, defaultActivityOrderForScope(activityOrderScope))
    setActivityOrders((current) => ({ ...current, [activityOrderScope]: nextOrder }))
    storeActivityOrder(activityOrderScope, nextOrder)
  }

  const handleCustomizationActivityReorder = (activeId: string, overId: string) => {
    if (!isActivityItemID(activeId) || !isActivityItemID(overId)) return
    const availableIDs = allActivityItems.map((item) => item.id)
    const oldIndex = availableIDs.indexOf(activeId)
    const newIndex = availableIDs.indexOf(overId)
    if (oldIndex === -1 || newIndex === -1) return

    const reorderedIDs = arrayMove(availableIDs, oldIndex, newIndex)
    const nextOrder = mergeVisibleActivityOrder(reorderedIDs, activityOrder, defaultActivityOrderForScope(activityOrderScope))
    setActivityOrders((current) => ({ ...current, [activityOrderScope]: nextOrder }))
    storeActivityOrder(activityOrderScope, nextOrder)
  }

  const handleActivityVisibilityChange = (id: string, visible: boolean) => {
    if (!isActivityItemID(id)) return
    const currentlyVisibleItems = allActivityItems.filter((item) => !effectiveHiddenActivityIDs.includes(item.id))
    if (!visible && currentlyVisibleItems.length <= 1) return

    const nextHiddenIDs = visible
      ? effectiveHiddenActivityIDs.filter((hiddenID) => hiddenID !== id)
      : [...new Set([...effectiveHiddenActivityIDs, id])]
    setHiddenActivityIDs(nextHiddenIDs)
    storeHiddenActivityIDs(nextHiddenIDs)

    const hiddenItem = allActivityItems.find((item) => item.id === id)
    if (!visible && hiddenItem?.active) {
      const fallback = allActivityItems.find((item) => !nextHiddenIDs.includes(item.id))
      if (fallback) selectPrimaryNavigation(fallback.id, fallback.onClick)
    }
  }

  const resizeActivityBar = (nextWidth: number) => {
    const clampedWidth = clampActivityBarWidth(nextWidth)
    setActivityBarWidth(clampedWidth)
    storeActivityBarWidth(clampedWidth)
  }

  const handleActivityBarResizePointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!activityBarExpanded) return
    event.preventDefault()
    const startX = event.clientX
    const startWidth = activityBarWidth
    const handlePointerMove = (moveEvent: PointerEvent) => {
      resizeActivityBar(startWidth + moveEvent.clientX - startX)
    }
    const handlePointerUp = () => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', handlePointerUp)
    }
    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', handlePointerUp, { once: true })
  }

  const handleActivityBarResizeKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (!activityBarExpanded) return
    if (event.key === 'ArrowLeft') {
      event.preventDefault()
      resizeActivityBar(activityBarWidth - ACTIVITY_BAR_WIDTH_KEYBOARD_STEP)
    } else if (event.key === 'ArrowRight') {
      event.preventDefault()
      resizeActivityBar(activityBarWidth + ACTIVITY_BAR_WIDTH_KEYBOARD_STEP)
    } else if (event.key === 'Home') {
      event.preventDefault()
      resizeActivityBar(ACTIVITY_BAR_MIN_WIDTH)
    } else if (event.key === 'End') {
      event.preventDefault()
      resizeActivityBar(ACTIVITY_BAR_MAX_WIDTH)
    }
  }

  const activityBar = (
    <WorkbenchAppSidebar
      expanded={activityBarExpanded}
      activityOrderScope={activityOrderScope}
      activityItems={activityItems.map((item) => ({
        ...item,
        active: optimisticNavigationId ? optimisticNavigationId === item.id : item.active,
        onClick: () => selectPrimaryNavigation(item.id, item.onClick),
      }))}
      customizationItems={allActivityItems}
      hiddenActivityIDs={effectiveHiddenActivityIDs}
      dragDisabled={settingsOpen}
      contextSwitcher={adventureContextVisible ? (
        <BookSwitcher
          books={books}
          currentBookName={currentBookName}
          currentChapterCount={summary?.chapter_count}
          currentWordCount={summary?.total_words}
          workspace={workspace}
          iconOnly={!activityBarExpanded}
          onSwitchBook={onQuickSwitchBook}
          onManageBooks={manageBooks}
        />
      ) : undefined}
      sidebarLabel={t('workbench.sidebar.label')}
      settingsLabel={t('workbench.activity.settings')}
      settingsActive={optimisticNavigationId ? optimisticNavigationId === 'settings' : settingsOpen}
      toggleLabel={activityBarExpanded ? t('workbench.activity.toggleCollapse') : t('workbench.activity.toggleExpand')}
      resizeLabel={t('layout.resize.activityBar')}
      minWidth={ACTIVITY_BAR_MIN_WIDTH}
      maxWidth={ACTIVITY_BAR_MAX_WIDTH}
      currentWidth={activityBarWidth}
      onOpenSettings={() => selectPrimaryNavigation('settings', onToggleSettings)}
      onToggle={onToggleActivityBarExpanded}
      onReorder={handleActivityReorder}
      onCustomizationReorder={handleCustomizationActivityReorder}
      onActivityVisibilityChange={handleActivityVisibilityChange}
      onResizePointerDown={handleActivityBarResizePointerDown}
      onResizeKeyDown={handleActivityBarResizeKeyDown}
    />
  )

  // Keep business content on one React subtree while its DOM host moves between
  // the desktop resizable workspace and the mobile shell. This preserves local
  // editor state when the viewport crosses the mobile breakpoint.
  const mainContentPortal = mainContentHost ? createPortal(main, mainContentHost, 'workbench-main-content') : null
  const mainContentSlot = (
    <StablePortalSlot
      host={mainContentHost}
      fallback={main}
      wrapFallback={false}
      data-nova-workbench-main-slot="true"
      className="h-full min-h-0 w-full min-w-0 overflow-hidden"
    />
  )

  if (isMobile) {
    const mobileNavigationTools = (
      <div className="nova-mobile-navigation-tools flex min-w-0 flex-col gap-3">
        {adventureContextVisible && <div className="nova-mobile-book-context">
          <span className="nova-mobile-section-label">{t('workbench.mobile.currentBook')}</span>
          <BookSwitcher
            books={books} currentBookName={currentBookName}
            currentChapterCount={summary?.chapter_count} currentWordCount={summary?.total_words}
            workspace={workspace} compact modal={false}
            onSwitchBook={async (path) => { const switched = await onQuickSwitchBook(path); if (switched) closeMobilePanes(); return switched }}
            onManageBooks={() => { closeMobilePanes(); manageBooks() }}
          />
        </div>}
        <div className="nova-mobile-quick-actions grid grid-cols-2 gap-2">
          <Button variant="secondary" className="w-full" onClick={() => { closeMobilePanes(); setCommandOpen(true) }} aria-label={t('command.openButton')}>
            <Search />{t('command.openButton')}
          </Button>
        </div>
      </div>
    )
    const mobileActivityItems: MobileNavItem[] = allActivityItems
      .map((item) => ({
        id: item.id,
        label: item.label,
        icon: item.icon,
        active: optimisticNavigationId ? optimisticNavigationId === item.id : item.active,
        onClick: () => selectPrimaryNavigation(item.id, item.onClick),
      }))

    return (
      <MobileWorkspaceHeaderProvider route={presentedLayout}>
        <WorkspaceMobileLayout
          topBar={<MobileWorkspaceHeaderOutlet title={settingsOpen ? t('workbench.activity.settings') : mobileActivityItems.find((item) => item.active)?.label || currentBookName} />}
          navigationTools={mobileNavigationTools}
          main={mainContentSlot}
          activityItems={mobileActivityItems}
          settingsItem={{
            id: 'settings',
            label: t('workbench.activity.settings'),
            icon: <Settings className="h-4 w-4" />,
            active: optimisticNavigationId ? optimisticNavigationId === 'settings' : settingsOpen,
            onClick: () => selectPrimaryNavigation('settings', onToggleSettings),
          }}
          closeLabel={t('common.close')}
          navigationLabel={t('workbench.mobile.navigation')}
        />
        {mainContentPortal}
      </MobileWorkspaceHeaderProvider>
    )
  }

  return (
    <MobileWorkspaceHeaderProvider route={presentedLayout}>
      <SidebarProvider
        open={activityBarExpanded}
        style={{
          '--sidebar-width': `${activityBarWidth}px`,
          '--sidebar-width-icon': `${ACTIVITY_BAR_COLLAPSED_WIDTH}px`,
        } as CSSProperties}
      >
        <WorkspaceLayout
          appSidebar={activityBar}
          routeLayoutKey={presentedLayout}
          main={mainContentSlot}
        />
      </SidebarProvider>
      {mainContentPortal}
    </MobileWorkspaceHeaderProvider>
  )
}

export function readStoredActivityBarWidth() {
  if (typeof window === 'undefined') return ACTIVITY_BAR_DEFAULT_WIDTH
  const raw = window.localStorage.getItem(ACTIVITY_BAR_WIDTH_STORAGE_KEY)
  if (raw === null) return ACTIVITY_BAR_DEFAULT_WIDTH
  const value = Number(raw)
  if (value === ACTIVITY_BAR_LEGACY_DEFAULT_WIDTH) return ACTIVITY_BAR_DEFAULT_WIDTH
  return Number.isFinite(value) ? clampActivityBarWidth(value) : ACTIVITY_BAR_DEFAULT_WIDTH
}

function storeActivityBarWidth(width: number) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(ACTIVITY_BAR_WIDTH_STORAGE_KEY, String(clampActivityBarWidth(width)))
}

function clampActivityBarWidth(width: number) {
  return Math.min(ACTIVITY_BAR_MAX_WIDTH, Math.max(ACTIVITY_BAR_MIN_WIDTH, Math.round(width)))
}
