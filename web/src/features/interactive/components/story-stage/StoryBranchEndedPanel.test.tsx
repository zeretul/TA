import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { TFunction } from 'i18next'
import { StoryBranchEndedPanel } from './StoryBranchEndedPanel'

// A translation stub keeps the assertions about behaviour rather than copy.
const t = ((key: string) => key) as unknown as TFunction

function renderPanel(props: Partial<Parameters<typeof StoryBranchEndedPanel>[0]> = {}) {
  const handlers = {
    onReviewStory: vi.fn(),
    onForkBeforeEnding: vi.fn(),
    onStartNewStory: vi.fn(),
  }
  const view = render(
    <StoryBranchEndedPanel
      outcome={{ terminal: true, type: 'death', reason: '生命归零，伤势超出承受' }}
      canFork
      hasEarlier={false}
      t={t}
      {...handlers}
      {...props}
    />,
  )
  return { ...view, ...handlers }
}

describe('StoryBranchEndedPanel', () => {
  // §3 asks for a plain ending notice and the ways on. The reason is the
  // committed outcome's own text, not a sentence the panel invents.
  it('states the ending with the committed reason and offers all three ways on', () => {
    renderPanel()

    expect(screen.getByTestId('story-branch-ended')).toHaveTextContent('storyStage.ended.title')
    expect(screen.getByTestId('story-branch-ended')).toHaveTextContent('生命归零，伤势超出承受')
    expect(screen.getByRole('button', { name: 'storyStage.ended.reviewStory' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'storyStage.ended.forkBefore' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'storyStage.ended.startNew' })).toBeInTheDocument()
  })

  // §3: 未实现的入口不展示为可用. When the stage cannot cut a fork there is
  // nothing behind the button, so it is dropped rather than shown dead.
  it('drops the fork entry instead of showing it unusable', () => {
    renderPanel({ canFork: false })

    expect(screen.queryByRole('button', { name: 'storyStage.ended.forkBefore' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'storyStage.ended.startNew' })).toBeInTheDocument()
  })

  it('says there is earlier history to load rather than pretending the story is all on screen', () => {
    renderPanel({ hasEarlier: true })

    expect(screen.getByRole('button', { name: 'storyStage.ended.reviewEarlier' })).toBeInTheDocument()
  })

  it('routes each entry to its own action', async () => {
    const user = userEvent.setup()
    const { onReviewStory, onForkBeforeEnding, onStartNewStory } = renderPanel()

    await user.click(screen.getByRole('button', { name: 'storyStage.ended.reviewStory' }))
    await user.click(screen.getByRole('button', { name: 'storyStage.ended.forkBefore' }))
    await user.click(screen.getByRole('button', { name: 'storyStage.ended.startNew' }))

    expect(onReviewStory).toHaveBeenCalledOnce()
    expect(onForkBeforeEnding).toHaveBeenCalledOnce()
    expect(onStartNewStory).toHaveBeenCalledOnce()
  })
})
