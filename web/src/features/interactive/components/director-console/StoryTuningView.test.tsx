import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GamePlanningTemplate, StorySummary, Teller } from '../../types'
import { StoryTuningView } from './StoryTuningView'

vi.mock('../../api', () => ({
  getActorStates: vi.fn().mockResolvedValue([{ id: 'state-basic', name: '基础状态' }]),
  getRuleSystems: vi.fn().mockResolvedValue([
    { id: 'd20', name: 'D20' },
    { id: 'xiuxian-rules', name: '修真检定' },
    { id: 'detective-rules', name: '侦探检定', actor_state_id: 'detective-state' },
  ]),
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
  id: 'cinematic-shots',
  name: '电影叙事',
  description: '',
  context_policy: {} as Teller['context_policy'],
  slots: [],
  custom: false,
}

const sliceOfLife: Teller = { ...teller, id: 'slice-banter', name: '温情日常' }
const absorbed: Teller = { ...teller, id: 'dialogue-piece', name: '对白戏' }

function story(turnCount = 0): StorySummary {
  return {
    id: 'story-1',
    title: '测试故事',
    origin: '',
    protagonist: { mode: 'default' },
    story_teller_id: teller.id,
    planning_template_id: planningTemplate.id,
    planning_mode: 'disabled',
    module_refs: { narrative_style_id: 'cinematic-shots', rule_system_id: 'd20', actor_state_id: 'state-basic', image_preset_id: 'realistic' },
    reply_target_chars: 2000,
    choice_count: 5,
    image_settings: { mode: 'manual', interval_turns: 3, preset_id: 'realistic' },
    check_settings: { difficulty_shift: 0, roll_modifier: 0 },
    opening: { mode: 'ai' },
    created_at: '',
    updated_at: '',
    branches: 1,
    events: 0,
    turn_count: turnCount,
  }
}

describe('StoryTuningView', () => {
  const onUpdate = vi.fn().mockResolvedValue(undefined)

  beforeEach(() => onUpdate.mockClear())

  it.each(['story', 'project'])('isolates a pending numeric draft when the %s changes with the same saved value', async (target) => {
    let finish!: () => void
    const update = vi.fn(() => new Promise<void>((resolve) => { finish = resolve }))
    const props = { story: story(), projectId: 'project-a', planningTemplates: [planningTemplate], tellers: [teller], imagePresets: [], stateDisplayPreference: 'preview' as const, onStateDisplayPreferenceChange: vi.fn(), onUpdate: update, group: 'narrative' as const }
    const view = render(<StoryTuningView {...props} />)
    const field = screen.getByRole('spinbutton', { name: '候选行动数' })
    fireEvent.change(field, { target: { value: '6' } })
    fireEvent.blur(field)
    await waitFor(() => expect(update).toHaveBeenCalledWith({ choice_count: 6 }))
    expect(field).toBeDisabled()
    view.rerender(<StoryTuningView {...props} projectId={target === 'project' ? 'project-b' : props.projectId} story={target === 'story' ? { ...props.story, id: 'story-2' } : props.story} />)
    expect(screen.getByRole('spinbutton', { name: '候选行动数' })).toHaveValue(5)
    expect(screen.getByRole('spinbutton', { name: '候选行动数' })).toBeEnabled()
    await act(async () => finish())
    expect(screen.getByRole('spinbutton', { name: '候选行动数' })).toHaveValue(5)
    expect(update).toHaveBeenCalledTimes(1)
  })

  it('shows the narrative-settings groups without a per-story supplement field', async () => {
    render(<StoryTuningView story={story()} planningTemplates={[planningTemplate]} tellers={[teller]} imagePresets={[{ version: 1, id: 'realistic', name: '写实', description: '', custom: false }]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onPlanningTemplateChange={vi.fn()} onUpdate={onUpdate} />)

    expect(screen.getByRole('heading', { name: '文风' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '剧情规划' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '回合判定' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '互动图像' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '状态面板' })).toBeInTheDocument()
    // The per-story "narrative requirements" layer was removed: writing
    // direction now comes from one shared writing-preferences document plus the
    // chosen style, so no free-text supplement may reappear here.
    expect(screen.queryByRole('heading', { name: '叙事要求' })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('本局叙事要求')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('switch', { name: '游戏规划' }))
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ planning_mode: 'enabled' }))
  })

  // UI12: a story's event source belongs to the world pack. The tuning surface
  // must not offer a second, independent event-package selector.
  it('does not offer a story-level event-package selector', async () => {
    render(<StoryTuningView story={story()} planningTemplates={[planningTemplate]} tellers={[teller]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    expect(screen.queryByText('事件包')).not.toBeInTheDocument()
  })

  it('offers only the recommended styles', async () => {
    render(<StoryTuningView story={story()} planningTemplates={[planningTemplate]} tellers={[teller, sliceOfLife, absorbed]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    fireEvent.click(screen.getByLabelText('叙事风格'))
    expect(screen.getByRole('option', { name: '温情日常' })).toBeInTheDocument()
    // A merged-away id is still resolvable by the backend for old stories, but
    // listing it here would offer twelve names for six styles again.
    expect(screen.queryByRole('option', { name: '对白戏' })).not.toBeInTheDocument()
  })

  it('keeps a story running on an absorbed style selectable', async () => {
    render(<StoryTuningView story={{ ...story(), story_teller_id: 'dialogue-piece' }} planningTemplates={[planningTemplate]} tellers={[teller, absorbed]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    expect(screen.getByLabelText('叙事风格')).toHaveTextContent('对白戏')
    fireEvent.click(screen.getByLabelText('叙事风格'))
    expect(screen.getByRole('option', { name: '对白戏' })).toBeInTheDocument()
  })

  it('switches only the planning template and leaves story modules untouched', async () => {
    const onPlanningTemplateChange = vi.fn().mockResolvedValue(undefined)
    const alternateTemplate: GamePlanningTemplate = {
      ...planningTemplate,
      id: 'mystery',
      name: '调查',
      custom: true,
    }

    render(
      <StoryTuningView
        story={story()}
        planningTemplates={[planningTemplate, alternateTemplate]}
        tellers={[teller]}
        imagePresets={[]}
        stateDisplayPreference="preview"
        onStateDisplayPreferenceChange={vi.fn()}
        onPlanningTemplateChange={onPlanningTemplateChange}
        onUpdate={onUpdate}
      />,
    )

    fireEvent.click(screen.getByLabelText('规划模板'))
    fireEvent.click(screen.getByRole('option', { name: '调查' }))

    await waitFor(() => expect(onPlanningTemplateChange).toHaveBeenCalledWith('mystery'))
    expect(onUpdate).not.toHaveBeenCalled()
  })

  it('shows complete compact values and saves reply-length presets or custom values', async () => {
    render(<StoryTuningView story={story()} planningTemplates={[planningTemplate]} tellers={[teller]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    expect(screen.getByLabelText('叙事风格')).toHaveTextContent('电影叙事')
    const replyLength = screen.getByRole('button', { name: '每回合目标字数' })
    expect(replyLength).toHaveTextContent('2000')

    fireEvent.click(replyLength)
    fireEvent.click(screen.getByRole('radio', { name: '600' }))
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ reply_target_chars: 600 }))

    onUpdate.mockClear()
    await waitFor(() => expect(replyLength).not.toBeDisabled())
    fireEvent.click(replyLength)
    fireEvent.change(screen.getByLabelText('自定义字数'), { target: { value: '1750' } })
    fireEvent.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ reply_target_chars: 1750 }))
  })

  it('changes checks after opening without rebuilding state', async () => {
    render(<StoryTuningView story={story(1)} planningTemplates={[planningTemplate]} tellers={[teller]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    expect(await screen.findByLabelText('规则系统')).not.toBeDisabled()
    fireEvent.click(screen.getByLabelText('规则系统'))
    fireEvent.click(await screen.findByRole('option', { name: '修真检定' }))
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ module_refs: { ...story(1).module_refs, event_package_ids: ['default'], rule_system_id: 'xiuxian-rules', rule_system_disabled: false } }))
    expect(onUpdate.mock.calls[0][0]).not.toHaveProperty('state_schema_policy')
    expect(screen.getByLabelText('全局难度')).not.toBeDisabled()
  })

  // The three purposes are chosen independently, and the two override purposes
  // can also follow the main narrative. That is a real choice, distinct from
  // leaving the purpose alone, so the two have to reach the API differently.
  it('shows an unset purpose as following the main narrative', async () => {
    render(<StoryTuningView story={{ ...story(), style_purposes: { dialogue: 'cinematic-shots' } }} planningTemplates={[planningTemplate]} tellers={[teller]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    expect(screen.getByLabelText('对白与聊天')).toHaveTextContent('电影叙事')
    expect(screen.getByLabelText('战斗描写')).toHaveTextContent('跟随主叙事')
  })

  it('saves a purpose override and can clear it back to inheriting', async () => {
    render(<StoryTuningView story={{ ...story(), style_purposes: { dialogue: 'cinematic-shots' } }} planningTemplates={[planningTemplate]} tellers={[teller, sliceOfLife]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    fireEvent.click(screen.getByLabelText('对白与聊天'))
    fireEvent.click(screen.getByRole('option', { name: '温情日常' }))
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ style_purposes: { dialogue: 'slice-banter' } }))

    onUpdate.mockClear()
    await waitFor(() => expect(screen.getByLabelText('对白与聊天')).not.toBeDisabled())
    fireEvent.click(screen.getByLabelText('对白与聊天'))
    fireEvent.click(screen.getByRole('option', { name: '跟随主叙事' }))
    // An empty string is what "follow the main narrative" means on the wire.
    // Omitting the field would say "leave it as it is", which is the opposite.
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ style_purposes: { dialogue: '' } }))
  })

  // A purpose can outlive its style: a story may still point at an id the
  // recommended set absorbed. The purpose select has to show that id, or it
  // would render a value it has no option for and look unset.
  it('keeps an absorbed purpose value selectable', async () => {
    render(<StoryTuningView story={{ ...story(), style_purposes: { dialogue: 'dialogue-piece' } }} planningTemplates={[planningTemplate]} tellers={[teller, absorbed]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    expect(screen.getByLabelText('对白与聊天')).toHaveTextContent('对白戏')
    fireEvent.click(screen.getByLabelText('对白与聊天'))
    expect(screen.getByRole('option', { name: '对白戏' })).toBeInTheDocument()
  })

  it('changes only the purpose the player touched', async () => {
    render(<StoryTuningView story={{ ...story(), style_purposes: { dialogue: 'cinematic-shots', combat: 'cinematic-shots' } }} planningTemplates={[planningTemplate]} tellers={[teller]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    fireEvent.click(screen.getByLabelText('战斗描写'))
    fireEvent.click(screen.getByRole('option', { name: '跟随主叙事' }))

    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1))
    expect(onUpdate).toHaveBeenCalledWith({ style_purposes: { combat: '' } })
  })

  // The world binds one rule system, and that binding is its suggestion. The
  // list has to say which entry is the suggestion, or the player cannot tell
  // the world's proposal from the alternatives once the list is open.
  it('splits the rule list into the world recommendation and the rest', async () => {
    const onRecommended = { ...story(), module_refs: { ...story().module_refs, rule_system_id: 'xiuxian-rules' } }
    render(<StoryTuningView story={onRecommended} recommendedRuleSystemId="xiuxian-rules" planningTemplates={[planningTemplate]} tellers={[teller]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    fireEvent.click(await screen.findByLabelText('规则系统'))
    await waitFor(() => expect(screen.getByRole('option', { name: '修真检定' })).toBeInTheDocument())
    expect(screen.getAllByText('世界推荐').length).toBeGreaterThan(0)
    expect(screen.getByText('其他规则')).toBeInTheDocument()
  })

  // The suggestion stays readable after the player picks something else, which
  // is the whole point of showing it beside the control rather than only inside
  // the list.
  it('keeps the world suggestion readable after the player picks another', async () => {
    render(<StoryTuningView story={story()} recommendedRuleSystemId="xiuxian-rules" planningTemplates={[planningTemplate]} tellers={[teller]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    await waitFor(() => expect(screen.getByText('修真检定')).toBeInTheDocument())
    // The picker itself still shows the player's own choice.
    expect(screen.getByLabelText('规则系统')).toHaveTextContent('D20')
  })

  // A rule system may declare the state system its checks read from. Offering
  // one on a story running a different state system would let the player build
  // a pairing whose checks can never fire.
  it('offers only the rule systems the current state system can run', async () => {
    render(<StoryTuningView story={story()} planningTemplates={[planningTemplate]} tellers={[teller]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    fireEvent.click(await screen.findByLabelText('规则系统'))
    await waitFor(() => expect(screen.getByRole('option', { name: '修真检定' })).toBeInTheDocument())
    expect(screen.queryByRole('option', { name: '侦探检定' })).not.toBeInTheDocument()
  })

  // A story may already run a pairing a preset no longer supports. Dropping the
  // option would render the value as unset, which looks like the player had
  // never chosen a rule system at all.
  it('keeps a rule system that falls out of scope selectable', async () => {
    const mismatched = { ...story(), module_refs: { ...story().module_refs, rule_system_id: 'detective-rules' } }
    render(<StoryTuningView story={mismatched} planningTemplates={[planningTemplate]} tellers={[teller]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    expect(await screen.findByLabelText('规则系统')).toHaveTextContent('侦探检定')
    fireEvent.click(screen.getByLabelText('规则系统'))
    expect(screen.getByRole('option', { name: '侦探检定' })).toBeInTheDocument()
  })
})
