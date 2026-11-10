import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { useLoreOverview } from './use-lore-overview'
const api = vi.hoisted(() => ({ getLoreOverview: vi.fn() }))
vi.mock('../../api', () => api)
const identity = { story_id: 's', branch_id: 'main' }
const response = () => ({ snapshot: identity, characters: identity, journey: identity, quests: identity, events: identity })
beforeEach(() => api.getLoreOverview.mockReset().mockResolvedValue(response()))
it('reads one atomic response including its snapshot even without withSnapshot', async () => {
  const { result } = renderHook(() => useLoreOverview({ storyId: 's', branchId: 'main', active: true }))
  await waitFor(() => expect(result.current.data).toEqual(response()))
  expect(api.getLoreOverview).toHaveBeenCalledTimes(1)
  expect(api.getLoreOverview).toHaveBeenCalledWith('s', { branchId: 'main' })
})
it('rejects mismatched or incomplete aggregate responses', async () => {
  api.getLoreOverview.mockResolvedValue({ ...response(), quests: { ...identity, branch_id: 'other' } })
  const { result } = renderHook(() => useLoreOverview({ storyId: 's', branchId: 'main', active: true }))
  await waitFor(() => expect(result.current.error).toBe('loreOverview.changedDuringRead'))
  expect(result.current.data).toBeNull()
  api.getLoreOverview.mockResolvedValue({ ...response(), snapshot: null })
  await act(() => result.current.reload())
  expect(result.current.data).toBeNull()
})
it('clears older data while a newer known revision is pending and drops an older reply', async () => {
  const { result, rerender } = renderHook(({ revision }) => useLoreOverview({ storyId: 's', branchId: 'main', active: true, revision }), { initialProps: { revision: '1' } })
  await waitFor(() => expect(result.current.data).toEqual(response()))
  let resolveOld!: (value: unknown) => void
  api.getLoreOverview.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
  rerender({ revision: '2' })
  await waitFor(() => expect(result.current.loading).toBe(true))
  expect(result.current.data).toBeNull()
  rerender({ revision: '3' })
  await waitFor(() => expect(result.current.data).toEqual(response()))
  await act(async () => resolveOld({ ...response(), characters: { ...identity, stale: true } }))
  expect(result.current.data).toEqual(response())
})
it.each(['resolve', 'reject'] as const)('drops a late %s after closing the overview', async (outcome) => {
  let resolve!: (value: unknown) => void
  let reject!: (error: Error) => void
  api.getLoreOverview.mockImplementationOnce(() => new Promise((yes, no) => { resolve = yes; reject = no }))
  const { result, rerender } = renderHook(({ active }) => useLoreOverview({ storyId: 's', branchId: 'main', active }), { initialProps: { active: true } })
  await waitFor(() => expect(result.current.loading).toBe(true))
  rerender({ active: false })
  await act(async () => outcome === 'resolve' ? resolve(response()) : reject(new Error('late failure')))
  expect(result.current.data).toBeNull()
  expect(result.current.error).toBeNull()
  expect(result.current.loading).toBe(false)
})
it.each(['resolve', 'reject'] as const)('retires an unmounted request before its late %s', async (outcome) => {
  let resolve!: (value: unknown) => void
  let reject!: (error: Error) => void
  api.getLoreOverview.mockImplementationOnce(() => new Promise((yes, no) => { resolve = yes; reject = no }))
  const { result, unmount } = renderHook(() => useLoreOverview({ storyId: 's', branchId: 'main', active: true }))
  await waitFor(() => expect(result.current.loading).toBe(true))
  const lastRender = result.current
  unmount()
  await act(async () => outcome === 'resolve' ? resolve(response()) : reject(new Error('late failure')))
  expect(result.current).toBe(lastRender)
  expect(api.getLoreOverview).toHaveBeenCalledTimes(1)
})
it('drops an older story response after a new story has loaded', async () => {
  let resolve!: (value: unknown) => void
  api.getLoreOverview.mockImplementationOnce(() => new Promise(yes => { resolve = yes }))
  const { result, rerender } = renderHook(({ storyId }) => useLoreOverview({ storyId, branchId: 'main', active: true }), { initialProps: { storyId: 'old' } })
  rerender({ storyId: 's' })
  await waitFor(() => expect(result.current.data).toEqual(response()))
  await act(async () => resolve(Object.fromEntries(Object.entries(response()).map(([key, value]) => [key, { ...value, story_id: 'old' }]))))
  expect(result.current.data).toEqual(response())
})
