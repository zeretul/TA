import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentQuickPromptSettings } from '@/features/settings/types'
import { useAgentQuickPromptControls } from './use-agent-quick-prompt-controls'

const settings = vi.hoisted(() => ({
  customized: true,
  loading: false,
  showInCommands: false,
  prompts: [
    { id: 'fill', name: '填充操作', prompt: 'Fill this prompt', behavior: 'fill', enabled: true },
    { id: 'send', name: '发送操作', prompt: 'Send this prompt', behavior: 'send', enabled: true },
    { id: 'hidden', name: '隐藏操作', prompt: 'Hidden prompt', behavior: 'fill', enabled: false },
  ] as AgentQuickPromptSettings[],
  save: vi.fn(),
}))

vi.mock('./use-agent-quick-prompts', () => ({ useAgentQuickPrompts: () => settings }))

function Harness({ showCards = true, onFill = vi.fn(), onSend = vi.fn() }: {
  showCards?: boolean
  onFill?: (prompt: string) => void
  onSend?: (prompt: string) => void
}) {
  const controls = useAgentQuickPromptControls({ scope: 'skills', disabled: false, onFill, onSend })
  return <>{showCards ? controls.cards : <p>Existing conversation</p>}</>
}

describe('AgentQuickPrompts', () => {
  beforeEach(() => {
    settings.customized = true
    settings.showInCommands = false
    settings.save.mockReset().mockResolvedValue(undefined)
  })

  it('fills or sends cards according to each prompt setting', async () => {
    const user = userEvent.setup()
    const onFill = vi.fn()
    const onSend = vi.fn()
    render(<Harness onFill={onFill} onSend={onSend} />)

    await user.click(screen.getByRole('button', { name: /填充操作/ }))
    expect(onFill).toHaveBeenCalledWith('Fill this prompt')
    expect(onSend).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: /发送操作/ }))
    expect(onSend).toHaveBeenCalledWith('Send this prompt')
    expect(screen.queryByText('隐藏操作')).not.toBeInTheDocument()
  })

  it('starts no conversation from the starter actions', () => {
    render(<Harness showCards={false} />)
    expect(screen.queryByRole('button', { name: /填充操作/ })).not.toBeInTheDocument()
    expect(screen.getByText('Existing conversation')).toBeInTheDocument()
  })
})
