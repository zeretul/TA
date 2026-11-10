import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { customAgentsForRuntime } from '@/features/agents/CustomAgentSelect'
import { getImagePresets } from '@/features/interactive/api'
import type { ImagePreset } from '@/features/interactive/types'
import { DEFAULT_IMAGE_PRESET_ID } from '@/features/interactive/image-settings'
import { DEFAULT_MODEL_PROFILE_ID, modelProfileID, modelProfileLabel, modelProfilesWithDefault } from './model-profiles'
import type { Settings } from './types'

const BUILTIN_IMAGE_AGENT_VALUE = '__builtin_image_agent__'

interface ImageGenerationPreferencesProps {
  value: Settings
  effective: Settings
  onChange: (patch: Partial<Settings>) => void
}

/**
 * Who generates images and which model answers the image Agent.
 *
 * These choices used to sit in the chat composer's image submenu, where they
 * looked like a per-message option. They are connection-level preferences, so
 * they belong beside the image model profiles they point at.
 */
export function ImageGenerationPreferences({ value, effective, onChange }: ImageGenerationPreferencesProps) {
  const { t } = useTranslation()
  const [presets, setPresets] = useState<ImagePreset[]>([])

  useEffect(() => {
    let cancelled = false
    getImagePresets()
      .then((next) => { if (!cancelled) setPresets(next.filter((preset) => !preset.invalid)) })
      .catch((error) => {
        console.error('[settings] Failed to load image presets', error)
      })
    return () => { cancelled = true }
  }, [])

  const modelOptions = useMemo(() => modelProfilesWithDefault(effective)
    .map((profile) => ({ id: modelProfileID(profile), label: modelProfileLabel(profile) || modelProfileID(profile) }))
    .filter((option) => Boolean(option.id)), [effective])

  const agentOptions = useMemo(() => [
    { id: BUILTIN_IMAGE_AGENT_VALUE, label: t('agents.image.title') },
    ...customAgentsForRuntime(effective.custom_agents, 'image').map((agent) => ({
      id: agent.id || '',
      label: agent.name || agent.id || '',
    })),
  ].filter((option) => Boolean(option.id)), [effective.custom_agents, t])

  const presetOptions = useMemo(
    () => presets.map((preset) => ({ id: preset.id, label: preset.name || preset.id })),
    [presets],
  )

  const selectedModel = value.agent_models?.image?.profile_id
    || effective.agent_models?.image?.profile_id
    || effective.agent_models?.default?.profile_id
    || DEFAULT_MODEL_PROFILE_ID
  const selectedAgent = value.default_image_agent_id || BUILTIN_IMAGE_AGENT_VALUE
  const selectedPreset = value.ide_image_preset_id || DEFAULT_IMAGE_PRESET_ID

  return (
    <>
      <Row label={t('chat.imageAgentModels.language')}>
        <Select
          value={selectedModel}
          onValueChange={(profileID) => onChange({
            agent_models: { ...value.agent_models, image: { ...value.agent_models?.image, profile_id: profileID } },
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
      <Row label={t('agents.custom.imageDefault')}>
        <Select
          value={selectedAgent}
          onValueChange={(agentID) => onChange({
            default_image_agent_id: agentID === BUILTIN_IMAGE_AGENT_VALUE ? '' : agentID,
          })}
        >
          <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
          <SelectContent className="nova-panel border text-[var(--nova-text)]">
            <SelectGroup>
              {agentOptions.map((option) => <SelectItem key={option.id} value={option.id}>{option.label}</SelectItem>)}
            </SelectGroup>
          </SelectContent>
        </Select>
      </Row>
      {presetOptions.length > 0 ? (
        <Row label={t('chat.imagePreset')}>
          <Select value={selectedPreset} onValueChange={(presetID) => onChange({ ide_image_preset_id: presetID })}>
            <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectGroup>
                {presetOptions.map((option) => <SelectItem key={option.id} value={option.id}>{option.label}</SelectItem>)}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Row>
      ) : null}
    </>
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
