import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CharacterChatResult, CharacterWindow } from '../../types'
import { useCharacterWindow, type UseCharacterWindowOptions } from './use-character-window'
const api = vi.hoisted(() => ({ getCharacterWindow: vi.fn(), sendCharacterMessage: vi.fn(), setCharacterMessageStatus: vi.fn() }))
vi.mock('../../api', () => api)
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (error: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const initial: UseCharacterWindowOptions = { storyId: 'story-a', branchId: 'main', characterId: 'person-a', active: true }
const result: CharacterChatResult = { story_id: 'story-a', branch_id: 'main', character_id: 'person-a', reply: 'Old answer', world_time: '', exchange_ids: [], affinity: 20, affinity_band: 'friendly', affinity_delta: 5 }
function windowFor(storyId: string, branchId: string, characterId: string): CharacterWindow {
  return { story_id: storyId, branch_id: branchId, character_id: characterId, scene: { title: '', location: '', world_time: '', present: [] }, characters: [], exchanges: [], matters: [], messages: [], has_more: false }
}
describe('private conversation request ownership', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    api.getCharacterWindow.mockImplementation((storyId, opts) => Promise.resolve(windowFor(storyId, opts.branchId, opts.characterId)))
  })
  for (const changed of [{ characterId: 'person-b' }, { branchId: 'fork' }, { storyId: 'story-b' }]) {
    it(`ignores a completed old send after changing ${Object.keys(changed)[0]}`, async () => {
      const pending = deferred<CharacterChatResult>(); api.sendCharacterMessage.mockReturnValue(pending.promise)
      const hook = renderHook(props => useCharacterWindow(props), { initialProps: initial })
      await waitFor(() => expect(hook.result.current.data).not.toBeNull())
      let sent!: Promise<boolean>
      act(() => { sent = hook.result.current.send('Old draft') })
      const next = { ...initial, ...changed }; hook.rerender(next)
      await waitFor(() => expect(hook.result.current.data).toMatchObject(windowFor(next.storyId, next.branchId, next.characterId)))
      const reads = api.getCharacterWindow.mock.calls.length
      await act(async () => { pending.resolve(result); expect(await sent).toBe(false) })
      expect(api.getCharacterWindow).toHaveBeenCalledTimes(reads)
      expect(hook.result.current).toMatchObject({ sending: false, affinityChange: null, sendError: null })
    })
  }
  it('a cancelled request failing later cannot clear a newer send in the same conversation', async () => {
    const old = deferred<CharacterChatResult>(), next = deferred<CharacterChatResult>()
    api.sendCharacterMessage.mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise)
    const hook = renderHook(props => useCharacterWindow(props), { initialProps: initial })
    let first!: Promise<boolean>, second!: Promise<boolean>
    act(() => { first = hook.result.current.send('Old draft') })
    act(() => { hook.result.current.cancel() })
    act(() => { second = hook.result.current.send('New draft') })
    await act(async () => { old.reject(new Error('Old failure')); await first })
    expect(hook.result.current).toMatchObject({ sending: true, cancelled: false, sendError: null })
    await act(async () => { next.resolve({ ...result, affinity_delta: 0 }); expect(await second).toBe(true) })
    expect(hook.result.current).toMatchObject({ sending: false, cancelled: false })
  })
  it('does not reread the old conversation after a late status change', async () => {
    const pending = deferred<void>(); api.setCharacterMessageStatus.mockReturnValue(pending.promise)
    const hook = renderHook(props => useCharacterWindow(props), { initialProps: initial })
    let marked!: Promise<void>
    act(() => { marked = hook.result.current.markMessage('message-a', 'handled') })
    hook.rerender({ ...initial, characterId: 'person-b' })
    await waitFor(() => expect(hook.result.current.data?.character_id).toBe('person-b'))
    const reads = api.getCharacterWindow.mock.calls.length
    await act(async () => { pending.resolve(); await marked })
    expect(api.getCharacterWindow).toHaveBeenCalledTimes(reads)
    expect(hook.result.current.data?.character_id).toBe('person-b')
  })
  it('discards pending reads when closed and resets busy when retargeted', async () => {
    const read = deferred<CharacterWindow>(); api.getCharacterWindow.mockReturnValueOnce(read.promise)
    const hook = renderHook(props => useCharacterWindow(props), { initialProps: initial })
    hook.rerender({ ...initial, active: false })
    await act(async () => { read.resolve(windowFor('story-a', 'main', 'person-a')) })
    expect(hook.result.current.data).toBeNull()
    expect(hook.result.current.loading).toBe(false)
    hook.rerender(initial)
    await waitFor(() => expect(hook.result.current.data?.character_id).toBe('person-a'))
    api.sendCharacterMessage.mockReturnValue(new Promise(() => {}))
    act(() => { void hook.result.current.send('Old draft') })
    hook.rerender({ ...initial, characterId: 'person-b' })
    await waitFor(() => expect(hook.result.current.sending).toBe(false))
  })
})
