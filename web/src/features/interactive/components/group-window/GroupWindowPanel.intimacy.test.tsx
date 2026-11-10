import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GroupWindow } from '../../types'

const mocks = vi.hoisted(() => ({
  getGroupWindow: vi.fn(),
  createGroup: vi.fn(),
  changeGroupMembers: vi.fn(),
  sendGroupMessage: vi.fn(),
}))

vi.mock('../../api', () => ({
  getGroupWindow: mocks.getGroupWindow,
  createGroup: mocks.createGroup,
  changeGroupMembers: mocks.changeGroupMembers,
  sendGroupMessage: mocks.sendGroupMessage,
}))

import { GroupWindowPanel } from './GroupWindowPanel'

function groupWindow(): GroupWindow {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    scene: { title: '渡口', location: '青石渡口', world_time: '第3日', present: ['liuqing', 'shen'] },
    groups: [{
      id: 'grp-1', name: '渡口三人', type: 'self', channel: 'scene', contact: 'face to face',
      members: [
        { id: 'liuqing', name: '柳青', reachable: true, present: true, gender: 'female' },
        { id: 'shen', name: '沈砚', reachable: true, present: true, gender: 'male' },
      ],
      reachable_members: 2, version: 1,
    }],
    exchanges: [{
      id: 'xch_1', story_id: 'story-1', branch_id: 'main', character_id: 'liuqing',
      author_id: 'liuqing', speaker: 'character', channel: 'group', group_id: 'grp-1',
      text: '天亮前走。', world_time: '第3日',
    }],
    cast: [
      { id: 'liuqing', name: '柳青', known_as: '柳青', present: true, channel: 'scene', unread_messages: 0, open_matters: 0, affinity: 46, affinity_band: 'friendly' },
      { id: 'shen', name: '沈砚', known_as: '沈砚', present: true, channel: 'scene', unread_messages: 0, open_matters: 0, affinity: 46, affinity_band: 'friendly' },
    ],
    has_more: false,
    protagonist_gender: 'male',
  }
}

/** The host holds the draft, exactly as StoryStage does. */
function Hosted() {
  const [draft, setDraft] = useState('')
  return (
    <GroupWindowPanel
      storyId="story-1"
      branchId="main"
      active
      groupId="grp-1"
      onSelectGroup={() => {}}
      onClose={() => {}}
      draft={draft}
      onDraftChange={setDraft}
    />
  )
}

describe('GroupWindowPanel does not host the intimacy activity', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks)) mock.mockReset()
    mocks.getGroupWindow.mockResolvedValue(groupWindow())
  })

  // T27 moved dual cultivation out of the chat: a group is a place to talk, and
  // the activity has its own window with its own entry. Offering the switch here
  // again would mean two products sharing one conversation.
  it('shows the group conversation and nothing about intimacy', async () => {
    render(<Hosted />)

    expect(await screen.findByTestId('group-window-transcript')).toHaveTextContent('天亮前走。')
    expect(screen.queryByRole('button', { name: '亲密交流' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '普通交谈' })).not.toBeInTheDocument()
    expect(screen.queryByTestId('intimacy-surface')).not.toBeInTheDocument()
  })
})
