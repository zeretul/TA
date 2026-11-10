import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { StorySummary, Teller } from './types'
import { useNarrativeStyleCatalog } from './use-narrative-style-catalog'

const getInteractiveTeller = vi.fn()
vi.mock('./api', () => ({
  getInteractiveTeller: (id: string) => getInteractiveTeller(id),
}))

const sourceDrama: Teller = {
  version: 1,
  id: 'source-drama',
  name: '朴实正剧',
  description: '',
  context_policy: {} as Teller['context_policy'],
  slots: [],
  custom: false,
}

// The server still resolves an absorbed id, but the list endpoint leaves it out
// because the file is never seeded. This is what one looks like when fetched by
// id: its own name, not the id it merged into.
const absorbedFetched: Teller = { ...sourceDrama, id: 'flippant-banter', name: '轻佻吐槽' }

function story(overrides: Partial<StorySummary> = {}): StorySummary {
  return {
    id: 'story-1',
    title: '测试故事',
    origin: '',
    protagonist: { mode: 'default' },
    story_teller_id: 'source-drama',
    planning_template_id: 'default',
    planning_mode: 'disabled',
    module_refs: { narrative_style_id: 'source-drama', rule_system_id: 'default', actor_state_id: 'default', image_preset_id: 'realistic' },
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
    ...overrides,
  }
}

describe('useNarrativeStyleCatalog', () => {
  // mockReset leaves the mock in a state where Vitest 4 reports the next
  // rejected promise as unhandled even when the hook catches it; every test here
  // sets its own implementation, so clearing the call history is enough.
  beforeEach(() => vi.clearAllMocks())

  it('returns the list untouched when the story names nothing missing', () => {
    const { result } = renderHook(() => useNarrativeStyleCatalog([sourceDrama], story()))
    expect(result.current).toEqual([sourceDrama])
    expect(getInteractiveTeller).not.toHaveBeenCalled()
  })

  // The defect this fixes: the settings picker filtered the list, the absorbed
  // id was not in it, and the select rendered the raw id instead of a name.
  it('fills in an absorbed style the story still runs on', async () => {
    getInteractiveTeller.mockResolvedValue(absorbedFetched)
    const { result } = renderHook(() => useNarrativeStyleCatalog([sourceDrama], story({ story_teller_id: 'flippant-banter' })))

    await waitFor(() => expect(result.current.map((item) => item.id)).toContain('flippant-banter'))
    expect(result.current.find((item) => item.id === 'flippant-banter')?.name).toBe('轻佻吐槽')
    expect(getInteractiveTeller).toHaveBeenCalledWith('flippant-banter')
  })

  it('fills in a purpose style as well as the main one', async () => {
    getInteractiveTeller.mockImplementation(async (id: string) => ({ ...sourceDrama, id, name: '旧文风 ' + id }))
    const { result } = renderHook(() => useNarrativeStyleCatalog([sourceDrama], story({
      story_teller_id: 'flippant-banter',
      style_purposes: { dialogue: 'dialogue-piece', combat: 'restrained-hard' },
    })))

    await waitFor(() => expect(result.current).toHaveLength(4))
    expect(result.current.map((item) => item.id).sort())
      .toEqual(['dialogue-piece', 'flippant-banter', 'restrained-hard', 'source-drama'])
  })

  it('leaves the list alone when the lookup fails', async () => {
    getInteractiveTeller.mockImplementation(() => Promise.reject(new Error('not found')))
    const { result } = renderHook(() => useNarrativeStyleCatalog([sourceDrama], story({ story_teller_id: 'flippant-banter' })))

    await waitFor(() => expect(getInteractiveTeller).toHaveBeenCalled())
    expect(result.current).toEqual([sourceDrama])
  })

  it('asks for each missing id once', async () => {
    getInteractiveTeller.mockResolvedValue(absorbedFetched)
    const { result, rerender } = renderHook(
      ({ current }: { current: StorySummary }) => useNarrativeStyleCatalog([sourceDrama], current),
      { initialProps: { current: story({ story_teller_id: 'flippant-banter' }) } },
    )
    await waitFor(() => expect(result.current).toHaveLength(2))
    rerender({ current: story({ story_teller_id: 'flippant-banter' }) })
    rerender({ current: story({ story_teller_id: 'flippant-banter' }) })
    expect(getInteractiveTeller).toHaveBeenCalledTimes(1)
  })
})
