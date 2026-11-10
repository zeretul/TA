import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import type { CombatWindow } from '../../types'
import { combatNarrationCommandId, useCombatWindow } from './use-combat-window'

const apiMocks = vi.hoisted(() => ({
  getCombatWindow: vi.fn(),
  playCombatRound: vi.fn(),
  narrateCombatRound: vi.fn(),
}))

vi.mock('../../api', () => apiMocks)

function windowData(overrides: Partial<CombatWindow> = {}): CombatWindow {
  return {
    story_id: 'story-1',
    branch_id: 'branch-1',
    scene: { node_id: 'node-1' },
    session_id: 'battle-1',
    round: 3,
    ended: false,
    has_more: false,
    sides: [],
    last_round: { event_id: 'round-2', outcome: { round: 2, steps: [] } },
    ...overrides,
  }
}

function mountHook(branchId = 'branch-1', sessionId = 'battle-1') {
  return renderHook(() => useCombatWindow({ storyId: 'story-1', branchId, sessionId, active: true }))
}

async function ready(result: { current: ReturnType<typeof useCombatWindow> }) {
  await waitFor(() => expect(result.current.data).not.toBeNull())
}

function narrationIds(): string[] {
  return apiMocks.narrateCombatRound.mock.calls.map(call => call[1].commandId)
}

beforeEach(() => {
  apiMocks.getCombatWindow.mockReset()
  apiMocks.playCombatRound.mockReset()
  apiMocks.narrateCombatRound.mockReset()
  apiMocks.getCombatWindow.mockResolvedValue(windowData())
  window.localStorage.clear()
})

describe('combatNarrationCommandId', () => {
  it('is stable for one round and differs across story, branch, battle and round', () => {
    const base = combatNarrationCommandId('story-1', 'branch-1', 'battle-1', 'round-2')
    expect(combatNarrationCommandId('story-1', 'branch-1', 'battle-1', 'round-2')).toBe(base)
    expect(combatNarrationCommandId('story-2', 'branch-1', 'battle-1', 'round-2')).not.toBe(base)
    expect(combatNarrationCommandId('story-1', 'branch-2', 'battle-1', 'round-2')).not.toBe(base)
    expect(combatNarrationCommandId('story-1', 'branch-1', 'battle-2', 'round-2')).not.toBe(base)
    expect(combatNarrationCommandId('story-1', 'branch-1', 'battle-1', 'round-3')).not.toBe(base)
  })

  it('cannot be forged by moving a separator between two parts', () => {
    expect(combatNarrationCommandId('a:b', 'c', 'd', 'e')).not.toBe(combatNarrationCommandId('a', 'b:c', 'd', 'e'))
  })
})

describe('useCombatWindow narration retry identity', () => {
  it('sends the same identity for the first passage and the manual retry', async () => {
    apiMocks.playCombatRound.mockResolvedValue(windowData({ last_round: { event_id: 'round-3', outcome: { round: 3, steps: [] } } }))
    // The first passage is generated but its response is lost.
    apiMocks.narrateCombatRound
      .mockRejectedValueOnce(new Error('network lost'))
      .mockResolvedValueOnce({})
    const { result } = mountHook()
    await ready(result)

    await act(async () => { await result.current.playRound([]) })
    await waitFor(() => expect(result.current.narrationFailure?.roundEventId).toBe('round-3'))
    await act(async () => { await result.current.narrateRound('round-3') })

    const expected = combatNarrationCommandId('story-1', 'branch-1', 'battle-1', 'round-3')
    expect(narrationIds()).toEqual([expected, expected])
    // The round's own command identity is a different thing and is not reused.
    expect(apiMocks.playCombatRound.mock.calls[0][1].commandId).not.toBe(expected)
    // Recovery writes prose; it never settles another round.
    expect(apiMocks.playCombatRound).toHaveBeenCalledTimes(1)
    expect(result.current.narrationFailure).toBeNull()
  })

  it('keeps the identity after the panel is closed and reopened', async () => {
    apiMocks.narrateCombatRound.mockRejectedValueOnce(new Error('network lost')).mockResolvedValueOnce({})
    const first = mountHook()
    await ready(first.result)
    await act(async () => { await first.result.current.narrateRound('round-2') })
    first.unmount()

    const second = mountHook()
    await ready(second.result)
    await act(async () => { await second.result.current.narrateRound('round-2') })

    const expected = combatNarrationCommandId('story-1', 'branch-1', 'battle-1', 'round-2')
    expect(narrationIds()).toEqual([expected, expected])
  })

  it('keeps a failed generation retryable under the same identity', async () => {
    apiMocks.narrateCombatRound
      .mockRejectedValueOnce(new Error('the model did not answer'))
      .mockRejectedValueOnce(new Error('the model did not answer'))
      .mockResolvedValueOnce({})
    const { result } = mountHook()
    await ready(result)

    let outcomes: boolean[] = []
    for (let attempt = 0; attempt < 3; attempt += 1) {
      await act(async () => { outcomes = [...outcomes, await result.current.narrateRound('round-2')] })
    }
    expect(outcomes).toEqual([false, false, true])
    expect(new Set(narrationIds()).size).toBe(1)
    expect(apiMocks.playCombatRound).not.toHaveBeenCalled()
  })

  it('uses another identity for another round', async () => {
    apiMocks.narrateCombatRound.mockResolvedValue({})
    const { result } = mountHook()
    await ready(result)
    await act(async () => { await result.current.narrateRound('round-1') })
    await act(async () => { await result.current.narrateRound('round-2') })
    const [one, two] = narrationIds()
    expect(one).toBe(combatNarrationCommandId('story-1', 'branch-1', 'battle-1', 'round-1'))
    expect(two).toBe(combatNarrationCommandId('story-1', 'branch-1', 'battle-1', 'round-2'))
    expect(one).not.toBe(two)
  })

  it('uses another identity on another branch, taking the branch the window reports', async () => {
    apiMocks.narrateCombatRound.mockResolvedValue({})
    apiMocks.getCombatWindow.mockResolvedValue(windowData({ branch_id: 'branch-2' }))
    // The caller did not name a branch: the identity must still bind the real one.
    const { result } = mountHook('')
    await ready(result)
    await act(async () => { await result.current.narrateRound('round-2') })
    expect(narrationIds()).toEqual([combatNarrationCommandId('story-1', 'branch-2', 'battle-1', 'round-2')])
    expect(narrationIds()[0]).not.toBe(combatNarrationCommandId('story-1', 'branch-1', 'battle-1', 'round-2'))
  })
})

