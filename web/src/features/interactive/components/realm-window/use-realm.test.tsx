import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import type { RealmState } from '../../realm-types'
import { useRealm } from './use-realm'
const api = vi.hoisted(() => ({ getRealm: vi.fn() }))
vi.mock('../../api', () => api)
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(yes => { resolve = yes }); return { promise, resolve } }
const state: RealmState = { story_id: 'story-a', branch_id: 'main', instances: [], available: [] }
beforeEach(() => vi.resetAllMocks())
it('clears old draftable choices immediately on branch change and ignores a read after close', async () => {
  api.getRealm.mockResolvedValueOnce(state)
  const hook = renderHook(props => useRealm(props), { initialProps: { storyId: 'story-a', branchId: 'main', active: true } })
  await waitFor(() => expect(hook.result.current.data).toEqual(state))
  const next = deferred<RealmState>(); api.getRealm.mockReturnValueOnce(next.promise)
  hook.rerender({ storyId: 'story-a', branchId: 'fork', active: true })
  expect(hook.result.current.data).toBeNull()
  hook.rerender({ storyId: 'story-a', branchId: 'fork', active: false })
  await act(async () => next.resolve({ ...state, branch_id: 'fork' }))
  expect(hook.result.current).toMatchObject({ data: null, loading: false })
})

it('refetches server-localized resource names on a language change and rejects the older read', async () => {
  const { setConfiguredLocale } = await import('@/i18n')
  const old = deferred<RealmState>()
  api.getRealm.mockReturnValueOnce(old.promise).mockResolvedValueOnce({ ...state, location: 'English projection' })
  const hook = renderHook(() => useRealm({ storyId: 'story-a', branchId: 'main', active: true }))
  await act(async () => { setConfiguredLocale('en-US') })
  await waitFor(() => expect(hook.result.current.data?.location).toBe('English projection'))
  await act(async () => old.resolve({ ...state, location: '旧中文响应' }))
  expect(hook.result.current.data?.location).toBe('English projection')
  expect(api.getRealm).toHaveBeenCalledTimes(2)
  hook.unmount()
  setConfiguredLocale('zh-CN')
})
