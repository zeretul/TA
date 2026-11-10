import { useEffect, useState } from 'react'
import { fetchSettings } from '@/features/settings/api'
import { isStoryModelReady } from '@/features/onboarding/model-status'

/**
 * Whether this machine has a language model a story could actually be written
 * with. Undefined while the first answer is still on its way, so a panel can
 * tell "not ready yet" from "nothing configured" and avoid flashing a warning
 * at every startup.
 *
 * Any surface that is about to spend a model call may ask this first: starting
 * a story, and drafting a world.
 */
export function useStoryModelReadiness(): boolean | undefined {
  const [ready, setReady] = useState<boolean | undefined>(undefined)
  useEffect(() => {
    let cancelled = false
    fetchSettings()
      .then((settings) => {
        if (!cancelled) setReady(isStoryModelReady(settings))
      })
      .catch((error) => {
        console.warn('[story-setup] could not read the model settings', error)
        if (!cancelled) setReady(undefined)
      })
    return () => { cancelled = true }
  }, [])
  return ready
}
