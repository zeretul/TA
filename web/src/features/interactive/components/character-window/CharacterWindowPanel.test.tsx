import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CharacterChatResult, CharacterWindow, CharacterWindowCharacter } from '../../types'

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
const attentionMocks = vi.hoisted(() => ({ getPlayerAttention: vi.fn(), updatePlayerAttention: vi.fn() }))
vi.mock('../../player-attention-api', () => attentionMocks)
beforeEach(() => {
  attentionMocks.getPlayerAttention.mockReset().mockResolvedValue({ v: 1, head: 'h1', characters: [], quests: [] })
  attentionMocks.updatePlayerAttention.mockReset().mockResolvedValue({ v: 1, head: 'h2', characters: ['liuqing'], quests: [] })
})

// The material control reads a person's picture as soon as the portrait is
// opened, so every render needs an answer rather than an unresolved promise.
function resetPictureMocks() {
  mocks.getCharacterPicture.mockReset()
  mocks.getCharacterPicture.mockResolvedValue({ source: '', overridden: false })
  mocks.importCharacterPicture.mockReset()
  mocks.clearCharacterPicture.mockReset()
  mocks.getPlacePicture.mockReset()
  mocks.getPlacePicture.mockResolvedValue({ overridden: false })
  mocks.importPlacePicture.mockReset()
  mocks.clearPlacePicture.mockReset()
}

function character(overrides: Partial<CharacterWindowCharacter> = {}): CharacterWindowCharacter {
  return {
    id: 'liuqing',
    name: '柳青',
    known_as: '柳青',
    present: true,
    channel: 'scene',
    unread_messages: 0,
    open_matters: 0,
    affinity: 46,
    affinity_band: 'friendly',
    ...overrides,
  }
}

function windowState(overrides: Partial<CharacterWindow> = {}): CharacterWindow {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    // The location is the story's own prose; `place` is the registered place the
    // server resolved it to, and the only name a picture can be bound to.
    scene: {
      title: '渡口',
      location: '青石渡口码头',
      place: '青石渡口',
      world_time: '第3日',
      present: ['liuqing'],
    },
    characters: [character()],
    exchanges: [],
    matters: [],
    messages: [],
    has_more: false,
    ...overrides,
  }
}

function chatResult(overrides: Partial<CharacterChatResult> = {}): CharacterChatResult {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    character_id: 'liuqing',
    reply: '那就明早辰时。',
    world_time: '第3日 辰时',
    exchange_ids: ['xch_2'],
    affinity: 46,
    affinity_band: 'friendly',
    affinity_delta: 0,
    ...overrides,
  }
}

/** The host holds the draft, exactly as StoryStage does. */
function Hosted({ projectId, onContinueStory }: { projectId?: string; onContinueStory?: (message: string, playerLetter?: { character_id: string; body: string }) => void }) {
  const [draft, setDraft] = useState('')
  return (
    <CharacterWindowPanel
      storyId="story-1"
      branchId="main"
      projectId={projectId}
      active
      characterId="liuqing"
      onSelectCharacter={() => {}}
      onClose={() => {}}
      onContinueStory={onContinueStory}
      draft={draft}
      onDraftChange={setDraft}
    />
  )
}

function renderPanel(projectId?: string) {
  return render(<Hosted projectId={projectId} />)
}

/**
 * Opens the arrangements summary above the history.
 *
 * Arrangements and letters in transit fold to one line so a long list cannot
 * push the conversation and the composer out of the panel; what they hold is one
 * click away, which is what these assertions have to do before reading it.
 */
async function openMatters() {
  await userEvent.setup().click(await screen.findByRole('button', { name: /^未了之事/ }))
}

/** The cast list is a different view of the same host-held panel. */
function HostedRoster() {
  const [draft, setDraft] = useState('')
  return (
    <CharacterWindowPanel
      storyId="story-1"
      branchId="main"
      active
      characterId=""
      onSelectCharacter={() => {}}
      onClose={() => {}}
      draft={draft}
      onDraftChange={setDraft}
    />
  )
}

