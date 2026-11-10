import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { IntimacyWindow } from '../../types'

const mocks = vi.hoisted(() => ({
  getIntimacyWindow: vi.fn(),
  inviteIntimacy: vi.fn(),
  respondIntimacy: vi.fn(),
  confirmIntimacy: vi.fn(),
  sendIntimacyMessage: vi.fn(),
  endIntimacy: vi.fn(),
}))

vi.mock('../../api', () => ({
  getIntimacyWindow: mocks.getIntimacyWindow,
  inviteIntimacy: mocks.inviteIntimacy,
  respondIntimacy: mocks.respondIntimacy,
  confirmIntimacy: mocks.confirmIntimacy,
  sendIntimacyMessage: mocks.sendIntimacyMessage,
  endIntimacy: mocks.endIntimacy,
}))

import { IntimacyWindowPanel } from './IntimacyWindowPanel'

function intimacyWindow(overrides: Partial<IntimacyWindow> = {}): IntimacyWindow {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    scene: { title: '渡口', location: '青石渡口', world_time: '第3日', present: ['liuqing', 'shen'] },
    activities: [],
    cast: [
      { id: 'liuqing', name: '柳青', known_as: '柳青', present: true, channel: 'scene', gender: 'female', unread_messages: 0, open_matters: 0, affinity: 46, affinity_band: 'friendly' },
      { id: 'shen', name: '沈砚', known_as: '沈砚', present: true, channel: 'scene', gender: 'male', unread_messages: 0, open_matters: 0, affinity: 46, affinity_band: 'friendly' },
    ],
    eligible: ['liuqing', 'shen'],
    model: { configured: true, profile_id: 'p1', model: 'test-model', provider: 'local', thinking_level: 'off' },
    ...overrides,
  }
}

function renderPanel(props: { focusCharacterId?: string; onClose?: () => void; onContinueStory?: (message: string) => void } = {}) {
  return render(
    <IntimacyWindowPanel
      storyId="story-1"
      branchId="main"
      active
      onClose={props.onClose || (() => {})}
      focusCharacterId={props.focusCharacterId}
      onContinueStory={props.onContinueStory}
    />,
  )
}

