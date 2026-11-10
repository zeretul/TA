import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CharacterWindow } from '../../types'

const mocks = vi.hoisted(() => ({
  getCharacterWindow: vi.fn(),
  sendCharacterMessage: vi.fn(),
  setCharacterMessageStatus: vi.fn(),
  getCharacterPicture: vi.fn(),
  importCharacterPicture: vi.fn(),
  clearCharacterPicture: vi.fn(),
  getPlacePicture: vi.fn(),
  importPlacePicture: vi.fn(),
  clearPlacePicture: vi.fn(),
}))

vi.mock('../../api', () => ({
  getCharacterWindow: mocks.getCharacterWindow,
  sendCharacterMessage: mocks.sendCharacterMessage,
  setCharacterMessageStatus: mocks.setCharacterMessageStatus,
  getCharacterPicture: mocks.getCharacterPicture,
  importCharacterPicture: mocks.importCharacterPicture,
  clearCharacterPicture: mocks.clearCharacterPicture,
  getPlacePicture: mocks.getPlacePicture,
  importPlacePicture: mocks.importPlacePicture,
  clearPlacePicture: mocks.clearPlacePicture,
}))

import { CharacterWindowPanel } from './CharacterWindowPanel'

function windowState(): CharacterWindow {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    scene: { title: '渡口', location: '青石渡口', world_time: '第3日', present: ['liuqing'] },
    characters: [{
      id: 'liuqing', name: '柳青', known_as: '柳青', present: true, channel: 'scene', gender: 'female',
      unread_messages: 0, open_matters: 0, affinity: 46, affinity_band: 'friendly',
    }],
    exchanges: [{
      id: 'xch_1', story_id: 'story-1', branch_id: 'main', character_id: 'liuqing',
      speaker: 'player', channel: 'scene', text: '明早我去渡口。', world_time: '第3日',
    }],
    matters: [],
    messages: [],
    has_more: false,
    protagonist_gender: 'male',
  }
}

/** The host holds the draft, exactly as StoryStage does. */
function Hosted() {
  const [draft, setDraft] = useState('')
  return (
    <CharacterWindowPanel
      storyId="story-1"
      branchId="main"
      active
      characterId="liuqing"
      onSelectCharacter={() => {}}
      onClose={() => {}}
      draft={draft}
      onDraftChange={setDraft}
    />
  )
}

describe('CharacterWindowPanel does not host the intimacy activity', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks)) mock.mockReset()
    mocks.setCharacterMessageStatus.mockResolvedValue(undefined)
    mocks.getCharacterWindow.mockResolvedValue(windowState())
    mocks.getCharacterPicture.mockResolvedValue({ source: '', overridden: false })
    mocks.getPlacePicture.mockResolvedValue({ overridden: false })
  })

  // T27 gave the activity its own window and its own toolbar entry. A person's
  // chat is a place to talk to that person; folding the activity back into it
  // would be the second entry T27 removed.
  it('shows the person’s conversation and nothing about intimacy', async () => {
    render(<Hosted />)

    expect(await screen.findByTestId('character-window-panel')).toBeInTheDocument()
    expect(await screen.findByText('明早我去渡口。')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '亲密交流' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '普通交谈' })).not.toBeInTheDocument()
    expect(screen.queryByTestId('intimacy-surface')).not.toBeInTheDocument()
  })

  it('keeps the ordinary composer working', async () => {
    render(<Hosted />)

    expect(await screen.findByRole('textbox')).toBeInTheDocument()
    expect(mocks.sendCharacterMessage).not.toHaveBeenCalled()
  })
})