function roundCommandIds(): string[] {
  return apiMocks.playCombatRound.mock.calls.map(call => call[1].commandId)
}

const strike = [{ actor_id: 'hero', kind: 'technique' as const, skill_id: 'strike', target_id: 'wolf' }]
const guard = [{ actor_id: 'hero', kind: 'rest' as const }]

describe('useCombatWindow command recovery', () => {
  it('keeps the identity when the answer is lost, so the retry reads the same round', async () => {
    apiMocks.playCombatRound
      .mockRejectedValueOnce(new APIError('network lost', { status: 0, code: 'client.network_error' }))
      .mockResolvedValueOnce(windowData({ last_round: { event_id: 'round-3', outcome: { round: 3, steps: [] } } }))
    const { result } = mountHook()
    await ready(result)

    await act(async () => { await result.current.playRound(strike) })
    expect(result.current.playErrorKind).toBe('unknown')
    expect(result.current.pendingCommandResendable).toBe(true)

    await act(async () => { await result.current.retryRound(strike) })

    const [first, second] = roundCommandIds()
    expect(second).toBe(first)
    expect(apiMocks.playCombatRound.mock.calls[1][1].actions).toEqual(strike)
    expect(result.current.pendingCommandId).toBeNull()
  })

  it('keeps the identity when the server failed after it may have committed', async () => {
    apiMocks.playCombatRound
      .mockRejectedValueOnce(new APIError('upstream is gone', { status: 503 }))
      .mockResolvedValueOnce({})
    const { result } = mountHook()
    await ready(result)

    await act(async () => { await result.current.playRound(strike) })
    await act(async () => { await result.current.retryRound(strike) })

    const [first, second] = roundCommandIds()
    expect(second).toBe(first)
  })

  it('mints a new identity for a corrected action after a definite refusal', async () => {
    apiMocks.playCombatRound
      .mockRejectedValueOnce(new APIError('Action rejected', { status: 400, code: 'api.combatWindow.actionRejected' }))
      .mockResolvedValueOnce({})
    const { result } = mountHook()
    await ready(result)

    await act(async () => { await result.current.playRound(strike) })
    expect(result.current.playErrorKind).toBe('refused')
    expect(result.current.pendingCommandId).toBeNull()

    await act(async () => { await result.current.retryRound(guard) })
    const [first, second] = roundCommandIds()
    expect(second).not.toBe(first)
    expect(apiMocks.playCombatRound.mock.calls[1][1].actions).toEqual(guard)
  })

  it('does not let a different action ride an unconfirmed identity', async () => {
    apiMocks.playCombatRound
      .mockRejectedValueOnce(new APIError('network lost', { status: 0, code: 'client.network_error' }))
      .mockResolvedValueOnce({})
    const { result } = mountHook()
    await ready(result)

    await act(async () => { await result.current.playRound(strike) })
    // The player changes their mind while the outcome is unknown. That is not a
    // new round: the first attempt may already have settled, and sending another
    // action beside it would settle twice for one fight.
    await act(async () => { await result.current.playRound(guard) })

    expect(apiMocks.playCombatRound).toHaveBeenCalledTimes(1)
    expect(result.current.playErrorKind).toBe('blocked')
    expect(result.current.pendingCommandId).toBe(roundCommandIds()[0])

    // What is waiting is resent exactly as it was minted.
    await act(async () => { await result.current.resendPending() })
    const [first, second] = roundCommandIds()
    expect(second).toBe(first)
    expect(apiMocks.playCombatRound.mock.calls[1][1].actions).toEqual(strike)
  })

  it('unbinds the identity on a conflict and lets a new choice continue', async () => {
    apiMocks.playCombatRound
      .mockRejectedValueOnce(new APIError('command reused', { status: 409, code: 'api.combatWindow.commandConflict' }))
      .mockResolvedValueOnce({})
    const { result } = mountHook()
    await ready(result)

    await act(async () => { await result.current.playRound(strike) })
    // The formal battle was read before anything was concluded.
    expect(apiMocks.getCombatWindow.mock.calls.length).toBeGreaterThan(1)
    expect(result.current.playErrorKind).toBe('conflict')
    expect(result.current.pendingCommandId).toBeNull()
    // The submission is over: a stuck "settling" would leave no way to continue.
    expect(result.current.playing).toBe(false)

    await act(async () => { await result.current.playRound(guard) })
    const [first, second] = roundCommandIds()
    expect(second).not.toBe(first)
    expect(apiMocks.playCombatRound.mock.calls[1][1].actions).toEqual(guard)
  })

  it('shows the round a conflict actually committed and ends the pending state', async () => {
    apiMocks.playCombatRound.mockRejectedValue(
      new APIError('command reused', { status: 409, code: 'api.combatWindow.commandConflict' }),
    )
    // The conflict is answered by reading the formal battle, and that read shows
    // the first attempt did commit: the battle has moved on by one exchange.
    let reads = 0
    apiMocks.getCombatWindow.mockImplementation(async () => {
      reads += 1
      return reads === 1
        ? windowData()
        : windowData({ round: 4, last_round: { event_id: 'round-3', outcome: { round: 3, steps: [] } } })
    })
    const { result } = mountHook()
    await ready(result)

    await act(async () => { await result.current.playRound(strike) })

    expect(result.current.playError).toBeNull()
    expect(result.current.pendingCommandId).toBeNull()
    expect(result.current.data?.last_round?.event_id).toBe('round-3')
    // The committed round is read back, never resolved a second time.
    expect(apiMocks.playCombatRound).toHaveBeenCalledTimes(1)
  })

  it('shows and resends the original lethal intent and target after the panel is reopened', async () => {
    apiMocks.playCombatRound
      .mockRejectedValueOnce(new APIError('network lost', { status: 0, code: 'client.network_error' }))
      .mockResolvedValueOnce({})
    const first = mountHook()
    await ready(first.result)
    await act(async () => { await first.result.current.playRound(strike, 'take_life') })
    const commandId = roundCommandIds()[0]
    first.unmount()

    const second = mountHook()
    await ready(second.result)
    // The unknown outcome is still known after the panel was reopened, and it is
    // reported as unknown rather than looking like a round nobody has played.
    expect(second.result.current.pendingCommandId).toBe(commandId)
    expect(second.result.current.playErrorKind).toBe('unknown')
    expect(second.result.current.pendingCommandIntent).toBe('take_life')
    expect(second.result.current.pendingCommandTargetID).toBe('wolf')
    await act(async () => { await second.result.current.resendPending() })

    const [before, after] = roundCommandIds()
    expect(after).toBe(before)
    expect(apiMocks.playCombatRound.mock.calls[1][1].actions).toEqual(strike)
    expect(apiMocks.playCombatRound.mock.calls[1][1].playerIntent).toBe('take_life')
  })

  it('restores an unknown outcome from storage after a full reload', async () => {
    apiMocks.playCombatRound
      .mockRejectedValueOnce(new APIError('network lost', { status: 0, code: 'client.network_error' }))
      .mockResolvedValueOnce({})
    const first = mountHook()
    await ready(first.result)
    await act(async () => { await first.result.current.playRound(strike) })
    first.unmount()

    // A reload is a fresh module and a fresh hook: only storage carries over.
    const second = mountHook()
    await ready(second.result)
    expect(second.result.current.pendingCommandId).toBe(roundCommandIds()[0])
    expect(second.result.current.playErrorKind).toBe('unknown')
    expect(second.result.current.pendingCommandResendable).toBe(true)
  })
})
