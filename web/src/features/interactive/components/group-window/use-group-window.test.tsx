import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GroupWindow, GroupWindowGroup } from '../../types'
import { useGroupWindow, type UseGroupWindowOptions } from './use-group-window'

const api = vi.hoisted(() => ({
  getGroupWindow: vi.fn(), sendGroupMessage: vi.fn(), createGroup: vi.fn(), changeGroupMembers: vi.fn(),
}))
vi.mock('../../api', () => api)

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail })
  return { promise, resolve, reject }
}

const initial: UseGroupWindowOptions = { storyId: 'story-a', branchId: 'main', groupId: 'group-a', active: true }
function windowFor(storyId: string, branchId: string, groupId: string): GroupWindow {
  return {
    story_id: storyId, branch_id: branchId,
    scene: { title: '', location: '', world_time: '', present: [] },
    groups: [{ id: groupId, name: groupId, type: 'self', channel: 'scene', contact: '', members: [], reachable_members: 0, version: 1 }],
    cast: [], exchanges: [], matters: [], has_more: false,
  }
}

describe('group window request ownership', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    api.getGroupWindow.mockImplementation((storyId, options) => Promise.resolve(windowFor(storyId, options.branchId, options.groupId)))
  })

  for (const changed of [{ groupId: 'group-b' }, { branchId: 'fork' }, { storyId: 'story-b' }]) {
    it(`does not reread or report a sent round after changing ${Object.keys(changed)[0]}`, async () => {
      const pending = deferred<{ silenced: string[] }>()
      api.sendGroupMessage.mockReturnValue(pending.promise)
      const hook = renderHook(props => useGroupWindow(props), { initialProps: initial })
      await waitFor(() => expect(hook.result.current.data?.groups[0].id).toBe('group-a'))
      let sent!: Promise<boolean>
      act(() => { sent = hook.result.current.send('Old group draft') })
      const next = { ...initial, ...changed }
      hook.rerender(next)
      await waitFor(() => expect(hook.result.current.data).toMatchObject(windowFor(next.storyId, next.branchId, next.groupId)))
      const reads = api.getGroupWindow.mock.calls.length
      await act(async () => { pending.resolve({ silenced: ['old-member'] }); expect(await sent).toBe(false) })
      expect(api.getGroupWindow).toHaveBeenCalledTimes(reads)
      expect(hook.result.current.data).toMatchObject(windowFor(next.storyId, next.branchId, next.groupId))
      expect(hook.result.current.silenced).toEqual([])
      expect(hook.result.current.sending).toBe(false)
    })
  }

  it('resets busy on retarget, without a late failure clearing the new send', async () => {
    const old = deferred<unknown>(), next = deferred<unknown>()
    api.sendGroupMessage.mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise)
    const hook = renderHook(props => useGroupWindow(props), { initialProps: initial })
    await waitFor(() => expect(hook.result.current.data).not.toBeNull())
    let first!: Promise<boolean>, second!: Promise<boolean>
    act(() => { first = hook.result.current.send('Old draft') })
    hook.rerender({ ...initial, groupId: 'group-b' })
    await waitFor(() => expect(hook.result.current.sending).toBe(false))
    act(() => { second = hook.result.current.send('New draft') })
    await act(async () => { old.reject(new Error('Old group failed')); await first })
    expect(hook.result.current.sending).toBe(true)
    expect(hook.result.current.sendError).toBeNull()
    await act(async () => { next.resolve({ silenced: [] }); expect(await second).toBe(true) })
    expect(hook.result.current.sending).toBe(false)
  })

  it('ignores an old creation result and membership result after leaving their scope', async () => {
    const creation = deferred<GroupWindowGroup>(), members = deferred<unknown>()
    api.createGroup.mockReturnValue(creation.promise)
    api.changeGroupMembers.mockReturnValue(members.promise)
    const hook = renderHook(props => useGroupWindow(props), { initialProps: initial })
    let created!: ReturnType<typeof hook.result.current.create>, changed!: Promise<boolean>
    act(() => { created = hook.result.current.create('Old room', ['person']); changed = hook.result.current.removeMember('person') })
    hook.rerender({ ...initial, branchId: 'fork' })
    await waitFor(() => expect(hook.result.current.data?.branch_id).toBe('fork'))
    const reads = api.getGroupWindow.mock.calls.length
    await act(async () => { creation.resolve({ id: 'old-room' } as GroupWindowGroup); members.resolve({}); expect(await created).toBeNull(); expect(await changed).toBe(false) })
    expect(api.getGroupWindow).toHaveBeenCalledTimes(reads)
    expect(hook.result.current).toMatchObject({ creating: false, changingMembers: false, createError: null, memberError: null })
  })

  it('invalidates pending reads on close and starts a fresh read on reopen', async () => {
    const old = deferred<GroupWindow>()
    api.getGroupWindow.mockReturnValueOnce(old.promise)
    const hook = renderHook(props => useGroupWindow(props), { initialProps: initial })
    hook.rerender({ ...initial, active: false })
    await act(async () => { old.resolve(windowFor('story-a', 'main', 'old-response')) })
    expect(hook.result.current.data).toBeNull()
    expect(hook.result.current.loading).toBe(false)
    hook.rerender(initial)
    await waitFor(() => expect(hook.result.current.data?.groups[0].id).toBe('group-a'))
  })
})
