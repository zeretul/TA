export const ONBOARDING_OPEN_EVENT = 'nova:onboarding-open'
export const SETTINGS_SECTION_EVENT = 'nova:settings-open-section'

export interface SettingsSectionRequest {
  section?: string
}

// Retain one navigation intent while the settings route is lazily mounting.
let pendingSettingsSection: string | undefined
export function requestSettingsSection(section: string) {
  pendingSettingsSection = section
  window.dispatchEvent(new CustomEvent(SETTINGS_SECTION_EVENT, { detail: { section } }))
}
export function takeSettingsSection(): string | undefined {
  const section = pendingSettingsSection
  pendingSettingsSection = undefined
  return section
}
