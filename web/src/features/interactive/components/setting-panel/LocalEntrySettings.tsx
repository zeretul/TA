import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus, X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { LocalEntitySettings } from '../../types'
import type { LoreEntry } from '../lore-overview/model'

const LOAD_MODES = ['auto', 'resident', 'manual'] as const

/**
 * 资料：本局新建页给一条资料定标签与取用方式。
 *
 * These two fields are the only part of the 本局新建 page the player owns. What a
 * person is, where they are, and what they have done all come from the story's
 * own turns; these two say how the player files them and how much of them the
 * model reads.
 *
 * Only a person carries a load mode. A place, a faction and a holding live in
 * the world Actor's own state, which every turn reads in full, so offering the
 * choice there would store a setting nothing acts on. Their tags are still the
 * player's to set.
 */
export function LocalEntrySettings({
  entry,
  settings,
  saving,
  error,
  onSave,
}: {
  entry: LoreEntry
  settings: LocalEntitySettings
  saving: boolean
  error?: string
  onSave: (next: LocalEntitySettings) => void
}) {
  const { t } = useTranslation()
  const [tagDraft, setTagDraft] = useState('')
  const tags = settings.tags ?? []

  const addTag = useCallback(() => {
    const tag = tagDraft.trim()
    setTagDraft('')
    if (!tag || tags.includes(tag)) return
    onSave({ ...settings, tags: [...tags, tag] })
  }, [tagDraft, tags, settings, onSave])

  const removeTag = useCallback((tag: string) => {
    onSave({ ...settings, tags: tags.filter((existing) => existing !== tag) })
  }, [tags, settings, onSave])

  const changeLoadMode = useCallback((value: string) => {
    // "默认" is not a fourth mode: it is the absence of one, which is how the
    // record goes back to the reading the entity had before this page existed.
    onSave({
      ...settings,
      load_mode: value === 'default' ? undefined : (value as LocalEntitySettings['load_mode']),
    })
  }, [settings, onSave])

  const mode = settings.load_mode ?? 'default'

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-2">
        <span className="text-xs font-medium text-[var(--nova-text-muted)]">
          {t('settingPanel.localSettings.tagsLabel')}
        </span>
        <div className="flex flex-wrap items-center gap-1.5">
          {tags.map((tag) => (
            <Badge key={tag} variant="secondary" className="gap-1 pr-1">
              <span>{tag}</span>
              <button
                type="button"
                className="rounded p-0.5 hover:bg-[var(--nova-surface-3)]"
                aria-label={t('settingPanel.localSettings.removeTag', { tag })}
                disabled={saving}
                onClick={() => removeTag(tag)}
              >
                <X className="size-3" />
              </button>
            </Badge>
          ))}
          {tags.length === 0 ? (
            <span className="text-xs text-[var(--nova-text-muted)]">{t('settingPanel.localSettings.noTags')}</span>
          ) : null}
        </div>
        <div className="flex items-center gap-2">
          <Input
            value={tagDraft}
            disabled={saving}
            placeholder={t('settingPanel.localSettings.tagPlaceholder')}
            onChange={(event) => setTagDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                addTag()
              }
            }}
            className="h-8"
          />
          <Button
            variant="outline"
            size="sm"
            className="shrink-0"
            disabled={saving || !tagDraft.trim()}
            onClick={addTag}
          >
            <Plus data-icon="inline-start" />
            {t('settingPanel.localSettings.addTag')}
          </Button>
        </div>
      </div>

      {entry.tab === 'people' ? (
        <div className="flex flex-col gap-2">
          <span className="text-xs font-medium text-[var(--nova-text-muted)]">
            {t('settingPanel.localSettings.loadModeLabel')}
          </span>
          <Select value={mode} onValueChange={changeLoadMode} disabled={saving}>
            <SelectTrigger className="h-8">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="default">{t('settingPanel.localSettings.loadMode.default')}</SelectItem>
              {LOAD_MODES.map((option) => (
                <SelectItem key={option} value={option}>
                  {t(`settingPanel.localSettings.loadMode.${option}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-[var(--nova-text-muted)]">
            {t(`settingPanel.localSettings.loadModeHint.${mode}`)}
          </p>
        </div>
      ) : null}

      {error ? <p className="text-xs text-[var(--nova-danger)]">{error}</p> : null}
    </div>
  )
}
