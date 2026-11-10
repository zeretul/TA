import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { fetchSettings } from '@/features/settings/api'
import { buildModelProfileOptions } from '@/features/settings/model-profile-options'
import type { LayeredSettings } from '@/features/settings/types'
import type { InteractiveStoryUpdateInput, StorySummary } from '../../types'
import { TuningRow, TuningSelect, type TuningSelectOption } from '../director-console/StoryTuningControls'

interface IntimacyModelSettingProps {
  story: StorySummary
  onUpdate?: (input: InteractiveStoryUpdateInput) => void | Promise<void>
}

/**
 * Radix refuses a select item whose value is the empty string, so "no model
 * chosen" needs a value of its own in the menu. It is mapped back to an empty
 * profile id on save, which is what the activity reads as unconfigured.
 */
const NO_PROFILE = '__none__'

/**
 * The model the intimacy activity speaks through.
 *
 * It is a separate binding on purpose: §4.1 forbids it falling back to the main
 * story's model, so this row exists rather than the activity borrowing the
 * narrative route. T27 keeps runtime choices out of game windows, so the choice
 * lives here, in the same narrative section as the story's own model, and the
 * window only reports what was chosen.
 *
 * The empty value is a real choice — "no model chosen yet" — and is saved as an
 * empty profile id, which is what makes the activity refuse to start instead of
 * quietly borrowing a route.
 */
export function IntimacyModelSetting({ story, onUpdate }: IntimacyModelSettingProps) {
  const { t } = useTranslation()
  const [catalog, setCatalog] = useState<LayeredSettings | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let cancelled = false
    void fetchSettings()
      .then((settings) => { if (!cancelled) setCatalog(settings) })
      .catch((loadError) => console.error('[game-settings] Failed to load model profiles', loadError))
    return () => { cancelled = true }
  }, [])

  const settings = story.intimacy_settings || {}
  const profileId = settings.profile_id || ''
  const thinkingLevel = settings.thinking_level || 'off'

  const profileOptions = useMemo<TuningSelectOption[]>(() => [
    { id: NO_PROFILE, label: t('intimacyWindow.model.none') },
    ...buildModelProfileOptions(catalog, t).map((option) => ({ id: option.id, label: option.label })),
  ], [catalog, t])

  const thinkingOptions = useMemo<TuningSelectOption[]>(
    () => ['off', 'low', 'medium', 'high'].map((level) => ({ id: level, label: t(`intimacyWindow.thinking.${level}`) })),
    [t],
  )

  const save = (next: { profile_id: string; thinking_level: string }) => {
    if (!onUpdate) return
    setBusy(true)
    void Promise.resolve(onUpdate({ intimacy_settings: next })).finally(() => setBusy(false))
  }

  return (
    <TuningRow
      title={t('gameSettings.intimacyModel.title')}
      description={t('gameSettings.intimacyModel.hint')}
      source={profileId ? 'story' : undefined}
      busy={busy}
      disabled={!onUpdate}
    >
      <TuningSelect
        value={profileId || NO_PROFILE}
        options={profileOptions}
        label={t('gameSettings.intimacyModel.profile')}
        disabled={!onUpdate || busy}
        onChange={(value) => save({ profile_id: value === NO_PROFILE ? '' : value, thinking_level: thinkingLevel })}
      />
      <TuningSelect
        value={thinkingLevel}
        options={thinkingOptions}
        label={t('gameSettings.intimacyModel.thinking')}
        disabled={!onUpdate || busy || !profileId}
        onChange={(value) => save({ profile_id: profileId, thinking_level: value })}
      />
    </TuningRow>
  )
}
