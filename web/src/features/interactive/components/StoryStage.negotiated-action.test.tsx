import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VirtuosoMockContext } from 'react-virtuoso'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  CharacterExchange,
  CharacterMatter,
  CharacterWindow,
  GroupWindow,
  Snapshot,
  StorySummary,
} from '../types'
import { StoryStage as ProjectStoryStage } from './StoryStage'
import {
  controllableInteractiveStream,
  getStageInput,
  resetStoryStageTestHarness,
  story,
} from './story-stage/story-stage-test-harness'

vi.mock('./combat-window/AdventureCombatPanel', () => ({ AdventureCombatPanel: () => <div data-testid="native-adventure-panel" /> }))

const testMocks = vi.hoisted(() => ({
  generateInteractiveImageMock: vi.fn(),
  getActiveInteractiveChatMock: vi.fn(),
  getCharacterWindowMock: vi.fn(),
  getGroupWindowMock: vi.fn(),
  getRealmMock: vi.fn(),
  recoverInteractiveAgentRuntimeMock: vi.fn(),
  sendInteractiveMessageMock: vi.fn(),
  streamActiveInteractiveChatMock: vi.fn(),
  submitInteractiveAgentCommandMock: vi.fn(),
  updateInteractiveTurnNarrativeMock: vi.fn(),
  useSkillCommandsMock: vi.fn(),
}))

const {
  getCharacterWindowMock,
  getGroupWindowMock,
  recoverInteractiveAgentRuntimeMock,
  sendInteractiveMessageMock,
  submitInteractiveAgentCommandMock,
} = testMocks

vi.mock('@/features/settings/api', () => ({
  fetchProjectSettings: vi.fn().mockResolvedValue({ effective: {} }),
  fetchSettings: vi.fn().mockResolvedValue({ effective: {} }),
  refreshProjectSettings: vi.fn().mockResolvedValue({ effective: {} }),
}))

vi.mock('@/features/agent-approval/AgentApprovalProvider', () => ({
  useAgentApprovalMode: () => ({ mode: 'write', initialized: true, saving: false, setMode: vi.fn().mockResolvedValue(true) }),
}))

vi.mock('@/features/conversation-config/use-conversation-config', () => ({
  useConversationConfig: () => conversationConfigController(),
}))

vi.mock('@/hooks/useSkillCommands', () => ({
  useSkillCommands: (...args: unknown[]) => testMocks.useSkillCommandsMock(...args),
}))

vi.mock('../api', () => ({
  analyzeInteractiveContext: vi.fn(),
  adoptStoryAssistantDirection: vi.fn(),
  askStoryAssistant: vi.fn(),
  changeGroupMembers: vi.fn(),
  clearCharacterPicture: vi.fn(),
  clearPlacePicture: vi.fn(),
  compactInteractiveContext: vi.fn(),
  createGroup: vi.fn(),
  generateInteractiveImage: testMocks.generateInteractiveImageMock,
  getActiveInteractiveChat: testMocks.getActiveInteractiveChatMock,
  getCharacterPicture: vi.fn().mockResolvedValue({ overridden: false }),
  getCharacterWindow: testMocks.getCharacterWindowMock,
  getCombatWindow: vi.fn().mockResolvedValue({ story_id: 'story-1', branch_id: 'main', active: false }),
  getGroupWindow: testMocks.getGroupWindowMock,
  getRealm: testMocks.getRealmMock,
  getInteractiveHistoryPage: vi.fn().mockResolvedValue({ turns: [], has_more: false }),
  getPlacePicture: vi.fn().mockResolvedValue({ overridden: false }),
  importCharacterPicture: vi.fn(),
  importPlacePicture: vi.fn(),
  narrateCombatRound: vi.fn(),
  playCombatRound: vi.fn(),
  recoverInteractiveAgentRuntime: testMocks.recoverInteractiveAgentRuntimeMock,
  removeInteractiveContextCompaction: vi.fn(),
  resolveInteractiveAsk: vi.fn(),
  sendCharacterMessage: vi.fn(),
  sendGroupMessage: vi.fn(),
  sendInteractiveMessage: testMocks.sendInteractiveMessageMock,
  setCharacterMessageStatus: vi.fn().mockResolvedValue(undefined),
  streamActiveInteractiveChat: testMocks.streamActiveInteractiveChatMock,
  submitInteractiveAgentCommand: testMocks.submitInteractiveAgentCommandMock,
  switchInteractiveTurnVersion: vi.fn(),
  updateInteractiveTurnNarrative: testMocks.updateInteractiveTurnNarrativeMock,
}))

