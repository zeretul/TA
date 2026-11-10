import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { LayeredSettings } from './types'
import { useStartupUpdateCheck } from './use-startup-update-check'
import { checkForUpdate, fetchSettings, fetchUpdateStatus } from './api'
import { toast } from '@/lib/toast'

vi.mock('./api', () => ({
  checkForUpdate: vi.fn(), fetchSettings: vi.fn(), fetchUpdateStatus: vi.fn(), GLOBAL_SETTINGS_TARGET: { kind: 'global' },
}))
vi.mock('./query', () => ({ subscribeSettingsTarget: vi.fn(() => () => undefined) }))
vi.mock('@/lib/toast', () => ({ toast: vi.fn() }))

let now = Date.UTC(2050, 0, 1)
const settings = (enabled: boolean) => ({ effective: { update_check_enabled: enabled } }) as LayeredSettings
async function settle() {
  await act(async () => { await Promise.resolve() })
  await act(async () => { await vi.runAllTimersAsync() })
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
  now += 9 * 60 * 60 * 1000
  vi.setSystemTime(now)
  localStorage.clear()
  vi.mocked(fetchSettings).mockResolvedValue(settings(true))
  vi.mocked(fetchUpdateStatus).mockResolvedValue({ current_version: '0.6.6', channel_configured: true })
  vi.mocked(checkForUpdate).mockResolvedValue({ current_version: '0.6.6', latest_version: '0.6.7', channel_configured: true, update_available: true, can_install: true, platform: 'windows-x64', release_url: '', published_at: '' })
})

describe('startup update notifications', () => {
  it('does not query update status or releases when disabled', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settings(false))
    renderHook(() => useStartupUpdateCheck())
    await settle()
    expect(fetchUpdateStatus).not.toHaveBeenCalled()
    expect(checkForUpdate).not.toHaveBeenCalled()
  })

  it('does not contact a release source when the channel is unconfigured', async () => {
    vi.mocked(fetchUpdateStatus).mockResolvedValue({ channel_configured: false })
    renderHook(() => useStartupUpdateCheck())
    await settle()
    expect(checkForUpdate).not.toHaveBeenCalled()
    expect(toast).not.toHaveBeenCalled()
  })

  it('notifies once and suppresses another startup within the cache interval', async () => {
    const first = renderHook(() => useStartupUpdateCheck())
    await settle()
    expect(checkForUpdate).toHaveBeenCalledOnce()
    expect(toast).toHaveBeenCalledOnce()
    first.unmount()
    renderHook(() => useStartupUpdateCheck())
    await settle()
    expect(checkForUpdate).toHaveBeenCalledOnce()
  })

  it('keeps a failed automatic check quiet and throttles retries', async () => {
    vi.mocked(checkForUpdate).mockRejectedValue(new Error('offline'))
    const first = renderHook(() => useStartupUpdateCheck())
    await settle()
    expect(toast).not.toHaveBeenCalled()
    first.unmount()
    renderHook(() => useStartupUpdateCheck())
    await settle()
    expect(checkForUpdate).toHaveBeenCalledOnce()
  })
})
