import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ConversationConfigController, ConversationConfigSnapshot } from '@/features/conversation-config/types'
import type { LoreItem } from '@/lib/api'
import type { GamePlanningTemplate, StorySummary, Teller } from '../types'
import { NewStorySetupPanel } from './NewStorySetupPanel'

const settingsMocks = vi.hoisted(() => ({
  fetchSettings: vi.fn(),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: settingsMocks.fetchSettings,
}))

vi.mock('../api', () => ({
  getActorStates: vi.fn().mockResolvedValue([{ id: 'state-basic', name: '基础状态' }]),
  getEventPackages: vi.fn().mockResolvedValue([]),
  getRuleSystems: vi.fn().mockResolvedValue([{ id: 'd20', name: 'D20' }]),
}))

vi.mock('@/features/agents/CustomAgentSelect', () => ({
  CustomAgentSelect: () => <div data-testid="custom-agent-select" />,
}))

const planningTemplate: GamePlanningTemplate = {
  version: 1,
  id: 'adventure',
  name: '冒险',
  description: '',
  sections: [{ id: 'direction', title: '方向', description: '规划方向。' }],
  custom: false,
}

const teller: Teller = {
  version: 1,
  id: 'cinematic',
  name: '电影化',
  description: '',
  context_policy: {} as Teller['context_policy'],
  slots: [],
  custom: false,
}

const loreCharacter: LoreItem = {
  id: 'hero',
  enabled: true,
  type: 'character',
  type_source: 'manual',
  name: '林川',
  importance: 'major',
  load_mode: 'auto',
  tags: ['航海士', '主角'],
  brief_description: '失忆的领航员',
  keywords: [],
  content: '林川熟悉旧港的每一条水道。',
  created_at: '2026-08-30T00:00:00Z',
  updated_at: '2026-08-31T00:00:00Z',
}

const alternateCharacter: LoreItem = {
  ...loreCharacter,
  id: 'companion',
  name: '顾岚',
  tags: ['同伴'],
  brief_description: '冷静的机关师',
  content: '顾岚擅长破解遗迹机关。',
}

function conversationConfigController(): ConversationConfigController {
  const snapshot: ConversationConfigSnapshot = {
    agent_kind: 'interactive_story',
    profile_id: 'default',
    thinking_level: 'medium',
    approval_mode: 'write',
    revision: 0,
  }
  return {
    snapshot,
    initialized: true,
    loading: false,
    saving: false,
    error: null,
    patch: vi.fn().mockResolvedValue(true),
    reload: vi.fn().mockResolvedValue(snapshot),
  }
}