function conversationConfigController() {
  return {
    snapshot: { agent_kind: 'interactive_story', profile_id: 'default', thinking_level: 'off', approval_mode: 'write', revision: 1 },
    initialized: true, loading: false, saving: false, error: null,
    patch: vi.fn().mockResolvedValue(true), reload: vi.fn(),
  }
}

/**
 * The arrangement under test: 陈婆婆 and the player settled a pickup, and the
 * player's last line is what the plan field starts from.
 *
 * The settlement is a committed `action` matter with no letter, which is the
 * only shape the window treats as an agreement. Nothing here is inferred from
 * the wording of the conversation.
 */
const AGREEMENT = '天亮前我来渡口接你。'
const MAIN_CHOICE = '去码头看看船'

function caopoExchange(): CharacterExchange[] {
  return [
    {
      id: 'ex-1', story_id: 'story-1', branch_id: 'main', character_id: 'caopo',
      speaker: 'character', channel: 'private', text: '那你天亮前来，我把篮子给你。',
      world_time: '卯时',
    },
    {
      id: 'ex-2', story_id: 'story-1', branch_id: 'main', character_id: 'caopo',
      speaker: 'player', channel: 'private', text: AGREEMENT,
      world_time: '卯时',
    },
  ]
}

function agreedMatter(): CharacterMatter {
  return {
    id: 'matter-1',
    character_id: 'caopo',
    kind: 'action',
    status: 'open',
    statement: '天亮前到渡口接陈婆婆',
    world_time: '卯时',
  }
}

function unagreedMatter(): CharacterMatter {
  return {
    id: 'matter-2',
    character_id: 'caopo',
    kind: 'understanding',
    status: 'open',
    statement: '陈婆婆似乎想让我帮忙打听一下渡钱',
    world_time: '卯时',
  }
}

function characterWindow(overrides: Partial<CharacterWindow> = {}): CharacterWindow {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    scene: { location: '青石渡口', world_time: '卯时', present: ['caopo'] },
    characters: [{
      id: 'caopo', name: '陈婆婆', known_as: '陈婆婆',
      present: true, channel: 'scene', unread_messages: 0, open_matters: 0,
      affinity: 0, affinity_band: 'neutral',
    }],
    messages: [],
    pending: [],
    has_more: false,
    ...overrides,
  }
}

function groupWindow(overrides: Partial<GroupWindow> = {}): GroupWindow {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    scene: { title: '渡口', location: '青石渡口', world_time: '卯时', present: ['hetai'] },
    groups: [{
      id: 'grp-1',
      name: '渡口三人',
      type: 'self',
      channel: 'scene',
      contact: 'same scene',
      reachable_members: 1,
      version: 1,
      members: [{ id: 'hetai', name: '何泰', role: 'boatman', present: true, reachable: true }],
    }],
    cast: [{ id: 'hetai', name: '何泰', known_as: '何泰', present: true }],
    exchanges: [],
    matters: [],
    has_more: false,
    ...overrides,
  } as GroupWindow
}

function snapshotWithChoices(): Snapshot {
  const turn = {
    id: 'turn-1',
    parent_id: null,
    branch_id: 'main',
    ts: '2026-06-28T00:00:00Z',
    user: '继续前进',
    narrative: '渡口的风把船绳吹得响。',
    turn_result: { state_updates: [], choices: [MAIN_CHOICE, '回客栈收拾行李'] },
  }
  return {
    story_id: 'story-1',
    branch_id: 'main',
    turns: [turn],
    current_turn: turn,
    state: {},
  }
}

