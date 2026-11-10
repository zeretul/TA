import { useState } from 'react'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CharacterExchange, GroupWindow, GroupWindowGroup } from '../../types'

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

/**
 * Opens one of the summaries above the transcript.
 *
 * The roster, the arrangements and the plan field each fold to a single line so
 * the messages keep the panel's remaining height; what they hold is one click
 * away, which is what these assertions have to do before reading it.
 */
async function openSummary(user: ReturnType<typeof userEvent.setup>, label: RegExp) {
  await user.click(await screen.findByRole('button', { name: label }))
}

const group: GroupWindowGroup = {
  id: 'grp-1',
  name: '渡口三人',
  type: 'self',
  channel: 'scene',
  contact: 'same scene',
  reachable_members: 2,
  last_exchange: '天亮前走。',
  version: 1,
  members: [
    { id: 'guard', name: '周校尉', role: 'guardsman', present: true, reachable: true },
    { id: 'steward', name: '陆管家', role: 'steward', present: true, reachable: true },
    { id: 'abbot', name: '空闻', role: 'abbot', present: false, reachable: false },
  ],
}

function line(id: string, author: string, text: string): CharacterExchange {
  return {
    id,
    story_id: 'story-1',
    branch_id: 'main',
    character_id: '',
    author_id: author,
    speaker: author === 'player' ? 'player' : 'character',
    channel: 'group',
    group_id: 'grp-1',
    text,
    world_time: '卯时',
  }
}

function windowState(overrides: Partial<GroupWindow> = {}): GroupWindow {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    scene: { title: '渡口', location: '青石渡口', world_time: '卯时', present: ['guard', 'steward'] },
    groups: [group],
    cast: [
      { id: 'guard', name: '周校尉', known_as: '周校尉', present: true },
      { id: 'steward', name: '陆管家', known_as: '陆管家', present: true },
      { id: 'abbot', name: '空闻', known_as: '空闻', present: false },
    ],
    exchanges: [],
    matters: [],
    has_more: false,
    ...overrides,
  } as GroupWindow
}

/** The host holds the draft, exactly as StoryStage does. */
function Hosted({ groupId, onActOnPlan }: { groupId: string; onActOnPlan?: (message: string) => void }) {
  const [draft, setDraft] = useState('')
  return (
    <GroupWindowPanel
      storyId="story-1"
      branchId="main"
      active
      groupId={groupId}
      onSelectGroup={vi.fn()}
      onClose={vi.fn()}
      onActOnPlan={onActOnPlan}
      draft={draft}
      onDraftChange={setDraft}
    />
  )
}

