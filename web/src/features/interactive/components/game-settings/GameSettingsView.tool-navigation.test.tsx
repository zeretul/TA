vi.mock('../SettingPanel', () => ({ SettingPanel: () => <div data-testid="creative-preferences" /> }))
vi.mock('./IntimacyModelSetting', () => ({ IntimacyModelSetting: () => <div data-testid="intimacy-model-setting" /> }))
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ToolNavigationIntent } from '@/components/Chat/tool-navigation'
import { useInteractiveStore } from '../../stores/interactive-store'
import type { StorySummary } from '../../types'
import { GameSettingsView } from './GameSettingsView'
import { getGamePlanningTemplates, getImagePresets, getInteractiveSnapshot, getInteractiveTellers, selectInteractiveStory, updateInteractiveStory } from '../../api'
import { APIError, listStoryPacks, saveStoryPackSettings } from '@/lib/api-client'
import { getResolvedLocale, setConfiguredLocale } from '@/i18n'

// The page reads the story list from the server, so which story is "being
// played" has to be part of the mocked response, not just of the store.
const storyIndex = vi.hoisted(() => ({ stories: [] as unknown[], current_story_id: '' }))
const packsMock = vi.hoisted(() => ({ packs: [] as unknown[] }))
const bindingMock = vi.hoisted(() => ({ calls: [] as unknown[] }))

vi.mock('../../api', () => ({
  getInteractiveStories: vi.fn(async () => storyIndex),
  getInteractiveSnapshot: vi.fn(async (storyId: string) => ({ story_id: storyId, branch_id: 'alternate-branch', state: {}, turns: [] })),
  getInteractiveTellers: vi.fn(async () => []),
  getGamePlanningTemplates: vi.fn(async () => []),
  getImagePresets: vi.fn(async () => []),
  selectInteractiveStory: vi.fn(async () => undefined),
  updateInteractiveStory: vi.fn(),
  getInteractiveTeller: vi.fn(async () => null),
}))

vi.mock('@/lib/api-client', () => ({
  APIError: class APIError extends Error {},
  listStoryPacks: vi.fn(async () => packsMock.packs),
  saveStoryPackSettings: vi.fn(),
}))

vi.mock('@/features/conversation-config/use-conversation-config', () => ({
  useConversationConfig: (binding: unknown) => { bindingMock.calls.push(binding); return ({
    snapshot: null,
    initialized: false,
    loading: false,
    saving: false,
    error: null,
    patch: vi.fn(async () => true),
    reload: vi.fn(async () => null),
  }) },
}))

vi.mock('@/components/Home/WorldSettingsEditor', () => ({
  WorldSettingsEditor: ({ value, onChange }: { value: { replyTargetChars: number }; onChange: (value: object) => void }) => <button data-testid="world-settings-editor" onClick={() => onChange({ ...value, replyTargetChars: value.replyTargetChars + 100 })}>Edit world {value.replyTargetChars}</button>,
  storyPackSettingsFrom: (value: object) => value,
  worldSettingsValue: (value?: object) => ({ planningMode: 'disabled', replyTargetChars: 2000, ...value }),
}))

vi.mock('@/components/Chat/ConfigManagerChat', () => ({
  ConfigManagerChat: ({ storyId, branchId }: { storyId?: string; branchId?: string }) => <div data-testid="config-manager-chat" data-story={storyId} data-branch={branchId} />,
}))

vi.mock('@/components/Chat/ModelProfileSwitcher', () => ({
  ModelProfileSwitcher: () => <div data-testid="model-profile-switcher" />,
}))

vi.mock('../director-console/StoryTuningView', () => ({
  StoryTuningView: ({ group, onUpdate, tellers = [], planningTemplates = [], imagePresets = [] }: { group?: string; onUpdate: (input: object) => void; tellers?: { name: string }[]; planningTemplates?: { name: string }[]; imagePresets?: { name: string }[] }) => <button data-testid={`tuning-${group || 'all'}`} data-catalog-labels={[...tellers, ...planningTemplates, ...imagePresets].map(item => item.name).join('|')} onClick={() => onUpdate({ reply_target_chars: 1234 })}>Edit {group}</button>,
}))

