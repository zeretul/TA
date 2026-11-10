import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { QuestState, QuestTask } from '../../quest-types'
import { QuestWindowPanel } from './QuestWindowPanel'

const apiMocks = vi.hoisted(() => ({
  getQuests: vi.fn(),
}))

vi.mock('../../api', () => apiMocks)
const attentionMocks = vi.hoisted(() => ({ getPlayerAttention: vi.fn(), updatePlayerAttention: vi.fn() }))
vi.mock('../../player-attention-api', () => attentionMocks)

// 见闻与事务 is a view of committed facts, not an operator console. These tests
// cover what the panel decides on its own: that it reads without settling
// anything, that an open-ended matter says 未约定期限 instead of inventing a
// countdown, that an expiry the clock already wrote is reported rather than
// offered as still open, that following reorders and writes nothing to the
// world, and that "fill the intent" only fills the main input. Accepting,
// handing in and collecting are asserted in the Go suites and go through the
// main story's own formal path.

function task(overrides: Partial<QuestTask> = {}): QuestTask {
  return {
    task_id: 'quest.herb_handover',
    task_version: '1.0',
    kind: 'errand',
    title: '送药',
    brief: '把采到的灵草送到约定的人手里。',
    status: 'accepted',
    objectives: [
      { id: 'quest.herb_handover.pill', description: '取得一株灵草。', completed: true },
      { id: 'quest.herb_handover.deliver', description: '把灵草交出去。', completed: false },
    ],
    rewards: [{ kind: 'currency', ref_id: '灵石', quantity: 20 }],
    settled: false,
    ...overrides,
  }
}

function state(overrides: Partial<QuestState> = {}): QuestState {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    location: '越国坊市外围散修人家',
    tasks: [task()],
    available: [],
    ...overrides,
  }
}

function renderPanel(onFillIntent = vi.fn(), overrides: Partial<QuestState> = {}) {
  apiMocks.getQuests.mockResolvedValue(state(overrides))
  render(<QuestWindowPanel storyId="story-1" branchId="main" active onClose={() => {}} onFillIntent={onFillIntent} />)
  return onFillIntent
}

beforeEach(() => {
  apiMocks.getQuests.mockReset()
  attentionMocks.getPlayerAttention.mockReset().mockResolvedValue({ v: 1, head: 'h1', characters: [], quests: [] })
  attentionMocks.updatePlayerAttention.mockReset().mockImplementation(async (_story, input) => ({ v: 1, head: 'h2', characters: [], quests: input.following ? [input.id] : [] }))
  window.localStorage.clear()
})

