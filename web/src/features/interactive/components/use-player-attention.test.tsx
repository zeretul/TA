import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { usePlayerAttention } from './use-player-attention'
import type { PlayerAttention } from '../player-attention-api'

const api = vi.hoisted(() => ({ getPlayerAttention: vi.fn(), updatePlayerAttention: vi.fn() }))
vi.mock('../player-attention-api', () => api)
const empty: PlayerAttention = { v: 1, head: 'h1', characters: [], quests: [] }
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
beforeEach(() => { vi.resetAllMocks(); api.getPlayerAttention.mockResolvedValue(empty) })

describe('canonical player attention', () => {
  it('saves a desired state with the captured head and ignores duplicate clicks', async () => {
    const saved = deferred<PlayerAttention>()
    api.updatePlayerAttention.mockReturnValue(saved.promise)
    const { result } = renderHook(() => usePlayerAttention({ storyId: 'a', branchId: 'main', active: true }))
    await waitFor(() => expect(result.current.data).toEqual(empty))
    let first!: Promise<boolean>
    act(() => { first = result.current.setFollowing('character', 'known', true); void result.current.setFollowing('character', 'known', true) })
    expect(result.current.data?.characters).toEqual(['known'])
    expect(api.updatePlayerAttention).toHaveBeenCalledTimes(1)
    expect(api.updatePlayerAttention).toHaveBeenCalledWith('a', { branch: 'main', expected_head: 'h1', kind: 'character', id: 'known', following: true })
    await act(async () => { saved.resolve({ ...empty, head: 'h2', characters: ['known'] }); await first })
    expect(result.current.data?.head).toBe('h2')
  })

  it('rolls back a failed unfollow and reloads the confirmed state', async () => {
    const original = { ...empty, quests: ['known'] }
    api.getPlayerAttention.mockResolvedValue(original)
    const failed = deferred<PlayerAttention>()
    api.updatePlayerAttention.mockReturnValue(failed.promise)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { result } = renderHook(() => usePlayerAttention({ storyId: 'a', branchId: 'main', active: true }))
    await waitFor(() => expect(result.current.data).toEqual(original))
    let operation!: Promise<boolean>
    act(() => { operation = result.current.setFollowing('quest', 'known', false) })
    expect(result.current.data?.quests).toEqual([])
    await act(async () => { failed.reject(new Error('拒绝')); expect(await operation).toBe(false) })
    expect(result.current.data).toEqual(original)
    expect(result.current.error).toBe('拒绝')
    expect(result.current.busy).toBe(false)
    warn.mockRestore()
  })

  it('does not apply a late save to another story or continue writes there', async () => {
    const pending = deferred<PlayerAttention>()
    api.updatePlayerAttention.mockReturnValue(pending.promise)
    const { result, rerender } = renderHook(({ storyId }) => usePlayerAttention({ storyId, branchId: 'main', active: true }), { initialProps: { storyId: 'a' } })
    await waitFor(() => expect(result.current.data).toEqual(empty))
    let operation!: Promise<boolean>
    act(() => { operation = result.current.setFollowing('quest', 'old', true) })
    api.getPlayerAttention.mockResolvedValue({ ...empty, head: 'b-head' })
    rerender({ storyId: 'b' })
    await waitFor(() => expect(result.current.data?.head).toBe('b-head'))
    await act(async () => { pending.resolve({ ...empty, head: 'a-late', quests: ['old'] }); expect(await operation).toBe(false) })
    expect(result.current.data).toEqual({ ...empty, head: 'b-head' })
    expect(api.updatePlayerAttention).toHaveBeenCalledTimes(1)
    expect(api.updatePlayerAttention.mock.calls[0][0]).toBe('a')
  })

  it('ignores stale reads after changing branches', async () => {
    const old = deferred<PlayerAttention>()
    api.getPlayerAttention.mockReturnValueOnce(old.promise).mockResolvedValue({ ...empty, head: 'fork', characters: ['fork-person'] })
    const { result, rerender } = renderHook(({ branchId }) => usePlayerAttention({ storyId: 'a', branchId, active: true }), { initialProps: { branchId: 'main' } })
    rerender({ branchId: 'fork' })
    await waitFor(() => expect(result.current.data?.head).toBe('fork'))
    await act(async () => { old.resolve({ ...empty, characters: ['main-person'] }) })
    expect(result.current.data?.characters).toEqual(['fork-person'])
  })
})