vi.mock('@/features/settings/SettingsPageFrame', () => ({
  SettingsPageFrame: ({ navigation, children }: { navigation?: React.ReactNode; children: React.ReactNode }) => (
    <div>
      {navigation}
      {children}
    </div>
  ),
}))

// The real navigation is a sidebar. Only which section this page considers
// active matters here, and that is what drives the sidebar's highlight.
vi.mock('@/components/navigation/sectioned-navigation', () => ({
  SectionedNavigation: ({ groups, activeId }: {
    groups: { id: string; items: { id: string }[] }[]
    activeId: string
  }) => (
    <nav data-testid="settings-nav">
      {groups.flatMap((group) => group.items).map((item) => (
        <button
          key={item.id}
          type="button"
          data-section={item.id}
          aria-current={activeId === item.id ? 'page' : undefined}
        >
          {item.id}
        </button>
      ))}
    </nav>
  ),
}))

function story(): StorySummary {
  return {
    id: 'story-1',
    title: '测试故事',
    origin: '',
    protagonist: { mode: 'default' },
    story_teller_id: 'rhythm',
    planning_template_id: 'adventure',
    planning_mode: 'disabled',
    module_refs: { rule_system_id: 'd20', actor_state_id: 'state-basic' },
    reply_target_chars: 2000,
    choice_count: 5,
    image_settings: { mode: 'manual', interval_turns: 3, preset_id: 'realistic' },
    check_settings: { difficulty_shift: 0, roll_modifier: 0 },
    opening: { mode: 'ai' },
    created_at: '',
    updated_at: '',
    branches: 1,
    events: 0,
    turn_count: 0,
  }
}

function intent(resource: string, nonce = 1): ToolNavigationIntent {
  return { target: { kind: 'config_resource', resource }, nonce }
}

function view(intentValue: ToolNavigationIntent | null, visible = true) {
  return (
    <GameSettingsView
      projectId="project-1"
      visible={visible}
      onOpenBooks={vi.fn()} onOpenAssistant={vi.fn()}
      toolNavigationIntent={intentValue}
    />
  )
}

async function renderWith(intentValue: ToolNavigationIntent | null, visible = true) {
  const result = render(view(intentValue, visible))
  await waitFor(() => expect(screen.getByTestId('settings-nav')).toBeInTheDocument())
  return { ...result, nav: screen.getByTestId('settings-nav') }
}

function activeSection(nav: HTMLElement): string | undefined {
  const current = nav.querySelector('[aria-current="page"]')
  return current?.getAttribute('data-section') ?? undefined
}

