import { Pencil, Sparkles } from 'lucide-react'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import type { StoryOpeningConfig } from '../../types'

interface StoryOpeningSelectorProps {
  value: StoryOpeningConfig
  onChange: (value: StoryOpeningConfig) => void
}

export function StoryOpeningSelector({ value, onChange }: StoryOpeningSelectorProps) {
  const { t } = useTranslation()
  const customTextRef = useRef(value.mode === 'custom' ? value.custom_text || '' : '')
  if (value.mode === 'custom') customTextRef.current = value.custom_text || ''

  const selectMode = (mode: string) => {
    if (mode === 'custom') {
      onChange({ mode, custom_text: customTextRef.current })
      return
    }
    onChange({ mode: 'ai' })
  }

  return (
    <section aria-labelledby="story-opening-choice-title" className="overflow-hidden rounded-xl border border-border bg-card">
      <div className="px-4 pb-3 pt-4 sm:px-5">
        <div className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1">
          <h3 id="story-opening-choice-title" className="text-sm font-semibold text-foreground">{t('storyPicker.setup.opening.title')}</h3>
          <p className="text-xs text-muted-foreground">{t('storyPicker.setup.opening.description')}</p>
        </div>
      </div>
      <Tabs value={value.mode} onValueChange={selectMode} className="gap-0 border-t border-border">
        {/* A story created while the preset library existed keeps the opening it
            was given. It is shown, not offered: the two paths below replace it
            only if the player picks one. */}
        {value.mode === 'preset' ? (
          <div className="border-b border-border bg-muted/30 px-4 py-3 sm:px-5">
            <div className="text-xs font-medium text-foreground">{t('storyPicker.setup.opening.legacyPresetTitle')}</div>
            <p className="mt-1 whitespace-pre-wrap text-xs leading-5 text-muted-foreground">{value.preset_text}</p>
            <p className="mt-1.5 text-[11px] leading-5 text-muted-foreground">{t('storyPicker.setup.opening.legacyPresetHint')}</p>
          </div>
        ) : null}
        <TabsList className="grid h-10 w-full grid-cols-2 rounded-none border-b border-border bg-muted/40 p-0 group-data-horizontal/tabs:h-10">
          <OpeningTab value="ai" icon={<Sparkles />} label={t('storyPicker.setup.opening.ai')} />
          <OpeningTab value="custom" icon={<Pencil />} label={t('storyPicker.setup.opening.custom')} />
        </TabsList>

        <TabsContent value="ai" className="m-0 min-h-48 p-4 sm:p-5">
          <div className="flex h-full flex-col justify-center rounded-lg border border-dashed border-border bg-background p-4">
            <span className="flex size-9 items-center justify-center rounded-lg border border-border bg-muted text-primary"><Sparkles className="size-4" /></span>
            <h4 className="mt-3 text-sm font-medium text-foreground">{t('storyPicker.setup.opening.aiTitle')}</h4>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('storyPicker.setup.opening.aiDescription')}</p>
          </div>
        </TabsContent>

        <TabsContent value="custom" className="m-0 min-h-48 p-4 sm:p-5">
          <Textarea autoResize value={value.mode === 'custom' ? value.custom_text || '' : ''} maxLength={4000} className="min-h-40 resize-y bg-background text-sm leading-6" placeholder={t('storyPicker.setup.opening.customPlaceholder')} onChange={(event) => { customTextRef.current = event.target.value; onChange({ mode: 'custom', custom_text: event.target.value }) }} />
          <div className="mt-1.5 text-right font-mono text-[10px] tabular-nums text-muted-foreground">{t('storyPicker.setup.opening.characterCount', { count: value.mode === 'custom' ? value.custom_text?.length || 0 : 0 })}</div>
        </TabsContent>
      </Tabs>
    </section>
  )
}

function OpeningTab({ value, icon, label }: { value: string; icon: React.ReactNode; label: string }) {
  return (
    <TabsTrigger value={value} className="h-10 min-w-0 rounded-none border-0 border-b-2 border-transparent px-2 text-xs shadow-none after:hidden data-[state=active]:border-primary data-[state=active]:bg-background data-[state=active]:shadow-none">
      <span className="flex min-w-0 items-center gap-1.5 [&_svg]:size-3.5">{icon}<span className="truncate">{label}</span></span>
    </TabsTrigger>
  )
}
