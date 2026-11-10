import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DEFAULT_MODEL_PROFILE_ID, modelProfileID, modelProfileLabel, modelProfilesWithDefault } from './model-profiles'
import { THINKING_LEVELS, normalizeThinkingLevel } from './thinking-levels'
import type { Settings } from './types'

interface CharacterChatPreferencesProps {
  value: Settings
  effective: Settings
  onChange: (patch: Partial<Settings>) => void
}

/**
 * Which model answers a private conversation and a small group's round, and how
 * much it reasons first.
 *
 * §T24 keeps these in the system settings' model-purpose area rather than in a
 * popup: a window reply is the game's other model call, so it belongs beside the
 * other purposes and is saved the same way. The purpose names an existing model
 * profile instead of copying one, so editing that profile changes it too.
 */
export function CharacterChatPreferences({ value, effective, onChange }: CharacterChatPreferencesProps) {
  const { t } = useTranslation()

  const modelOptions = useMemo(() => modelProfilesWithDefault(effective)
    .map((profile) => ({ id: modelProfileID(profile), label: modelProfileLabel(profile) || modelProfileID(profile) }))
    .filter((option) => Boolean(option.id)), [effective])

  const selectedModel = value.agent_models?.character_chat?.profile_id
    || effective.agent_models?.character_chat?.profile_id
    || effective.agent_models?.default?.profile_id
    || DEFAULT_MODEL_PROFILE_ID
  const selectedThinking = normalizeThinkingLevel(value.agent_models?.character_chat?.thinking_level)
    ?? normalizeThinkingLevel(effective.agent_models?.character_chat?.thinking_level)
    ?? 'off'

  return (
    <div className="nova-settings-row rounded-md px-2 py-1.5">
      <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('settings.characterChat.title')}</div>
      <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('settings.characterChat.help')}
      </div>
      <div className="flex flex-col gap-2">
        <Row label={t('settings.characterChat.model')}>
          <Select
            value={selectedModel}
            onValueChange={(profileID) => onChange({
              agent_models: { ...value.agent_models, character_chat: { ...value.agent_models?.character_chat, profile_id: profileID } },
            })}
          >
            <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectGroup>
                {modelOptions.map((option) => <SelectItem key={option.id} value={option.id}>{option.label}</SelectItem>)}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Row>
        <Row label={t('settings.characterChat.thinkingLevel')}>
          <Select
            value={selectedThinking}
            onValueChange={(level) => onChange({
              agent_models: { ...value.agent_models, character_chat: { ...value.agent_models?.character_chat, thinking_level: level } },
            })}
          >
            <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectGroup>
                {THINKING_LEVELS.map((level) => (
                  <SelectItem key={level} value={level}>{t(`chat.modelProfile.thinking.${level}`, { defaultValue: level })}</SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Row>
      </div>
    </div>
  )
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <SettingsFieldRow
      title={label}
      className="nova-settings-row rounded-md border-0 bg-transparent px-2 py-1.5"
      contentClassName="sm:w-44 sm:flex-none"
      controlClassName="flex-1"
    >
      {children}
    </SettingsFieldRow>
  )
}