/** A stage whose branch can be switched, which the cross-branch case needs. */
function StageHarness({
  initialSnapshot,
  branchId = 'main',
  onDone,
  gameplay,
}: {
  initialSnapshot: Snapshot
  gameplay?: NonNullable<NonNullable<StorySummary['pack_binding']>['gameplay']>
  branchId?: string
  onDone?: (options?: { silent?: boolean }) => Promise<Snapshot | void>
}) {
  const currentStory = story()
  if (gameplay) currentStory.pack_binding = { pack_id: 'world', scope: 'pack:world', gameplay }
  return (
    <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
      <ProjectStoryStage
        projectId="project-story"
        workspace="/tmp/book"
        stories={[currentStory]}
        story={currentStory}
        tellers={[]}
        storyId="story-1"
        branchId={branchId}
        snapshot={{ ...initialSnapshot, branch_id: branchId }}
        onDone={onDone || (() => Promise.resolve(undefined))}
      />
    </VirtuosoMockContext.Provider>
  )
}

/**
 * Opens the chat shell and steps into one conversation.
 *
 * The row is found by its id rather than by name: an open conversation's panel
 * also carries the name in its own controls, so a name query would match twice.
 */
async function openConversation(
  user: ReturnType<typeof userEvent.setup>,
  kind: 'person' | 'group',
  id: string,
) {
  const toggle = screen.queryByRole('button', { name: '打开聊天窗口' })
  if (toggle) await user.click(toggle)
  const panel = await screen.findByTestId('chat-window-panel')
  const row = await waitFor(() => {
    const found = panel.querySelector<HTMLElement>(
      `[data-testid="chat-window-row"][data-chat-kind="${kind}"][data-chat-id="${id}"]`,
    )
    if (!found) throw new Error(`chat row ${kind}/${id} missing`)
    return found
  })
  await user.click(row)
  return panel
}

/** Expands the group plan block, which is folded to one line by default. */
async function openGroupPlan(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: /把商定的做法带回故事/ }))
  return screen.findByPlaceholderText('你打算怎么做…')
}

/** Picks one of the stage's own suggested actions into the draft. */
async function chooseMainLine(user: ReturnType<typeof userEvent.setup>, choice: string) {
  await user.click(screen.getByRole('button', { name: '获取行动选择' }))
  await user.click(await screen.findByRole('button', { name: choice }))
}

/** The one button that carries an arrangement into the next turn. */
async function actOnPlan(user: ReturnType<typeof userEvent.setup>) {
  const button = await screen.findByRole('button', { name: '加入下轮行动' })
  expect(button).toBeEnabled()
  await user.click(button)
  return screen.findByText('下轮行动')
}

beforeEach(() => {
  resetStoryStageTestHarness(testMocks)
  recoverInteractiveAgentRuntimeMock.mockReset()

  getCharacterWindowMock.mockReset()
  getCharacterWindowMock.mockImplementation(async (_storyId: string, options: { characterId?: string } = {}) => {
    if (options.characterId === 'caopo') {
      return characterWindow({
        character_id: 'caopo',
        exchanges: caopoExchange(),
        matters: [agreedMatter()],
      })
    }
    return characterWindow()
  })

  getGroupWindowMock.mockReset()
  getGroupWindowMock.mockImplementation(async (_storyId: string, options: { groupId?: string } = {}) => {
    if (options.groupId === 'grp-1') {
      return groupWindow({ group_id: 'grp-1' })
    }
    return groupWindow()
  })
})

