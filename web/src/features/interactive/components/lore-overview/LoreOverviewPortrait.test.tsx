import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CharacterWindow, CharacterWindowCharacter, Snapshot } from '../../types'
import { EntryDetail, EntryRow, LoreOverviewPanel } from './LoreOverviewPanel'
import { buildLocalEntities, buildLoreOverview, type LoreEntry, type LoreOverviewSources } from './model'

const api = vi.hoisted(() => ({ getLoreOverview: vi.fn() }))
vi.mock('../../api', () => api)

const thumbnail = '/api/portraits/canon_chen_qiaoqian.png'
const display = `${thumbnail}?size=display`
const caption = '人物参考图（视觉细节为原创演绎）'
const scoped = (head = 'first', revision = 1, story = 'story-1', branch = 'main') =>
  `/api/interactive/stories/${story}/media/art?actor=author&branch=${branch}&head=${head}&revision=${revision}`
const noop = () => {}

function person(overrides: Partial<CharacterWindowCharacter> = {}): CharacterWindowCharacter {
  return {
    id: 'author', name: '同名人物', source_key: 'canon_chen_qiaoqian', identity_source: 'author_catalog',
    present: true, channel: 'scene', unread_messages: 0, open_matters: 0, affinity: 0, affinity_band: 'neutral',
    ...overrides,
  }
}

function aggregate(people = [person()], story = 'story-1', branch = 'main', revision = 1): LoreOverviewSources & {
  snapshot: Snapshot; characters: CharacterWindow
} {
  const identity = { story_id: story, branch_id: branch }
  return {
    snapshot: { ...identity, context_revision: revision, turns: [], state: {} },
    characters: { ...identity, scene: { location: '庭院' }, characters: people, has_more: false },
    journey: { ...identity, clock_seconds: 0, routes: [] },
    quests: { ...identity, tasks: [], available: [] },
    events: { ...identity, clock_seconds: 0, events: [] },
  }
}

function entry(overrides: Partial<CharacterWindowCharacter> = {}): LoreEntry {
  return buildLoreOverview(aggregate([person(overrides)])).byTab.people[0]
}

function detail(value: LoreEntry, projectId = 'project-1') {
  return <EntryDetail entry={value} projectId={projectId} onBack={noop} onOpenLink={noop} />
}

function panel(data: ReturnType<typeof aggregate>, active = true) {
  return <LoreOverviewPanel storyId={data.snapshot.story_id} branchId={data.snapshot.branch_id}
    active={active} snapshot={data.snapshot} projectId="project-1" onClose={noop} />
}

beforeEach(() => {
  api.getLoreOverview.mockReset()
  window.localStorage.clear()
})

