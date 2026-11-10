import type { LayeredSettings, Settings } from '@/features/settings/types'
import { modelEndpointsWithDefault, modelProfilesWithDefault } from '@/features/settings/model-profiles'

export function hasUsableLanguageModel(settings: Settings | undefined): boolean {
  const endpoints = new Map(modelEndpointsWithDefault(settings).map((endpoint) => [endpoint.id?.trim(), endpoint]))
  return modelProfilesWithDefault(settings).some((profile) => {
    const endpoint = endpoints.get(profile.endpoint_id?.trim())
    return Boolean(endpoint?.api_key?.trim()) && Boolean(profile.model?.trim())
  })
}

/**
 * Whether a story could be written on this machine.
 *
 * The backend answers this through the same provider resolution a real call
 * runs, so it counts environment overrides and provider presets that supply
 * their own base URL, and it does not demand an API key a local provider does
 * not need. Reading the persisted layers alone gets those cases wrong, so the
 * backend's answer wins when it is present and the inference is only the
 * fallback for an older server that does not send it.
 */
export function isStoryModelReady(settings: LayeredSettings | undefined): boolean {
  const answered = settings?.runtime?.story_model_ready
  if (typeof answered === 'boolean') return answered
  return hasUsableLanguageModel(settings?.effective)
}