describe('GroupWindowPanel', () => {
  beforeEach(() => {
    mocks.getGroupWindow.mockReset()
    mocks.createGroup.mockReset()
    mocks.changeGroupMembers.mockReset()
    mocks.sendGroupMessage.mockReset()
  })

  it('keeps the reading anchor when an earlier page is prepended', async () => {
    const user = userEvent.setup()
    mocks.getGroupWindow.mockResolvedValueOnce(windowState({ exchanges: [line('latest', 'guard', 'Latest page')], has_more: true, cursor: 'older' }))
      .mockResolvedValueOnce(windowState({ exchanges: [line('old', 'guard', 'Earlier page')], has_more: false }))
    render(<Hosted groupId="grp-1" />)
    await screen.findByText('Latest page')
    const transcript = screen.getByTestId('group-window-transcript')
    Object.defineProperty(transcript, 'scrollHeight', { configurable: true, get: () => transcript.textContent?.includes('Earlier page') ? 1400 : 1000 })
    transcript.scrollTop = 120
    await user.click(screen.getByRole('button', { name: '载入更早的对话' }))
    await screen.findByText('Earlier page')
    expect(transcript.scrollTop).toBe(520)
  })

  it('lists groups with the share of the roster that can hear', async () => {
    mocks.getGroupWindow.mockResolvedValue(windowState())
    render(<Hosted groupId="" />)

    const row = await screen.findByTestId('group-window-row')
    expect(within(row).getByText('渡口三人')).toBeInTheDocument()
    expect(within(row).getByText('2/3 人收得到')).toBeInTheDocument()
    expect(within(row).getByText('天亮前走。')).toBeInTheDocument()
  })

  it('marks a member who is not here as out of earshot instead of hiding them', async () => {
    const user = userEvent.setup()
    mocks.getGroupWindow.mockResolvedValue(windowState())
    render(<Hosted groupId="grp-1" />)

    await openSummary(user, /^成员/)
    expect(await screen.findByText('空闻')).toBeInTheDocument()
    expect(screen.getByText('听不到')).toBeInTheDocument()
  })

  it('shows what was actually said, attributed to its author', async () => {
    mocks.getGroupWindow.mockResolvedValue(windowState({
      exchanges: [
        line('x1', 'player', '天亮前走。'),
        line('x2', 'guard', '船家只肯等半个时辰。'),
      ],
    }))
    render(<Hosted groupId="grp-1" />)

    const transcript = await screen.findByTestId('group-window-transcript')
    expect(within(transcript).getByText('天亮前走。')).toBeInTheDocument()
    expect(within(transcript).getByText('船家只肯等半个时辰。')).toBeInTheDocument()
    expect(within(transcript).getByText(/周校尉/)).toBeInTheDocument()
    expect(within(transcript).getByText(/你/)).toBeInTheDocument()
  })

  it('commits a round and reads it back rather than showing an uncommitted line', async () => {
    const user = userEvent.setup()
    mocks.getGroupWindow
      .mockResolvedValueOnce(windowState())
      .mockResolvedValueOnce(windowState({
        exchanges: [
          line('x1', 'player', '走水路。'),
          line('x2', 'steward', '船钱我来出。'),
        ],
      }))
    mocks.sendGroupMessage.mockResolvedValue({ story_id: 'story-1', branch_id: 'main', group_id: 'grp-1', lines: [], world_time: '卯时', silenced: ['abbot'] })

    render(<Hosted groupId="grp-1" />)
    const box = await screen.findByPlaceholderText('在群里说…')
    await user.type(box, '走水路。')
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(mocks.sendGroupMessage).toHaveBeenCalledWith('story-1', expect.objectContaining({ groupId: 'grp-1', message: '走水路。' })))
    expect(await screen.findByText('船钱我来出。')).toBeInTheDocument()
    expect(screen.getByText('空闻 不在场，这一轮听不到。')).toBeInTheDocument()
  })

  it('keeps the draft when a round fails and says so', async () => {
    const user = userEvent.setup()
    mocks.getGroupWindow.mockResolvedValue(windowState())
    mocks.sendGroupMessage.mockRejectedValue(new Error('模型不可用'))

    render(<Hosted groupId="grp-1" />)
    const box = await screen.findByPlaceholderText('在群里说…')
    await user.type(box, '走水路。')
    await user.click(screen.getByRole('button', { name: '发送' }))

    expect(await screen.findByText(/模型不可用/)).toBeInTheDocument()
    expect(box).toHaveValue('走水路。')
  })

  it('hands a settled plan to the story instead of acting inside the group', async () => {
    const user = userEvent.setup()
    const onActOnPlan = vi.fn()
    mocks.getGroupWindow.mockResolvedValue(windowState())

    render(<Hosted groupId="grp-1" onActOnPlan={onActOnPlan} />)
    await openSummary(user, /^把商定的做法带回故事/)
    const plan = await screen.findByPlaceholderText('你打算怎么做…')
    await user.type(plan, '我去码头雇船。')
    await user.click(screen.getByRole('button', { name: '加入下轮行动' }))

    expect(onActOnPlan).toHaveBeenCalledWith('我去码头雇船。')
    expect(mocks.sendGroupMessage).not.toHaveBeenCalled()
  })

  it('offers only people the story already knows when creating a group', async () => {
    const user = userEvent.setup()
    mocks.getGroupWindow.mockResolvedValue(windowState({ groups: [] }))
    mocks.createGroup.mockResolvedValue(group)

    render(<Hosted groupId="" />)
    await user.click(await screen.findByRole('button', { name: /建一个群/ }))

    expect(screen.getAllByTestId('group-window-cast-row')).toHaveLength(3)
    await user.type(screen.getByPlaceholderText('群名'), '渡口三人')
    await user.click(screen.getAllByTestId('group-window-cast-row')[0])
    await user.click(screen.getByRole('button', { name: '建群' }))

    await waitFor(() => expect(mocks.createGroup).toHaveBeenCalledWith('story-1', expect.objectContaining({ name: '渡口三人', members: ['guard'] })))
  })

  it('does not carry a staged plan into another group', async () => {
    const user = userEvent.setup()
    mocks.getGroupWindow.mockResolvedValue(windowState({ groups: [group, { ...group, id: 'grp-2', name: '山路同伴' }] }))
    const onActOnPlan = vi.fn()
    const view = render(<Hosted groupId="grp-1" onActOnPlan={onActOnPlan} />)
    await openSummary(user, /^把商定的做法带回故事/)
    await user.type(screen.getByPlaceholderText('你打算怎么做…'), 'Only the ferry group agreed to hire a boat.')
    view.rerender(<Hosted groupId="grp-2" onActOnPlan={onActOnPlan} />)
    await waitFor(() => expect(screen.getByRole('heading', { name: '山路同伴' })).toBeInTheDocument())
    const plan = screen.queryByPlaceholderText('你打算怎么做…')
    if (!plan) await openSummary(user, /^把商定的做法带回故事/)
    expect(screen.getByPlaceholderText('你打算怎么做…')).toHaveValue('')
    expect(onActOnPlan).not.toHaveBeenCalled()
  })

  it('does not clear either conversation draft when an old send completes after switching', async () => {
    const user = userEvent.setup()
    mocks.getGroupWindow.mockResolvedValue(windowState({ groups: [group, { ...group, id: 'grp-2', name: '山路同伴' }] }))
    let finish!: (value: { silenced: string[] }) => void
    mocks.sendGroupMessage.mockReturnValue(new Promise(resolve => { finish = resolve }))
    const firstDraft = vi.fn(), nextDraft = vi.fn(), changed = vi.fn()
    const props = { storyId: 'story-1', branchId: 'main', active: true, onSelectGroup: vi.fn(), onClose: vi.fn(), onChanged: changed }
    const view = render(<GroupWindowPanel {...props} groupId="grp-1" draft="渡口的草稿" onDraftChange={firstDraft} />)
    await user.click(await screen.findByRole('button', { name: '发送' }))
    view.rerender(<GroupWindowPanel {...props} groupId="grp-2" draft="山路的新草稿" onDraftChange={nextDraft} />)
    await act(async () => { finish({ silenced: ['abbot'] }) })
    expect(screen.getByPlaceholderText('在群里说…')).toHaveValue('山路的新草稿')
    expect(firstDraft).not.toHaveBeenCalled()
    expect(nextDraft).not.toHaveBeenCalled()
    expect(changed).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: '发送' })).toBeEnabled()
  })
})

it('reports a remote radio group as connected rather than physically present', async () => {
  const radio: GroupWindowGroup = { ...group, channel: 'message', members: group.members.map(member => ({ ...member, present: false })) }
  mocks.getGroupWindow.mockResolvedValue(windowState({ groups: [radio], scene: { location: 'Bridge', present: [] } }))
  const user = userEvent.setup()
  render(<Hosted groupId="grp-1" />)
  const summary = await screen.findByRole('button', { name: /2 人已接通/ })
  expect(summary).not.toHaveTextContent('人在场')
  await user.click(summary)
  expect(screen.getByText('未接通')).toBeVisible()
  expect(screen.queryByText('听不到')).not.toBeInTheDocument()
})
