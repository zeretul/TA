import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useInteractiveStore } from './stores/interactive-store'
import { useStoryIndexRecovery } from './use-story-index-recovery'

const getInteractiveStoriesMock = vi.hoisted(() => vi.fn())

vi.mock('./api', () => ({
  getInteractiveStories: getInteractiveStoriesMock,
}))

function story(id: string) {
  return { id, title: id } as never
}

describe('useStoryIndexRecovery', () => {
  beforeEach(() => {
    getInteractiveStoriesMock.mockReset()
    window.localStorage.clear()
    useInteractiveStore.setState({ stories: [], currentStoryId: '', currentBranchId: 'main' })
  })

  afterEach(() => {
    window.localStorage.clear()
  })

  it('restores the current story the server reports for this workspace', async () => {
    getInteractiveStoriesMock.mockResolvedValue({ current_story_id: 'story-b', stories: [story('story-a'), story('story-b')] })

    renderHook(() => useStoryIndexRecovery('/book-a', true))

    await waitFor(() => expect(useInteractiveStore.getState().currentStoryId).toBe('story-b'))
    expect(getInteractiveStoriesMock).toHaveBeenCalledTimes(1)
  })

  it('leaves an empty workspace empty instead of restoring anything', async () => {
    renderHook(() => useStoryIndexRecovery('', true))

    await waitFor(() => expect(useInteractiveStore.getState().stories).toEqual([]))
    expect(useInteractiveStore.getState().currentStoryId).toBe('')
    expect(getInteractiveStoriesMock).not.toHaveBeenCalled()
  })

  it('drops the previous workspace stories before the new index lands', async () => {
    getInteractiveStoriesMock.mockResolvedValueOnce({ current_story_id: 'story-a', stories: [story('story-a')] })
    const { rerender } = renderHook(({ workspace }) => useStoryIndexRecovery(workspace, true), {
      initialProps: { workspace: '/book-a' },
    })
    await waitFor(() => expect(useInteractiveStore.getState().currentStoryId).toBe('story-a'))

    let resolveSecond!: (value: unknown) => void
    getInteractiveStoriesMock.mockReturnValueOnce(new Promise((resolve) => { resolveSecond = resolve }))
    rerender({ workspace: '/book-b' })

    // The switch clears first, so the library cannot offer the other world's story.
    await waitFor(() => expect(useInteractiveStore.getState().currentStoryId).toBe(''))

    resolveSecond({ current_story_id: 'story-c', stories: [story('story-c')] })
    await waitFor(() => expect(useInteractiveStore.getState().currentStoryId).toBe('story-c'))
  })

  it('ignores a reply that arrives after the surface stopped owning the index', async () => {
    let resolveFirst!: (value: unknown) => void
    getInteractiveStoriesMock.mockReturnValueOnce(new Promise((resolve) => { resolveFirst = resolve }))
    const { rerender } = renderHook(({ enabled }) => useStoryIndexRecovery('/book-a', enabled), {
      initialProps: { enabled: true },
    })

    rerender({ enabled: false })
    resolveFirst({ current_story_id: 'story-a', stories: [story('story-a')] })

    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(useInteractiveStore.getState().currentStoryId).toBe('')
  })

  it('does not read the index while the game surface owns it', async () => {
    renderHook(() => useStoryIndexRecovery('/book-a', false))

    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(getInteractiveStoriesMock).not.toHaveBeenCalled()
  })
})
