import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { StoryStageControls } from './StoryStageControls'

function renderControls() {
  return render(
    <StoryStageControls
      isMobile={false}
      picker={{ stories: [], currentStoryId: '', onSelect: () => {}, onDeleteStories: () => {} }}
      history={{ items: [], onSelect: () => {} }}
      chatWindowVisible={false}
      onToggleChatWindow={() => {}}
      combatWindowVisible={false}
      onToggleCombatWindow={() => {}}
      realmWindowVisible={false}
      onToggleRealmWindow={() => {}}
      questWindowVisible={false}
      onToggleQuestWindow={() => {}}
      loreOverviewVisible={false}
      onToggleLoreOverview={() => {}}
    />,
  )
}

describe('StoryStageControls', () => {
  // T27: private chat and groups were two buttons opening two windows for the
  // same act, so they became one 聊天 entry. This list is the whole set: a
  // window added back here has to be a deliberate change to the design, not a
  // leftover.
  it('offers one chat entry instead of separate person and group windows', () => {
    renderControls()

    const labels = screen.getAllByRole('button')
      .map((button) => button.getAttribute('aria-label') || button.getAttribute('title') || '')
      .filter((label) => label.startsWith('打开'))
    expect(labels).toEqual([
      '打开聊天窗口',
      '打开交战',
      '打开探索信息',
      '打开见闻与事务',
      '打开资料总览',
    ])
    expect(screen.queryByTitle('人物')).not.toBeInTheDocument()
    expect(screen.queryByTitle('群聊')).not.toBeInTheDocument()
  })

  // The dual-cultivation window is its own entry again (T27), but only where the
  // world actually supports that activity. A detective story must not grow a
  // 双修 button just because the toolbar has a slot for it.
  it('shows the dual-cultivation entry only when the caller says the world has it', () => {
    renderControls()
    expect(screen.queryByTitle('双修')).not.toBeInTheDocument()

    render(
      <StoryStageControls
        isMobile={false}
        picker={{ stories: [], currentStoryId: '', onSelect: () => {}, onDeleteStories: () => {} }}
        history={{ items: [], onSelect: () => {} }}
        intimacyWindowVisible={false}
        onToggleIntimacyWindow={() => {}}
      />,
    )
    expect(screen.getByTitle('双修')).toBeInTheDocument()
  })
})