describe('lore portrait resolution and source boundaries', () => {
  it('keeps thumbnail, display and caption attached to stable identity, without matching names', () => {
    const data = aggregate([
      person({ avatar: thumbnail, avatar_display: display, image_caption: caption }),
      person({ id: 'invented', source_key: '', identity_source: 'story' }),
    ])
    const people = buildLoreOverview(data).byTab.people
    expect(people.map(value => ({ id: value.id, title: value.title, avatar: value.avatar,
      display: value.avatarDisplay, caption: value.imageCaption }))).toEqual([
      { id: 'author', title: '同名人物', avatar: thumbnail, display, caption },
      { id: 'invented', title: '同名人物', avatar: undefined, display: undefined, caption: undefined },
    ])
    expect(buildLocalEntities(data).characters.map(value => value.id)).toEqual(['invented'])
  })

  it('uses the real display URL in detail while leaving the directory on the thumbnail', () => {
    const value = entry({ avatar: thumbnail, avatar_display: display })
    const view = render(<EntryRow entry={value} onOpen={noop} />)
    expect(view.container.querySelector('img')).toHaveAttribute('src', thumbnail)
    view.rerender(detail(value))
    const image = view.container.querySelector('img')!
    expect(image).toHaveAttribute('src', display)
    expect(image.className).toContain('h-24 w-24')
    expect(image.className).toContain('object-contain')
    expect(screen.queryByTestId('lore-overview-portrait-toggle')).not.toBeInTheDocument()
  })

  it.each([
    { name: 'chosen avatar', input: { avatar: '/api/projects/p/attachments/mine', avatar_lore_path: 'assets/lore/image.png' }, expected: '/api/projects/p/attachments/mine' },
    { name: 'project lore image', input: { avatar_lore_path: 'assets/lore/image.png' }, expected: '/api/projects/project-1/files/asset?path=assets%2Flore%2Fimage.png' },
    { name: 'no image', input: {}, expected: null },
  ])('falls back to $name when display is absent', ({ input, expected }) => {
    const view = render(detail(entry(input)))
    if (expected) expect(view.container.querySelector('img')).toHaveAttribute('src', expected)
    else expect(view.container.querySelector('img')).toBeNull()
    expect(screen.queryByTestId('lore-overview-portrait-toggle')).not.toBeInTheDocument()
  })

  it('preserves the supplied display URL without constructing a larger derivative', () => {
    const url = '/api/catalog-art/reviewed.webp?version=3&token=opaque'
    const view = render(detail(entry({ avatar: thumbnail, avatar_display: url })))
    expect(view.container.querySelector('img')).toHaveAttribute('src', url)
  })

  it.each([true, false])('does not substitute another image after failure (display present: %s)', hasDisplay => {
    const view = render(detail(entry({ avatar: thumbnail, avatar_display: hasDisplay ? display : undefined,
      avatar_lore_path: 'assets/lore/image.png', image_caption: caption })))
    fireEvent.error(view.container.querySelector('img')!)
    expect(view.container.querySelector('img')).toBeNull()
    expect(screen.queryByText(caption)).not.toBeInTheDocument()
  })
})

describe('display-only portraits require an explicit image open', () => {
  it('keeps both directory and selected-person detail free of protected image elements until the portrait button is pressed', async () => {
    const user = userEvent.setup()
    const value = entry({ avatar_display: scoped(), image_caption: caption })
    const view = render(<EntryRow entry={value} onOpen={noop} />)
    expect(view.container.querySelector('img')).toBeNull()
    view.rerender(detail(value))
    expect(view.container.querySelector('img')).toBeNull()
    expect(screen.queryByText(caption)).not.toBeInTheDocument()
    const toggle = screen.getByRole('button', { name: '查看同名人物的画像' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await user.click(toggle)
    expect(view.container.querySelectorAll('img')).toHaveLength(1)
    expect(view.container.querySelector('img')).toHaveAttribute('src', scoped())
    expect(screen.getByText(caption)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '收起画像' }))
    expect(view.container.querySelector('img')).toBeNull()
    expect(screen.queryByText(caption)).not.toBeInTheDocument()
  })

  it('uses the projection contract rather than a protected-URL spelling heuristic', async () => {
    const url = '/another-read-only-route/opaque-original.webp'
    const view = render(detail(entry({ avatar_display: url })))
    expect(view.container.querySelector('img')).toBeNull()
    await userEvent.click(screen.getByTestId('lore-overview-portrait-toggle'))
    expect(view.container.querySelector('img')).toHaveAttribute('src', url)
  })

  it('hides the caption and leaves a neutral fallback on denial, without a public retry', async () => {
    const view = render(detail(entry({ avatar_display: scoped(), image_caption: caption })))
    await userEvent.click(screen.getByTestId('lore-overview-portrait-toggle'))
    fireEvent.error(view.container.querySelector('img')!)
    expect(view.container.querySelector('img')).toBeNull()
    expect(screen.queryByText(caption)).not.toBeInTheDocument()
    expect(view.container.querySelector('img[src^="/api/media/"]')).toBeNull()
  })

  it.each(['actor', 'identity source', 'project', 'media head', 'media revision', 'caption'] as const)(
    'requires a fresh explicit open after %s changes', async changed => {
      const value = entry({ avatar_display: scoped(), image_caption: caption })
      const view = render(detail(value))
      await userEvent.click(screen.getByTestId('lore-overview-portrait-toggle'))
      expect(view.container.querySelector('img')).toHaveAttribute('src', scoped())
      const next = { ...value }
      let project = 'project-1'
      if (changed === 'actor') next.id = 'other-with-the-same-name'
      if (changed === 'identity source') next.identitySource = 'story'
      if (changed === 'project') project = 'project-2'
      if (changed === 'media head') next.avatarDisplay = scoped('second')
      if (changed === 'media revision') next.avatarDisplay = scoped('first', 2)
      if (changed === 'caption') next.imageCaption = '更新后的参考图说明'
      view.rerender(detail(next, project))
      expect(view.container.querySelector('img')).toBeNull()
      expect(screen.getByTestId('lore-overview-portrait-toggle')).toHaveAttribute('aria-expanded', 'false')
      await userEvent.click(screen.getByTestId('lore-overview-portrait-toggle'))
      expect(view.container.querySelector('img')).toHaveAttribute('src', next.avatarDisplay)
    },
  )

  it('removes the image when the server withdraws the display projection and does not restore an earlier open', async () => {
    const value = entry({ avatar_display: scoped(), image_caption: caption })
    const view = render(detail(value))
    await userEvent.click(screen.getByTestId('lore-overview-portrait-toggle'))
    view.rerender(detail(entry()))
    expect(view.container.querySelector('img')).toBeNull()
    expect(screen.queryByText(caption)).not.toBeInTheDocument()
    expect(screen.queryByTestId('lore-overview-portrait-toggle')).not.toBeInTheDocument()
    view.rerender(detail(value))
    expect(view.container.querySelector('img')).toBeNull()
    expect(screen.getByTestId('lore-overview-portrait-toggle')).toHaveAttribute('aria-expanded', 'false')
  })
})

