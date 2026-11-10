import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { AgentQuickPromptSettings } from '@/features/settings/types'
import { AgentQuickPrompts } from './AgentQuickPrompts'
import { agentQuickPromptDefaults, type AgentQuickPromptScope } from './defaults'
import { useAgentQuickPrompts } from './use-agent-quick-prompts'

const NO_PROMPTS: AgentQuickPromptSettings[] = []

/**
 * One page-scoped list owns the starter actions and the slash suggestions.
 *
 * The actions stay as plain one-click entries in the assistant body. They used
 * to sit behind a command-management dialog, which turned a short list of
 * everyday edits into a second workbench the player had to learn.
 */
export function useAgentQuickPromptControls({ scope, disabled, onFill, onSend }: {
  scope: AgentQuickPromptScope | undefined
  disabled: boolean
  onFill: (prompt: string) => void
  onSend: (prompt: string) => void | Promise<unknown>
}) {
  const { t } = useTranslation()
  const defaults = useMemo(() => (scope ? agentQuickPromptDefaults(t, scope) : null), [scope, t])
  const settings = useAgentQuickPrompts(scope, defaults?.prompts ?? NO_PROMPTS)
  const loading = disabled || settings.loading

  return {
    commands: scope && settings.showInCommands ? settings.prompts : NO_PROMPTS,
    cards: defaults ? (
      <AgentQuickPrompts
        title={defaults.title}
        prompts={settings.prompts}
        disabled={loading}
        onFill={onFill}
        onSend={onSend}
      />
    ) : null,
  }
}