describe('Game settings section for a resource link', () => {
  beforeEach(() => {
    setConfiguredLocale('zh-CN')
    vi.mocked(listStoryPacks).mockReset().mockImplementation(async () => packsMock.packs as never)
    vi.mocked(getInteractiveTellers).mockReset().mockResolvedValue([])
    vi.mocked(getGamePlanningTemplates).mockReset().mockResolvedValue([])
    vi.mocked(getImagePresets).mockReset().mockResolvedValue([])
    bindingMock.calls = []
    vi.mocked(saveStoryPackSettings).mockReset()
    storyIndex.stories = [story()]
    storyIndex.current_story_id = 'story-1'
    packsMock.packs = []
    useInteractiveStore.getState().setStories([story()], 'story-1')
  })

  it('refreshes localized catalogs while mounted without losing dirty defaults or either selected target', async () => {
    const other = { ...story(), id: 'story-2', title: 'Editing story B' }
    storyIndex.stories = [story(), other]
    vi.mocked(listStoryPacks).mockImplementation(async () => ['a', 'b'].map(id => ({
      id: `world-${id}`, name: getResolvedLocale() === 'en-US' ? `English world ${id}` : `中文世界${id}`,
      settings: { replyTargetChars: getResolvedLocale() === 'en-US' ? 9900 : 2000 },
      settings_revision: getResolvedLocale() === 'en-US' ? `remote-${id}` : `base-${id}`,
    })) as never)
    vi.mocked(getInteractiveTellers).mockImplementation(async () => [{ id: 'rhythm', name: getResolvedLocale() === 'en-US' ? 'English style' : '中文文风' }] as never)
    vi.mocked(getGamePlanningTemplates).mockImplementation(async () => [{ id: 'planning', name: getResolvedLocale() === 'en-US' ? 'English plan' : '中文规划' }] as never)
    vi.mocked(getImagePresets).mockImplementation(async () => [{ id: 'image', name: getResolvedLocale() === 'en-US' ? 'English images' : '中文图片' }] as never)
    await renderWith(null)
    fireEvent.click(screen.getByRole('combobox', { name: '切换要设置的世界' }))
    fireEvent.click(await screen.findByRole('option', { name: '中文世界b' }))
    fireEvent.click(screen.getByTestId('world-settings-editor'))
    fireEvent.click(screen.getByRole('combobox', { name: '切换要设置的故事' }))
    fireEvent.click(await screen.findByRole('option', { name: 'Editing story B' }))
    await screen.findByText('当前分支：alternate-branch')
    await act(async () => setConfiguredLocale('en-US'))
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Switch the world to configure' })).toHaveTextContent('English world b'))
    expect(screen.getByRole('combobox', { name: 'Switch the story to configure' })).toHaveTextContent('Editing story B')
    expect(screen.getByTestId('world-settings-editor')).toHaveTextContent('2100')
    await waitFor(() => expect(screen.getByTestId('tuning-narrative')).toHaveAttribute('data-catalog-labels', 'English style|English plan|English images'))
    expect(bindingMock.calls).toContainEqual({ mode: 'interactive', project_id: 'project-1', story_id: 'story-2', branch_id: 'alternate-branch' })
    expect(useInteractiveStore.getState().currentStoryId).toBe('story-1')
    expect(selectInteractiveStory).not.toHaveBeenCalled()
    vi.mocked(saveStoryPackSettings).mockResolvedValue({ settings: { replyTargetChars: 2100 }, revision: 'saved' } as never)
    fireEvent.click(screen.getByRole('button', { name: 'Save these defaults' }))
    await waitFor(() => expect(saveStoryPackSettings).toHaveBeenCalledWith('world-b', expect.objectContaining({ replyTargetChars: 2100 }), 'base-b'))
  })

  it('ignores a late prior-language catalog after switching back with a dirty draft', async () => {
    const pending: (() => void)[] = []
    vi.mocked(listStoryPacks).mockImplementation(async () => {
      if (getResolvedLocale() === 'en-US') return new Promise(resolve => pending.push(() => resolve([{ id: 'world-a', name: 'Stale English world', settings_revision: 'stale', settings: { replyTargetChars: 9900 } }] as never)))
      return [{ id: 'world-a', name: '中文世界', settings_revision: 'base', settings: { replyTargetChars: 2000 } }] as never
    })
    for (const load of [getInteractiveTellers, getGamePlanningTemplates, getImagePresets]) {
      vi.mocked(load).mockImplementation(async () => {
        if (getResolvedLocale() === 'en-US') return new Promise(resolve => pending.push(() => resolve([{ id: 'stale', name: 'Stale English catalog' }] as never)))
        return [{ id: 'rhythm', name: '中文目录' }] as never
      })
    }
    await renderWith(null)
    await screen.findByText('中文世界')
    fireEvent.click(screen.getByTestId('world-settings-editor'))
    await act(async () => setConfiguredLocale('en-US'))
    await waitFor(() => expect(pending).toHaveLength(4))
    await act(async () => setConfiguredLocale('zh-CN'))
    await waitFor(() => expect(screen.getByTestId('tuning-narrative')).toHaveAttribute('data-catalog-labels', '中文目录|中文目录|中文目录'))
    await act(async () => pending.forEach(resolve => resolve()))
    expect(screen.getByTestId('world-settings-editor')).toHaveTextContent('2100')
    expect(screen.getByText('中文世界')).toBeInTheDocument()
    expect(screen.queryByText('Stale English world')).not.toBeInTheDocument()
    expect(screen.getByTestId('tuning-narrative')).toHaveAttribute('data-catalog-labels', '中文目录|中文目录|中文目录')
    expect(useInteractiveStore.getState().currentStoryId).toBe('story-1')
  })

  it('gives long world and story headers their own narrow-card text row', async () => {
    const longWorld = '澜川群侠传：潮声旧事与长路江湖的完整世界'
    const longStory = 'The very long adventure title that must not squeeze the settings header'
    packsMock.packs = [{ id: 'world-a', name: longWorld }, { id: 'world-b', name: 'Another world' }]
    storyIndex.stories = [{ ...story(), title: longStory }, { ...story(), id: 'story-2', title: 'Another story' }]
    await renderWith(null)
    for (const label of ['切换要设置的世界', '切换要设置的故事']) {
      const selector = await screen.findByRole('combobox', { name: label })
      const header = selector.parentElement!
      expect(header).toHaveClass('grid', 'grid-cols-[auto_minmax(0,1fr)]')
      expect(header).toHaveClass('@[36rem]/settings-header:grid-cols-[auto_minmax(0,1fr)_auto]')
      expect(header.closest('section')).toHaveClass('@container/settings-header')
      expect(selector).toHaveClass('col-start-2', 'w-full', '@[36rem]/settings-header:col-start-auto', '@[36rem]/settings-header:w-64')
      const text = header.children[1]
      expect(text).toHaveTextContent(label.includes('世界') ? longWorld : longStory)
      expect(text).not.toHaveClass('flex-1')
    }
    // The layout change must leave the locally selected setting target usable.
    fireEvent.click(screen.getByRole('combobox', { name: '切换要设置的世界' }))
    fireEvent.click(await screen.findByRole('option', { name: 'Another world' }))
    expect(screen.getAllByText(/修改目标：Another world/)).toHaveLength(2)
    expect(selectInteractiveStory).not.toHaveBeenCalled()
  })

  it.each(['success', 'failure', 'conflict'])('locks the world target until a delayed %s response completes', async (outcome) => {
    packsMock.packs = [{ id: 'world-a', name: 'World A', settings_revision: 'a1' }, { id: 'world-b', name: 'World B', settings_revision: 'b1' }]
    let settle!: () => void
    vi.mocked(saveStoryPackSettings).mockImplementation(() => new Promise((resolve, reject) => { settle = () => outcome === 'success' ? resolve({ settings: { replyTargetChars: 2100 }, revision: 'a2' } as never) : reject(outcome === 'conflict' ? Object.assign(new APIError('conflict', { status: 409 }), { status: 409 }) : new Error('save failed')) }))
    await renderWith(null)
    fireEvent.click(await screen.findByTestId('world-settings-editor'))
    fireEvent.click(screen.getByRole('button', { name: '保存新局默认' }))
    const target = screen.getByRole('combobox', { name: '切换要设置的世界' })
    expect(target).toBeDisabled()
    await act(async () => settle())
    await waitFor(() => expect(target).not.toBeDisabled())
    fireEvent.click(target)
    fireEvent.click(await screen.findByRole('option', { name: 'World B' }))
    await waitFor(() => expect(screen.getByTestId('world-settings-editor')).toHaveTextContent('2000'))
    expect(saveStoryPackSettings).toHaveBeenCalledWith('world-a', expect.objectContaining({ replyTargetChars: 2100 }), 'a1')
    expect(screen.getAllByText(/修改目标：World B/)).toHaveLength(2)
    fireEvent.click(await screen.findByTestId('world-settings-editor'))
    fireEvent.click(screen.getByRole('button', { name: '保存新局默认' }))
    expect(target).toBeDisabled()
    await act(async () => settle())
    await waitFor(() => expect(target).not.toBeDisabled())
    fireEvent.click(target)
    fireEvent.click(await screen.findByRole('option', { name: 'World A' }))
    await waitFor(() => expect(screen.getByTestId('world-settings-editor')).toHaveTextContent(outcome === 'success' ? '2100' : '2000'))
    expect(saveStoryPackSettings).toHaveBeenLastCalledWith('world-b', expect.objectContaining({ replyTargetChars: 2100 }), 'b1')
  })

  it('opens the images section for an image preset link', async () => {
    const { nav } = await renderWith(intent('image_preset'))
    await waitFor(() => expect(activeSection(nav)).toBe('media'))
  })

  it('saves to the locally selected story without changing the story in play', async () => {
    const other = { ...story(), id: 'story-2', title: '另一条故事' }
    storyIndex.stories = [story(), other]
    vi.mocked(updateInteractiveStory).mockResolvedValue(other)
    await renderWith(null)
    fireEvent.click(screen.getByRole('combobox', { name: '切换要设置的故事' }))
    fireEvent.click(await screen.findByRole('option', { name: '另一条故事' }))
    await screen.findByText('当前分支：alternate-branch')
    expect(screen.getAllByText(/修改目标：另一条故事/)).toHaveLength(3)
    fireEvent.click(screen.getByTestId('tuning-narrative'))
    await waitFor(() => expect(updateInteractiveStory).toHaveBeenCalledWith('story-2', { reply_target_chars: 1234 }))
    expect(selectInteractiveStory).not.toHaveBeenCalled()
    expect(useInteractiveStore.getState().currentStoryId).toBe('story-1')
    expect(bindingMock.calls).toContainEqual({ mode: 'interactive', project_id: 'project-1', story_id: 'story-2', branch_id: 'alternate-branch' })
    expect(bindingMock.calls).not.toContainEqual(expect.objectContaining({ story_id: 'story-2', branch_id: 'main' }))
    expect(screen.queryByTestId('config-manager-chat')).not.toBeInTheDocument()
  })

  it('ignores a save response from the prior workspace and after unmount', async () => {
    packsMock.packs = [{ id: 'world-a', name: 'First workspace world', settings_revision: 'a1' }]
    let settle!: () => void
    vi.mocked(saveStoryPackSettings).mockImplementation(() => new Promise(resolve => { settle = () => resolve({ settings: { replyTargetChars: 9900 }, revision: 'old-response' } as never) }))
    const result = await renderWith(null)
    fireEvent.click(await screen.findByTestId('world-settings-editor'))
    fireEvent.click(screen.getByRole('button', { name: '保存新局默认' }))
    packsMock.packs = [{ id: 'world-b', name: 'Second workspace world', settings_revision: 'b1' }]
    result.rerender(<GameSettingsView projectId="project-2" visible onOpenBooks={vi.fn()} onOpenAssistant={vi.fn()} />)
    await screen.findByText('Second workspace world')
    await act(async () => settle())
    expect(screen.getByTestId('world-settings-editor')).toHaveTextContent('2000')
    fireEvent.click(await screen.findByTestId('world-settings-editor'))
    fireEvent.click(screen.getByRole('button', { name: '保存新局默认' }))
    result.unmount()
    await act(async () => settle())
    expect(saveStoryPackSettings).toHaveBeenLastCalledWith('world-b', expect.objectContaining({ replyTargetChars: 2100 }), 'b1')
  })

  it('does not merge a delayed story update into another workspace', async () => {
    let settle!: () => void
    vi.mocked(updateInteractiveStory).mockImplementation(() => new Promise(resolve => { settle = () => resolve({ ...story(), title: 'Stale title' }) }))
    const result = await renderWith(null)
    fireEvent.click(screen.getByTestId('tuning-narrative'))
    result.rerender(<GameSettingsView projectId="project-2" visible onOpenBooks={vi.fn()} onOpenAssistant={vi.fn()} />)
    await act(async () => settle())
    expect(useInteractiveStore.getState().stories.find(item => item.id === 'story-1')?.title).not.toBe('Stale title')
  })

  it('opens the rules section for a rule system link', async () => {
    const { nav } = await renderWith(intent('rule_system'))
    await waitFor(() => expect(activeSection(nav)).toBe('rules'))
  })

  it('opens the narrative section for a style link', async () => {
    const { nav } = await renderWith(intent('narrative_style'))
    await waitFor(() => expect(activeSection(nav)).toBe('narrative'))
  })

  // Skills and agent profiles have no player-facing control of their own. The
  // assistant is the only surface that can still change them, so that is where
  // their links land.
  it('does not create a second assistant section for an assistant resource', async () => {
    const { nav } = await renderWith(intent('skill'))
    expect(activeSection(nav)).not.toBe('assistant')
    expect(screen.queryByTestId('config-manager-chat')).not.toBeInTheDocument()
  })

  // Narration, rules and media only exist while a story is open. With no story
  // there is nothing to scroll to, and the assistant is still the right place.
  it('keeps world defaults reachable when no story is open', async () => {
    storyIndex.stories = []
    storyIndex.current_story_id = ''
    // A world pack keeps the page out of its no-story empty state, which is the
    // state this fallback is about: the assistant is still reachable there.
    packsMock.packs = [{
      id: 'pack-1', name: '测试世界', title: '测试世界', world_count: 1, character_count: 1, builtin: false,
    }]
    useInteractiveStore.getState().setStories([], '')
    const { nav } = await renderWith(intent('rule_system'))
    await waitFor(() => expect(activeSection(nav)).toBe('world'))
  })

  // The router defers which route it presents, so the link can arrive while this
  // page is still hidden. Scrolling a hidden container measures zero heights and
  // would land nowhere, so the intent has to survive until the page is in front.
  it('waits for the page to come forward before scrolling', async () => {
    const { nav, rerender } = await renderWith(intent('image_preset'), false)
    // Let every pending effect settle while the page is still hidden. If the
    // link were consumed here it would scroll a container with no layout, so the
    // section must still be the one the page opens on.
    await act(async () => { await Promise.resolve() })
    expect(activeSection(nav)).toBe('story')

    rerender(view(intent('image_preset'), true))
    await waitFor(() => expect(activeSection(nav)).toBe('media'))
  })
})

