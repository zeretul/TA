import { act, renderHook } from '@testing-library/react'
import type { PointerEvent, WheelEvent } from 'react'
import { expect, it } from 'vitest'
import { useVirtuosoBottomLock } from './useVirtuosoBottomLock'

it('follows footer expansion from the bottom throughout layout updates', () => {
  const scroller = document.createElement('div')
  Object.defineProperty(scroller, 'clientHeight', { value: 500 })
  Object.defineProperty(scroller, 'scrollHeight', { value: 1000, configurable: true })
  scroller.scrollTop = 500
  const { result } = renderHook(() => useVirtuosoBottomLock({ itemCount: 3, autoFollowEnabled: false, resolveScroller: () => scroller }))
  act(() => { result.current.beginAfterContentInteraction(); result.current.releaseBottomLock() })
  for (const height of [1100, 1250, 1400, 1000]) {
    Object.defineProperty(scroller, 'scrollHeight', { value: height, configurable: true })
    act(() => result.current.restoreAfterContentScrollPosition())
    expect(scroller.scrollTop).toBe(height - 500)
  }
})

it('preserves the reading position when expansion starts away from the bottom', () => {
  const scroller = document.createElement('div')
  Object.defineProperty(scroller, 'clientHeight', { value: 500 })
  Object.defineProperty(scroller, 'scrollHeight', { value: 1000, configurable: true })
  scroller.scrollTop = 200
  const { result } = renderHook(() => useVirtuosoBottomLock({ itemCount: 3, autoFollowEnabled: false, resolveScroller: () => scroller }))
  act(() => result.current.beginAfterContentInteraction())
  Object.defineProperty(scroller, 'scrollHeight', { value: 1400, configurable: true })
  act(() => result.current.restoreAfterContentScrollPosition())
  expect(scroller.scrollTop).toBe(200)
})

it.each(['wheel', 'touch'] as const)('stops following when the user scrolls upward with %s during expansion', (input) => {
  const scroller = document.createElement('div')
  Object.defineProperty(scroller, 'clientHeight', { value: 500 })
  Object.defineProperty(scroller, 'scrollHeight', { value: 1000, configurable: true })
  scroller.scrollTop = 500
  const { result } = renderHook(() => useVirtuosoBottomLock({ itemCount: 3, autoFollowEnabled: false, resolveScroller: () => scroller }))
  act(() => result.current.beginAfterContentInteraction())
  if (input === 'wheel') {
    act(() => result.current.onWheel({ deltaY: -100 } as WheelEvent<HTMLDivElement>))
  } else {
    const footer = document.createElement('div')
    footer.setAttribute('data-nova-chat-after-content', '')
    scroller.append(footer)
    act(() => result.current.onPointerDown({ target: footer, currentTarget: scroller, pointerType: 'touch', clientY: 200 } as unknown as PointerEvent<HTMLDivElement>))
    act(() => result.current.onPointerMove({ clientY: 220 } as PointerEvent<HTMLDivElement>))
  }
  scroller.scrollTop = 400
  Object.defineProperty(scroller, 'scrollHeight', { value: 1400, configurable: true })
  act(() => result.current.restoreAfterContentScrollPosition())
  expect(scroller.scrollTop).toBe(400)
})

it('keeps a revealed turn in place when the list finishes laying out', () => {
  // Virtuoso reports "at bottom" once the completed turn is measured, and an
  // idle layout pass then writes scrollTop to the end. Both would undo a reveal
  // that deliberately placed the page on a turn's first paragraph.
  const scroller = document.createElement('div')
  Object.defineProperty(scroller, 'clientHeight', { value: 500 })
  Object.defineProperty(scroller, 'scrollHeight', { value: 1000, configurable: true })
  scroller.scrollTop = 500
  const { result } = renderHook(() => useVirtuosoBottomLock({ itemCount: 3, autoFollowEnabled: false, resolveScroller: () => scroller }))

  act(() => result.current.scrollElementIntoView(document.createElement('p')))
  // The reveal lands mid-list, not at the end.
  scroller.scrollTop = 500
  act(() => result.current.onScroll({ currentTarget: scroller } as unknown as Parameters<typeof result.current.onScroll>[0]))
  act(() => result.current.onAtBottomStateChange(true))

  Object.defineProperty(scroller, 'scrollHeight', { value: 1300, configurable: true })
  act(() => result.current.syncIdleBottomLayout())

  expect(scroller.scrollTop).toBe(500)
})

it('resumes following once the player scrolls down from a revealed turn', () => {
  const scroller = document.createElement('div')
  Object.defineProperty(scroller, 'clientHeight', { value: 500 })
  Object.defineProperty(scroller, 'scrollHeight', { value: 1000, configurable: true })
  scroller.scrollTop = 500
  const { result } = renderHook(() => useVirtuosoBottomLock({ itemCount: 3, autoFollowEnabled: false, resolveScroller: () => scroller }))

  act(() => result.current.scrollElementIntoView(document.createElement('p')))
  act(() => result.current.onWheel({ deltaY: 120 } as WheelEvent<HTMLDivElement>))
  scroller.scrollTop = 800
  act(() => result.current.onScroll({ currentTarget: scroller } as unknown as Parameters<typeof result.current.onScroll>[0]))

  Object.defineProperty(scroller, 'scrollHeight', { value: 1400, configurable: true })
  act(() => result.current.syncIdleBottomLayout())

  expect(scroller.scrollTop).toBe(900)
})