describe('CharacterWindowPanel affinity reading', () => {
  it('follows the known person without sending a message', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState())
    renderPanel()
    const follow = await screen.findByRole('button', { name: '关注人物' })
    await waitFor(() => expect(follow).toBeEnabled())
    fireEvent.click(follow)
    await screen.findByRole('button', { name: '取消人物关注' })
    expect(attentionMocks.updatePlayerAttention).toHaveBeenCalledWith('story-1', { branch: 'main', expected_head: 'h1', kind: 'character', id: 'liuqing', following: true })
    expect(mocks.sendCharacterMessage).not.toHaveBeenCalled()
  })
  beforeEach(() => {
    mocks.getCharacterWindow.mockReset()
    mocks.sendCharacterMessage.mockReset()
    mocks.setCharacterMessageStatus.mockReset()
    mocks.setCharacterMessageStatus.mockResolvedValue(undefined)
    resetPictureMocks()
  })

  it('preserves the private-chat reading position when an earlier page arrives', async () => {
    const user = userEvent.setup()
    const exchange = { id: 'latest', story_id: 'story-1', branch_id: 'main', character_id: 'liuqing', speaker: 'character' as const, channel: 'private' as const, text: 'Latest private page', world_time: '' }
    mocks.getCharacterWindow.mockResolvedValueOnce(windowState({ exchanges: [exchange], has_more: true, cursor: 'older' }))
      .mockResolvedValueOnce(windowState({ exchanges: [{ ...exchange, id: 'old', text: 'Earlier private page' }], has_more: false }))
    renderPanel()
    await screen.findByText('Latest private page')
    const transcript = screen.getByTestId('character-window-transcript')
    Object.defineProperty(transcript, 'scrollHeight', { configurable: true, get: () => transcript.textContent?.includes('Earlier private page') ? 1400 : 1000 })
    transcript.scrollTop = 120
    await user.click(screen.getByRole('button', { name: '载入更早的交谈' }))
    await screen.findByText('Earlier private page')
    expect(transcript.scrollTop).toBe(520)
  })

  it('shows the number and the formal bond, not just a bar', async () => {
    mocks.getCharacterWindow.mockResolvedValue(
      windowState({
        characters: [
          character({
            relationship: {
              id: 'rel_1',
              character_id: 'liuqing',
              kind: 'friend',
              statement: '两人正式结为朋友',
              world_time: '第2日',
            },
          }),
        ],
      }),
    )

    renderPanel()

    const line = await screen.findByTestId('character-window-affinity')
    expect(line).toHaveTextContent('好感：46｜关系：朋友')
  })

  it('says nothing formal stands for a character with no bond', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState())

    renderPanel()

    const line = await screen.findByTestId('character-window-affinity')
    expect(line).toHaveTextContent('好感：46｜关系：尚无明确关系')
  })

  it('shows a newcomer as neutral rather than as a missing value', async () => {
    mocks.getCharacterWindow.mockResolvedValue(
      windowState({ characters: [character({ affinity: 0, affinity_band: 'neutral' })] }),
    )

    renderPanel()

    const line = await screen.findByTestId('character-window-affinity')
    expect(line).toHaveTextContent('好感：0')
    expect(line).toHaveTextContent('尚无明确关系')
  })

  it('reports a change of feeling in words, and only when something changed', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState())
    mocks.sendCharacterMessage.mockResolvedValueOnce(chatResult())
    const user = userEvent.setup()

    renderPanel()

    // An ordinary reply that moved nothing says nothing.
    await user.type(await screen.findByRole('textbox'), '明早见。')
    await user.click(screen.getByRole('button', { name: '发送' }))
    await waitFor(() => expect(mocks.sendCharacterMessage).toHaveBeenCalledTimes(1))
    expect(screen.queryByTestId('character-window-affinity-change')).not.toBeInTheDocument()

    // A real move gets one plain sentence. The wording carries the direction,
    // not a signed number the player would have to interpret.
    mocks.getCharacterWindow.mockResolvedValue(
      windowState({ characters: [character({ affinity: 49, affinity_band: 'friendly' })] }),
    )
    mocks.sendCharacterMessage.mockResolvedValueOnce(chatResult({ affinity: 49, affinity_delta: 3 }))
    await user.type(screen.getByRole('textbox'), '药我替你送去了。')
    await user.click(screen.getByRole('button', { name: '发送' }))

    const change = await screen.findByTestId('character-window-affinity-change')
    expect(change).toHaveTextContent('柳青')
    expect(change).not.toHaveTextContent('+3')
  })
})

