import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Minus, PanelRight, SquareDashed } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { loadStoryWindowGeometry, saveStoryWindowGeometry } from './story-window-persistence'

/**
 * How a story window is presented. Docked is the side panel the stage has always
 * drawn; floating is the same panel dragged and resized over the story; minimized
 * keeps the window and everything in it alive behind a fixed entry.
 */
export type StoryWindowMode = 'docked' | 'floating' | 'minimized'

/**
 * Position and size in pixels, relative to the application's visible viewport.
 *
 * The stage's own body column is not the space a window may use: a conversation
 * dragged there could never cover the story's prose, and a battle window with a
 * long round would be squeezed into a column the player did not choose. The
 * window floats over the whole application instead, which is why these are
 * viewport coordinates and the frame is positioned `fixed` while floating.
 */
export interface StoryWindowGeometry {
  x: number
  y: number
  width: number
  height: number
}

export interface StoryWindowControls {
  mode: StoryWindowMode
  /** Switches between the docked side panel and a free-floating window. */
  toggleFloating: () => void
  minimize: () => void
  restore: () => void
}

export interface StoryWindowActionsProps {
  mode: StoryWindowMode
  onToggleFloating: () => void
  onMinimize: () => void
}

/** The smallest a floating window may be dragged down to, in pixels. */
const MIN_WINDOW_WIDTH = 300
const MIN_WINDOW_HEIGHT = 240
/** The gap kept between a floating window and the viewport's edges. */
const WINDOW_MARGIN = 16
/** How far a second floating window is offset so it does not sit exactly on the first. */
const WINDOW_CASCADE_STEP = 28
/** One keyboard resize step, and the larger step Shift asks for. */
const KEYBOARD_RESIZE_STEP = 16
const KEYBOARD_RESIZE_STEP_LARGE = 64

/**
 * Which edges a resize handle moves. A floating window opens against the
 * viewport's right side, so every handle grows it away from that anchor: the left
 * edge and the bottom edge each follow the pointer while the opposite stays put.
 */
type ResizeEdge = 'bottom-right' | 'left' | 'bottom'

/**
 * The header controls a window offers. They are handed to the panel so they sit
 * beside its own close button rather than in a second title bar stacked above it.
 */
export function StoryWindowActions({ mode, onToggleFloating, onMinimize }: StoryWindowActionsProps) {
  const { t } = useTranslation()
  const floating = mode === 'floating'
  return (
    <span data-window-drag-handle className="flex shrink-0 items-center">
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        onClick={onToggleFloating}
        aria-label={floating ? t('storyStage.window.dock') : t('storyStage.window.float')}
        title={floating ? t('storyStage.window.dock') : t('storyStage.window.float')}
      >
        {floating ? <PanelRight className="h-4 w-4" /> : <SquareDashed className="h-4 w-4" />}
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        onClick={onMinimize}
        aria-label={t('storyStage.window.minimize')}
        title={t('storyStage.window.minimize')}
      >
        <Minus className="h-4 w-4" />
      </Button>
    </span>
  )
}

/** Keeps a geometry inside the viewport it floats over. */
export function clampStoryWindowGeometry(
  geometry: StoryWindowGeometry,
  container: { width: number; height: number },
): StoryWindowGeometry {
  const minimum = {
    ...geometry,
    width: Math.max(MIN_WINDOW_WIDTH, geometry.width),
    height: Math.max(MIN_WINDOW_HEIGHT, geometry.height),
  }
  // A viewport that has not been measured has no edges to clamp against.
  // Bounding the window by a zero box would shrink it to the minimum on the first
  // frame, before the document has laid out, and it would stay there.
  if (container.width <= 0 || container.height <= 0) {
    return { ...minimum, x: Math.max(0, minimum.x), y: Math.max(0, minimum.y) }
  }
  const availableWidth = Math.max(MIN_WINDOW_WIDTH, container.width - WINDOW_MARGIN)
  const availableHeight = Math.max(MIN_WINDOW_HEIGHT, container.height - WINDOW_MARGIN)
  const width = clamp(minimum.width, MIN_WINDOW_WIDTH, availableWidth)
  const height = clamp(minimum.height, MIN_WINDOW_HEIGHT, availableHeight)
  return {
    width,
    height,
    x: clamp(minimum.x, 0, Math.max(0, container.width - width)),
    y: clamp(minimum.y, 0, Math.max(0, container.height - height)),
  }
}

