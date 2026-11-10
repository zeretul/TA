import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
vi.mock('@/features/interactive/api', () => ({ getRuleSystems: vi.fn().mockResolvedValue([{ id: 'recommended', name: 'World checks', actor_state_id: 'world-state' }, { id: 'd20', name: 'D20' }, { id: 'foreign', name: 'Foreign', actor_state_id: 'other' }]) }))
import { WorldSettingsEditor, worldSettingsValue, storyPackSettingsFrom } from './WorldSettingsEditor'
import { RECOMMENDED_NARRATIVE_STYLE_IDS } from '@/features/interactive/narrative-style'
import type { StoryPackSummary } from '@/lib/api-client'
import type { Teller, GamePlanningTemplate } from '@/features/interactive/types'

function renderEditor(style = 'source-drama') {
  const tellers = [...RECOMMENDED_NARRATIVE_STYLE_IDS, 'deadpan-humour', 'classic'].map(id => ({ id, name: id })) as Teller[]
  return render(<WorldSettingsEditor pack={{ id: 'world' } as StoryPackSummary}
    value={{ ...worldSettingsValue(undefined), styleMain: style, planningTemplateId: 'custom-plan' }} onChange={vi.fn()}
    tellers={tellers} planningTemplates={[{ id: 'custom-plan', name: 'Planning title', description: 'Long planning description that belongs below the trigger.' } as GamePlanningTemplate]}
    modules={[]} saving={false} />)
}

describe('world defaults selection', () => {
  it('offers six styles even when old files remain installed', () => {
    renderEditor()
    fireEvent.click(screen.getByRole('combobox', { name: '主叙事' }))
    expect(screen.getAllByRole('option')).toHaveLength(6)
    expect(screen.queryByRole('option', { name: 'deadpan-humour' })).not.toBeInTheDocument()
  })
  it('keeps a saved legacy selection without offering every other legacy style', () => {
    renderEditor('deadpan-humour')
    expect(screen.getByRole('combobox', { name: '主叙事' })).toHaveTextContent('deadpan-humour')
    fireEvent.click(screen.getByRole('combobox', { name: '主叙事' }))
    expect(screen.getAllByRole('option')).toHaveLength(7)
    expect(screen.queryByRole('option', { name: 'classic' })).not.toBeInTheDocument()
  })
  it('shows only the planning title inside the fixed-height trigger', () => {
    renderEditor()
    fireEvent.click(screen.getByRole('button', { name: /故事规划/ }))
    const trigger = screen.getByRole('combobox', { name: '规划模板' })
    expect(trigger).toHaveTextContent('Planning title')
    expect(within(trigger).queryByText(/Long planning description/)).not.toBeInTheDocument()
    expect(screen.getByText(/Long planning description/)).toBeInTheDocument()
  })
})



it('saves a compatible alternative while retaining the world recommendation', async () => {
  const onChange = vi.fn()
  render(<WorldSettingsEditor pack={{ id: 'world' } as StoryPackSummary}
    value={worldSettingsValue(undefined)} onChange={onChange} tellers={[]} planningTemplates={[]}
    modules={[{ kind: 'rule_system', id: 'recommended', name: 'World checks' }, { kind: 'actor_state', id: 'world-state', name: 'World state' }]} saving={false} />)
  fireEvent.click(screen.getByRole('button', { name: /检定与角色状态/ }))
  fireEvent.click(screen.getByRole('combobox', { name: '检定系统' }))
  fireEvent.click(await screen.findByRole('option', { name: 'D20' }))
  expect(storyPackSettingsFrom(onChange.mock.calls[0][0]).rule_system_id).toBe('d20')
  expect(screen.queryByRole('option', { name: 'Foreign' })).not.toBeInTheDocument()
  expect(screen.getByText(/世界推荐.*World checks/)).toBeInTheDocument()
})
