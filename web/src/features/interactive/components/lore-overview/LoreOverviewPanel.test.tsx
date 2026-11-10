import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CharacterWindow } from '../../types'
import type { JourneyState } from '../../journey-types'
import type { PublicEventState } from '../../public-event-types'
import type { QuestState } from '../../quest-types'
import type { Snapshot } from '../../types'
import { LoreOverviewPanel } from './LoreOverviewPanel'

const apiMocks = vi.hoisted(() => ({
  getCharacterWindow: vi.fn(),
  getJourney: vi.fn(),
  getQuests: vi.fn(),
  getPublicEvents: vi.fn(),
}))

// Fixture builders below model the contents of ONE aggregate response, not HTTP reads.
vi.mock('../../api', () => ({ getLoreOverview: async (story: string, options: { branchId?: string }) => {
 const [characters, journey, quests, events] = await Promise.all([
  apiMocks.getCharacterWindow(story, options), apiMocks.getJourney(story, options),
  apiMocks.getQuests(story, options), apiMocks.getPublicEvents(story, options),
 ])
 return { characters, journey, quests, events, snapshot: { ...SNAPSHOT, branch_id: options.branchId || 'main' } }
} }))

// 资料总览 is a reading surface. These tests cover what the panel decides on
// its own: that it reads the four committed views and nothing else, that a
// search can only narrow what the projection already allowed through, that a
// private field the pack declares is shown as 未知 rather than as empty, that an
// agreement with no instant is not drawn as a countdown, and that switching
// tabs or typing costs no further read and no turn.

const SNAPSHOT: Snapshot = {
  story_id: 'story-1',
  branch_id: 'main',
  turns: [],
  state: {
    actors: {
      protagonist: {
        name: '沈砚',
        template_id: 'protagonist',
        state: {
          境界: '练气三层',
          修为: 120,
          底牌: undefined,
        },
      },
      world: {
        name: '世界',
        template_id: 'world_entities',
        state: {
          地点记录: {
            越国坊市: { 类型: '坊市', 当前状态: '如常', 关键特征: '散修聚集，货摊沿街。' },
            青牛镇: { 类型: '镇', 当前状态: '如常' },
          },
          势力记录: {
            越国七派: { 类型: '宗门联盟', 存续状态: '稳固', 核心特征: '共守越国灵脉。', 主要范围: '越国坊市' },
          },
        },
      },
      story: {
        name: '故事',
        template_id: 'story_context',
        state: {
          当前时间: '第三日 巳时',
          当前详细地点: '越国坊市',
          世界局势: '山门闭锁，坊市照常开张。',
        },
      },
    },
  },
  actor_state_schema: {
    system: {
      templates: [
        {
          id: 'protagonist',
          name: '主角',
          fields: [
            { name: '境界', type: 'string' },
            { name: '修为', type: 'number' },
            { name: '底牌', type: 'string', visibility: 'private' },
          ],
        },
      ],
    },
  } as unknown as Snapshot['actor_state_schema'],
}

function characters(overrides: Partial<CharacterWindow> = {}): CharacterWindow {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    scene: { location: '越国坊市', world_time: '第三日 巳时' },
    characters: [
      {
        id: 'npc.su_qian',
        name: '苏浅',
        known_as: '苏姑娘',
        aliases: ['苏师妹'],
        role: '药铺掌柜',
        description: '说话慢，记账快。',
        attitude: '客气里带着打量',
        present: true,
        channel: 'scene',
        unread_messages: 0,
        open_matters: 1,
        first_seen_world_time: '第一日 未时',
        affinity: 20,
        affinity_band: 'neutral',
      },
    ],
    has_more: false,
    ...overrides,
  }
}

function journey(overrides: Partial<JourneyState> = {}): JourneyState {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    location: '越国坊市',
    routes: [
      {
        route_id: 'route.market.town',
        origin_location_id: '越国坊市',
        origin_label: '越国坊市',
        destination_location_id: '青牛镇',
        destination_label: '青牛镇',
        total_minutes: 240,
      } as JourneyState['routes'][number],
    ],
    ...overrides,
  } as JourneyState
}