describe('CharacterWindowPanel letter mode', () => {
  beforeEach(() => {
    mocks.getCharacterWindow.mockReset()
    mocks.sendCharacterMessage.mockReset()
    mocks.setCharacterMessageStatus.mockReset()
    mocks.setCharacterMessageStatus.mockResolvedValue(undefined)
    resetPictureMocks()
  })

  it('sends a letter through the main-story turn with its exact recipient and body', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState({ communication_mode: 'letter', characters: [character({ present: false })], scene: { present: [] } }))
    const continueStory = vi.fn()
    const user = userEvent.setup()
    render(<Hosted onContinueStory={continueStory} />)

    await user.type(await screen.findByRole('textbox'), '明日到渡口等我。')
    await user.click(screen.getByRole('button', { name: '寄出' }))

    expect(continueStory).toHaveBeenCalledWith(
      expect.stringContaining('明日到渡口等我。'),
      { character_id: 'liuqing', body: '明日到渡口等我。' },
    )
    expect(mocks.sendCharacterMessage).not.toHaveBeenCalled()
  })

  it('talks to a committed present recipient without posting a letter', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState({ communication_mode: 'letter' }))
    mocks.sendCharacterMessage.mockResolvedValueOnce(chatResult())
    const continueStory = vi.fn()
    const user = userEvent.setup()
    render(<Hosted onContinueStory={continueStory} />)
    await user.type(await screen.findByRole('textbox'), '就在这里谈谈。')
    await user.click(screen.getByRole('button', { name: '发送' }))
    await waitFor(() => expect(mocks.sendCharacterMessage).toHaveBeenCalledTimes(1))
    expect(mocks.sendCharacterMessage.mock.calls[0][1]).toMatchObject({ characterId: 'liuqing', message: '就在这里谈谈。' })
    expect(continueStory).not.toHaveBeenCalled()
    expect(screen.queryByRole('button', { name: '寄出' })).not.toBeInTheDocument()
  })

  it('keeps a missing presence declaration on the letter route', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState({ communication_mode: 'letter', scene: { present: [] } }))
    const continueStory = vi.fn()
    const user = userEvent.setup()
    render(<Hosted onContinueStory={continueStory} />)
    await user.type(await screen.findByRole('textbox'), '等信抵达再答。')
    await user.click(screen.getByRole('button', { name: '寄出' }))
    expect(continueStory).toHaveBeenCalledWith(expect.any(String), { character_id: 'liuqing', body: '等信抵达再答。' })
    expect(mocks.sendCharacterMessage).not.toHaveBeenCalled()
  })

  it('shows letters as still travelling or ready for the next story turn', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState({
      clock_seconds: 86_400,
      matters: [{
        id: 'mat_letter', character_id: 'liuqing', kind: 'action', status: 'open',
        statement: '一封寄给柳青的信：明日到渡口等我。', world_time: '第3日',
        letter: { body: '明日到渡口等我。', sent_at_seconds: 0, earliest_delivery_seconds: 86_400 },
      }],
    }))
    renderPanel()
    await openMatters()
    expect(await screen.findByText('已到预计送达的时辰，尚无收件消息。')).toBeInTheDocument()
  })

  it('keeps the sent letter and written reply visible after formal delivery', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState({
      clock_seconds: 86_400,
      matters: [
        {
          id: 'mat_letter', character_id: 'liuqing', kind: 'action', status: 'settled', settled_by: 'mat_reply',
          statement: '一封寄给柳青的信，柳青尚未收到。', world_time: '第3日',
          letter: { body: '明日到渡口等我。', sent_at_seconds: 0, earliest_delivery_seconds: 86_400 },
        },
        {
          id: 'mat_reply', character_id: 'liuqing', kind: 'action', status: 'settled', settled_by: 'mat_letter',
          statement: '柳青收到来信后回信：明早辰时渡口见。', world_time: '第4日',
          letter_reply: { body: '明早辰时渡口见。' },
        },
      ],
    }))
    renderPanel()
    await openMatters()

    expect(await screen.findByText('明日到渡口等我。')).toBeInTheDocument()
    expect(screen.queryByText(/柳青尚未收到/)).not.toBeInTheDocument()
    expect(screen.getByText('对方已经收到这封信。')).toBeInTheDocument()
    expect(screen.getByText('柳青收到来信后回信：明早辰时渡口见。')).toBeInTheDocument()
    expect(screen.getByText('书面回信：“明早辰时渡口见。”')).toBeInTheDocument()
  })

  it('labels an oral answer as receipt without inventing a written reply', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState({
      matters: [
        { id: 'mat_letter', character_id: 'liuqing', kind: 'action', status: 'settled', settled_by: 'mat_receipt', statement: '寄给柳青的信。', world_time: '第3日', letter: { body: '留房', sent_at_seconds: 0, earliest_delivery_seconds: 86_400 } },
        { id: 'mat_receipt', character_id: 'liuqing', kind: 'action', status: 'settled', settled_by: 'mat_letter', statement: '柳青当面说收到了信。', world_time: '第4日' },
      ],
    }))
    renderPanel()
    await openMatters()
    expect(await screen.findByText('柳青当面说收到了信。')).toBeInTheDocument()
    expect(screen.getByText('收信记录')).toBeInTheDocument()
    expect(screen.queryByText(/书面回信/)).not.toBeInTheDocument()
  })
})

