import { MessageCircle, Sparkles, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { AgentQuickPromptSettings } from '@/features/settings/types'

interface AgentQuickPromptsProps {
  title: string
  prompts: AgentQuickPromptSettings[]
  disabled?: boolean
  onFill: (prompt: string) => void
  onSend: (prompt: string) => void | Promise<unknown>
}

export function AgentQuickPrompts({ title, prompts, disabled, onFill, onSend }: AgentQuickPromptsProps) {
  const { t } = useTranslation()
  const visiblePrompts = prompts.filter((prompt) => prompt.enabled)

  return (
    <section className="border-b bg-background p-3">
      <header className="mb-2 flex min-h-7 items-center gap-2">
        <Sparkles className="size-3.5 text-muted-foreground" aria-hidden="true" />
        <h2 className="min-w-0 flex-1 truncate text-xs font-medium text-muted-foreground">{title}</h2>
      </header>
      {visiblePrompts.length > 0 ? (
        <div className="grid grid-cols-2 gap-2">
          {visiblePrompts.map((prompt) => (
            <Button
              key={prompt.id}
              type="button"
              variant="outline"
              disabled={disabled}
              className="h-auto min-w-0 justify-start px-3 py-2 text-left text-xs"
              onClick={() => {
                if (prompt.behavior === 'send') {
                  void onSend(prompt.prompt)
                  return
                }
                onFill(prompt.prompt)
              }}
              aria-label={t('chat.quick.runLabel', { name: prompt.name, behavior: t(`chat.quick.behavior.${prompt.behavior}`) })}
              title={t('chat.quick.runLabel', { name: prompt.name, behavior: t(`chat.quick.behavior.${prompt.behavior}`) })}
            >
              <MessageCircle data-icon="inline-start" />
              <span className="min-w-0 flex-1 truncate">{prompt.name}</span>
              {prompt.behavior === 'send' ? <Zap data-icon="inline-end" className="text-muted-foreground" /> : null}
            </Button>
          ))}
        </div>
      ) : (
        <p className="rounded-lg border border-dashed px-3 py-3 text-xs text-muted-foreground">
          {t('chat.quick.settings.emptySection')}
        </p>
      )}
    </section>
  )
}
