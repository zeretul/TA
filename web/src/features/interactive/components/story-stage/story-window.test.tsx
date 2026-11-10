import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import {
  StoryWindow,
  cascadeStoryWindowGeometry,
  clampStoryWindowGeometry,
  defaultStoryWindowGeometry,
  resizeStoryWindowGeometry,
} from './story-window'

/**
 * These tests hold the floating window to U4's window rules and to the repair
 * note in 设计稿/Claude_返修优先与剧情助手续工_20261018.md:51:
 *
 *   "聊天与战斗浮窗目前只有左下角透明16px缩放区，测试只断言其存在。先正常页面复现
 *    用户问题，再修为可发现、易拖动的边/角缩放，真实验证拖拽前后宽高变化、内容布局、
 *    窄屏与长文本、最小化恢复和多窗不遮挡。拖窗不推进游戏、不重发、不丢草稿/在途回复。"
 *
 * So the gestures below assert the painted width and height actually change. A
 * test that only finds the handle is exactly the test that let the invisible
 * 16px corner ship.
 *
 * A window is docked, floated, moved, resized, minimized and restored, and none
 * of those gestures is a story action. The window therefore keeps its contents
 * mounted while minimized — a draft and a request already in flight belong to the
 * panel, and unmounting the frame would lose both — and its geometry is pulled
 * back inside the viewport rather than left hanging off an edge.
 *
 * The viewport, not the stage's body column, is the space a floating window may
 * use: a conversation the player drags must be able to cover the story's prose,
 * and a battle window must not be trapped in the column it was docked in. That is
 * also why the window remembers its own place in local storage per window kind.
 */

function renderWindow(
  children?: (controls: { windowActions: React.ReactNode }) => React.ReactNode,
  windowKey?: string,
) {
  return render(
    <div style={{ width: 900, height: 700 }}>
      <StoryWindow label="人物" windowKey={windowKey}>
        {children || ((controls) => (
          <header>
            <h2>标题</h2>
            {controls.windowActions}
            <p>会话内容</p>
          </header>
        ))}
      </StoryWindow>
    </div>,
  )
}

/** The viewport the gestures below are clamped against. */
const VIEWPORT = { width: 1024, height: 768 }

/**
 * jsdom has no layout engine, so a window's measurements are whatever the test
 * says they are. The window reads the viewport — the space it actually floats
 * over — so the viewport is the thing given a size here. A remembered geometry
 * is clamped against the same numbers.
 */
beforeEach(() => {
  window.localStorage.clear()
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: VIEWPORT.width })
  Object.defineProperty(window, 'innerHeight', { configurable: true, writable: true, value: VIEWPORT.height })
})

function floatingDialog() {
  fireEvent.click(screen.getByRole('button', { name: '改为浮窗' }))
  return screen.getByRole('dialog', { name: '人物' })
}

function paintedSize(dialog: HTMLElement) {
  return { width: parseFloat(dialog.style.width), height: parseFloat(dialog.style.height) }
}

function paintedBox(dialog: HTMLElement) {
  return { x: parseFloat(dialog.style.left), y: parseFloat(dialog.style.top), ...paintedSize(dialog) }
}