describe('CharacterWindowPanel roster gender', () => {
  beforeEach(() => {
    mocks.getCharacterWindow.mockReset()
    mocks.sendCharacterMessage.mockReset()
    mocks.setCharacterMessageStatus.mockReset()
    resetPictureMocks()
  })

  function renderRoster() {
    return render(<HostedRoster />)
  }

  it('shows a gender the story settled beside the standing', async () => {
    mocks.getCharacterWindow.mockResolvedValue(
      windowState({ characters: [character({ role: '船工', gender: 'male' })] }),
    )
    renderRoster()
    const row = await screen.findByTestId('character-window-roster-row')
    expect(row.textContent).toContain('男')
    expect(row.textContent).toContain('船工')
  })

  it('shows nothing where nobody settled a gender', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState({ characters: [character({ role: '船工' })] }))
    renderRoster()
    const row = await screen.findByTestId('character-window-roster-row')
    // No placeholder word: an unsettled gender is not something the player
    // failed to find out, so there is nothing to say about it.
    expect(row.textContent).toContain('柳青')
    expect(row.textContent).toContain('船工')
    expect(row.textContent).not.toMatch(/男|女/)
  })
})

describe('CharacterWindowPanel portrait', () => {
  beforeEach(() => {
    mocks.getCharacterWindow.mockReset()
    mocks.sendCharacterMessage.mockReset()
    mocks.setCharacterMessageStatus.mockReset()
    mocks.setCharacterMessageStatus.mockResolvedValue(undefined)
    resetPictureMocks()
  })

  it('keeps the larger picture folded away until the player asks for it', async () => {
    mocks.getCharacterWindow.mockResolvedValue(
      windowState({
        characters: [
          character({
            avatar: '/api/portraits/canon_chen_qiaoqian.png',
            avatar_display: '/api/portraits/canon_chen_qiaoqian.png?size=display',
          }),
        ],
      }),
    )
    const user = userEvent.setup()

    renderPanel()

    const toggle = await screen.findByTestId('character-window-portrait-toggle')
    expect(toggle).toHaveTextContent('查看柳青的画像')
    expect(screen.queryByRole('img', { name: '柳青的画像' })).not.toBeInTheDocument()

    await user.click(toggle)

    const picture = screen.getByRole('img', { name: '柳青的画像' })
    expect(picture).toHaveAttribute('src', '/api/portraits/canon_chen_qiaoqian.png?size=display')
    expect(toggle).toHaveTextContent('收起画像')
  })

  it('offers a place to add a picture for a person who has none', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState())
    const user = userEvent.setup()

    renderPanel()

    // The conversation still opens, and the portrait affordance is there — it
    // says there is a picture to add rather than offering to show one.
    expect(await screen.findByRole('textbox')).toBeInTheDocument()
    const toggle = screen.getByTestId('character-window-portrait-toggle')
    expect(toggle).toHaveTextContent('添加画像')
    expect(screen.queryByRole('img', { name: '柳青的画像' })).not.toBeInTheDocument()

    await user.click(toggle)
    expect(screen.getByTestId('character-picture-control')).toBeInTheDocument()
    expect(screen.getByText('这个人还没有图片，显示的是名字首字。')).toBeInTheDocument()
  })

  it('drops back to the small avatar when the larger picture cannot load', async () => {
    mocks.getCharacterWindow.mockResolvedValue(
      windowState({
        characters: [
          character({
            avatar: '/api/portraits/canon_chen_qiaoqian.png',
            avatar_display: '/api/portraits/canon_chen_qiaoqian.png?size=display',
          }),
        ],
      }),
    )
    const user = userEvent.setup()

    renderPanel()
    await user.click(await screen.findByTestId('character-window-portrait-toggle'))

    fireEvent.error(screen.getByRole('img', { name: '柳青的画像' }))

    // The enlarged picture is gone; the block stays so another picture can be
    // added, and it no longer claims there is one to view.
    expect(screen.queryByRole('img', { name: '柳青的画像' })).not.toBeInTheDocument()
    expect(screen.getByTestId('character-window-portrait-toggle')).toHaveTextContent('收起画像')
    // The conversation itself is unaffected.
    expect(screen.getByRole('textbox')).toBeInTheDocument()
  })

  it('draws the world book picture through the project the story belongs to', async () => {
    mocks.getCharacterWindow.mockResolvedValue(
      windowState({
        characters: [
          character({
            avatar: undefined,
            avatar_lore_path: 'assets/lore/images/liuqing/image.png',
          }),
        ],
      }),
    )
    const user = userEvent.setup()

    renderPanel('project-1')

    // The open conversation enlarges it, through the project the story reads.
    await user.click(await screen.findByTestId('character-window-portrait-toggle'))
    expect(screen.getByRole('img', { name: '柳青的画像' }).getAttribute('src')).toBe(
      '/api/projects/project-1/files/asset?path=assets%2Flore%2Fimages%2Fliuqing%2Fimage.png',
    )
  })

  it('illustrates the place the story stands in without asking which place', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState())
    mocks.getPlacePicture.mockResolvedValue({ overridden: false })
    mocks.importPlacePicture.mockResolvedValue({
      place: '青石渡口',
      url: '/api/media/u-abc.png',
      picture: { url: '/api/media/u-abc.png', overridden: true },
    })
    const user = userEvent.setup()

    renderPanel()

    const toggle = await screen.findByTestId('scene-picture-toggle')
    // The place is the scene's own registered place, so the control is folded
    // until the author opens it — reading a place's picture is not part of
    // reading the story.
    expect(mocks.getPlacePicture).not.toHaveBeenCalled()
    await user.click(toggle)

    // The prose location 青石渡口码头 names the registered place 青石渡口, and it
    // is the latter a picture binds to.
    expect(mocks.getPlacePicture).toHaveBeenCalledWith('story-1', '青石渡口')
    expect(await screen.findByTestId('place-picture-control')).toBeInTheDocument()
    expect(screen.getByText('这个地方还没有图片，只显示文字。')).toBeInTheDocument()

    const file = new File(['png'], 'dukou.png', { type: 'image/png' })
    await user.upload(screen.getByLabelText('选择图片'), file)

    // The file goes to the place on screen, not to a place name the author typed.
    expect(mocks.importPlacePicture).toHaveBeenCalledWith('story-1', '青石渡口', file)
    // The control now draws the picture it just imported and offers to drop it.
    await waitFor(() => {
      expect(screen.getByRole('presentation')).toHaveAttribute('src', '/api/media/u-abc.png')
    })
    expect(screen.getByText('当前显示的是导入的地点图片。')).toBeInTheDocument()
  })

  it('hands every keystroke to the host, so a draft can outlive the window', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState())
    const onDraftChange = vi.fn()
    const user = userEvent.setup()

    render(
      <CharacterWindowPanel
        storyId="story-1"
        branchId="main"
        active
        characterId="liuqing"
        onSelectCharacter={() => {}}
        onClose={() => {}}
        draft=""
        onDraftChange={onDraftChange}
      />,
    )

    await user.type(await screen.findByRole('textbox'), '明')

    // The panel keeps no copy of its own: the host is the only holder.
    expect(onDraftChange).toHaveBeenCalled()
    expect(onDraftChange.mock.calls.at(-1)?.[0]).toBe('明')
  })

  it('shows the draft the host hands back', async () => {
    mocks.getCharacterWindow.mockResolvedValue(windowState())

    render(
      <CharacterWindowPanel
        storyId="story-1"
        branchId="main"
        active
        characterId="liuqing"
        onSelectCharacter={() => {}}
        onClose={() => {}}
        draft="写到一半的话"
        onDraftChange={() => {}}
      />,
    )

    expect(await screen.findByRole('textbox')).toHaveValue('写到一半的话')
  })
})
