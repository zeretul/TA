import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { fetchSettings, patchSettings } from '@/features/settings/api'
import { BookMarked, ChevronDown, Folder } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { isSaveShortcut } from '@/lib/keyboard'
import { Textarea } from '@/components/ui/textarea'

/**
 * The directory shown when the workspace narrative file is the only entry.
 * The file name stays internal: the player reads it as the workspace default.
 */
export function CreatorDirectory() {
  const { t } = useTranslation()
  return (
    <div className="p-2">
      <div className="flex h-8 items-center gap-2 rounded px-2 text-xs text-[var(--nova-text-muted)]">
        <ChevronDown className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        <Folder className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        <span className="font-medium">{t('settingPanel.rootDirectory')}</span>
      </div>
      <div className="ml-5 border-l border-[var(--nova-border)] pl-2">
        <div className="flex h-8 items-center gap-2 rounded-[var(--nova-radius)] bg-[var(--nova-active)] px-2 text-xs text-[var(--nova-text)]">
          <BookMarked className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
          <span className="truncate">{t('settingPanel.workspaceNarrative.title')}</span>
        </div>
      </div>
    </div>
  )
}

export function CreatorEditor({
  content,
  setContent,
  onSave,
}: {
  content: string
  setContent: (value: string) => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-4">
      <Textarea
        autoResize={false}
        className="nova-field h-full min-h-[520px] resize-none font-mono text-sm leading-7 shadow-none focus-visible:ring-0"
        value={content}
        onChange={(event) => setContent(event.target.value)}
        placeholder={t('settingPanel.placeholder.creator')}
        onKeyDown={(event) => {
          if (isSaveShortcut(event)) {
            event.preventDefault()
            event.stopPropagation()
            onSave()
          }
        }}
      />
    </div>
  )
}

/** Edits the user-level Game Agent prompt that all adventure worlds share. */
export function SharedNarrativeRulesEditor() {
  const { t } = useTranslation()
  const [content, setContent] = useState('')
  const [saved, setSaved] = useState('')
  const [revision, setRevision] = useState<string>()
  const [status, setStatus] = useState<'loading' | 'loadError' | 'ready' | 'saving' | 'saved' | 'saveError'>('loading')

  useEffect(() => {
    let active = true
    fetchSettings().then(settings => {
      if (!active) return
      const rules = settings.effective.agent_prompts?.interactive_story?.system_prompt ?? ''
      setContent(rules)
      setSaved(rules)
      setRevision(settings.revisions?.user)
      setStatus('ready')
    }).catch(() => { if (active) setStatus('loadError') })
    return () => { active = false }
  }, [])

  const save = async () => {
    setStatus('saving')
    try {
      const settings = await patchSettings('user', {
        agent_prompts: { interactive_story: { system_prompt: content } },
      }, revision)
      const rules = settings.effective.agent_prompts?.interactive_story?.system_prompt ?? ''
      setContent(rules)
      setSaved(rules)
      setRevision(settings.revisions?.user)
      setStatus('saved')
    } catch {
      setStatus('saveError')
    }
  }

  const disabled = status === 'loading' || status === 'loadError' || status === 'saving'
  return (
    <div className="flex min-w-0 flex-col gap-2 rounded-xl border border-border p-4">
      <label htmlFor="shared-narrative-rules" className="text-sm font-medium">{t('settingPanel.sharedNarrative.title')}</label>
      <p className="text-xs text-muted-foreground">{t('settingPanel.sharedNarrative.hint')}</p>
      <Textarea id="shared-narrative-rules" className="min-h-80 resize-y text-sm leading-7"
        autoResize={false} value={content} disabled={disabled}
        onChange={event => { setContent(event.target.value); setStatus('ready') }} />
      <div className="flex flex-wrap items-center gap-3">
        <Button type="button" size="sm" disabled={disabled || saved === content} onClick={() => void save()}>
          {t('settingPanel.sharedNarrative.save')}
        </Button>
        {status !== 'ready' ? <span role="status" className="text-xs text-muted-foreground">{t('settingPanel.sharedNarrative.' + status)}</span> : null}
      </div>
    </div>
  )
}
