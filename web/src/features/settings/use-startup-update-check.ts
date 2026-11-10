import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { APP_VERSION } from '@/app-version'
import { toast } from '@/lib/toast'
import { checkForUpdate, fetchSettings, fetchUpdateStatus, GLOBAL_SETTINGS_TARGET } from './api'
import { subscribeSettingsTarget } from './query'

const CHECK_INTERVAL_MS = 8 * 60 * 60 * 1000
const CACHE_KEY = 'ta.updates.lastCheck'
let lastAttempt = 0

// This is a notification-only startup check. No download, apply or polling is
// started here. A failed check must never interrupt the game.
export function useStartupUpdateCheck() {
  const { t } = useTranslation()
  const [enabled, setEnabled] = useState<boolean | null>(null)
  useEffect(() => {
    let mounted = true
    const accept = (data: Awaited<ReturnType<typeof fetchSettings>>) => {
      if (mounted) setEnabled(data.effective.update_check_enabled ?? true)
    }
    void fetchSettings().then(accept).catch(() => undefined)
    const unsubscribe = subscribeSettingsTarget(GLOBAL_SETTINGS_TARGET, accept)
    return () => { mounted = false; unsubscribe() }
  }, [])
  useEffect(() => {
    if (!enabled) return
    const now = Date.now()
    let previous = lastAttempt
    try {
      const cached = JSON.parse(localStorage.getItem(CACHE_KEY) || 'null')
      if (cached?.version === APP_VERSION && typeof cached.at === 'number') previous = Math.max(previous, cached.at)
    } catch { /* Storage can be unavailable; retain the in-memory throttle. */ }
    if (previous > 0 && now >= previous && now - previous < CHECK_INTERVAL_MS) return
    const controller = new AbortController()
    // Defer until after the effect is committed so StrictMode's probe cannot
    // reserve the cache and cancel the only real attempt.
    const timer = window.setTimeout(() => {
      lastAttempt = now
      try { localStorage.setItem(CACHE_KEY, JSON.stringify({ version: APP_VERSION, at: now })) } catch { /* optional cache */ }
      void fetchUpdateStatus(controller.signal).then(status => {
        if (!status.channel_configured || controller.signal.aborted) return null
        return checkForUpdate(controller.signal)
      }).then(result => {
        if (!result?.update_available || controller.signal.aborted) return
        toast(t('settings.updates.startupAvailable', { version: result.latest_version }))
      }).catch(() => undefined)
    }, 0)
    return () => { window.clearTimeout(timer); controller.abort() }
  }, [enabled, t])
}