it('recognizes a custom cultivation module profile without enabling the setting in another world', async () => {
  const custom = { ...story(), module_refs: { actor_state_id: 'world-actor-state-authored' }, pack_binding: { pack_id: 'custom', scope: 'pack:custom', gameplay: { status_template: 'xiuxian-state', growth: 'cultivation', combat: 'cultivation', communication: 'cultivation-signal', opening: 'scene' } } }
  storyIndex.stories = [custom]
  useInteractiveStore.getState().setStories([custom], 'story-1')
  const mounted = await renderWith(null)
  expect(await screen.findByTestId('intimacy-model-setting')).toBeInTheDocument()
  const other = { ...custom, pack_binding: { ...custom.pack_binding, gameplay: { ...custom.pack_binding.gameplay, status_template: 'fantasy-state-v1', growth: 'fantasy', combat: 'fantasy' } } }
  storyIndex.stories = [other]
  act(() => useInteractiveStore.getState().setStories([other], 'story-1'))
  mounted.rerender(view(null))
  expect(screen.queryByTestId('intimacy-model-setting')).not.toBeInTheDocument()
})

it('reads a legacy custom cultivation schema without borrowing another story schema', async () => {
  setConfiguredLocale('zh-CN')
  const legacy = { ...story(), module_refs: { actor_state_id: 'world-actor-state-legacy' } }
  storyIndex.stories = [legacy]
  useInteractiveStore.setState({ stories: [legacy], currentStoryId: 'story-1', currentBranchId: 'main', snapshot: null })
  vi.mocked(getInteractiveSnapshot).mockResolvedValueOnce({
    story_id: 'story-1', branch_id: 'main', turns: [], state: {},
    graph: { nodes: [], branches: [] }, turn_count: 0, turn_start: 0, has_earlier_turns: false,
    actor_state_schema: { version: 1, revision: 1, system: { templates: [{ id: 'protagonist', name: '修行者', fields: [{ name: '境界', type: 'string', value_format: 'stage' }] }] } },
  })
  const mounted = await renderWith(null)
  expect(await screen.findByTestId('intimacy-model-setting')).toBeInTheDocument()
  const other = { ...story(), id: 'story-2', module_refs: { actor_state_id: 'world-actor-state-unknown' } }
  storyIndex.stories = [other]
  act(() => useInteractiveStore.setState({ stories: [other], currentStoryId: 'story-2', currentBranchId: 'main' }))
  mounted.rerender(view(null))
  expect(screen.queryByTestId('intimacy-model-setting')).not.toBeInTheDocument()
})