/** Where a window first appears when it leaves the dock: the side it came from. */
export function defaultStoryWindowGeometry(
  container: { width: number; height: number },
  preferredWidth = 420,
): StoryWindowGeometry {
  // A viewport that has not been measured yet is not one the window can be
  // clamped into; a plausible screen is the honest fallback.
  const measured = container.width > 0 && container.height > 0
    ? container
    : { width: 900, height: 700 }
  const width = Math.min(preferredWidth, Math.max(MIN_WINDOW_WIDTH, measured.width - WINDOW_MARGIN * 2))
  const height = Math.min(640, Math.max(MIN_WINDOW_HEIGHT, measured.height - WINDOW_MARGIN * 2))
  return clampStoryWindowGeometry(
    { x: measured.width - width - WINDOW_MARGIN, y: WINDOW_MARGIN, width, height },
    measured,
  )
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max)
}

/**
 * Applies a resize gesture to a window's geometry.
 *
 * The edge named by `edge` follows the pointer; every other edge stays where the
 * player put it, so growing from the bottom-right moves the right and bottom edges
 * outward without the content jumping to a new corner. The result is clamped to
 * the viewport, which is why the same function serves both the pointer and the
 * keyboard.
 */
export function resizeStoryWindowGeometry(
  origin: StoryWindowGeometry,
  edge: ResizeEdge,
  dx: number,
  dy: number,
  container: { width: number; height: number },
): StoryWindowGeometry {
  const next = { ...origin }
  if (edge === 'left') {
    next.width = origin.width - dx
  }
  if (edge === 'bottom-right') {
    next.width = container.width > 0 ? Math.min(origin.width + dx, container.width - origin.x) : origin.width + dx
  }
  if (edge === 'bottom-right' || edge === 'bottom') {
    next.height = container.height > 0 ? Math.min(origin.height + dy, container.height - origin.y) : origin.height + dy
  }
  const clamped = clampStoryWindowGeometry(next, container)
  // Keep the right edge pinned when the left one moves, and the top edge pinned
  // when the bottom one moves, even after the clamp has trimmed the size.
  if (edge === 'left') {
    next.x = origin.x + origin.width - clamped.width
  }
  return clampStoryWindowGeometry({ ...next, width: clamped.width, height: clamped.height }, container)
}

/** Where a second floating window is placed so it does not hide the first one. */
export function cascadeStoryWindowGeometry(
  base: StoryWindowGeometry,
  container: { width: number; height: number },
  step: number,
): StoryWindowGeometry {
  return clampStoryWindowGeometry(
    { ...base, x: base.x - WINDOW_CASCADE_STEP * step, y: base.y + WINDOW_CASCADE_STEP * step },
    container,
  )
}

interface StoryWindowProps {
  /** Names the window for assistive technology and for the minimized entry. */
  label: string
  icon?: ReactNode
  /** Stacks this window's minimized entry above the ones already there. */
  minimizedOrder?: number
  /**
   * How many floating windows are already open, so this one opens offset from
   * them instead of exactly on top of the first.
   */
  cascadeStep?: number
  /** The window's own panel. It receives the controls to place in its header. */
  children: (controls: StoryWindowControls & { windowActions: ReactNode }) => ReactNode
  /**
   * The width the panel takes when docked, and the width it opens at the first
   * time it floats. A battle needs more room for its prose than a chat does, and
   * §5 forbids leaving the battle window pinned in a 420px column.
   */
  dockedWidth?: number
  /**
   * Names this kind of window in local storage, so its size and place are
   * remembered per window and restored after a reload. A window without one can
   * still be dragged and resized; it simply forgets where it was.
   */
  windowKey?: string
  className?: string
}

/**
 * A story window that can be docked, floated, moved, resized and minimized.
 *
 * Everything the window holds — the draft, the reading position, and any request
 * already in flight — belongs to the panel inside it. Minimizing therefore hides
 * the frame instead of unmounting it: the conversation keeps its place, a reply
 * still being written still arrives, and restoring shows exactly what the player
 * left. None of those gestures is a story action, so none of them advances time,
 * sends a line, or settles anything.
 */