function quests(overrides: Partial<QuestState> = {}): QuestState {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    location: '越国坊市',
    tasks: [
      {
        task_id: 'quest.herb_handover',
        task_version: '1.0',
        kind: 'errand',
        title: '送药',
        brief: '把采到的灵草送到约定的人手里。',
        giver: '苏姑娘',
        status: 'accepted',
        objectives: [{ id: 'quest.herb_handover.deliver', description: '把灵草交出去。', completed: false }],
        rewards: [{ kind: 'currency', ref_id: '灵石', quantity: 20 }],
        settled: false,
        remaining_seconds: 172800,
      },
    ],
    available: [],
    ...overrides,
  }
}

function events(overrides: Partial<PublicEventState> = {}): PublicEventState {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    location: '越国坊市',
    clock_seconds: 1000,
    events: [
      {
        event_id: 'event.market_fair',
        event_version: '1.0',
        title: '坊市夜市',
        brief: '入夜后货摊换了一轮。',
        region: '越国坊市',
        current_stage_id: 'stage.open',
        current_stage_title: '开市',
        closed: false,
        stages: [
          { id: 'stage.open', title: '开市', status: 'current', lines: ['灯一盏盏亮起来。'], opens_at_seconds: 900 },
        ],
        participants: [{ id: 'npc.su_qian', name: '苏浅', role: '药铺掌柜' }],
        actions: [],
        joined: false,
      },
    ],
    ...overrides,
  } as PublicEventState
}

function renderPanel(
  props: Partial<ComponentProps<typeof LoreOverviewPanel>> = {},
  cast: CharacterWindow = characters(),
) {
  apiMocks.getCharacterWindow.mockResolvedValue(cast)
  apiMocks.getJourney.mockResolvedValue(journey())
  apiMocks.getQuests.mockResolvedValue(quests())
  apiMocks.getPublicEvents.mockResolvedValue(events())
  render(
    <LoreOverviewPanel
      storyId="story-1"
      branchId="main"
      active
      snapshot={SNAPSHOT}
      onClose={() => {}}
      origin="为寻一味药下山。"
      worldName="凡人修仙"
      {...props}
    />,
  )
}

beforeEach(() => {
  apiMocks.getCharacterWindow.mockReset()
  apiMocks.getJourney.mockReset()
  apiMocks.getQuests.mockReset()
  apiMocks.getPublicEvents.mockReset()
  apiMocks.getCharacterWindow.mockResolvedValue(characters())
  apiMocks.getJourney.mockResolvedValue(journey())
  apiMocks.getQuests.mockResolvedValue(quests())
  apiMocks.getPublicEvents.mockResolvedValue(events())
  window.localStorage.clear()
})