describe('NewStorySetupPanel', () => {
  beforeEach(() => {
    settingsMocks.fetchSettings.mockResolvedValue({
      effective: {
        openai_model: 'test-model',
        // A profile with a model name is not enough on its own: the endpoint
        // still needs a key, which is what the setup panel checks before it
        // offers the picker.
        openai_api_key: 'test-key',
        model_profiles: [
          { id: 'default', name: '默认模型', endpoint_id: 'default', model: 'test-model' },
          { id: 'creative', name: '创意模型', endpoint_id: 'default', model: 'creative-model' },
        ],
      },
    })
  })

  it('submits a Lore protagonist snapshot source and a player-written opening from one start flow', async () => {
    const user = userEvent.setup()
    const onCreate = vi.fn().mockResolvedValue(undefined)
    const controller = conversationConfigController()
    render(
      <NewStorySetupPanel
        projectId="project-1"
        tellers={[teller]}
        planningTemplates={[planningTemplate]}
        loreItems={[loreCharacter, alternateCharacter]}
        conversationConfig={controller}
        onCreate={onCreate}
      />,
    )

    expect(screen.queryByLabelText('故事线名称')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('故事简介（可选）')).not.toBeInTheDocument()
    // A start has two shapes now: the model writes the first scene, or the
    // player does. The world's preset library is no longer a third door.
    expect(screen.getByRole('tab', { name: 'AI 生成' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByRole('tab', { name: /世界预设/ })).not.toBeInTheDocument()
    expect(screen.getByText('林川')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '更换' }))
    expect(await screen.findByRole('option', { name: /林川/ })).toBeInTheDocument()
    await user.click(screen.getByRole('option', { name: /顾岚/ }))
    await user.click(screen.getByRole('tab', { name: '自定义' }))
    await user.type(screen.getByRole('textbox'), '港口的灯逐盏熄灭。')
    // The model and thinking strength are the story's own settings and are
    // edited on the game settings page; starting carries them through as-is.
    expect(screen.queryByRole('combobox', { name: '模型配置' })).not.toBeInTheDocument()
    expect(controller.patch).not.toHaveBeenCalled()
    expect(onCreate).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: '开始故事' }))

    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1))
    expect(onCreate.mock.calls[0]?.[0]).toMatchObject({
      title: '',
      origin: '',
      profile_id: 'default',
      thinking_level: 'medium',
      protagonist: { mode: 'lore', source_lore_item_id: 'companion' },
      planning_template_id: planningTemplate.id,
      opening: { mode: 'custom', custom_text: '港口的灯逐盏熄灭。' },
      check_settings: { difficulty_shift: 0, roll_modifier: 0, rule_state_consumption_mode: 'hybrid_auto', rule_visibility_mode: 'audit_only' },
      image_settings: { mode: 'manual', interval_turns: 3, preset_id: 'realistic' },
    })
  })

  it('keeps the Lore choice active and starts on the AI path when the player switches back from a draft', async () => {
    const user = userEvent.setup()
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(
      <NewStorySetupPanel
        projectId="project-1"
        tellers={[teller]}
        planningTemplates={[planningTemplate]}
        loreItems={[alternateCharacter]}
        conversationConfig={conversationConfigController()}
        onCreate={onCreate}
      />,
    )

    expect(screen.getByRole('radio', { name: '从资料库选择' })).toBeChecked()
    expect(screen.getByText(/没有带“主角”标签的角色/)).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'AI 生成' })).toHaveAttribute('aria-selected', 'true')

    // A draft typed under the custom tab must not ride along once the player
    // picks AI: the model writes the scene, and the stale text is a draft, not
    // a request.
    await user.click(screen.getByRole('tab', { name: '自定义' }))
    await user.type(screen.getByRole('textbox'), '雨夜里的旧城。')
    await user.click(screen.getByRole('tab', { name: 'AI 生成' }))
    await user.click(screen.getByRole('button', { name: '开始故事' }))

    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1))
    expect(onCreate.mock.calls[0]?.[0]).toMatchObject({
      protagonist: { mode: 'default' },
      opening: { mode: 'ai' },
    })
    expect(onCreate.mock.calls[0]?.[0]).not.toHaveProperty('opening.custom_text')
  })

  it('summarises the story settings and points at the game settings page', async () => {
    render(
      <NewStorySetupPanel
        projectId="project-1"
        tellers={[teller]}
        planningTemplates={[planningTemplate]}
        loreItems={[]}
        conversationConfig={conversationConfigController()}
        onOpenPresets={vi.fn()}
        onCreate={vi.fn()}
      />,
    )

    expect(await screen.findByText('这一局的写法与规则')).toBeInTheDocument()
    expect(screen.getByText('规划开启 · 检定开启 · 手动配图')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '去游戏设置调整' })).toBeEnabled()
    // Editing those settings is the game settings page's job, so none of their
    // controls stay here where they would be a second, competing copy.
    expect(screen.queryByRole('combobox', { name: '模型配置' })).not.toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: '思考强度' })).not.toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: '游戏规划' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '回合判定' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '互动图像' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '状态面板' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /高级设置/ })).not.toBeInTheDocument()
    expect(screen.queryByText('主舞台展示')).not.toBeInTheDocument()
    expect(screen.getByTestId('story-setup-footer')).toHaveClass('shrink-0')
  })

  it('submits every check default when a released story summary omits zero values', async () => {
    const user = userEvent.setup()
    const onCreate = vi.fn().mockResolvedValue(undefined)
    const story: StorySummary = {
      id: 'released-story',
      title: '雾港旧事',
      title_source: 'pending',
      origin: '',
      protagonist: { mode: 'lore', name: '林川', profile: loreCharacter.content, source_lore_item_id: loreCharacter.id },
      story_teller_id: teller.id,
      planning_template_id: planningTemplate.id,
      planning_mode: 'enabled',
      reply_target_chars: 2000,
      choice_count: 5,
      opening: { mode: 'ai' },
      check_settings: {
        rule_state_consumption_mode: 'hybrid_auto',
        rule_visibility_mode: 'audit_only',
      },
      created_at: '2026-08-30T00:00:00Z',
      updated_at: '2026-08-30T00:00:00Z',
      branches: 1,
      events: 0,
      turn_count: 0,
    }

    render(
      <NewStorySetupPanel
        projectId="project-1"
        story={story}
        tellers={[teller]}
        planningTemplates={[planningTemplate]}
        loreItems={[loreCharacter]}
        conversationConfig={conversationConfigController()}
        onCreate={onCreate}
      />,
    )

    await user.click(screen.getByRole('button', { name: '开始故事' }))

    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1))
    expect(onCreate.mock.calls[0]?.[0]).toMatchObject({
      check_settings: {
        difficulty_shift: 0,
        roll_modifier: 0,
        rule_state_consumption_mode: 'hybrid_auto',
        rule_visibility_mode: 'audit_only',
      },
    })
  })

  it('waits for the effective opening runtime configuration before enabling start', async () => {
    const loadingConfig: ConversationConfigController = {
      ...conversationConfigController(),
      snapshot: null,
      initialized: false,
      loading: true,
    }
    const props = {
      projectId: 'project-1',
      tellers: [teller],
      planningTemplates: [planningTemplate],
      loreItems: [loreCharacter],
      onCreate: vi.fn(),
    }
    const { rerender } = render(<NewStorySetupPanel {...props} conversationConfig={loadingConfig} />)

    expect(screen.getByRole('button', { name: /加载中/ })).toBeDisabled()

    rerender(<NewStorySetupPanel {...props} conversationConfig={conversationConfigController()} />)

    await waitFor(() => expect(screen.getByRole('button', { name: '开始故事' })).toBeEnabled())
    expect(screen.queryByRole('combobox', { name: '模型配置' })).not.toBeInTheDocument()
  })

  it('preserves released story metadata when resuming setup without exposing the old fields', async () => {
    const user = userEvent.setup()
    const onCreate = vi.fn().mockResolvedValue(undefined)
    const story: StorySummary = {
      id: 'legacy-story',
      title: '雾港旧事',
      title_source: 'user',
      origin: '必须保留的旧故事设想。',
      protagonist: { mode: 'lore', name: '林川', profile: loreCharacter.content, source_lore_item_id: loreCharacter.id },
      story_teller_id: teller.id,
      planning_template_id: planningTemplate.id,
      planning_mode: 'enabled',
      reply_target_chars: 2000,
      choice_count: 5,
      opening: { mode: 'ai' },
      created_at: '2026-08-30T00:00:00Z',
      updated_at: '2026-08-30T00:00:00Z',
      branches: 1,
      events: 0,
      turn_count: 0,
    }

    render(
      <NewStorySetupPanel
        projectId="project-1"
        story={story}
        tellers={[teller]}
        planningTemplates={[planningTemplate]}
        loreItems={[loreCharacter]}
        conversationConfig={conversationConfigController()}
        onCreate={onCreate}
      />,
    )

    expect(screen.queryByDisplayValue('雾港旧事')).not.toBeInTheDocument()
    expect(screen.queryByDisplayValue('必须保留的旧故事设想。')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '开始故事' }))

    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1))
    expect(onCreate.mock.calls[0]?.[0]).toMatchObject({
      title: '雾港旧事',
      origin: '必须保留的旧故事设想。',
    })
  })
  it('points at the model settings instead of offering a picker when nothing can write a story yet', async () => {
    settingsMocks.fetchSettings.mockResolvedValue({ effective: { model_profiles: [] } })
    render(
      <NewStorySetupPanel
        projectId="project-1"
        tellers={[teller]}
        planningTemplates={[planningTemplate]}
        loreItems={[loreCharacter]}
        conversationConfig={conversationConfigController()}
        onCreate={vi.fn()}
      />,
    )

    expect(await screen.findByText('还差一个能写故事的模型')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '去设置模型' })).toBeInTheDocument()
  })
})
