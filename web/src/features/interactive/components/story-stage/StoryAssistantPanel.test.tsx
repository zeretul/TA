import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { TFunction } from 'i18next'
import { StoryAssistantPanel } from './StoryAssistantPanel'
import type { UseStoryAssistantResult } from './use-story-assistant'

// A translation stub keeps the assertions about behaviour rather than copy.
const t = ((key: string, options?: Record<string, unknown>) =>
  options?.message ? `${key}:${String(options.message)}` : key) as unknown as TFunction

function assistant(overrides: Partial<UseStoryAssistantResult> = {}): UseStoryAssistantResult {
  return {
    answer: null,
    asking: null,
    error: null,
    adopting: false,
    adoptError: null,
    adoptConflict: false,
    adopted: null,
    canAdopt: true,
    ask: vi.fn(async () => {}),
    adopt: vi.fn(async () => true),
    clear: vi.fn(),
    ...overrides,
  }
}

function renderPanel(props: Partial<Parameters<typeof StoryAssistantPanel>[0]> = {}) {
  const onUseWording = vi.fn()
  const value = {
    assistant: assistant(),
    expanded: true,
    onToggle: vi.fn(),
    onClose: vi.fn(),
    input: '',
    onUseWording,
    t,
    ...props,
  }
  const view = render(<StoryAssistantPanel {...value} />)
  return { ...view, onUseWording, value }
}

const answer = {
  story_id: 'st_1', branch_id: 'main', turn_id: 'ev_1', action: 'analyze' as const,
  read: '门口那两个人并不认识你，他们看的是你腰间的袋子。',
  suggestions: [
    { label: '先绕开', detail: '避开他们，走旁边的巷子', action: '绕开门口的人，从旁边巷子走' },
  ],
  context_revision: 7,
  stale: false,
}

describe('StoryAssistantPanel', () => {
  it('asks for an analysis only when the player clicks', async () => {
    const { value } = renderPanel()
    expect(value.assistant.ask).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: 'storyStage.assistant.analyze' }))
    expect(value.assistant.ask).toHaveBeenCalledWith({ action: 'analyze', playerInput: '' })
  })

  it('passes the unsent input as context, not as a submission', async () => {
    const { value } = renderPanel({ input: '我想先看看门口' })
    await userEvent.click(screen.getByRole('button', { name: 'storyStage.assistant.inspire' }))
    expect(value.assistant.ask).toHaveBeenCalledWith({ action: 'inspire', playerInput: '我想先看看门口' })
  })

  it('can show an analysis without suggestions after inspiration supplied some', () => {
    const { rerender, value } = renderPanel({
      assistant: assistant({ answer: { ...answer, action: 'inspire' } }),
    })
    expect(screen.getByText('先绕开')).toBeVisible()
    const analysis = { ...answer, read: '石桌上只搁着一张折起的来帖。', suggestions: null }
    rerender(<StoryAssistantPanel {...value} assistant={assistant({ answer: analysis })} />)
    expect(screen.getByText(analysis.read)).toBeVisible()
    expect(screen.queryByText('先绕开')).not.toBeInTheDocument()
  })

  it('puts a suggestion in the input without sending it', async () => {
    const { onUseWording } = renderPanel({
      assistant: assistant({ answer: { ...answer, stale: false } }),
    })

    await userEvent.click(screen.getByRole('button', { name: 'storyStage.assistant.useThis' }))
    expect(onUseWording).toHaveBeenCalledWith('绕开门口的人，从旁边巷子走')
  })

  it('adopts only through the separate adopt action', async () => {
    const { value } = renderPanel({
      assistant: assistant({ answer: { ...answer, stale: false } }),
    })

    expect(value.assistant.adopt).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: /storyStage\.assistant\.adopt/ }))
    expect(value.assistant.adopt).toHaveBeenCalledWith('绕开门口的人，从旁边巷子走')
  })

  it('labels advice written for a turn the player has left', () => {
    renderPanel({ assistant: assistant({ answer: { ...answer, stale: true } }) })
    expect(screen.getByText('storyStage.assistant.stale')).toBeTruthy()
    // The analysis is still readable: the player asked for it.
    expect(screen.getByText(answer.read)).toBeTruthy()
  })

  it('offers no adopt button on a branch without a plan', () => {
    renderPanel({ assistant: assistant({ answer: { ...answer, stale: false }, canAdopt: false }) })
    expect(screen.queryByRole('button', { name: /storyStage\.assistant\.adopt/ })).toBeNull()
    expect(screen.getByText('storyStage.assistant.noPlan')).toBeTruthy()
  })

  it('reports a failed question without pretending there is an answer', () => {
    renderPanel({ assistant: assistant({ error: 'model unavailable' }) })
    expect(screen.getByText('storyStage.assistant.failed:model unavailable')).toBeTruthy()
  })
})