export function StoryWindow({ label, icon, minimizedOrder = 0, cascadeStep = 0, dockedWidth = 420, windowKey, children, className }: StoryWindowProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<StoryWindowMode>('docked')
  const [geometry, setGeometry] = useState<StoryWindowGeometry | null>(null)
  const frameRef = useRef<HTMLDivElement | null>(null)
  const geometryRef = useRef<StoryWindowGeometry | null>(null)
  // The mode a minimized window returns to. Minimizing twice must not lose it.
  const restoreModeRef = useRef<Exclude<StoryWindowMode, 'minimized'>>('docked')
  const dragRef = useRef<{
    pointerId: number
    startX: number
    startY: number
    originX: number
    originY: number
    width: number
    height: number
    container: { width: number; height: number }
  } | null>(null)
  const resizeRef = useRef<{
    pointerId: number
    startX: number
    startY: number
    origin: StoryWindowGeometry
    edge: ResizeEdge
    container: { width: number; height: number }
  } | null>(null)
  const [gesture, setGesture] = useState<'drag' | 'resize' | null>(null)

  // The window floats over the application, not over the stage's own column, so
  // the space it may use is the visible viewport. Measuring the stage instead
  // would pin every window inside a column the player cannot drag it out of.
  const containerSize = useCallback((): { width: number; height: number } => {
    if (typeof window === 'undefined') return { width: 0, height: 0 }
    // A viewport that reports nothing (a document still being laid out) has no
    // edges to clamp against, so the window keeps the geometry it already has.
    if (window.innerWidth > 0 && window.innerHeight > 0) {
      return { width: window.innerWidth, height: window.innerHeight }
    }
    return { width: 0, height: 0 }
  }, [])

  // A floating window is written straight to the element while it is being
  // dragged, so moving it does not re-render the conversation inside it. React
  // only learns the final geometry when the gesture ends.
  const paint = useCallback((next: StoryWindowGeometry) => {
    geometryRef.current = next
    const frame = frameRef.current
    if (!frame) return
    frame.style.left = `${next.x}px`
    frame.style.top = `${next.y}px`
    frame.style.width = `${next.width}px`
    frame.style.height = `${next.height}px`
  }, [])

  /**
   * Writes the window's place to local storage as an interface preference, never
   * as a story fact. The geometry is kept while docked too, so floating it again
   * — this session or the next one — returns the size the player chose.
   */
  const remember = useCallback((floating: boolean) => {
    saveStoryWindowGeometry(windowKey ?? '', geometryRef.current, floating)
  }, [windowKey])

  const toggleFloating = useCallback(() => {
    if (mode === 'minimized') return
    if (mode === 'floating') {
      remember(false)
      setMode('docked')
      return
    }
    const container = containerSize()
    // A window that has floated before returns to where the player left it. Only
    // a first float is placed, and two windows opened from the dock would
    // otherwise land on exactly the same spot with one hiding the other, so each
    // is offset by the number already open.
    const next = geometryRef.current
      ? clampStoryWindowGeometry(geometryRef.current, container)
      : cascadeStoryWindowGeometry(defaultStoryWindowGeometry(container, dockedWidth), container, cascadeStep)
    geometryRef.current = next
    setGeometry(next)
    remember(true)
    setMode('floating')
  }, [cascadeStep, containerSize, dockedWidth, mode, remember])

  const minimize = useCallback(() => {
    setMode((current) => {
      if (current === 'minimized') return current
      restoreModeRef.current = current
      return 'minimized'
    })
  }, [])

  const restore = useCallback(() => {
    setMode((current) => (current === 'minimized' ? restoreModeRef.current : current))
  }, [])

  // A reload returns the window where the player left it. The remembered value is
  // clamped to the viewport it is opening in, so a place chosen on a wide screen
  // cannot strand the window past the edge of a narrow one — its header has to
  // stay grabbable or the window can never be brought back.
  const restoredRef = useRef(false)
  useLayoutEffect(() => {
    if (restoredRef.current || !windowKey) return
    restoredRef.current = true
    const remembered = loadStoryWindowGeometry(windowKey)
    if (!remembered) return
    const next = clampStoryWindowGeometry(remembered, containerSize())
    geometryRef.current = next
    setGeometry(next)
    if (remembered.floating) {
      restoreModeRef.current = 'floating'
      setMode('floating')
    }
  }, [containerSize, windowKey])

  // The viewport can change under a floating window — a narrower browser window,
  // a zoomed page, a rotated phone — so a geometry that was inside it a moment
  // ago is pulled back in rather than left hanging off an edge.
  useLayoutEffect(() => {
    if (mode !== 'floating' || typeof window === 'undefined') return
    const pullBack = () => {
      const current = geometryRef.current
      if (!current) return
      const container = containerSize()
      if (container.width <= 0 || container.height <= 0) return
      const next = clampStoryWindowGeometry(current, container)
      if (
        next.x === current.x && next.y === current.y
        && next.width === current.width && next.height === current.height
      ) return
      paint(next)
      setGeometry(next)
      remember(true)
    }
    window.addEventListener('resize', pullBack)
    return () => window.removeEventListener('resize', pullBack)
  }, [containerSize, mode, paint, remember])

  // A drag that began in this window must end with it even if the pointer leaves
  // the frame, which is what pointer capture is for.
  useEffect(() => {
    if (!gesture) return
    const frame = frameRef.current
    return () => {
      if (frame) {
        dragRef.current = null
        resizeRef.current = null
      }
    }
  }, [gesture])

  const startDrag = (event: React.PointerEvent<HTMLDivElement>) => {
    if (mode !== 'floating') return
    const target = event.target as HTMLElement | null
    // The header is the handle: it is the part of the window that is always
    // empty of content and always at the top. A control inside it is a control,
    // not a handle, so pressing close must not start moving the window.
    if (!target?.closest('header, [data-window-drag-handle]')) return
    if (target.closest('button, a, input, textarea, select, [role="button"]')) return
    const container = containerSize()
    const origin = geometryRef.current ?? defaultStoryWindowGeometry(container)
    dragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      originX: origin.x,
      originY: origin.y,
      width: origin.width,
      height: origin.height,
      container,
    }
    event.currentTarget.setPointerCapture?.(event.pointerId)
    setGesture('drag')
    event.preventDefault()
  }

  const moveDrag = (event: React.PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    paint(clampStoryWindowGeometry({
      x: drag.originX + (event.clientX - drag.startX),
      y: drag.originY + (event.clientY - drag.startY),
      width: drag.width,
      height: drag.height,
    }, drag.container))
  }

  const endDrag = (event: React.PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    event.currentTarget.releasePointerCapture?.(event.pointerId)
    dragRef.current = null
    setGesture(null)
    if (geometryRef.current) {
      setGeometry(geometryRef.current)
      remember(true)
    }
  }

  const startResize = (edge: ResizeEdge) => (event: React.PointerEvent<HTMLDivElement>) => {
    const origin = geometryRef.current
    if (mode !== 'floating' || !origin) return
    event.stopPropagation()
    resizeRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      origin,
      edge,
      container: containerSize(),
    }
    event.currentTarget.setPointerCapture?.(event.pointerId)
    setGesture('resize')
    event.preventDefault()
  }

  const moveResize = (event: React.PointerEvent<HTMLDivElement>) => {
    const resize = resizeRef.current
    if (!resize || resize.pointerId !== event.pointerId) return
    paint(resizeStoryWindowGeometry(
      resize.origin,
      resize.edge,
      event.clientX - resize.startX,
      event.clientY - resize.startY,
      resize.container,
    ))
  }

  const endResize = (event: React.PointerEvent<HTMLDivElement>) => {
    const resize = resizeRef.current
    if (!resize || resize.pointerId !== event.pointerId) return
    event.currentTarget.releasePointerCapture?.(event.pointerId)
    resizeRef.current = null
    setGesture(null)
    if (geometryRef.current) {
      setGeometry(geometryRef.current)
      remember(true)
    }
  }

  // A pointer drag is not available to everyone. The handle is a focusable
  // separator, so the same three edges can be resized from the keyboard with the
  // arrow keys: left/right change the width, up/down the height.
  const resizeFromKeyboard = (edge: ResizeEdge) => (event: React.KeyboardEvent<HTMLDivElement>) => {
    const origin = geometryRef.current
    if (mode !== 'floating' || !origin) return
    const step = event.shiftKey ? KEYBOARD_RESIZE_STEP_LARGE : KEYBOARD_RESIZE_STEP
    let dx = 0
    let dy = 0
    switch (event.key) {
      case 'ArrowLeft':
        dx = -step
        break
      case 'ArrowRight':
        dx = step
        break
      case 'ArrowUp':
        dy = -step
        break
      case 'ArrowDown':
        dy = step
        break
      default:
        return
    }
    event.preventDefault()
    event.stopPropagation()
    const next = resizeStoryWindowGeometry(origin, edge, dx, dy, containerSize())
    paint(next)
    setGeometry(next)
    remember(true)
  }

  const controls = useMemo<StoryWindowControls & { windowActions: ReactNode }>(() => ({
    mode,
    toggleFloating,
    minimize,
    restore,
    windowActions: <StoryWindowActions mode={mode} onToggleFloating={toggleFloating} onMinimize={minimize} />,
  }), [minimize, mode, restore, toggleFloating])

  const minimized = mode === 'minimized'
  // A minimized window keeps the position it had, so restoring it does not jump.
  const asFloating = geometry !== null && (mode === 'floating' || (minimized && restoreModeRef.current === 'floating'))

  return (
    <>
      <div
        ref={frameRef}
        role="dialog"
        aria-label={label}
        aria-hidden={minimized || undefined}
        inert={minimized || undefined}
        data-story-window={mode}
        data-gesture={gesture || undefined}
        onPointerDown={startDrag}
        onPointerMove={moveDrag}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        className={cn(
          'flex min-h-0 flex-col bg-[var(--nova-surface)] text-[var(--nova-text)]',
          asFloating
            ? 'fixed z-30 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] shadow-[var(--nova-shadow)]'
            : 'absolute inset-y-0 right-0 z-30 border-l border-[var(--nova-border)] shadow-[var(--nova-shadow)]',
          mode === 'floating' && 'touch-none select-none',
          minimized && 'pointer-events-none invisible',
          className,
        )}
        // Docked, the window is bounded by the column it sits in, not by the
        // viewport: a records sidebar can leave that column narrower than the
        // window wants to be, and a window wider than its column hangs off the
        // left edge, taking its header and close button with it.
        style={asFloating && geometry
          ? { left: geometry.x, top: geometry.y, width: geometry.width, height: geometry.height }
          : { width: `min(${dockedWidth}px, 100%)` }}
      >
        {children(controls)}
        {mode === 'floating' ? (
          <>
            {/*
              The resize affordance is drawn, not merely present: a visible grip
              on the bottom-right corner plus a full-height left edge and a
              full-width bottom edge, each with a generous hit area. The corner
              is the only one that is always clear of the panel's own controls.
            */}
            <div
              role="separator"
              aria-label={t('storyStage.window.resize')}
              aria-orientation="vertical"
              tabIndex={0}
              data-window-resize-handle="left"
              onPointerDown={startResize('left')}
              onPointerMove={moveResize}
              onPointerUp={endResize}
              onPointerCancel={endResize}
              onKeyDown={resizeFromKeyboard('left')}
              className="group/handle absolute inset-y-3 left-0 z-20 w-2 cursor-ew-resize touch-none"
            >
              <span
                aria-hidden
                className="absolute inset-y-0 left-0 w-1 rounded-full bg-[var(--nova-border)] opacity-0 transition-opacity group-hover/handle:opacity-100"
              />
            </div>
            <div
              role="separator"
              aria-label={t('storyStage.window.resize')}
              aria-orientation="horizontal"
              tabIndex={0}
              data-window-resize-handle="bottom"
              onPointerDown={startResize('bottom')}
              onPointerMove={moveResize}
              onPointerUp={endResize}
              onPointerCancel={endResize}
              onKeyDown={resizeFromKeyboard('bottom')}
              className="group/handle absolute inset-x-3 bottom-0 z-20 h-2 cursor-ns-resize touch-none"
            >
              <span
                aria-hidden
                className="absolute inset-x-0 bottom-0 h-1 rounded-full bg-[var(--nova-border)] opacity-0 transition-opacity group-hover/handle:opacity-100"
              />
            </div>
            <div
              role="separator"
              aria-label={t('storyStage.window.resize')}
              tabIndex={0}
              data-window-resize-handle="corner"
              onPointerDown={startResize('bottom-right')}
              onPointerMove={moveResize}
              onPointerUp={endResize}
              onPointerCancel={endResize}
              onKeyDown={resizeFromKeyboard('bottom-right')}
              title={t('storyStage.window.resize')}
              className="group/handle absolute bottom-0 right-0 z-20 flex size-6 cursor-nwse-resize touch-none items-end justify-end rounded-br-[var(--nova-radius)]"
            >
              {/* The grip is always visible, so a player can find the resize
                  gesture without having to hover the exact corner first. */}
              <svg
                aria-hidden
                viewBox="0 0 12 12"
                className="pointer-events-none m-1 size-3 text-[var(--nova-text-muted)] opacity-60 transition-opacity group-hover/handle:opacity-100"
              >
                <path d="M1 11 L11 1 M5 11 L11 5 M9 11 L11 9" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" fill="none" />
              </svg>
            </div>
          </>
        ) : null}
      </div>
      {minimized ? (
        <button
          type="button"
          onClick={restore}
          aria-label={t('storyStage.window.restoreNamed', { name: label })}
          title={t('storyStage.window.restore')}
          data-testid="story-window-minimized-entry"
          style={{ bottom: `${12 + minimizedOrder * 40}px` }}
          className="absolute right-3 z-40 flex h-8 items-center gap-1.5 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 text-[11px] text-[var(--nova-text-muted)] shadow-[var(--nova-shadow)] transition-colors hover:text-[var(--nova-text)]"
        >
          {icon}
          <span>{label}</span>
        </button>
      ) : null}
    </>
  )
}