describe('QuestWindowPanel as a view', () => {
  it('imports successful legacy preferences and retains rejected IDs for retry', async () => {
    window.localStorage.setItem('ta.questWindow.followed.story-1.main', '["unknown","quest.herb_handover"]')
    attentionMocks.updatePlayerAttention.mockRejectedValueOnce(new Error('尚未知晓'))
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    renderPanel()
    const button = await screen.findByRole('button', { name: '导入本地关注' })
    await waitFor(() => expect(button).toBeEnabled())
    fireEvent.click(button)
    await waitFor(() => expect(window.localStorage.getItem('ta.questWindow.followed.story-1.main')).toBe('["unknown"]'))
    expect(attentionMocks.updatePlayerAttention).toHaveBeenCalledTimes(2)
    expect(await screen.findByRole('button', { name: '取消关注' })).toBeInTheDocument()
    warn.mockRestore()
  })
  it('reads the matters and settles nothing', async () => {
    renderPanel()
    expect(await screen.findByText('送药')).toBeInTheDocument()
    // The only call is the read. The panel holds no write path at all.
    expect(apiMocks.getQuests).toHaveBeenCalledTimes(1)
    expect(Object.keys(apiMocks)).toEqual(['getQuests'])
    expect(screen.queryByRole('button', { name: '接取' })).toBeNull()
    expect(screen.queryByRole('button', { name: '交付' })).toBeNull()
    expect(screen.queryByRole('button', { name: '领奖' })).toBeNull()
  })

  it('says 未约定期限 for a matter with no agreed limit', async () => {
    renderPanel()
    expect(await screen.findByText('未约定期限')).toBeInTheDocument()
    expect(screen.queryByText(/已到期/)).toBeNull()
  })

  it('reports an expiry the clock already wrote, and does not offer to collect it', async () => {
    renderPanel(vi.fn(), {
      tasks: [task({ status: 'expired', expired: true, deadline_seconds: 1000, deadline_text: '已到期' })],
    })
    // An expired agreement is history, not a matter in hand.
    expect(await screen.findByText('已了结（1）')).toBeInTheDocument()
    expect(screen.queryByText('在手的事')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '已了结（1）' }))
    expect(screen.getByText('约定已到期')).toBeInTheDocument()
    expect(screen.getByText('已到期', { selector: 'span' })).toBeInTheDocument()
  })

  it('shows the agreed deadline as the rules state it', async () => {
    renderPanel(vi.fn(), {
      tasks: [task({ deadline_seconds: 172800, deadline_text: '约 2日后' })],
    })
    expect(await screen.findByText('约定期限：约 2日后')).toBeInTheDocument()
  })

  it('does not read a finished matter as a missed deadline', async () => {
    // The rules keep the agreed instant on the record after settlement, and the
    // clock moves past it. Reporting that as 已到期 would tell the player they
    // failed a matter they finished; the limit stopped binding when it was done.
    renderPanel(vi.fn(), {
      tasks: [task({
        status: 'completed', settled: true,
        deadline_seconds: 172800, deadline_text: '已到期', remaining_seconds: -100,
      })],
    })
    fireEvent.click(await screen.findByRole('button', { name: '已了结（1）' }))
    fireEvent.click(screen.getByRole('button', { name: /展开「送药」/ }))
    expect(screen.getByText('报酬已领。')).toBeInTheDocument()
    expect(screen.queryByText('约定已到期')).toBeNull()
    expect(screen.queryByText('约定期限：已到期')).toBeNull()
  })

  it('puts a followed matter first without writing anything to the world', async () => {
    renderPanel(vi.fn(), {
      tasks: [
        task({ task_id: 'quest.a', title: '甲事', deadline_seconds: 100 }),
        task({ task_id: 'quest.b', title: '乙事' }),
      ],
    })
    await screen.findByText('甲事')
    const titles = () => screen.getAllByText(/^(甲事|乙事)$/).map((node) => node.textContent)
    expect(titles()).toEqual(['甲事', '乙事'])

    // Following the second matter floats it, and the only call is still the read.
    fireEvent.click(screen.getAllByRole('button', { name: '关注' })[1])
    expect(titles()).toEqual(['乙事', '甲事'])
    expect(apiMocks.getQuests).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(attentionMocks.updatePlayerAttention).toHaveBeenCalledWith('story-1', { branch: 'main', expected_head: 'h1', kind: 'quest', id: 'quest.b', following: true }))
    expect(window.localStorage.getItem('ta.questWindow.followed.story-1.main')).toBeNull()
  })

  it('keeps followed matters separate for another branch', async () => {
    window.localStorage.setItem('ta.questWindow.followed.story-1.main', '["quest.a"]')
    renderPanel()
    await screen.findByText('送药')
    // The other branch's attention is not this life's attention.
    expect(screen.getAllByRole('button', { name: '关注' }).length).toBeGreaterThan(0)
  })

  it('fills the main input with an intent instead of acting', async () => {
    const onFillIntent = renderPanel()
    await screen.findByText('送药')
    fireEvent.click(screen.getByRole('button', { name: /展开「送药」/ }))
    fireEvent.click(screen.getByRole('button', { name: '写下这个打算' }))
    expect(onFillIntent).toHaveBeenCalledWith('继续办「送药」：')
    // Filling the box is not sending: the player still edits and sends it.
    expect(apiMocks.getQuests).toHaveBeenCalledTimes(1)
  })

  it('asks for the payment once, and never a second time after settlement', async () => {
    const onFillIntent = renderPanel(vi.fn(), {
      tasks: [task({ status: 'completed', settled: false })],
    })
    // A finished matter is history, so it lives behind its own section.
    fireEvent.click(await screen.findByRole('button', { name: '已了结（1）' }))
    fireEvent.click(screen.getByRole('button', { name: /展开「送药」/ }))
    fireEvent.click(screen.getByRole('button', { name: '写下这个打算' }))
    expect(onFillIntent).toHaveBeenCalledWith('去交付「送药」并领回约定报酬。')
  })

  it('does not promise a second payment for a settled matter', async () => {
    const onFillIntent = renderPanel(vi.fn(), {
      tasks: [task({ status: 'completed', settled: true })],
    })
    fireEvent.click(await screen.findByRole('button', { name: '已了结（1）' }))
    fireEvent.click(screen.getByRole('button', { name: /展开「送药」/ }))
    expect(screen.getByText('报酬已领。')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '写下这个打算' }))
    expect(onFillIntent).toHaveBeenCalledWith('「送药」的报酬已经领过，不再重复。')
  })

  it('keeps unknown opportunities collapsed until asked', async () => {
    renderPanel(vi.fn(), {
      tasks: [],
      available: [{
        task_id: 'quest.su_qian_materials',
        task_version: '1.0',
        kind: 'personal',
        title: '苏浅的委托',
        brief: '替苏浅寻一份药材。',
        giver: '苏浅',
        deadline_seconds: 172800,
      }],
    })
    const toggle = await screen.findByRole('button', { name: '已知机会（1）' })
    expect(screen.queryByText('苏浅的委托')).toBeNull()
    fireEvent.click(toggle)
    expect(screen.getByText('苏浅的委托')).toBeInTheDocument()
    // The offer states its terms before acceptance, and the limit starts then.
    expect(screen.getByText('应下后限 2 日')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '接取' })).toBeNull()
  })

  it('states an offer interval in the units it is really measured in', async () => {
    renderPanel(vi.fn(), {
      tasks: [],
      available: [
        { task_id: 'quest.sub_minute', task_version: '1.0', kind: 'personal', title: '半分钟的约定', brief: '。', deadline_seconds: 30 },
        { task_id: 'quest.minutes', task_version: '1.0', kind: 'personal', title: '分钟的约定', brief: '。', deadline_seconds: 90 },
        { task_id: 'quest.hour', task_version: '1.0', kind: 'personal', title: '小时的约定', brief: '。', deadline_seconds: 3600 },
        { task_id: 'quest.hour_minute', task_version: '1.0', kind: 'personal', title: '零头的约定', brief: '。', deadline_seconds: 5400 },
        { task_id: 'quest.day_hour', task_version: '1.0', kind: 'personal', title: '跨日的约定', brief: '。', deadline_seconds: 90000 },
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: '已知机会（5）' }))
    // The remainder is stated, not dropped: 5400s is one hour and thirty minutes.
    expect(screen.getByText('应下后限 1 分钟')).toBeInTheDocument()
    expect(screen.getByText('应下后限 2 分钟')).toBeInTheDocument()
    expect(screen.getByText('应下后限 1 小时')).toBeInTheDocument()
    expect(screen.getByText('应下后限 1 小时 30 分钟')).toBeInTheDocument()
    expect(screen.getByText('应下后限 1 日 1 小时')).toBeInTheDocument()
    // A 时辰 is 7200s and a 刻 is 900s, so neither unit may name these spans.
    expect(screen.queryByText(/应下后限.*(?:时辰|刻)/)).toBeNull()
  })

  it('keeps an offer the world raised in hand, not in history', async () => {
    // The world raised this matter and the character knows about it, but the
    // player has not taken it on. Filing it under 已了结 said the opposite and
    // hid the one action still open on it.
    renderPanel(vi.fn(), { tasks: [task({ status: 'offered' })] })
    expect(await screen.findByText('在手的事')).toBeInTheDocument()
    expect(screen.getByText('尚未应下')).toBeInTheDocument()
    expect(screen.queryByText('已了结（1）')).toBeNull()
  })

  it('fills an accept intent for an offer and never accepts it itself', async () => {
    const onFillIntent = renderPanel(vi.fn(), { tasks: [task({ status: 'offered' })] })
    await screen.findByText('送药')
    fireEvent.click(screen.getByRole('button', { name: /展开「送药」/ }))
    fireEvent.click(screen.getByRole('button', { name: '写下这个打算' }))
    expect(onFillIntent).toHaveBeenCalledWith('我想应下「送药」这桩事：')
    // Filling the box is still not taking it on: only the read happened.
    expect(apiMocks.getQuests).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('button', { name: '接取' })).toBeNull()
  })

  it('reports a failed read and offers a retry', async () => {
    apiMocks.getQuests.mockRejectedValue(new Error('offline'))
    render(<QuestWindowPanel storyId="story-1" branchId="main" active onClose={() => {}} onFillIntent={() => {}} />)
    expect(await screen.findByText('offline')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重试' })).toBeInTheDocument()
  })
})