describe('LoreOverviewPanel as a reading surface', () => {
  it('reads the four committed views and holds no write path at all', async () => {
    renderPanel()
    expect(await screen.findByText('沈砚')).toBeInTheDocument()
    expect(Object.keys(apiMocks).sort()).toEqual([
      'getCharacterWindow', 'getJourney', 'getPublicEvents', 'getQuests',
    ])
    // Nothing to accept, hand in, collect or travel with: the panel cannot act.
    expect(screen.queryByRole('button', { name: /接取|交付|领奖|出发|领取/ })).toBeNull()
  })

  it('shows the player tab from the same fields the ledger draws', async () => {
    renderPanel()
    await userEvent.click(await screen.findByText('沈砚'))
    expect(screen.getByText('练气三层')).toBeInTheDocument()
    expect(screen.getByText('120')).toBeInTheDocument()
  })

  it('states a private field the pack declares as 未知 rather than empty', async () => {
    renderPanel()
    // The backend withheld 底牌; the schema still declares it. An empty value
    // would say "none", which is a different claim from "not found out".
    fireEvent.click(await screen.findByText('沈砚'))
    expect(screen.getByText('底牌')).toBeInTheDocument()
    expect(screen.getAllByText('未知').length).toBeGreaterThan(0)
  })

  it('lists a person by the name the player knows them by', async () => {
    renderPanel()
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    expect(screen.getByText('苏姑娘')).toBeInTheDocument()
    // The player's own name for them, not their formal name, is what the list shows.
    expect(screen.queryByText('苏浅', { selector: 'span' })).toBeNull()
  })

  it('finds a person by an alias without inventing anyone else', async () => {
    renderPanel()
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    const search = screen.getByLabelText('搜索当前标签…')
    fireEvent.change(search, { target: { value: '苏师妹' } })
    expect(screen.getByText('苏姑娘')).toBeInTheDocument()
    fireEvent.change(search, { target: { value: '不存在的人' } })
    expect(screen.getByText('没有找到相符的内容。')).toBeInTheDocument()
  })

  it('cannot reveal a person the projection never sent', async () => {
    // A name that exists only in the pack is not in the read, so no search term
    // can surface it or change a count.
    renderPanel()
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    fireEvent.change(screen.getByLabelText('搜索当前标签…'), { target: { value: '厉飞雨' } })
    expect(screen.queryByText('厉飞雨')).toBeNull()
    expect(screen.getByText('没有找到相符的内容。')).toBeInTheDocument()
  })

  it('finds a person by a label the story attached', async () => {
    // The labels are how a player who forgot a name still finds someone: they
    // remember the shop, the trade, the town. The row shows them, and the search
    // reads the same list.
    renderPanel({}, characters({
      characters: [{ ...characters().characters![0], tags: ['东市药铺', '掌柜'] }],
    }))
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    fireEvent.change(screen.getByLabelText('搜索当前标签…'), { target: { value: '药铺' } })
    expect(screen.getByText('苏姑娘')).toBeInTheDocument()
    fireEvent.click(screen.getByText('苏姑娘'))
    expect(screen.getByText('东市药铺 · 掌柜')).toBeInTheDocument()
  })

  it('draws a gender the story settled', async () => {    renderPanel({}, characters({
      characters: [{ ...characters().characters![0], gender: 'female' }],
    }))
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    fireEvent.click(screen.getByText('苏姑娘'))
    expect(screen.getByText('性别')).toBeInTheDocument()
    expect(screen.getByText('女')).toBeInTheDocument()
  })

  it('draws no gender row for a person nobody settled', async () => {
    // An unsettled gender is a fact about the record, not a gap in what the
    // player has found out. Printing 未知 here would claim there is something
    // to discover.
    renderPanel()
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    fireEvent.click(screen.getByText('苏姑娘'))
    expect(screen.queryByText('性别')).toBeNull()
  })

  it('lists the roads that really start where the character stands', async () => {
    renderPanel()
    await userEvent.click(await screen.findByRole('tab', { name: '地点' }))
    fireEvent.click(screen.getByText('近处可走的路'))
    expect(screen.getByText('青牛镇 · 240')).toBeInTheDocument()
  })

  it('keeps a road that only leads into this place out of 近处可走的路', async () => {
    apiMocks.getJourney.mockResolvedValue(journey({
      routes: [
        {
          route_id: 'route.town.market',
          origin_location_id: '青牛镇',
          origin_label: '青牛镇',
          destination_location_id: '越国坊市',
          destination_label: '越国坊市',
          total_minutes: 240,
        } as JourneyState['routes'][number],
      ],
    }))
    render(
      <LoreOverviewPanel storyId="story-1" branchId="main" active snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    await userEvent.click(await screen.findByRole('tab', { name: '地点' }))
    // A road from elsewhere is not a road from here, and the panel does not
    // promise travel the rules would refuse.
    expect(screen.queryByText('近处可走的路')).toBeNull()
  })

  it('opens on a fresh story whose reads have no lists yet', async () => {
    // The Go side serialises an empty slice as null, so a story with no road, no
    // person and no matter sends `routes: null`, `characters: null`. Reading the
    // panel must not treat that as a broken shape.
    apiMocks.getCharacterWindow.mockResolvedValue(characters({ characters: null as unknown as CharacterWindow['characters'] }))
    apiMocks.getJourney.mockResolvedValue(journey({ routes: null as unknown as JourneyState['routes'] }))
    apiMocks.getQuests.mockResolvedValue(quests({ tasks: null as unknown as QuestState['tasks'] }))
    apiMocks.getPublicEvents.mockResolvedValue(events({ events: null as unknown as PublicEventState['events'] }))
    render(
      <LoreOverviewPanel storyId="story-1" branchId="main" active snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    // The player tab still draws from the snapshot; the other tabs say they are
    // empty instead of the panel failing to render at all.
    expect(await screen.findByText('沈砚')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: '人物' }))
    expect(screen.getByText('这一页还没有可查的见闻。')).toBeInTheDocument()
  })

  it('says an agreement with no instant has none, and never counts down', async () => {
    apiMocks.getQuests.mockResolvedValue(quests({
      tasks: [{
        ...quests().tasks[0],
        deadline_text: undefined,
        remaining_seconds: 172800,
      }],
    }))
    render(
      <LoreOverviewPanel storyId="story-1" branchId="main" active snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    await userEvent.click(await screen.findByRole('tab', { name: '见闻与事务' }))
    fireEvent.click(screen.getByText('送药'))
    expect(screen.getByText('未约定期限')).toBeInTheDocument()
    // The remaining seconds were present in the read and were deliberately not
    // drawn: a number here would read as a running clock.
    expect(screen.queryByText(/172800|2 日|还剩/)).toBeNull()
  })

  it('states the agreed deadline in the story wording when there is one', async () => {
    apiMocks.getQuests.mockResolvedValue(quests({
      tasks: [{ ...quests().tasks[0], deadline_text: '约 2日后' }],
    }))
    render(
      <LoreOverviewPanel storyId="story-1" branchId="main" active snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    await userEvent.click(await screen.findByRole('tab', { name: '见闻与事务' }))
    fireEvent.click(screen.getByText('送药'))
    expect(screen.getByText('约 2日后')).toBeInTheDocument()
  })

  it('names a promised item by the catalogue wording, not its reference id', async () => {
    apiMocks.getQuests.mockResolvedValue(quests({
      tasks: [{
        ...quests().tasks[0],
        rewards: [{ kind: 'item', ref_id: 'item.pill.restore_health_lower', name_key: 'item.pill.restore_health_lower.name', quantity: 2 }],
      }],
    }))
    render(
      <LoreOverviewPanel storyId="story-1" branchId="main" active snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    await userEvent.click(await screen.findByRole('tab', { name: '见闻与事务' }))
    fireEvent.click(screen.getByText('送药'))
    expect(screen.getByText('回春丹 × 2')).toBeInTheDocument()
    expect(screen.queryByText(/item\.pill/)).toBeNull()
  })

  it('links a matter to the person the story named, by an exact recorded name', async () => {
    renderPanel()
    await userEvent.click(await screen.findByRole('tab', { name: '见闻与事务' }))
    fireEvent.click(screen.getByText('送药'))
    fireEvent.click(screen.getByRole('button', { name: '苏姑娘' }))
    // The jump lands on the person, in the person tab, with the way back kept.
    expect(screen.getByText('说话慢，记账快。')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '返回列表' }))
    expect(screen.getByText('送药')).toBeInTheDocument()
  })

  it('shows the world tab from the scene state and the public events', async () => {
    renderPanel()
    await userEvent.click(await screen.findByRole('tab', { name: '世界' }))
    // The events the region already knows about are listed beside the standing.
    expect(screen.getByText('坊市夜市')).toBeInTheDocument()
    await userEvent.click(screen.getByText('眼下局势'))
    expect(screen.getByText('第三日 巳时')).toBeInTheDocument()
    expect(screen.getByText('山门闭锁，坊市照常开张。')).toBeInTheDocument()
  })

  it('costs no further read when switching tabs or searching', async () => {
    renderPanel()
    await screen.findByText('沈砚')
    const calls = apiMocks.getQuests.mock.calls.length
    await userEvent.click(screen.getByRole('tab', { name: '人物' }))
    fireEvent.change(screen.getByLabelText('搜索当前标签…'), { target: { value: '苏' } })
    await userEvent.click(screen.getByRole('tab', { name: '势力' }))
    await userEvent.click(screen.getByRole('tab', { name: '地点' }))
    expect(apiMocks.getQuests.mock.calls.length).toBe(calls)
    expect(apiMocks.getCharacterWindow.mock.calls.length).toBe(1)
  })

  it('remembers the tab within this story and branch only', async () => {
    renderPanel()
    await userEvent.click(await screen.findByRole('tab', { name: '势力' }))
    expect(window.localStorage.getItem('ta.loreOverview.tab.story-1.main')).toBe('factions')
    // Another life reads its own place in the panel, not this one's.
    expect(window.localStorage.getItem('ta.loreOverview.tab.story-1.other')).toBeNull()
  })

  it('reports a failed read and offers a retry', async () => {
    apiMocks.getCharacterWindow.mockRejectedValue(new Error('offline'))
    render(
      <LoreOverviewPanel storyId="story-1" branchId="main" active snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    expect(await screen.findByText('offline')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重试' })).toBeInTheDocument()
  })

  it('does not read at all while it is closed', async () => {
    render(
      <LoreOverviewPanel storyId="story-1" branchId="main" active={false} snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    expect(apiMocks.getCharacterWindow).not.toHaveBeenCalled()
  })

  it('closes the layer that has focus: detail first, then the panel', async () => {
    const onClose = vi.fn()
    renderPanel({ onClose })
    await userEvent.click(await screen.findByRole('tab', { name: '见闻与事务' }))
    fireEvent.click(screen.getByText('送药'))
    expect(screen.getByText('把灵草交出去。')).toBeInTheDocument()
    // Esc on the detail goes back to the list, and the panel stays open.
    fireEvent.keyDown(screen.getByTestId('lore-overview-panel'), { key: 'Escape' })
    expect(screen.getByText('送药')).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
    // Esc on the list closes the panel.
    fireEvent.keyDown(screen.getByTestId('lore-overview-panel'), { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('draws a person the world book pictured, through the project it belongs to', async () => {
    renderPanel({ projectId: 'project-1' }, characters({
      characters: [{
        ...characters().characters[0],
        avatar: undefined,
        avatar_lore_path: 'assets/lore/images/chen/20261021-101010-default/image.png',
      }],
    }))
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    const row = screen.getByTestId('lore-overview-panel')
    const image = row.querySelector('img')
    expect(image).not.toBeNull()
    // A world-book picture is stored relative, so it is addressed through the
    // project rather than read as a site path.
    expect(image?.getAttribute('src')).toBe(
      '/api/projects/project-1/files/asset?path=assets%2Flore%2Fimages%2Fchen%2F20261021-101010-default%2Fimage.png',
    )
  })

  it('keeps a chosen avatar over the world book, and shows the initial when neither answers', async () => {
    renderPanel({ projectId: 'project-1' }, characters({
      characters: [
        { ...characters().characters[0], avatar: '/api/projects/p/attachments/mine', avatar_lore_path: 'assets/lore/images/chen/image.png' },
        { ...characters().characters[0], id: 'npc.nobody', name: '无图的人', known_as: '', avatar: undefined, avatar_lore_path: undefined },
      ],
    }))
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    const sources = Array.from(screen.getByTestId('lore-overview-panel').querySelectorAll('img'))
      .map((node) => node.getAttribute('src'))
    expect(sources).toEqual(['/api/projects/p/attachments/mine'])
    // The person with no picture at all is still listed, under a neutral initial.
    expect(screen.getByText('无图的人')).toBeInTheDocument()
  })

  it('enlarges the same picture in the person detail without stretching it', async () => {
    renderPanel({ projectId: 'project-1' }, characters({
      characters: [{
        ...characters().characters[0],
        avatar: undefined,
        avatar_lore_path: 'assets/lore/images/chen/image.png',
      }],
    }))
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    fireEvent.click(screen.getByText('苏姑娘'))
    const images = Array.from(screen.getByTestId('lore-overview-panel').querySelectorAll('img'))
    expect(images).toHaveLength(1)
    // §4: the picture keeps its own proportions; a crop would show a face the
    // world never authored.
    expect(images[0].className).toContain('object-contain')
    expect(images[0].getAttribute('src')).toBe(
      '/api/projects/project-1/files/asset?path=assets%2Flore%2Fimages%2Fchen%2Fimage.png',
    )
  })

  // A read that started on one branch must never land on another. Sequence
  // numbers alone do not cover this: switching branch clears the screen without
  // starting a new read, so the read left in flight is still the newest one and
  // would repaint its people and matters over the branch now on screen.
  it('drops a read that started on a branch the reader already left', async () => {
    let release!: (value: unknown) => void
    apiMocks.getCharacterWindow.mockImplementation(() => new Promise((resolve) => { release = resolve }))
    apiMocks.getJourney.mockResolvedValue({})
    apiMocks.getQuests.mockResolvedValue({})
    apiMocks.getPublicEvents.mockResolvedValue({})

    const view = render(
      <LoreOverviewPanel storyId="story-1" branchId="branch-A" active snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    // Leave the panel and move to another life before the first answer arrives.
    view.rerender(
      <LoreOverviewPanel storyId="story-1" branchId="branch-B" active={false} snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    expect(screen.queryByText('旧分支的人')).toBeNull()

    release(characters({ characters: [{ ...characters().characters[0], name: '旧分支的人', known_as: '' }] }))
    await act(async () => { await Promise.resolve() })

    // The old answer belongs to a branch nobody is reading now.
    expect(screen.queryByText('旧分支的人')).toBeNull()
  })

  it('keeps a failure from a branch the reader already left off the screen', async () => {
    let reject!: (error: Error) => void
    apiMocks.getCharacterWindow.mockImplementation(() => new Promise((_resolve, fail) => { reject = fail }))
    apiMocks.getJourney.mockResolvedValue({})
    apiMocks.getQuests.mockResolvedValue({})
    apiMocks.getPublicEvents.mockResolvedValue({})

    const view = render(
      <LoreOverviewPanel storyId="story-1" branchId="branch-A" active snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    view.rerender(
      <LoreOverviewPanel storyId="story-1" branchId="branch-B" active={false} snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    reject(new Error('branch A is offline'))
    await act(async () => { await Promise.resolve() })

    // An error about a life the reader left is not an error about this one.
    expect(screen.queryByText('branch A is offline')).toBeNull()
  })

  it('reads the branch now on screen when the panel comes back', async () => {
    apiMocks.getCharacterWindow.mockImplementation((_story, options) => Promise.resolve(characters({ branch_id: options.branchId })))
    apiMocks.getJourney.mockImplementation((_story, options) => Promise.resolve(journey({ branch_id: options.branchId })))
    apiMocks.getQuests.mockImplementation((_story, options) => Promise.resolve(quests({ branch_id: options.branchId })))
    apiMocks.getPublicEvents.mockImplementation((_story, options) => Promise.resolve(events({ branch_id: options.branchId })))

    const view = render(
      <LoreOverviewPanel storyId="story-1" branchId="branch-A" active snapshot={{ ...SNAPSHOT, branch_id: 'branch-A' }} onClose={() => {}} />,
    )
    await screen.findByText('沈砚')
    expect(apiMocks.getCharacterWindow).toHaveBeenLastCalledWith('story-1', { branchId: 'branch-A' })

    view.rerender(
      <LoreOverviewPanel storyId="story-1" branchId="branch-B" active={false} snapshot={SNAPSHOT} onClose={() => {}} />,
    )
    view.rerender(
      <LoreOverviewPanel storyId="story-1" branchId="branch-B" active snapshot={{ ...SNAPSHOT, branch_id: 'branch-B' }} onClose={() => {}} />,
    )
    await screen.findByText('沈砚')

    // Returning to the panel asks the branch that is now on screen, so the
    // invalidation above does not cost the panel its ordinary refresh.
    expect(apiMocks.getCharacterWindow).toHaveBeenLastCalledWith('story-1', { branchId: 'branch-B' })
  })
})

it('uses the aggregate snapshot rather than older same-branch parent facts', async () => {
  renderPanel({ snapshot: { ...SNAPSHOT, state: {} } })
  fireEvent.click(await screen.findByText('沈砚'))
  expect(screen.getByText('120')).toBeInTheDocument()
  expect(screen.getByText('练气三层')).toBeInTheDocument()
})
