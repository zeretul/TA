import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import type { StorySummary } from '../../types'
import { IntimacyModelSetting } from './IntimacyModelSetting'

const fetchSettings = vi.fn()
vi.mock('@/features/settings/api', () => ({ fetchSettings: () => fetchSettings() }))

const catalog = {
  effective: {
    openai_model: 'story-model',
    model_profiles: [{ id: 'p-cultivation', name: '双修专用', model: 'cultivation-model' }],
  },
  user: {},
  workspace: {},
}

function story(overrides: Partial<StorySummary> = {}): StorySummary {
  return {
    id: 'story-1',
    title: '测试故事',
    origin: '',
    protagonist: { mode: 'default' },
    story_teller_id: 'cinematic-shots',
    planning_template_id: 'adventure',
    planning_mode: 'disabled',
    module_refs: { actor_state_id: 'xiuxian-state' },
    reply_target_chars: 2000,
    choice_count: 5,
    image_settings: { mode: 'manual', interval_turns: 3, preset_id: 'realistic' },
    check_settings: { difficulty_shift: 0, roll_modifier: 0 },
    opening: { mode: 'ai' },
    created_at: '',
    updated_at: '',
    branches: 1,
    events: 0,
    turn_count: 1,
    ...overrides,
  }
}

beforeEach(() => {
  setConfiguredLocale('zh-CN')
  fetchSettings.mockReset()
  fetchSettings.mockResolvedValue(catalog)
})

describe('IntimacyModelSetting', () => {
  // Radix throws on an item whose value is the empty string, and the throw takes
  // the whole settings page down with it, so "not chosen yet" has to be a real
  // value in the menu while still saving as an empty profile id.
  it('renders the unconfigured row without crashing and keeps the thinking level locked', async () => {
    render(<IntimacyModelSetting story={story()} onUpdate={vi.fn()} />)

    const trigger = await screen.findByRole('combobox', { name: '双修模型' })
    expect(trigger).toHaveTextContent('不指定')
    expect(screen.getByRole('combobox', { name: '双修思考强度' })).toBeDisabled()
  })

  it('clears the binding back to an empty profile id when "not specified" is picked', async () => {
    const user = userEvent.setup()
    const onUpdate = vi.fn().mockResolvedValue(undefined)
    render(<IntimacyModelSetting story={story({ intimacy_settings: { profile_id: 'p-cultivation', thinking_level: 'low' } })} onUpdate={onUpdate} />)

    const profile = await screen.findByRole('combobox', { name: '双修模型' })
    await waitFor(() => expect(profile).toHaveTextContent('双修专用'))
    await user.click(profile)
    await user.click(await screen.findByRole('option', { name: '不指定' }))
    expect(onUpdate).toHaveBeenCalledExactlyOnceWith({ intimacy_settings: { profile_id: '', thinking_level: 'low' } })
  })

  it('saves the chosen profile and unlocks the thinking level once one is bound', async () => {
    const user = userEvent.setup()
    const onUpdate = vi.fn().mockResolvedValue(undefined)
    render(<IntimacyModelSetting story={story({ intimacy_settings: { profile_id: 'p-cultivation', thinking_level: 'off' } })} onUpdate={onUpdate} />)

    const profile = await screen.findByRole('combobox', { name: '双修模型' })
    await waitFor(() => expect(profile).toHaveTextContent('双修专用'))
    const thinking = screen.getByRole('combobox', { name: '双修思考强度' })
    expect(thinking).toBeEnabled()

    await user.click(thinking)
    await user.click(await screen.findByRole('option', { name: '低' }))
    expect(onUpdate).toHaveBeenCalledExactlyOnceWith({ intimacy_settings: { profile_id: 'p-cultivation', thinking_level: 'low' } })
  })
})