describe('lore overview aggregate identity and media lifecycle', () => {
  it('does not read while inactive, and entering or returning to a person is not a portrait-open gesture', async () => {
    const user = userEvent.setup()
    const data = aggregate([person({ avatar_display: scoped(), image_caption: caption })])
    api.getLoreOverview.mockResolvedValue(data)
    const view = render(panel(data, false))
    expect(api.getLoreOverview).not.toHaveBeenCalled()
    expect(view.container.querySelector('img')).toBeNull()
    view.rerender(panel(data))
    await user.click(await screen.findByRole('tab', { name: '人物' }))
    const row = () => view.container.querySelector<HTMLElement>('[data-lore-entry="author"]')!
    expect(row()).not.toBeNull()
    expect(view.container.querySelector('img')).toBeNull()
    await user.click(row())
    expect(view.container.querySelector('img')).toBeNull()
    await user.click(screen.getByTestId('lore-overview-portrait-toggle'))
    expect(view.container.querySelector('img')).toHaveAttribute('src', scoped())
    fireEvent.keyDown(screen.getByTestId('lore-overview-panel'), { key: 'Escape' })
    expect(view.container.querySelector('img')).toBeNull()
    await user.click(row())
    expect(view.container.querySelector('img')).toBeNull()
    expect(screen.getByTestId('lore-overview-portrait-toggle')).toHaveAttribute('aria-expanded', 'false')
    expect(api.getLoreOverview).toHaveBeenCalledTimes(1)
  })

  it('withholds every portrait from a cross-branch aggregate response', async () => {
    const data = aggregate([person({ avatar_display: scoped() })])
    api.getLoreOverview.mockResolvedValue({ ...data, characters: { ...data.characters, branch_id: 'other' } })
    const view = render(panel(data))
    await waitFor(() => expect(screen.getByText('故事已变化，请重新载入见闻录。')).toBeInTheDocument())
    expect(view.container.querySelector('img')).toBeNull()
    expect(view.container.querySelector('[data-lore-entry]')).toBeNull()
  })

  it.each(['story', 'branch'] as const)('never carries portrait-open state into another %s', async changed => {
    const data = aggregate([person({ avatar_display: scoped() })])
    api.getLoreOverview.mockResolvedValueOnce(data)
    const view = render(panel(data))
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    await userEvent.click(view.container.querySelector<HTMLElement>('[data-lore-entry="author"]')!)
    await userEvent.click(screen.getByTestId('lore-overview-portrait-toggle'))
    expect(view.container.querySelector('img')).toHaveAttribute('src', scoped())
    const story = changed === 'story' ? 'story-2' : 'story-1'
    const branch = changed === 'branch' ? 'other' : 'main'
    const next = aggregate([person({ avatar_display: scoped('next', 1, story, branch) })], story, branch)
    api.getLoreOverview.mockResolvedValueOnce(next)
    view.rerender(panel(next))
    expect(view.container.querySelector('img')).toBeNull()
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    await userEvent.click(view.container.querySelector<HTMLElement>('[data-lore-entry="author"]')!)
    expect(view.container.querySelector('img')).toBeNull()
    await userEvent.click(screen.getByTestId('lore-overview-portrait-toggle'))
    expect(view.container.querySelector('img')).toHaveAttribute('src', scoped('next', 1, story, branch))
  })

  it('removes an open portrait during revision refresh and ignores a stale aggregate carrying an old URL', async () => {
    const first = aggregate([person({ avatar_display: scoped(), image_caption: caption })])
    api.getLoreOverview.mockResolvedValueOnce(first)
    const view = render(panel(first))
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    await userEvent.click(view.container.querySelector<HTMLElement>('[data-lore-entry="author"]')!)
    await userEvent.click(screen.getByTestId('lore-overview-portrait-toggle'))
    expect(view.container.querySelector('img')).toHaveAttribute('src', scoped())
    let resolveStale!: (value: ReturnType<typeof aggregate>) => void
    api.getLoreOverview.mockImplementationOnce(() => new Promise(resolve => { resolveStale = resolve }))
    view.rerender(panel(aggregate(first.characters.characters, 'story-1', 'main', 2)))
    expect(view.container.querySelector('img')).toBeNull()
    const latest = aggregate([person({ avatar_display: scoped('latest', 3), image_caption: caption })], 'story-1', 'main', 3)
    api.getLoreOverview.mockResolvedValueOnce(latest)
    view.rerender(panel(latest))
    const toggle = await screen.findByTestId('lore-overview-portrait-toggle')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(view.container.querySelector('img')).toBeNull()
    await userEvent.click(toggle)
    expect(view.container.querySelector('img')).toHaveAttribute('src', scoped('latest', 3))
    await act(async () => resolveStale(first))
    expect(view.container.querySelector('img')).toHaveAttribute('src', scoped('latest', 3))
    view.rerender(panel(latest, false))
    expect(view.container.querySelector('img')).toBeNull()
  })

  it('requires another portrait open after closing and reopening the same overview', async () => {
    const data = aggregate([person({ avatar_display: scoped(), image_caption: caption })])
    api.getLoreOverview.mockResolvedValue(data)
    const view = render(panel(data))
    await userEvent.click(await screen.findByRole('tab', { name: '人物' }))
    await userEvent.click(view.container.querySelector<HTMLElement>('[data-lore-entry="author"]')!)
    await userEvent.click(screen.getByTestId('lore-overview-portrait-toggle'))
    expect(view.container.querySelector('img')).toHaveAttribute('src', scoped())
    view.rerender(panel(data, false))
    expect(view.container.querySelector('img')).toBeNull()
    expect(screen.queryByText(caption)).not.toBeInTheDocument()
    view.rerender(panel(data))
    const toggle = await screen.findByTestId('lore-overview-portrait-toggle')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(view.container.querySelector('img')).toBeNull()
    expect(screen.queryByText(caption)).not.toBeInTheDocument()
    expect(api.getLoreOverview).toHaveBeenCalledTimes(2)
    await userEvent.click(toggle)
    expect(view.container.querySelector('img')).toHaveAttribute('src', scoped())
  })

})
