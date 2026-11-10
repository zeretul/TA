import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import { useStoryAssistant } from './use-story-assistant'
import type { StoryAssistantResult } from '../../types'

const apiMock = vi.hoisted(() => ({
  askStoryAssistant: vi.fn(),
  adoptStoryAssistantDirection: vi.fn(),
}))

vi.mock('../../api', () => ({
  askStoryAssistant: apiMock.askStoryAssistant,
  adoptStoryAssistantDirection: apiMock.adoptStoryAssistantDirection,
}))

function answer(overrides: Partial<StoryAssistantResult> = {}): StoryAssistantResult {
  return {
    story_id: 'st_1', branch_id: 'main', turn_id: 'ev_1', action: 'analyze',
    read: '门口的两个人并不认识你。', suggestions: [], context_revision: 7,
    ...overrides,
  }
}

const base = { storyId: 'st_1', branchId: 'main', turnId: 'ev_1', planRevision: '3' }

describe('useStoryAssistant', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps an answer bound to the turn it was asked about', async () => {
    apiMock.askStoryAssistant.mockResolvedValue(answer())
    const { result } = renderHook(() => useStoryAssistant(base))

    await act(async () => {
      await result.current.ask({ action: 'analyze', playerInput: '先看看门口的人' })
    })

    expect(apiMock.askStoryAssistant).toHaveBeenCalledWith('st_1', {
      branchId: 'main', turnId: 'ev_1', action: 'analyze',
      playerInput: '先看看门口的人',
    })
    expect(result.current.answer?.read).toContain('门口')
    expect(result.current.answer?.stale).toBe(false)
  })

  it('answers a second question after switching action in the same turn', async () => {
    apiMock.askStoryAssistant
      .mockResolvedValueOnce(answer({ action: 'analyze', read: '门口的两个人并不认识你。' }))
      .mockResolvedValueOnce(answer({
        action: 'inspire',
        read: '你还没问过那个挑担的人。',
        suggestions: [{ label: '问挑担的人', detail: '他刚进城，可能见过门口那两个人。', action: '过去问挑担的人从哪来' }],
      }))
    const { result } = renderHook(() => useStoryAssistant(base))

    await act(async () => {
      await result.current.ask({ action: 'analyze' })
    })
    await act(async () => {
      await result.current.ask({ action: 'inspire' })
    })

    // The first answer must not have left the panel locked: a player who reads
    // the analysis and then asks for ideas is the ordinary way this is used.
    expect(apiMock.askStoryAssistant).toHaveBeenCalledTimes(2)
    expect(apiMock.askStoryAssistant.mock.calls[1][1]).toMatchObject({ action: 'inspire', turnId: 'ev_1' })
    expect(result.current.asking).toBeNull()
    expect(result.current.answer?.action).toBe('inspire')
    expect(result.current.answer?.suggestions).toHaveLength(1)
  })

  it('answers again for the same moment when the player asks twice', async () => {
    apiMock.askStoryAssistant
      .mockResolvedValueOnce(answer({ read: '第一遍：门口那两个人。' }))
      .mockResolvedValueOnce(answer({ read: '第二遍：往东的岔路。' }))
    const { result } = renderHook(() => useStoryAssistant(base))

    await act(async () => {
      await result.current.ask({ action: 'inspire' })
    })
    await act(async () => {
      await result.current.ask({ action: 'inspire' })
    })

    expect(result.current.answer?.read).toContain('第二遍')
    expect(result.current.answer?.stale).toBe(false)
  })

  it('still answers after the story advances to the next turn', async () => {
    apiMock.askStoryAssistant
      .mockResolvedValueOnce(answer({ turn_id: 'ev_1' }))
      .mockResolvedValueOnce(answer({ turn_id: 'ev_2', read: '新回合的场面。' }))
    const { result, rerender } = renderHook(
      (props: typeof base) => useStoryAssistant(props),
      { initialProps: base },
    )

    await act(async () => {
      await result.current.ask({ action: 'analyze' })
    })
    rerender({ ...base, turnId: 'ev_2' })
    await act(async () => {
      await result.current.ask({ action: 'analyze' })
    })

    // Advancing the turn clears the previous failure and the previous adoption,
    // but it must not leave the panel unable to ask about the new scene.
    expect(apiMock.askStoryAssistant.mock.calls[1][1]).toMatchObject({ turnId: 'ev_2' })
    expect(result.current.asking).toBeNull()
    expect(result.current.error).toBeNull()
    expect(result.current.answer?.read).toContain('新回合')
    expect(result.current.answer?.stale).toBe(false)
  })

  it('marks an answer that lands after the branch moved on as stale', async () => {
    apiMock.askStoryAssistant.mockResolvedValue(answer())
    const { result, rerender } = renderHook(
      (props: typeof base) => useStoryAssistant(props),
      { initialProps: base },
    )

    await act(async () => {
      await result.current.ask({ action: 'inspire' })
    })
    // The world moves on while the answer sits in the panel.
    rerender({ ...base, turnId: 'ev_2' })

    expect(result.current.answer?.stale).toBe(true)
  })

  it('writes nothing when the question fails', async () => {
    apiMock.askStoryAssistant.mockRejectedValue(new Error('model unavailable'))
    const { result } = renderHook(() => useStoryAssistant(base))

    await act(async () => {
      await result.current.ask({ action: 'analyze' })
    })

    expect(result.current.error).toContain('model unavailable')
    expect(result.current.answer).toBeNull()
    expect(apiMock.adoptStoryAssistantDirection).not.toHaveBeenCalled()
  })

  it('drops an answer when the branch changes', async () => {
    apiMock.askStoryAssistant.mockResolvedValue(answer())
    const { result, rerender } = renderHook(
      (props: typeof base) => useStoryAssistant(props),
      { initialProps: base },
    )

    await act(async () => {
      await result.current.ask({ action: 'analyze' })
    })
    expect(result.current.answer).not.toBeNull()

    rerender({ ...base, branchId: 'fork' })
    await waitFor(() => expect(result.current.answer).toBeNull())
  })

  it('does not adopt when the branch has no plan to record it in', async () => {
    const { result } = renderHook(() => useStoryAssistant({ ...base, planRevision: undefined }))

    expect(result.current.canAdopt).toBe(false)
    await act(async () => {
      await result.current.adopt('先去城南的药铺')
    })
    expect(apiMock.adoptStoryAssistantDirection).not.toHaveBeenCalled()
  })

  it('keeps a failed adoption retryable with the same wording', async () => {
    apiMock.adoptStoryAssistantDirection
      .mockRejectedValueOnce(new Error('conflict'))
      .mockResolvedValueOnce({
        story_id: 'st_1', branch_id: 'main', turn_id: 'ev_1', scope: 'branch',
        branch_plan: { markdown: '## Near-term beats\n\n- 先去城南的药铺', revision: '4' },
        context_revision: 8, applied: true,
      })
    const { result } = renderHook(() => useStoryAssistant(base))

    let first = true
    await act(async () => {
      first = await result.current.adopt('先去城南的药铺')
    })
    expect(first).toBe(false)
    expect(result.current.adoptError).toContain('conflict')

    let second = false
    await act(async () => {
      second = await result.current.adopt('先去城南的药铺')
    })
    expect(second).toBe(true)
    expect(apiMock.adoptStoryAssistantDirection).toHaveBeenCalledTimes(2)
    expect(apiMock.adoptStoryAssistantDirection.mock.calls[0][1]).toEqual(
      apiMock.adoptStoryAssistantDirection.mock.calls[1][1],
    )
  })

  it('sends the plan revision the player was shown', async () => {
    apiMock.adoptStoryAssistantDirection.mockResolvedValue({
      story_id: 'st_1', branch_id: 'main', turn_id: 'ev_1', scope: 'branch',
      branch_plan: { markdown: '## Near-term beats\n\n- 先去城南的药铺', revision: '4' },
      context_revision: 8, applied: true,
    })
    const { result } = renderHook(() => useStoryAssistant(base))

    await act(async () => {
      await result.current.adopt('先去城南的药铺')
    })

    expect(apiMock.adoptStoryAssistantDirection).toHaveBeenCalledWith('st_1', {
      branchId: 'main', turnId: 'ev_1', direction: '先去城南的药铺', baseRevision: '3',
    })
  })

  it('adopts a second direction against the revision the first adoption wrote', async () => {
    apiMock.adoptStoryAssistantDirection
      .mockResolvedValueOnce({
        story_id: 'st_1', branch_id: 'main', turn_id: 'ev_1', scope: 'branch',
        branch_plan: { markdown: '## Near-term beats\n\n- 先去城南的药铺', revision: '4' },
        context_revision: 8, applied: true,
      })
      .mockResolvedValueOnce({
        story_id: 'st_1', branch_id: 'main', turn_id: 'ev_1', scope: 'branch',
        branch_plan: { markdown: '## Near-term beats\n\n- 先去城南的药铺\n- 顺路打听丹药行情', revision: '5' },
        context_revision: 9, applied: true,
      })
    const onPlanAdvanced = vi.fn()
    const { result } = renderHook(() => useStoryAssistant({ ...base, onPlanAdvanced }))

    await act(async () => {
      await result.current.adopt('先去城南的药铺')
    })
    await act(async () => {
      await result.current.adopt('顺路打听丹药行情')
    })

    expect(apiMock.adoptStoryAssistantDirection.mock.calls[1][1]).toMatchObject({
      direction: '顺路打听丹药行情', baseRevision: '4',
    })
    expect(onPlanAdvanced).toHaveBeenCalledTimes(2)
  })

  it('stops asking after the panel is cleared and ignores the late answer', async () => {
    let release: (value: StoryAssistantResult) => void = () => {}
    apiMock.askStoryAssistant.mockReturnValue(new Promise<StoryAssistantResult>((resolve) => { release = resolve }))
    const { result } = renderHook(() => useStoryAssistant(base))

    let pending: Promise<void> = Promise.resolve()
    act(() => {
      pending = result.current.ask({ action: 'analyze' })
    })
    expect(result.current.asking).toBe('analyze')

    act(() => {
      result.current.clear()
    })
    expect(result.current.asking).toBeNull()

    await act(async () => {
      release(answer({ read: '迟到的回答' }))
      await pending
    })
    expect(result.current.answer).toBeNull()
  })

  it('frees the adoption after a cancelled attempt so a later direction is not locked out', async () => {
    apiMock.adoptStoryAssistantDirection.mockImplementation(() => new Promise(() => {}))
    const { result } = renderHook(() => useStoryAssistant(base))

    act(() => {
      void result.current.adopt('先去城南的药铺')
    })
    expect(result.current.adopting).toBe(true)

    act(() => {
      result.current.clear()
    })
    expect(result.current.adopting).toBe(false)

    apiMock.adoptStoryAssistantDirection.mockResolvedValue({
      story_id: 'st_1', branch_id: 'main', turn_id: 'ev_1', scope: 'branch',
      branch_plan: { markdown: '## Near-term beats\n\n- 顺路打听丹药行情', revision: '4' },
      context_revision: 8, applied: true,
    })
    await act(async () => {
      await result.current.adopt('顺路打听丹药行情')
    })
    expect(apiMock.adoptStoryAssistantDirection).toHaveBeenLastCalledWith('st_1', expect.objectContaining({
      direction: '顺路打听丹药行情',
    }))
  })

  it('clears the previous failure when the story moves to another turn', async () => {
    apiMock.askStoryAssistant.mockRejectedValue(new Error('model unavailable'))
    apiMock.adoptStoryAssistantDirection.mockRejectedValue(new Error('ended turn'))
    const { result, rerender } = renderHook(
      (props: typeof base) => useStoryAssistant(props),
      { initialProps: base },
    )

    await act(async () => {
      await result.current.ask({ action: 'analyze' })
      await result.current.adopt('先去城南的药铺')
    })
    expect(result.current.error).toContain('model unavailable')
    expect(result.current.adoptError).toContain('ended turn')

    rerender({ ...base, turnId: 'ev_2' })
    await waitFor(() => expect(result.current.error).toBeNull())
    expect(result.current.adoptError).toBeNull()
  })

  it('separates a plan conflict from other refusals and keeps the advice on screen', async () => {
    apiMock.askStoryAssistant.mockResolvedValue(answer({
      suggestions: [{ label: '问药铺掌柜', detail: '他知道城里最近的动静', action: '去药铺问掌柜' }],
    }))
    apiMock.adoptStoryAssistantDirection.mockRejectedValue(
      new APIError('branch plan changed', { status: 409, code: 'api.storyAssistant.planConflict' }),
    )
    const { result } = renderHook(() => useStoryAssistant(base))

    await act(async () => {
      await result.current.ask({ action: 'inspire' })
    })
    await act(async () => {
      await result.current.adopt('去药铺问掌柜')
    })

    expect(result.current.adoptConflict).toBe(true)
    expect(result.current.answer?.suggestions).toHaveLength(1)

    // A non-conflict refusal must not ask the player to re-read the plan.
    apiMock.adoptStoryAssistantDirection.mockRejectedValue(
      new APIError('turn already ended', { status: 409, code: 'api.storyAssistant.staleTurn' }),
    )
    await act(async () => {
      await result.current.adopt('去药铺问掌柜')
    })
    expect(result.current.adoptConflict).toBe(false)
    expect(result.current.adoptError).not.toBeNull()
  })
})
