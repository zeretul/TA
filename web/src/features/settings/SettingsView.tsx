import { cloneElement, isValidElement, useEffect, useId, useRef, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import { ChevronDown, ChevronUp } from 'lucide-react'
import { useReducedMotionConfig } from 'motion/react'
import type { AnimationPlaybackControls } from 'motion/react'
import { useTranslation } from 'react-i18next'
import type { ImageAPIEndpointSettings, ImageAPIProfileSettings, ModelEndpointSettings, ModelProfileSettings, Settings } from './types'
import { GLOBAL_SETTINGS_TARGET } from './api'
import { useLayeredSettingsDraft } from './use-layered-settings-draft'
import { FontPicker } from './FontPicker'
import { LoadingState } from '@/components/common/LoadingState'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { SettingsPageFrame } from './SettingsPageFrame'
import { SectionedNavigation } from '@/components/navigation/sectioned-navigation'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { LOCALE_OPTIONS } from '@/i18n'
import {
  DEFAULT_MODEL_PROFILE_ID,
  modelEndpointID,
  modelEndpointsWithDefault,
  modelProfileID,
  modelProfilesWithDefault,
} from './model-profiles'
import { ModelProfilesEditor } from './ModelProfilesEditor'
import { CharacterChatPreferences } from './CharacterChatPreferences'
import { DEFAULT_IMAGE_API_PROFILE_ID, imageAPIEndpointID, imageAPIEndpointsWithDefault, imageAPIProfileID, imageAPIProfilesWithDefault } from './image-profiles'
import { ImageAPIProfilesEditor } from './ImageAPIProfilesEditor'
import { ImageGenerationPreferences } from './ImageGenerationPreferences'
import { SETTINGS_SECTION_EVENT, takeSettingsSection, type SettingsSectionRequest } from '@/features/onboarding/events'
import { scrollSettingsSectionIntoView } from './settings-section-scroll'
import { applyReadingTypographySettings, applyUIFontSize } from './font-variables'
import {
  DEFAULT_READING_FONT_SIZE,
  DEFAULT_UI_FONT_SIZE,
  READING_FONT_SIZE_STEPS,
  UI_FONT_SIZE_STEPS,
} from './font-size-steps'
import { TextSizeControl } from './TextSizeControl'
import { LANAccessSettings } from './LANAccessSettings'
import { EMPTY_SPEECH_SETTINGS, SpeechSettingsEditor } from './SpeechSettingsEditor'
import { AboutUpdates } from './AboutUpdates'

type SettingsSectionId = 'model' | 'speech' | 'image' | 'paths' | 'access' | 'appearance' | 'updates'

const SETTINGS_SECTION_IDS: SettingsSectionId[] = ['appearance', 'model', 'speech', 'image', 'paths', 'access', 'updates']

type SettingsSection = {
  id: SettingsSectionId
  group: string
  title: string
  children: ReactNode
}

const FIELD_INHERIT_VALUE = '__inherit__'

export function SettingsView({ visible = true }: { visible?: boolean }) {
  const { t } = useTranslation()
  const reducedMotion = useReducedMotionConfig()
  const { layered, draft, setDraft, error, autosaveStatus, autosaveError, saveNow } = useLayeredSettingsDraft({
    target: GLOBAL_SETTINGS_TARGET,
    layer: 'user',
    sourcePrefix: 'settings-view',
  })
  const [activeSection, setActiveSection] = useState<SettingsSectionId>('appearance')
  const [expandedSections, setExpandedSections] = useState<Record<SettingsSectionId, boolean>>({
    appearance: true,
    model: true,
    speech: false,
    image: false,
    paths: false,
    access: false,
    updates: false,
  })
  const contentRef = useRef<HTMLDivElement | null>(null)
  const sectionRefs = useRef<Partial<Record<SettingsSectionId, HTMLElement | null>>>({})
  const sectionScrollFrameRef = useRef<number | null>(null)
  const sectionScrollAnimationRef = useRef<AnimationPlaybackControls | null>(null)
  const sectionScrollTargetRef = useRef<SettingsSectionId | null>(null)

  const effective = layered?.effective ?? {}
  const inherited = layered?.inherited?.user ?? {}

  const setField = <K extends keyof Settings>(k: K, v: Settings[K]) =>
    setDraft((d) => ({ ...d, [k]: v }))

  const setModelProfiles = (profiles: ModelProfileSettings[]) => {
    setDraft((d) => ({
      ...d,
      openai_api_key: '',
      openai_base_url: '',
      openai_model: '',
      openai_context_window_tokens: null,
      model_profiles: profiles,
    }))
  }

  const setModelEndpoints = (endpoints: ModelEndpointSettings[]) => {
    setDraft((d) => ({
      ...d,
      openai_api_key: '',
      openai_base_url: '',
      model_endpoints: endpoints,
    }))
  }

  const setDefaultModelProfile = (profileID: string) => {
    setDraft((d) => ({
      ...d,
      agent_models: {
        ...d.agent_models,
        default: { ...d.agent_models?.default, profile_id: profileID },
      },
    }))
  }

  const setImageAPIProfiles = (profiles: ImageAPIProfileSettings[]) => {
    setDraft((d) => ({
      ...d,
      image_api_profiles: profiles,
    }))
  }

  const setImageAPIEndpoints = (endpoints: ImageAPIEndpointSettings[]) => {
    setDraft((d) => ({
      ...d,
      image_api_endpoints: endpoints,
    }))
  }


  const sections: SettingsSection[] = [
    {
      id: 'appearance',
      group: t('settings.group.common'),
      title: t('settings.section.appearance'),
      children: (
        <>
          <LanguageSelect label={t('settings.appearance.language')} value={draft.language}
                          inherited={inherited.language}
                          onChange={(v) => setField('language', v)} />
          <ThemeSelect label={t('settings.appearance.theme')} value={draft.theme}
                       inherited={inherited.theme}
                       onChange={(v) => setField('theme', v)} />
          <MotionIntensitySelect label={t('settings.appearance.motionIntensity')} value={draft.motion_intensity}
                                 inherited={inherited.motion_intensity}
                                 onChange={(v) => setField('motion_intensity', v)} />
          <ValueRow label={t('settings.appearance.clipsEnabled')}>
            <div className="flex min-w-0 flex-col gap-1">
              <Switch
                checked={draft.clips_enabled ?? effective.clips_enabled ?? true}
                onCheckedChange={(value) => setField('clips_enabled', value)}
                aria-label={t('settings.appearance.clipsEnabled')}
              />
              <p className="text-xs text-muted-foreground">{t('settings.appearance.clipsEnabledDescription')}</p>
            </div>
          </ValueRow>
          <FieldRow label={t('settings.appearance.uiFont')}>
            <FontPicker value={draft.ui_font_family}
                        inherited={inherited.ui_font_family}
                        allowInherit
                        onValueChange={(v) => setField('ui_font_family', v)} />
          </FieldRow>
          <TextSizeRow
            label={t('settings.appearance.uiFontSize')}
            description={t('settings.appearance.uiFontSizeDescription')}
          >
            <TextSizeControl
              value={draft.ui_font_size ?? effective.ui_font_size ?? DEFAULT_UI_FONT_SIZE}
              steps={UI_FONT_SIZE_STEPS}
              defaultValue={DEFAULT_UI_FONT_SIZE}
              ariaLabel={t('settings.appearance.uiFontSize')}
              disabled={!layered}
              onValueChange={(value) => {
                applyUIFontSize(value)
                setField('ui_font_size', value)
              }}
            />
          </TextSizeRow>
          <FieldRow label={t('settings.appearance.readingFont')}>
            <FontPicker value={draft.reading_font_family}
                        inherited={inherited.reading_font_family}
                        allowInherit
                        onValueChange={(v) => setField('reading_font_family', v)} />
          </FieldRow>
          <TextSizeRow
            label={t('settings.appearance.readingFontSize')}
            description={t('settings.appearance.readingFontSizeDescription')}
          >
            <TextSizeControl
              value={draft.reading_font_size ?? effective.reading_font_size ?? DEFAULT_READING_FONT_SIZE}
              steps={READING_FONT_SIZE_STEPS}
              defaultValue={DEFAULT_READING_FONT_SIZE}
              ariaLabel={t('settings.appearance.readingFontSize')}
              disabled={!layered}
              onValueChange={(value) => {
                applyReadingTypographySettings({
                  readingFont: draft.reading_font_family || effective.reading_font_family,
                  readingFontSize: value,
                })
                setField('reading_font_size', value)
              }}
            />
          </TextSizeRow>
        </>
      ),
    },
    {
      id: 'model',
      group: t('settings.group.common'),
      title: t('settings.section.model'),
      children: (
        <>
          <ModelProfilesEditor
            endpoints={modelEndpointsForEditor(draft, effective)}
            effectiveEndpoints={modelEndpointsWithDefault(effective)}
            profiles={modelProfilesForEditor(draft, effective)}
            effectiveProfiles={modelProfilesWithDefault(effective)}
            defaultProfileID={draft.agent_models?.default?.profile_id ?? ''}
            effectiveDefaultProfileID={inherited.agent_models?.default?.profile_id || DEFAULT_MODEL_PROFILE_ID}
            onDefaultProfileChange={setDefaultModelProfile}
            onEndpointsChange={setModelEndpoints}
            onProfilesChange={setModelProfiles}
          />
          <CharacterChatPreferences
            value={draft}
            effective={effective}
            onChange={(patch) => setDraft((current) => ({ ...current, ...patch }))}
          />
        </>
      ),
    },
    {
      id: 'speech',
      group: t('settings.group.common'),
      title: t('speech.title'),
      children: <SpeechSettingsEditor value={draft.speech ?? effective.speech ?? EMPTY_SPEECH_SETTINGS} onChange={value => setField('speech', value)} visible={visible} />,
    },
    {
      id: 'image',
      group: t('settings.group.common'),
      title: t('settings.section.imageApi'),
      children: (
        <>
          <ImageGenerationPreferences
            value={draft}
            effective={effective}
            onChange={(patch) => setDraft((current) => ({ ...current, ...patch }))}
          />
          <ImageAPIProfilesEditor
            endpoints={imageAPIEndpointsForEditor(draft, effective)}
            effectiveEndpoints={imageAPIEndpointsWithDefault(effective)}
            profiles={imageAPIProfilesForEditor(draft, effective)}
            effectiveProfiles={imageAPIProfilesWithDefault(effective)}
            defaultProfileID={draft.default_image_api_profile_id ?? ''}
            effectiveDefaultProfileID={inherited.default_image_api_profile_id || DEFAULT_IMAGE_API_PROFILE_ID}
            onDefaultProfileChange={(v) => setField('default_image_api_profile_id', v)}
            onEndpointsChange={setImageAPIEndpoints}
            onProfilesChange={setImageAPIProfiles}
          />
        </>
      ),
    },
    {
      id: 'paths',
      group: t('settings.group.common'),
      title: t('settings.section.paths'),
      children: (
        <>
          {/* The skill directory is an AI runtime resource location, not a
              player-facing choice, so it is shown rather than edited. The
              data-root mechanism itself is untouched: the resolver, the
              TA_DIR override and the layout all still work, and the value
              stays visible for advanced inspection. */}
          <ReadOnly label={t('settings.paths.skillsDir')} value={layered?.effective?.skills_dir || layered?.paths?.denova_dir || layered?.paths?.nova_dir} />
          <ReadOnly label={t('settings.paths.dataDir')} value={layered?.paths?.denova_dir || layered?.paths?.nova_dir} />
          <ReadOnly label={t('settings.paths.userConfig')} value={layered?.paths?.user_config} />
        </>
      ),
    },
    {
      id: 'access',
      group: t('settings.group.common'),
      title: t('settings.section.access'),
      children: (
        visible && <LANAccessSettings draft={draft} inherited={inherited}
          onChange={patch => setDraft(current => ({ ...current, ...patch }))} />
      ),
    },
    {
      id: 'updates',
      group: t('settings.group.common'),
      title: t('settings.updates.title'),
      children: <AboutUpdates
        autoCheck={draft.update_check_enabled ?? effective.update_check_enabled ?? true}
        onAutoCheckChange={value => setField('update_check_enabled', value)}
        visible={visible}
      />,
    },
  ]

  const cancelSectionScroll = useCallback(() => {
    sectionScrollTargetRef.current = null
    if (sectionScrollFrameRef.current !== null) {
      window.cancelAnimationFrame(sectionScrollFrameRef.current)
      sectionScrollFrameRef.current = null
    }
    sectionScrollAnimationRef.current?.stop()
    sectionScrollAnimationRef.current = null
  }, [])

  useEffect(() => cancelSectionScroll, [cancelSectionScroll])

  const syncActiveSection = useCallback(() => {
    if (sectionScrollTargetRef.current) return
    const current = settingsSectionAtScrollPosition(contentRef.current, sectionRefs.current)
    if (current) setActiveSection(current)
  }, [])

  useEffect(() => {
    if (!visible) {
      cancelSectionScroll()
      return
    }
    if (!layered) return
    const frame = window.requestAnimationFrame(syncActiveSection)
    return () => window.cancelAnimationFrame(frame)
  }, [cancelSectionScroll, layered, syncActiveSection, visible])

  const jumpToSection = useCallback((id: SettingsSectionId) => {
    cancelSectionScroll()
    setActiveSection(id)
    setExpandedSections((prev) => ({ ...prev, [id]: true }))
    sectionScrollTargetRef.current = id
    sectionScrollFrameRef.current = window.requestAnimationFrame(() => {
      sectionScrollFrameRef.current = null
      const container = contentRef.current
      const section = sectionRefs.current[id]
      if (!container || !section) {
        sectionScrollTargetRef.current = null
        return
      }
      sectionScrollAnimationRef.current = scrollSettingsSectionIntoView(container, section, {
        reducedMotion: Boolean(reducedMotion),
        onComplete: () => {
          sectionScrollAnimationRef.current = null
          if (sectionScrollTargetRef.current === id) sectionScrollTargetRef.current = null
        },
      })
    })
  }, [cancelSectionScroll, reducedMotion])

  const toggleSection = (id: SettingsSectionId) => {
    setExpandedSections((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  useEffect(() => {
    if (!visible || !layered) return
    const section = takeSettingsSection()
    if (isSettingsSectionId(section)) requestAnimationFrame(() => jumpToSection(section))
  }, [visible, layered, jumpToSection])

  useEffect(() => {
    const openSection = (event: Event) => {
      const detail = (event as CustomEvent<SettingsSectionRequest>).detail
      const section = detail?.section
      if (!isSettingsSectionId(section)) return
      if (!visible || !layered) return
      takeSettingsSection()
      requestAnimationFrame(() => {
        jumpToSection(section)
      })
    }
    window.addEventListener(SETTINGS_SECTION_EVENT, openSection)
    return () => window.removeEventListener(SETTINGS_SECTION_EVENT, openSection)
  }, [jumpToSection, visible, layered])

  const onContentScroll = () => {
    syncActiveSection()
  }

  const navGroups = sections.reduce<Array<{ group: SettingsSection['group']; items: SettingsSection[] }>>((groups, section) => {
    const last = groups[groups.length - 1]
    if (last?.group === section.group) {
      last.items.push(section)
    } else {
      groups.push({ group: section.group, items: [section] })
    }
    return groups
  }, [])
  const navPanel = (
    <SectionedNavigation
      groups={navGroups.map((group) => ({
        id: group.group,
        title: group.group,
        items: group.items.map((section) => ({ id: section.id, title: section.title })),
      }))}
      activeId={activeSection}
      onSelect={jumpToSection}
    />
  )

  return (
    <SettingsPageFrame
      visible={visible}
      title={t('settings.systemTitle')}
      className="nova-settings-view"
      error={error}
      errorTitle={t('settings.error.save')}
      navigation={layered ? navPanel : undefined}
      onSaveShortcut={() => saveNow().catch(() => undefined)}
      actions={(
        <AutosaveStatusIndicator
          status={autosaveStatus}
          error={autosaveError}
          onRetry={() => saveNow().catch(() => undefined)}
        />
      )}
    >
      {!layered ? (
        <LoadingState label={t('common.loading')} className="min-h-0 flex-1" />
      ) : (
        <div
          ref={contentRef}
          data-nova-settings-content="true"
          onScroll={onContentScroll}
          onWheelCapture={cancelSectionScroll}
          onPointerDownCapture={cancelSectionScroll}
          onKeyDownCapture={cancelSectionScroll}
          onTouchStartCapture={cancelSectionScroll}
          className="h-full min-h-0 overflow-y-auto overscroll-contain px-4 py-5 sm:px-6"
        >
          <div className="mx-auto w-full min-w-0 max-w-5xl">
            {sections.map((section) => (
              <Section
                key={section.id}
                id={section.id}
                ref={(node) => {
                  sectionRefs.current[section.id] = node
                }}
                group={section.group}
                title={section.title}
                expanded={expandedSections[section.id]}
                onToggle={() => toggleSection(section.id)}
              >
                {section.children}
              </Section>
            ))}
          </div>
        </div>
      )}
    </SettingsPageFrame>
  )
}

function settingsSectionAtScrollPosition(
  container: HTMLElement | null,
  refs: Partial<Record<SettingsSectionId, HTMLElement | null>>,
): SettingsSectionId | null {
  if (!container) return null
  const threshold = container.getBoundingClientRect().top + 72
  let nearestBefore: { id: SettingsSectionId; top: number } | null = null
  let nearestAfter: { id: SettingsSectionId; top: number } | null = null
  for (const [id, node] of Object.entries(refs) as Array<[SettingsSectionId, HTMLElement | null]>) {
    if (!node) continue
    const top = node.getBoundingClientRect().top
    if (top <= threshold && (!nearestBefore || top > nearestBefore.top)) nearestBefore = { id, top }
    if (top > threshold && (!nearestAfter || top < nearestAfter.top)) nearestAfter = { id, top }
  }
  return nearestBefore?.id ?? nearestAfter?.id ?? null
}

export function modelProfilesForEditor(draft: Settings, effective: Settings): ModelProfileSettings[] {
  const localProfiles = draft.model_profiles ?? []
  const hasLocalDefault = localProfiles.some((profile) => modelProfileID(profile) === DEFAULT_MODEL_PROFILE_ID)
  const hasLocalDefaultSelection = Boolean(draft.agent_models?.default?.profile_id?.trim())
  const hasLegacyDefault = Boolean(draft.openai_api_key || draft.openai_base_url || draft.openai_model || draft.openai_context_window_tokens)
  if (hasLocalDefault || hasLocalDefaultSelection || hasLegacyDefault) {
    return preserveDraftOnlyModelProfiles(modelProfilesWithDefault(draft), localProfiles)
  }
  const inherited = modelProfilesWithDefault(effective)
  const localIDs = new Set(localProfiles.map(modelProfileID).filter(Boolean))
  return [
    ...inherited.filter((profile) => !localIDs.has(modelProfileID(profile))),
    ...localProfiles,
  ]
}

export function modelEndpointsForEditor(draft: Settings, effective: Settings): ModelEndpointSettings[] {
  const localEndpoints = draft.model_endpoints ?? []
  const hasLocalDefault = localEndpoints.some((endpoint) => modelEndpointID(endpoint) === 'default')
  const hasLocalDefaultSelection = Boolean(draft.agent_models?.default?.profile_id?.trim())
  const hasLegacyDefault = Boolean(draft.openai_api_key || draft.openai_base_url)
  if (hasLocalDefault || hasLocalDefaultSelection || hasLegacyDefault) return modelEndpointsWithDefault(draft)
  const inherited = modelEndpointsWithDefault(effective)
  const localIDs = new Set(localEndpoints.map(modelEndpointID).filter(Boolean))
  return [
    ...inherited.filter((endpoint) => !localIDs.has(modelEndpointID(endpoint))).map(stripInheritedModelEndpointSecret),
    ...localEndpoints,
  ]
}

function isSettingsSectionId(value: unknown): value is SettingsSectionId {
  return typeof value === 'string' && SETTINGS_SECTION_IDS.includes(value as SettingsSectionId)
}

function preserveDraftOnlyModelProfiles(profiles: ModelProfileSettings[], draftProfiles: ModelProfileSettings[]): ModelProfileSettings[] {
  const draftOnlyProfiles = draftProfiles.filter((profile) => !modelProfileID(profile))
  if (draftOnlyProfiles.length === 0) return profiles
  return [...profiles, ...draftOnlyProfiles]
}

function stripInheritedModelEndpointSecret(endpoint: ModelEndpointSettings): ModelEndpointSettings {
  return { ...endpoint, api_key: '' }
}

export function imageAPIProfilesForEditor(draft: Settings, effective: Settings): ImageAPIProfileSettings[] {
  const localProfiles = draft.image_api_profiles ?? []
  const hasLocalDefault = localProfiles.some((profile) => imageAPIProfileID(profile) === DEFAULT_IMAGE_API_PROFILE_ID)
  const hasLocalDefaultSelection = Boolean(draft.default_image_api_profile_id?.trim())
  if (hasLocalDefault || hasLocalDefaultSelection) {
    return imageAPIProfilesWithDefault(draft)
  }
  const inherited = imageAPIProfilesWithDefault(effective)
  const localIDs = new Set(localProfiles.map(imageAPIProfileID).filter(Boolean))
  return [
    ...inherited.filter((profile) => !localIDs.has(imageAPIProfileID(profile))),
    ...localProfiles,
  ]
}

export function imageAPIEndpointsForEditor(draft: Settings, effective: Settings): ImageAPIEndpointSettings[] {
  const localEndpoints = draft.image_api_endpoints ?? []
  const hasLocalDefault = localEndpoints.some((endpoint) => imageAPIEndpointID(endpoint) === 'default')
  const hasLocalDefaultSelection = Boolean(draft.default_image_api_profile_id?.trim())
  if (hasLocalDefault || hasLocalDefaultSelection) return imageAPIEndpointsWithDefault(draft)
  const inherited = imageAPIEndpointsWithDefault(effective)
  const localIDs = new Set(localEndpoints.map(imageAPIEndpointID).filter(Boolean))
  return [
    ...inherited.filter((endpoint) => !localIDs.has(imageAPIEndpointID(endpoint))).map(stripInheritedImageAPIEndpointSecret),
    ...localEndpoints,
  ]
}

function stripInheritedImageAPIEndpointSecret(endpoint: ImageAPIEndpointSettings): ImageAPIEndpointSettings {
  return { ...endpoint, api_key: '' }
}

function Section({
  ref,
  id,
  group,
  title,
  expanded,
  onToggle,
  children,
}: {
  ref?: (node: HTMLElement | null) => void
  id: SettingsSectionId
  group: string
  title: string
  expanded: boolean
  onToggle: () => void
  children: ReactNode
}) {
  return (
    <section ref={ref} data-onboarding-anchor={id === 'model' ? 'settings-model' : undefined} className="scroll-mt-4 border-b border-[var(--nova-border)] py-4 first:pt-0 last:border-b-0">
      <button
        type="button"
        onClick={onToggle}
        className="nova-nav-item mb-2 flex w-full items-center justify-between rounded-[var(--nova-radius)] px-1.5 py-1 text-left"
        aria-expanded={expanded}
      >
        <span className="min-w-0">
          <span className="mr-2 text-[11px] text-[var(--nova-text-faint)]">{group}</span>
          <span className="font-medium text-[var(--nova-text)]">{title}</span>
        </span>
        {expanded ? (
          <ChevronUp className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        ) : (
          <ChevronDown className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        )}
      </button>
      {expanded && (
        <div className="nova-settings-section-card flex flex-col gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">{children}</div>
      )}
    </section>
  )
}

function FieldRow({ label, children }: { label: string; children: ReactNode }) {
  const generatedID = useId()
  const childID = isValidElement<{ id?: string }>(children) ? children.props.id : undefined
  const controlID = childID || generatedID
  const control = isValidElement<{ id?: string }>(children)
    ? cloneElement(children, { id: controlID })
    : children
  return (
    <SettingsFieldRow
      title={label}
      htmlFor={controlID}
      className="nova-settings-row rounded-md border-0 bg-transparent px-2 py-1.5"
      contentClassName="sm:w-44 sm:flex-none"
      controlClassName="flex-1"
    >
      {control}
    </SettingsFieldRow>
  )
}

function ValueRow({ label, children }: { label: string; children: ReactNode }) {
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

function TextSizeRow({ label, description, children }: { label: string; description: string; children: ReactNode }) {
  return (
    <SettingsFieldRow
      title={label}
      description={description}
      className="nova-settings-row rounded-md border-0 bg-transparent px-2 py-2"
      contentClassName="sm:w-44 sm:flex-none"
      controlClassName="flex-1 sm:max-w-md"
    >
      {children}
    </SettingsFieldRow>
  )
}

function ReadOnly({ label, value }: { label: string; value?: string }) {
  const { t } = useTranslation()
  return (
    <ValueRow label={label}>
      <code className="min-h-7 flex-1 truncate rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1.5 text-[var(--nova-text-muted)]">
        {value || t('common.notSet')}
      </code>
    </ValueRow>
  )
}

function LanguageSelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedLabel = t(LOCALE_OPTIONS.find((option) => option.value === (inherited || 'auto'))?.labelKey || 'locale.auto')
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full" aria-label={label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.defaultValue', { value: inheritedLabel })}</SelectItem>
            {LOCALE_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

const THEME_OPTIONS = [
  { value: 'dark', labelKey: 'settings.theme.dark' },
  { value: 'light', labelKey: 'settings.theme.light' },
  { value: 'system', labelKey: 'settings.theme.system' },
] as const

const MOTION_INTENSITY_OPTIONS = [
  { value: 'system', labelKey: 'settings.motion.system' },
  { value: 'full', labelKey: 'settings.motion.full' },
  { value: 'reduced', labelKey: 'settings.motion.reduced' },
  { value: 'off', labelKey: 'settings.motion.off' },
] as const

function ThemeSelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = inherited || 'dark'
  const inheritedLabel = t(THEME_OPTIONS.find((option) => option.value === inheritedValue)?.labelKey || 'settings.theme.dark')
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full" aria-label={label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.defaultValue', { value: inheritedLabel })}</SelectItem>
            {THEME_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

function MotionIntensitySelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = inherited || 'system'
  const inheritedLabel = t(MOTION_INTENSITY_OPTIONS.find((option) => option.value === inheritedValue)?.labelKey || 'settings.motion.system')
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full" aria-label={label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.defaultValue', { value: inheritedLabel })}</SelectItem>
            {MOTION_INTENSITY_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}