describe('story window geometry', () => {
  it('keeps a window inside the viewport it floats over', () => {
    const clamped = clampStoryWindowGeometry({ x: 5000, y: -40, width: 5000, height: 20 }, { width: 900, height: 700 })
    expect(clamped.width).toBeLessThanOrEqual(900)
    expect(clamped.height).toBeGreaterThanOrEqual(240)
    expect(clamped.x).toBe(900 - clamped.width)
    expect(clamped.y).toBe(0)
  })

  it('opens against the edge the docked window came from', () => {
    const geometry = defaultStoryWindowGeometry({ width: 900, height: 700 })
    expect(geometry.x + geometry.width).toBe(900 - 16)
    expect(geometry.y).toBe(16)
  })

  it('opens a wider window when the panel asks for one', () => {
    // §5 forbids leaving the battle window pinned in a 420px column, so a panel
    // that needs more room states it and the float opens at that width.
    const wide = defaultStoryWindowGeometry({ width: 900, height: 700 }, 560)
    expect(wide.width).toBe(560)
    expect(wide.x + wide.width).toBe(900 - 16)
    // A stage too narrow for the request still wins: the window may not hang
    // off the edge it is anchored to.
    const narrow = defaultStoryWindowGeometry({ width: 520, height: 700 }, 560)
    expect(narrow.width).toBe(520 - 32)
  })

  it('grows away from the anchored right and top edges', () => {
    const origin = { x: 400, y: 40, width: 420, height: 600 }
    const container = { width: 900, height: 700 }

    // Dragging the left edge further left widens the window and moves its left
    // edge, while the right edge stays exactly where it was.
    const wider = resizeStoryWindowGeometry(origin, 'left', -60, 0, container)
    expect(wider.width).toBe(480)
    expect(wider.x).toBe(340)
    expect(wider.x + wider.width).toBe(origin.x + origin.width)

    // Dragging the bottom edge down makes it taller; the top edge does not move.
    const taller = resizeStoryWindowGeometry(origin, 'bottom', 0, 40, container)
    expect(taller.height).toBe(640)
    expect(taller.y).toBe(origin.y)
  })

  it('never shrinks below the minimum or past the viewport', () => {
    const origin = { x: 400, y: 40, width: 420, height: 600 }
    const container = { width: 900, height: 700 }

    const tiny = resizeStoryWindowGeometry(origin, 'bottom-right', -5000, -5000, container)
    expect(tiny.width).toBe(300)
    expect(tiny.height).toBe(240)

    const huge = resizeStoryWindowGeometry(origin, 'bottom-right', 5000, 5000, container)
    expect(huge).toEqual({ x: 400, y: 40, width: 500, height: 660 })
  })

  it('offsets a second floating window so the first is not hidden', () => {
    const base = defaultStoryWindowGeometry({ width: 900, height: 700 })
    const second = cascadeStoryWindowGeometry(base, { width: 900, height: 700 }, 1)
    expect(second.x).toBe(base.x - 28)
    expect(second.y).toBe(base.y + 28)
    expect(second.width).toBe(base.width)
  })
})