describe('StoryStage arranged action entering the next turn', () => {
  it('stages a realm intention without sending, keeps typed prose, and clears it on branch change', async () => {
    const user = userEvent.setup()
    testMocks.getRealmMock.mockResolvedValue({ story_id: 'story-1', branch_id: 'main', clock_seconds: 0, instances: [], available: [{
      realm_id: 'realm.mountain_herb_cave', realm_version: '1.0', name: '山溪旧药窟', name_key: 'game.realm.explore.herbCave.name', theme: '', entry_area_id: 'entry',
    }] })
    const stage = render(<StageHarness initialSnapshot={snapshotWithChoices()} />)
    await user.type(getStageInput(), '我先整理行李。')
    await user.click(screen.getByRole('button', { name: '打开探索信息' }))
    await user.click(await screen.findByRole('button', { name: '将进入此地加入打算' }))
    expect(screen.getByText('我想进入山溪旧药窟探索。')).toBeInTheDocument()
    expect(getStageInput()).toHaveTextContent('我先整理行李。')
    expect(screen.queryByTestId('realm-panel')).not.toBeInTheDocument()
    expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
    expect(submitInteractiveAgentCommandMock).not.toHaveBeenCalled()
    stage.rerender(<StageHarness initialSnapshot={snapshotWithChoices()} branchId="fork" />)
    await waitFor(() => expect(screen.queryByText('我想进入山溪旧药窟探索。')).not.toBeInTheDocument())
  })

  it('keeps cooperation as a removable draft beside existing prose, never a direct work request', async () => {
    const user = userEvent.setup()
    testMocks.getRealmMock.mockResolvedValue({ story_id: 'story-1', branch_id: 'main', instances: [], available: [], cooperations: [{
      project_id: 'hidden-project', project_version: '1.0', realm_id: 'hidden-realm', instance_id: 'instance-a',
      name_key: 'game.cooperation.projects.herbcave.name', description_key: 'game.cooperation.projects.herbcave.description',
      player_role_key: 'game.cooperation.projects.herbcave.playerRole', partner_role_key: 'game.cooperation.projects.herbcave.partnerRole',
      partner_id: 'known-partner', partner_name: '已认识的同道', location_id: 'market', location_name: '坊市外围',
      phase: 'none', duration_seconds: 600, partner_energy_cost: 0, player_costs: [],
      can_ask: true, can_work_if_agreed: true, can_work: false, can_report: false, report_facts: [],
    }] })
    const stage = render(<StageHarness initialSnapshot={snapshotWithChoices()} />)
    await user.type(getStageInput(), '先把我的其他发现收好。')
    await user.click(screen.getByRole('button', { name: '打开探索信息' }))
    await user.click(await screen.findByRole('button', { name: '先问是否愿意，不透露线索' }))
    expect(screen.getByText(/现在只问意愿，不透露我发现的线索，也不开始工作/)).toBeInTheDocument()
    expect(getStageInput()).toHaveTextContent('先把我的其他发现收好。')
    expect(screen.queryByTestId('realm-panel')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '发送' })).toBeEnabled()
    expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
    expect(submitInteractiveAgentCommandMock).not.toHaveBeenCalled()
    expect(document.body.textContent).not.toMatch(/hidden-project|hidden-realm/)
    stage.rerender(<StageHarness initialSnapshot={snapshotWithChoices()} branchId="fork" />)
    await waitFor(() => expect(screen.queryByText(/现在只问意愿/)).not.toBeInTheDocument())
  })

  it('puts a settled private-chat arrangement in the draft without starting a turn', async () => {
    const user = userEvent.setup()
    render(<StageHarness initialSnapshot={snapshotWithChoices()} />)

    await openConversation(user, 'person', 'caopo')
    await actOnPlan(user)

    const staged = await screen.findByText('下轮行动')
    expect(staged).toBeInTheDocument()
    expect(screen.getByText(AGREEMENT)).toBeInTheDocument()
    // Adding an arrangement is not a turn: no request, no clock, no settlement.
    expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
    expect(submitInteractiveAgentCommandMock).not.toHaveBeenCalled()
    // The stage draft itself is untouched — the arrangement waits beside it.
    expect(getStageInput()).toHaveTextContent('')
  })

  it('can send a staged intention alone, and removing it disables an empty send again', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValueOnce(stream.readable)
    render(<StageHarness initialSnapshot={snapshotWithChoices()} />)
    await openConversation(user, 'person', 'caopo')
    await actOnPlan(user)
    expect(getStageInput()).toHaveTextContent('')
    expect(screen.getByRole('button', { name: '发送' })).toBeEnabled()
    await user.click(screen.getByRole('button', { name: '移出下轮行动' }))
    expect(screen.getByRole('button', { name: '发送' })).toBeDisabled()
    await openConversation(user, 'person', 'caopo')
    await actOnPlan(user)
    await user.click(screen.getByRole('button', { name: '发送' }))
    await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
    expect(sendInteractiveMessageMock.mock.calls[0][0].message).toBe(AGREEMENT)
    await act(async () => { stream.close() })
  })

  it('lets the player rewrite or drop the arrangement before sending', async () => {
    const user = userEvent.setup()
    render(<StageHarness initialSnapshot={snapshotWithChoices()} />)

    await openConversation(user, 'person', 'caopo')
    await actOnPlan(user)
    await screen.findByText('下轮行动')

    // "改到输入框" moves the words into the player's own box and removes the
    // entry, so the same plan cannot be sent twice.
    await user.click(screen.getByRole('button', { name: '改到输入框' }))
    await waitFor(() => expect(screen.queryByText('下轮行动')).not.toBeInTheDocument())
    expect(getStageInput()).toHaveTextContent(AGREEMENT)
    expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
  })

  it('drops a staged arrangement without touching the draft', async () => {
    const user = userEvent.setup()
    render(<StageHarness initialSnapshot={snapshotWithChoices()} />)

    await openConversation(user, 'person', 'caopo')
    await actOnPlan(user)
    await screen.findByText('下轮行动')

    await user.click(screen.getByRole('button', { name: '移出下轮行动' }))
    await waitFor(() => expect(screen.queryByText('下轮行动')).not.toBeInTheDocument())
    expect(getStageInput()).toHaveTextContent('')
    expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
  })

  it('keeps the arrangement while the player goes back to chatting', async () => {
    const user = userEvent.setup()
    render(<StageHarness initialSnapshot={snapshotWithChoices()} />)

    const first = await openConversation(user, 'person', 'caopo')
    await actOnPlan(user)
    await screen.findByText('下轮行动')

    // Closing the conversation is not a send: the arrangement is still waiting.
    await user.click(within(first).getByRole('button', { name: '关闭聊天窗口' }))
    await waitFor(() => expect(screen.queryByTestId('chat-window-panel')).not.toBeInTheDocument())
    expect(screen.getByText('下轮行动')).toBeInTheDocument()
    expect(screen.getByText(AGREEMENT)).toBeInTheDocument()

    // Going back in to keep talking leaves it untouched as well.
    await openConversation(user, 'person', 'caopo')
    expect(await screen.findByRole('button', { name: '加入下轮行动' })).toBeInTheDocument()
    expect(screen.getByText('下轮行动')).toBeInTheDocument()
    expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
  })

  it('sends the arrangement and the main-line choice as exactly one turn', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)
    const onDone = vi.fn().mockResolvedValue(undefined)
    render(<StageHarness initialSnapshot={snapshotWithChoices()} onDone={onDone} />)

    await openConversation(user, 'person', 'caopo')
    await actOnPlan(user)
    await screen.findByText('下轮行动')

    await chooseMainLine(user, MAIN_CHOICE)
    expect(getStageInput()).toHaveTextContent(MAIN_CHOICE)

    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
    expect(sendInteractiveMessageMock).toHaveBeenCalledWith(expect.objectContaining({
      mode: 'story',
      story_id: 'story-1',
      branch: 'main',
      message: `${AGREEMENT}\n${MAIN_CHOICE}`,
    }))
    // One request is one turn: nothing was started alongside it.
    expect(submitInteractiveAgentCommandMock).not.toHaveBeenCalled()
    expect(onDone).not.toHaveBeenCalled()

    await act(async () => { stream.close() })
    // The arrangement is spent once the turn is accepted.
    await waitFor(() => expect(screen.queryByText('下轮行动')).not.toBeInTheDocument())
  })

  it('keeps the arrangement when the send is refused, and a retry does not duplicate it', async () => {
    const user = userEvent.setup()
    sendInteractiveMessageMock.mockRejectedValueOnce(new Error('连接中断'))
    render(<StageHarness initialSnapshot={snapshotWithChoices()} />)

    await openConversation(user, 'person', 'caopo')
    await actOnPlan(user)
    await screen.findByText('下轮行动')

    await chooseMainLine(user, MAIN_CHOICE)
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
    expect(sendInteractiveMessageMock.mock.calls[0][0].message).toBe(`${AGREEMENT}\n${MAIN_CHOICE}`)
    // The refusal leaves the words with the player to retry.
    expect(await screen.findByText('下轮行动')).toBeInTheDocument()
    expect(screen.getByText(AGREEMENT)).toBeInTheDocument()

    const stream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValueOnce(stream.readable)
    await chooseMainLine(user, MAIN_CHOICE)
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(2))
    expect(sendInteractiveMessageMock.mock.calls[1][0].message).toBe(`${AGREEMENT}\n${MAIN_CHOICE}`)
    await act(async () => { stream.close() })
  })

  it('carries a group plan the same way', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)
    render(<StageHarness initialSnapshot={snapshotWithChoices()} />)

    await openConversation(user, 'group', 'grp-1')
    const planField = await openGroupPlan(user)
    await user.type(planField, '让何泰把船撑到下游')
    await user.click(screen.getByRole('button', { name: '加入下轮行动' }))

    await screen.findByText('下轮行动')
    expect(screen.getByText('让何泰把船撑到下游')).toBeInTheDocument()
    expect(sendInteractiveMessageMock).not.toHaveBeenCalled()

    await user.type(getStageInput(), '先看看天色')
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
    expect(sendInteractiveMessageMock.mock.calls[0][0].message).toBe('让何泰把船撑到下游\n先看看天色')
    await act(async () => { stream.close() })
  })

  it('offers no entry when the conversation settled nothing', async () => {
    const user = userEvent.setup()
    getCharacterWindowMock.mockImplementation(async (_storyId: string, options: { characterId?: string } = {}) => {
      if (options.characterId === 'caopo') {
        // A guess and a question are not agreements: nothing here is open and
        // settled, so the window must not offer to act on it.
        return characterWindow({
          character_id: 'caopo',
          exchanges: caopoExchange(),
          matters: [unagreedMatter()],
        })
      }
      return characterWindow()
    })
    render(<StageHarness initialSnapshot={snapshotWithChoices()} />)

    await openConversation(user, 'person', 'caopo')
    await screen.findByText('那你天亮前来，我把篮子给你。')
    expect(screen.queryByRole('button', { name: '加入下轮行动' })).not.toBeInTheDocument()
    expect(screen.queryByText('下轮行动')).not.toBeInTheDocument()
  })

  it('does not carry an arrangement across a branch change', async () => {
    const user = userEvent.setup()
    const { rerender } = render(<StageHarness initialSnapshot={snapshotWithChoices()} />)

    await openConversation(user, 'person', 'caopo')
    await actOnPlan(user)
    await screen.findByText('下轮行动')

    // The arrangement was made on `main`; another branch never heard it.
    rerender(<StageHarness initialSnapshot={snapshotWithChoices()} branchId="side" />)

    await waitFor(() => expect(screen.queryByText('下轮行动')).not.toBeInTheDocument())
    expect(screen.queryByText(AGREEMENT)).not.toBeInTheDocument()
    expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
  })
})

it('opens fantasy combat from the actual main toolbar instead of the cultivation panel', async () => {
  const user = userEvent.setup()
  render(<StageHarness initialSnapshot={snapshotWithChoices()} gameplay={{ status_template: 'fantasy-state-v1', growth: 'fantasy', combat: 'fantasy', communication: 'letter', opening: 'scene' }} />)
  await user.click(screen.getByRole('button', { name: '打开诸境交锋' }))
  expect(await screen.findByTestId('native-adventure-panel')).toBeVisible()
  expect(screen.queryByTestId('combat-window-panel')).not.toBeInTheDocument()
})