describe('IntimacyWindowPanel', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks)) mock.mockReset()
    mocks.getIntimacyWindow.mockResolvedValue(intimacyWindow())
  })

  // T27: the activity has its own window, and opening it is a plain read of what
  // the activity already is. Nothing is invited or sent by looking.
  it('reads the activity without inviting or sending anything', async () => {
    renderPanel()

    expect(await screen.findByTestId('intimacy-surface')).toBeInTheDocument()
    await waitFor(() => expect(mocks.getIntimacyWindow).toHaveBeenCalled())
    expect(mocks.inviteIntimacy).not.toHaveBeenCalled()
    expect(mocks.sendIntimacyMessage).not.toHaveBeenCalled()
  })

  // Closing the window is a display act. A pending invitation stays pending and
  // a running activity keeps running, so the close handler must not reach any
  // service that would end one.
  it('closes without ending anything', async () => {
    const onClose = vi.fn()
    const user = userEvent.setup()
    renderPanel({ onClose })

    await screen.findByTestId('intimacy-surface')
    await user.click(screen.getByRole('button', { name: '关闭双修窗口' }))

    expect(onClose).toHaveBeenCalledTimes(1)
    expect(mocks.endIntimacy).not.toHaveBeenCalled()
  })

  // §3.3: the multi-person form builds a roster the player then confirms. The
  // invitation is not the one-to-one direct-entry kind.
  it('builds a roster for several people instead of entering directly', async () => {
    const user = userEvent.setup()
    mocks.inviteIntimacy.mockResolvedValue({
      story_id: 'story-1', branch_id: 'main', activity_id: 'int_1',
      activity: { id: 'int_1', story_id: 'story-1', branch_id: 'main', phase: 'inviting', version: 1 },
      reply: '我去。',
    })
    mocks.getIntimacyWindow.mockResolvedValueOnce(intimacyWindow())
    mocks.getIntimacyWindow.mockResolvedValue(intimacyWindow({
      activity_id: 'int_1',
      activities: [{
        id: 'int_1', story_id: 'story-1', branch_id: 'main', phase: 'inviting', version: 1,
        participants: [{ character_id: 'liuqing', status: 'accepted', adult: true }],
        participant_names: { liuqing: '柳青' },
        roster: ['liuqing'],
      }],
    }))

    renderPanel()
    const inviteButtons = await screen.findAllByRole('button', { name: '邀请双修' })
    await user.click(inviteButtons[0])

    await waitFor(() => expect(mocks.inviteIntimacy).toHaveBeenCalledTimes(1))
    expect(mocks.inviteIntimacy.mock.calls[0][1]).toMatchObject({ characterId: 'liuqing', oneOnOne: false })
    // §3.3 keeps the player's own confirmation even when everyone agreed.
    expect(await screen.findByRole('button', { name: '按已同意名单开始' })).toBeInTheDocument()
    expect(mocks.confirmIntimacy).not.toHaveBeenCalled()
  })

  // Opened from one person's conversation, the window asks that person and only
  // them, which is the direct-entry path the service already has.
  it('asks the person the window was opened for, and only them', async () => {
    const user = userEvent.setup()
    mocks.inviteIntimacy.mockResolvedValue({
      story_id: 'story-1', branch_id: 'main', activity_id: 'int_1',
      activity: { id: 'int_1', story_id: 'story-1', branch_id: 'main', phase: 'active', version: 1 },
      reply: '好。',
      exchange_ids: ['xch_9'],
    })
    mocks.getIntimacyWindow.mockResolvedValueOnce(intimacyWindow())
    mocks.getIntimacyWindow.mockResolvedValue(intimacyWindow({
      activity_id: 'int_1',
      activities: [{
        id: 'int_1', story_id: 'story-1', branch_id: 'main', phase: 'active', version: 1,
        participants: [{ character_id: 'liuqing', status: 'accepted', adult: true }],
        participant_names: { liuqing: '柳青' },
        roster: ['liuqing'],
      }],
      exchanges: [{
        id: 'xch_9', story_id: 'story-1', branch_id: 'main', character_id: 'liuqing',
        author_id: 'liuqing', speaker: 'character', channel: 'private', text: '好。', world_time: '第3日',
      }],
    }))

    renderPanel({ focusCharacterId: 'liuqing' })
    await user.click(await screen.findByRole('button', { name: '邀请双修' }))

    await waitFor(() => expect(mocks.inviteIntimacy).toHaveBeenCalledTimes(1))
    expect(mocks.inviteIntimacy.mock.calls[0][1]).toMatchObject({ characterId: 'liuqing', oneOnOne: true })
    expect(await screen.findByText('好。')).toBeInTheDocument()
    expect(screen.getByTestId('intimacy-surface-roster')).toHaveTextContent('柳青')
  })

  // The person's own authored data decides whether they are an adult, so an
  // empty eligible list is shown as such rather than offered as an invitation.
  it('cannot start anything for a person the story has not established as adult', async () => {
    mocks.getIntimacyWindow.mockResolvedValue(intimacyWindow({ eligible: [] }))

    renderPanel({ focusCharacterId: 'liuqing' })

    expect(await screen.findByText('此刻没有可以邀请的人。')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '邀请双修' })).not.toBeInTheDocument()
  })

  it('shows each person’s own answer rather than a single ready state', async () => {
    mocks.getIntimacyWindow.mockResolvedValue(intimacyWindow({
      activities: [{
        id: 'int_1', story_id: 'story-1', branch_id: 'main', phase: 'inviting', version: 2,
        participants: [
          { character_id: 'liuqing', status: 'accepted', adult: true },
          { character_id: 'shen', status: 'conditional', condition: '先了结眼前的事', adult: true },
        ],
        participant_names: { liuqing: '柳青', shen: '沈砚' },
        roster: ['liuqing'],
      }],
    }))

    renderPanel()

    const roster = await screen.findByTestId('intimacy-surface-roster')
    expect(roster).toHaveTextContent('柳青')
    expect(roster).toHaveTextContent('已同意')
    expect(roster).toHaveTextContent('沈砚')
    expect(roster).toHaveTextContent('有条件')
    expect(roster).toHaveTextContent('先了结眼前的事')
    expect(roster).not.toHaveTextContent('大家都准备好了')
  })

  // T27: the model binding is chosen in game settings, so the window only says
  // which model is in use. It must never grow a picker of its own again.
  it('reports the model in use without offering a picker', async () => {
    renderPanel()

    const line = await screen.findByTestId('intimacy-window-model')
    expect(line).toHaveTextContent('test-model')
    expect(line).not.toHaveTextContent('还没有为双修选择专用模型')
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  })

  it('says the model is unset and where to set it, still without a picker', async () => {
    mocks.getIntimacyWindow.mockResolvedValue(intimacyWindow({
      model: { configured: false, profile_id: '', model: '', provider: '', thinking_level: 'off' },
    }))

    renderPanel()

    const line = await screen.findByTestId('intimacy-window-model')
    expect(line).toHaveTextContent('还没有为双修选择专用模型。')
    expect(line).toHaveTextContent('游戏设置')
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  })
})