describe('story window gestures', () => {
  it('keeps the panel mounted while minimized, so a draft and a request survive', () => {
    renderWindow()
    expect(screen.getByText('会话内容')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '最小化' }))
    // The frame is still in the document: minimizing hides it, it does not
    // discard the conversation inside it.
    expect(screen.getByText('会话内容')).toBeInTheDocument()
    const entry = screen.getByTestId('story-window-minimized-entry')
    fireEvent.click(entry)
    expect(screen.getByText('会话内容')).toBeInTheDocument()
    expect(screen.queryByTestId('story-window-minimized-entry')).toBeNull()
  })

  it('floats and docks without unmounting the panel', () => {
    renderWindow()
    fireEvent.click(screen.getByRole('button', { name: '改为浮窗' }))
    expect(screen.getByRole('dialog', { name: '人物' })).toHaveAttribute('data-story-window', 'floating')
    expect(screen.getByText('会话内容')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '停靠回侧栏' }))
    expect(screen.getByRole('dialog', { name: '人物' })).toHaveAttribute('data-story-window', 'docked')
  })

  it('moves on the header and leaves a header control alone', () => {
    renderWindow()
    fireEvent.click(screen.getByRole('button', { name: '改为浮窗' }))
    const dialog = screen.getByRole('dialog', { name: '人物' })
    const header = dialog.querySelector('header')
    expect(header).not.toBeNull()

    fireEvent.pointerDown(header as HTMLElement, { pointerId: 1, clientX: 100, clientY: 100 })
    fireEvent.pointerMove(header as HTMLElement, { pointerId: 1, clientX: 40, clientY: 130 })
    fireEvent.pointerUp(header as HTMLElement, { pointerId: 1, clientX: 40, clientY: 130 })
    // The move landed, and the panel inside it is still the same one.
    expect(dialog.style.left).not.toBe('')
    expect(screen.getByText('会话内容')).toBeInTheDocument()

    // A control is not a handle: pressing the minimize button must not begin a
    // drag, so the window keeps its position while the button does its own job.
    const before = dialog.style.left
    const minimize = screen.getByRole('button', { name: '最小化' })
    fireEvent.pointerDown(minimize, { pointerId: 2, clientX: 300, clientY: 300 })
    fireEvent.pointerMove(minimize, { pointerId: 2, clientX: 500, clientY: 500 })
    fireEvent.pointerUp(minimize, { pointerId: 2, clientX: 500, clientY: 500 })
    expect(dialog.style.left).toBe(before)
  })

  it('does not start a drag while docked', () => {
    renderWindow()
    const dialog = screen.getByRole('dialog', { name: '人物' })
    const header = dialog.querySelector('header') as HTMLElement
    fireEvent.pointerDown(header, { pointerId: 1, clientX: 100, clientY: 100 })
    fireEvent.pointerMove(header, { pointerId: 1, clientX: 10, clientY: 10 })
    fireEvent.pointerUp(header, { pointerId: 1, clientX: 10, clientY: 10 })
    expect(dialog.style.left).toBe('')
  })

  it('offers its resize affordances only once floating', () => {
    renderWindow()
    expect(screen.queryAllByRole('separator', { name: '调整窗口大小' })).toHaveLength(0)
    fireEvent.click(screen.getByRole('button', { name: '改为浮窗' }))
    // A visible corner grip plus the two edges. The corner grip is the one a
    // player can see without hovering, so it is asserted by name as well.
    const handles = screen.getAllByRole('separator', { name: '调整窗口大小' })
    expect(handles.map((handle) => handle.getAttribute('data-window-resize-handle'))).toEqual([
      'left',
      'bottom',
      'corner',
    ])
    const corner = screen.getByTitle('调整窗口大小')
    expect(corner.getAttribute('data-window-resize-handle')).toBe('corner')
    // The grip is drawn, not a transparent hit area.
    expect(corner.querySelector('svg')).not.toBeNull()
    // And the corner is large enough to hit without pixel-perfect aim.
    expect(corner.className).toContain('size-6')
    expect(corner.className).toContain('right-0')
  })

  it('actually changes the painted width and height when dragged', () => {
    renderWindow()
    const dialog = floatingDialog()
    const header = screen.getByText('标题')
    fireEvent.pointerDown(header, { pointerId: 6, clientX: 600, clientY: 40 })
    fireEvent.pointerMove(dialog, { pointerId: 6, clientX: 450, clientY: 40 })
    fireEvent.pointerUp(dialog, { pointerId: 6, clientX: 450, clientY: 40 })
    const before = paintedBox(dialog)
    expect(before.width).toBeGreaterThan(0)
    expect(before.height).toBeGreaterThan(0)

    const corner = screen.getAllByRole('separator', { name: '调整窗口大小' })[2]
    fireEvent.pointerDown(corner, { pointerId: 7, clientX: 500, clientY: 500 })
    fireEvent.pointerMove(corner, { pointerId: 7, clientX: 600, clientY: 560 })
    fireEvent.pointerUp(corner, { pointerId: 7, clientX: 600, clientY: 560 })

    const after = paintedSize(dialog)
    expect(after.width).toBe(before.width + 100)
    expect(after.height).toBe(before.height + 60)
    expect(paintedBox(dialog).x).toBe(before.x)
    expect(screen.getByText('会话内容')).toBeInTheDocument()
  })

  it('resizes from the left edge and the bottom edge separately', () => {
    renderWindow()
    const dialog = floatingDialog()
    const before = paintedSize(dialog)

    const left = screen.getAllByRole('separator', { name: '调整窗口大小' })[0]
    fireEvent.pointerDown(left, { pointerId: 8, clientX: 500, clientY: 300 })
    fireEvent.pointerMove(left, { pointerId: 8, clientX: 440, clientY: 300 })
    fireEvent.pointerUp(left, { pointerId: 8, clientX: 440, clientY: 300 })
    expect(paintedSize(dialog).width).toBe(before.width + 60)
    expect(paintedSize(dialog).height).toBe(before.height)

    const bottom = screen.getAllByRole('separator', { name: '调整窗口大小' })[1]
    fireEvent.pointerDown(bottom, { pointerId: 9, clientX: 500, clientY: 500 })
    fireEvent.pointerMove(bottom, { pointerId: 9, clientX: 500, clientY: 450 })
    fireEvent.pointerUp(bottom, { pointerId: 9, clientX: 500, clientY: 450 })
    expect(paintedSize(dialog).width).toBe(before.width + 60)
    expect(paintedSize(dialog).height).toBe(before.height - 50)
  })

  it('resizes from the keyboard, so a pointer is not the only way', () => {
    renderWindow()
    const dialog = floatingDialog()
    const before = paintedSize(dialog)
    const corner = screen.getAllByRole('separator', { name: '调整窗口大小' })[2]

    // Left narrows the window because the grip is on the right edge.
    fireEvent.keyDown(corner, { key: 'ArrowLeft' })
    expect(paintedSize(dialog).width).toBe(before.width - 16)
    fireEvent.keyDown(corner, { key: 'ArrowDown', shiftKey: true })
    expect(paintedSize(dialog).height).toBe(before.height + 64)
    // A key the handle does not use leaves the geometry alone.
    fireEvent.keyDown(corner, { key: 'Enter' })
    expect(paintedSize(dialog).width).toBe(before.width - 16)
  })

  it('keeps the resized geometry across minimize and restore', () => {
    renderWindow()
    const dialog = floatingDialog()
    const corner = screen.getAllByRole('separator', { name: '调整窗口大小' })[2]
    fireEvent.pointerDown(corner, { pointerId: 11, clientX: 500, clientY: 500 })
    fireEvent.pointerMove(corner, { pointerId: 11, clientX: 450, clientY: 450 })
    fireEvent.pointerUp(corner, { pointerId: 11, clientX: 450, clientY: 450 })
    const resized = paintedSize(dialog)

    fireEvent.click(screen.getByRole('button', { name: '最小化' }))
    fireEvent.click(screen.getByTestId('story-window-minimized-entry'))

    expect(screen.getByRole('dialog', { name: '人物' })).toHaveAttribute('data-story-window', 'floating')
    expect(paintedSize(dialog)).toEqual(resized)
  })

  it('floats over the application rather than inside the stage column', () => {
    renderWindow()
    const dialog = floatingDialog()
    // The stage here is a 900×700 box inside a 1024×768 viewport. The window is
    // positioned against the viewport and pinned with `fixed`, so it is not
    // trapped in the column it was docked in.
    expect(dialog.className).toContain('fixed')
    expect(dialog.className).not.toContain('absolute')
    const box = paintedBox(dialog)
    expect(box.x + box.width).toBe(VIEWPORT.width - 16)
    expect(box.y).toBe(16)
  })

  it('remembers where it was left, per window kind', () => {
    const first = renderWindow(undefined, 'character')
    const dialog = floatingDialog()
    const corner = screen.getAllByRole('separator', { name: '调整窗口大小' })[2]
    fireEvent.pointerDown(corner, { pointerId: 21, clientX: 500, clientY: 500 })
    fireEvent.pointerMove(corner, { pointerId: 21, clientX: 440, clientY: 540 })
    fireEvent.pointerUp(corner, { pointerId: 21, clientX: 440, clientY: 540 })
    const left = paintedBox(dialog)
    first.unmount()

    // Reopening the window — this session or the next one — returns it floating
    // exactly where it was, without the player floating it again.
    renderWindow(undefined, 'character')
    const reopened = screen.getByRole('dialog', { name: '人物' })
    expect(reopened).toHaveAttribute('data-story-window', 'floating')
    expect(paintedBox(reopened)).toEqual(left)

    // A different kind of window has its own place; it does not inherit this one.
    renderWindow(undefined, 'combat')
    const other = screen.getAllByRole('dialog', { name: '人物' })[1]
    expect(other).toHaveAttribute('data-story-window', 'docked')
  })

  it('docking again is remembered, so the next open is a docked panel', () => {
    const first = renderWindow(undefined, 'character')
    floatingDialog()
    fireEvent.click(screen.getByRole('button', { name: '停靠回侧栏' }))
    first.unmount()

    renderWindow(undefined, 'character')
    expect(screen.getByRole('dialog', { name: '人物' })).toHaveAttribute('data-story-window', 'docked')
  })

  it('pulls a remembered place back in when the viewport has narrowed', () => {
    // A window last used on a wide screen is not stranded past the edge of a
    // narrow one: its header has to stay grabbable or it can never come back.
    window.localStorage.setItem(
      'ta.story-window.geometry.character',
      JSON.stringify({ x: 1600, y: 300, width: 700, height: 640, floating: true }),
    )
    Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: 800 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, writable: true, value: 600 })

    renderWindow(undefined, 'character')
    const dialog = screen.getByRole('dialog', { name: '人物' })
    const box = paintedBox(dialog)
    expect(box.width).toBeLessThanOrEqual(800)
    expect(box.height).toBeLessThanOrEqual(600)
    expect(box.x).toBeGreaterThanOrEqual(0)
    expect(box.y).toBeGreaterThanOrEqual(0)
    expect(box.x + box.width).toBeLessThanOrEqual(800)
    expect(box.y + box.height).toBeLessThanOrEqual(600)
  })

  it('carries the window controls into the panel header', () => {
    renderWindow((controls) => (
      <header>
        <h2>标题</h2>
        {controls.windowActions}
      </header>
    ))
    fireEvent.click(screen.getByRole('button', { name: '改为浮窗' }))
    expect(screen.getByRole('dialog', { name: '人物' })).toHaveAttribute('data-story-window', 'floating')
    // The panel draws the controls, so they must live inside the panel's own
    // header rather than in a second title bar stacked above it.
    expect(screen.getByRole('button', { name: '停靠回侧栏' }).closest('header')).not.toBeNull()
  })
})
