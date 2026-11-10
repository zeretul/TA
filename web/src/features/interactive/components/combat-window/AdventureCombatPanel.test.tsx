import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { AdventureCombatPanel } from './AdventureCombatPanel'
import { setConfiguredLocale } from '@/i18n'

const mocks = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/lib/api-client', () => ({
  requestJSON: mocks.request,
  jsonHeaders: {},
  createAgentCommandID: () => 'round-command',
  APIError: class APIError extends Error { status = 409 },
}))
const state = {
  head: 'event-1', family: 'space', enemy_condition: 'wounded',
  actor: { health: 80, max_health: 100, energy: 50, max_energy: 100, skills: { shot: 1 }, inventory: {}, assets: {}, battle: { id: 'battle-1', scale: 'person', opponent_name: 'Opponent with a long known name', round: 1, status: 'active' } },
  skills: [{ id: 'shot', name: 'Precision shot', kind: 'attack', scale: 'person', cost: 10 }], items: [],
}
beforeEach(() => { localStorage.clear(); mocks.request.mockReset(); mocks.request.mockResolvedValue(state) })
const props = { storyId: 'story', branchId: 'branch', active: true, onClose: vi.fn() }

it('submits a learned technique against the read head, and closing makes no game request', async () => {
  const onClose = vi.fn()
  render(<AdventureCombatPanel {...props} onClose={onClose} />)
  await screen.findByText('Opponent with a long known name')
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:shot' } })
  fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith(expect.stringContaining('/adventure-action'), expect.objectContaining({ body: JSON.stringify({ command_id: 'round-command', expected_head: 'event-1', branch: 'branch', action: { action: 'battle_round', choice: 'skill', skill_id: 'shot' } }) })))
  await waitFor(() => expect(localStorage.length).toBe(0))
  const count = mocks.request.mock.calls.length
  fireEvent.click(screen.getByRole('button', { name: '收起星际交战' }))
  expect(onClose).toHaveBeenCalledOnce()
  expect(mocks.request).toHaveBeenCalledTimes(count)
})

it('resends the identical command after a lost answer and reopening', async () => {
  mocks.request.mockImplementation(async (_url: string, options?: object) => { if (options) throw new Error('connection lost'); return state })
  const view = render(<AdventureCombatPanel {...props} />)
  await screen.findByText('Opponent with a long known name')
  fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
  await screen.findByText(/尚未收到确认/)
  const original = mocks.request.mock.calls.find((call) => call[1])?.[1]
  view.unmount()
  render(<AdventureCombatPanel {...props} />)
  await screen.findByText('Opponent with a long known name')
  fireEvent.click(screen.getByRole('button', { name: '确认上一回合结果' }))
  await waitFor(() => expect(mocks.request.mock.calls.filter((call) => call[1])).toHaveLength(2))
  expect(mocks.request.mock.calls.filter((call) => call[1])[1][1]).toEqual(original)
})

it('does not offer fabricated encounters when the story has no battle', async () => {
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, battle: undefined } })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByText(/这里还没有交战/)
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
})

it('offers learned recovery techniques in the active combat scale', async () => {
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, skills: { recovery: 1, cleanse: 1 } }, skills: [
    { id: 'recovery', name: 'Recovery', kind: 'energy', power: 8, scale: 'person', cost: 0 },
    { id: 'cleanse', name: 'Cleanse', kind: 'antidote', power: 4, scale: 'person', cost: 2 },
  ] })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByRole('option', { name: /Recovery/ })
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:cleanse' } })
  expect(screen.getByText('解除毒素 4')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith(expect.stringContaining('/adventure-action'), expect.objectContaining({ body: expect.stringContaining('"skill_id":"cleanse"') })))
})

it('ignores an older story read that completes after changing the target', async () => {
  let resolveOld!: (value: typeof state) => void
  mocks.request.mockImplementation((url: string) => url.includes('/stories/story/')
    ? new Promise(resolve => { resolveOld = resolve })
    : Promise.resolve({ ...state, actor: { ...state.actor, battle: { ...state.actor.battle, opponent_name: 'New story opponent' } } }))
  const view = render(<AdventureCombatPanel {...props} />)
  view.rerender(<AdventureCombatPanel {...props} storyId="other" />)
  await screen.findByText('New story opponent')
  await act(async () => resolveOld(state))
  expect(screen.getByText('New story opponent')).toBeInTheDocument()
  expect(screen.queryByText('Opponent with a long known name')).not.toBeInTheDocument()
})

it('keeps a weakened actor out of physical actions while allowing an explicit unrewarded verbal standoff', async () => {
  mocks.request.mockResolvedValue({ ...state, family: 'wuxia', enemy_condition: 'softened', actor: { ...state.actor, soft_limbs: true } })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByText('Opponent with a long known name')
  expect(screen.getByRole('option', { name: '攻击' })).toBeDisabled()
  expect(screen.getByRole('option', { name: '撤离' })).toBeDisabled()
  expect(screen.getByRole('option', { name: '防守' })).toBeDisabled()
  expect(screen.getByRole('button', { name: '执行这一回合' })).toBeDisabled()
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'talk' } })
  fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith(expect.stringContaining('/adventure-action'), expect.objectContaining({ body: JSON.stringify({ command_id: 'round-command', expected_head: 'event-1', branch: 'branch', action: { action: 'battle_round', choice: 'talk' } }) })))
})

it('blocks source-tagged rushing footwork and offers acupuncture healing separately from enemy sealing', async () => {
  mocks.request.mockResolvedValue({ ...state, family: 'wuxia', actor: { ...state.actor, fog_area_id: 'source-fog', skills: { rush: 1, needles: 1 } }, skills: [
    { id: 'rush', name: '梯云纵', kind: 'pilot', scale: 'person', cost: 2, rush: true },
    { id: 'needles', name: '子午针灸经', kind: 'heal', scale: 'person', cost: 2, enemy_effects: [{ kind: 'seal', power: 1 }] },
  ] })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByText('Opponent with a long known name')
  expect(screen.getByRole('option', { name: /梯云纵/ })).toBeDisabled()
  expect(screen.getByRole('option', { name: /子午针灸经.*对敌封穴/ })).toBeEnabled()
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:needles:opponent' } })
  expect(screen.getByText('本轮封住突进身法')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith(expect.stringContaining('/adventure-action'), expect.objectContaining({ body: JSON.stringify({ command_id: 'round-command', expected_head: 'event-1', branch: 'branch', action: { action: 'battle_round', choice: 'skill', skill_id: 'needles', target_id: 'opponent' } }) })))
})

it('shows the actual own-health full-force price and its learned foundation mitigation', async () => {
  const skill = { id: 'seven', name: '七伤拳', kind: 'attack', scale: 'person', cost: 9, backlash: { health_percent: 8, injury: 6, foundation_skills: ['foundation'], foundation_rank: 2, mitigated_health_percent: 2, mitigated_injury: 1 } }
  mocks.request.mockResolvedValue({ ...state, family: 'wuxia', skills: [skill], actor: { ...state.actor, health: 100, max_health: 100, skills: { seven: 1 } } })
  const first = render(<AdventureCombatPanel {...props} />)
  await screen.findByText('Opponent with a long known name')
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:seven' } })
  expect(screen.getByText('全力催动自损生命8，伤势加深6。')).toBeInTheDocument()
  first.unmount()
  mocks.request.mockResolvedValue({ ...state, family: 'wuxia', skills: [skill], actor: { ...state.actor, health: 100, max_health: 100, skills: { seven: 1, foundation: 2 } } })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByText('Opponent with a long known name')
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:seven' } })
  expect(screen.getByText('全力催动自损生命2，伤势加深1。')).toBeInTheDocument()
  expect(mocks.request.mock.calls.every(call => !call[1])).toBe(true)
})

it('offers the actual charged mental-recovery tool without allowing shocked physical offense', async () => {
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, mental_corruption: 60, mental_shock: 1, mental_exposure_id: 'known-storm', item_charges: { stone: 2 }, inventory: { stone: 1 }, skills: { ward: 1 }, battle: { ...state.actor.battle, opponent_id: 'known-storm' } }, skills: [{ id: 'ward', name: '虚灵低语抵御法', kind: 'guard', scale: 'person', cost: 9, effects: [{ kind: 'mental_guard', power: 40 }] }], items: [{ id: 'stone', name: '祈愿石子', kind: 'tool', scale: 'person', effect: 'mental_restore', max_charges: 3 }] })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByText('Opponent with a long known name')
  expect(screen.getByRole('option', { name: '攻击' })).toBeDisabled()
  expect(screen.getByRole('option', { name: /虚灵低语抵御法/ })).toBeEnabled()
  expect(screen.getByRole('option', { name: /祈愿石子.*余2次/ })).toBeEnabled()
  expect(screen.getByRole('option', { name: /主动倾听/ })).toBeInTheDocument()
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'item:stone' } })
  fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith(expect.stringContaining('/adventure-action'), expect.objectContaining({ body: JSON.stringify({ command_id: 'round-command', expected_head: 'event-1', branch: 'branch', action: { action: 'battle_round', choice: 'item', item_id: 'stone' } }) })))
})

it('uses formal story time for zero-shield cooldown and disables a second overcharge', async () => {
  mocks.request.mockResolvedValue({ ...state, clock_seconds: 20, actor: { ...state.actor, inventory: { bracelet: 1, capacitor: 1, cell: 3 }, equipment: { 'person:armor': 'bracelet' }, item_charges: { bracelet: 2 }, item_ready_at: { bracelet: 140 }, skills: { boost: 1 } }, skills: [{ id: 'boost', name: '动能护盾增压', kind: 'guard', scale: 'person', cost: 9, required_tool: 'bracelet', materials: { capacitor: 1, cell: 3 }, effects: [{ kind: 'shield_boost', power: 50 }] }], items: [{ id: 'bracelet', name: '护盾手环', kind: 'armor', effect: 'shield', scale: 'person', max_charges: 3 }] })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByText('Opponent with a long known name')
  expect(screen.getByText(/冷却还需120剧情秒，当前零护盾/)).toBeInTheDocument()
  expect(screen.getByRole('option', { name: /动能护盾增压/ })).toBeDisabled()
  expect(mocks.request.mock.calls.every(call => !call[1])).toBe(true)
})

it('shows the full paid overclock risk and saved ship lock without allowing a locked action', async () => {
  mocks.request.mockResolvedValue({ ...state, clock_seconds: 20,
    actor: { ...state.actor, skills: { boost: 1 }, assets: { frigate: { health: 80, max_health: 90, shield: 0, fuel: 10, permanent_damage: 10, disabled_until: 10820 } }, battle: { ...state.actor.battle, scale: 'ship', asset_id: 'frigate' } },
    skills: [{ id: 'boost', name: 'Engine overdrive', kind: 'pilot', scale: 'ship', cost: 9, effects: [{ kind: 'overclock', power: 40 }] }],
  })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByText(/动力停机：还需10800秒故事时间/)
  expect(screen.getByText(/动力套筒永久损耗：10点耐久上限/)).toBeInTheDocument()
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:boost' } })
  expect(screen.getByText(/以140%逃逸推力尝试脱离/)).toHaveTextContent('抽干气力与燃料')
  expect(screen.getByRole('button', { name: '执行这一回合' })).toBeDisabled()
  expect(mocks.request.mock.calls.filter(call => call[1])).toHaveLength(0)
})

it('offers the learned near-field electronic disruption in a machine encounter, not as a human attack', async () => {
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, skills: { arc: 1 }, battle: { ...state.actor.battle, scale: 'machine', can_disrupt_electronics: true } }, skills: [
    { id: 'arc', name: 'Plasma arc', kind: 'control', scale: 'person', cost: 9, effects: [{ kind: 'electronic_disrupt', power: 1 }] },
  ] })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByRole('option', { name: /Plasma arc/ })
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:arc' } })
  expect(screen.getByText(/扰乱十米内已暴露的电子节点一轮/)).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith(expect.stringContaining('/adventure-action'), expect.objectContaining({ body: expect.stringContaining('"skill_id":"arc"') })))
})

it('allows only inward recovery, not mixed physical defense or needle tools, with weakened limbs', async () => {
  mocks.request.mockResolvedValue({ ...state, family: 'wuxia', actor: { ...state.actor, soft_limbs: true, skills: { mixed: 1, inward: 1, needles: 1 }, inventory: { needle: 1 } }, skills: [
    { id: 'mixed', name: 'Mixed guard and recovery', kind: 'heal', scale: 'person', cost: 2, effects: [{ kind: 'guard', power: 25 }, { kind: 'energy', power: 30 }] },
    { id: 'inward', name: 'Inward breathing', kind: 'energy', scale: 'person', cost: 2 },
    { id: 'needles', name: 'Needle healing', kind: 'heal', scale: 'person', cost: 2, required_tool: 'needle' },
  ] })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByRole('option', { name: /Mixed guard/ })
  expect(screen.getByRole('option', { name: /Mixed guard/ })).toBeDisabled()
  expect(screen.getByRole('option', { name: /Needle healing/ })).toBeDisabled()
  expect(screen.getByRole('option', { name: /Inward breathing/ })).not.toBeDisabled()
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'guard' } })
  expect(screen.getByRole('button', { name: '执行这一回合' })).toBeDisabled()
})

it('does not enable a personal plasma skill against an ordinary human merely because scales match', async () => {
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, skills: { arc: 1 } }, skills: [{ id: 'arc', name: 'Plasma arc', kind: 'control', scale: 'person', cost: 9, effects: [{ kind: 'electronic_disrupt', power: 1 }] }] })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByRole('option', { name: /Plasma arc/ })
  expect(screen.getByRole('option', { name: /Plasma arc/ })).toBeDisabled()
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:arc' } })
  expect(screen.getByRole('button', { name: '执行这一回合' })).toBeDisabled()
})

it('keeps a generated tool usable for the same healing and hostile techniques', async () => {
  mocks.request.mockResolvedValue({ ...state, family: 'wuxia', actor: { ...state.actor, skills: { needles: 1 }, inventory: { custom_needles: 1 } },
    skills: [{ id: 'needles', name: 'Needle technique', kind: 'heal', scale: 'person', cost: 2, required_tool: 'needles_base', enemy_effects: [{ kind: 'seal', power: 1 }] }],
    items: [{ id: 'custom_needles', generated_from: 'needles_base', name: 'Reworked needles', kind: 'tool', scale: 'person' }],
  })
  render(<AdventureCombatPanel {...props} />)
  expect(await screen.findByRole('option', { name: /Needle technique.*消耗/ })).toBeEnabled()
  expect(screen.getByRole('option', { name: /Needle technique.*对敌封穴/ })).toBeEnabled()
})

it('does not enable hostile techniques when their required tool is missing', async () => {
  mocks.request.mockResolvedValue({ ...state, family: 'wuxia', actor: { ...state.actor, skills: { needles: 1 }, inventory: { custom_needles: 0 } },
    skills: [{ id: 'needles', name: 'Needle technique', kind: 'heal', scale: 'person', cost: 2, required_tool: 'needles_base', enemy_effects: [{ kind: 'seal', power: 1 }] }],
    items: [{ id: 'custom_needles', generated_from: 'needles_base', name: 'Reworked needles', kind: 'tool', scale: 'person' }],
  })
  render(<AdventureCombatPanel {...props} />)
  expect(await screen.findByRole('option', { name: /Needle technique.*对敌封穴/ })).toBeDisabled()
})

for (const unavailable of [{ fuel: 0 }, { health: 0 }, { disabled_until: 100 }]) {
  it(`offers only an explicit defeated withdrawal for an unavailable battle asset ${JSON.stringify(unavailable)}`, async () => {
    mocks.request.mockResolvedValue({ ...state, clock_seconds: 20, actor: { ...state.actor,
      assets: { machine: { health: 80, max_health: 100, shield: 0, fuel: 5, ...unavailable } },
      battle: { ...state.actor.battle, scale: 'machine', asset_id: 'machine' },
    } })
    render(<AdventureCombatPanel {...props} />)
    await screen.findByText('Opponent with a long known name')
    expect(screen.getByRole('option', { name: '攻击' })).toBeDisabled()
    expect(screen.getByRole('option', { name: '防守' })).toBeDisabled()
    expect(screen.getByRole('option', { name: '败退结算' })).toBeEnabled()
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'escape' } })
    expect(screen.getByRole('button', { name: '执行这一回合' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
    await waitFor(() => expect(mocks.request).toHaveBeenCalledWith(expect.stringContaining('/adventure-action'), expect.objectContaining({ body: expect.stringContaining('"choice":"escape"') })))
  })
}


it('offers yielding only when the rules explicitly allow it', async () => {
  const allowed = { ...state, family: 'wuxia', actor: { ...state.actor, soft_limbs: true, battle: { ...state.actor.battle, can_surrender: true } } }
  mocks.request.mockResolvedValue(allowed)
  const view = render(<AdventureCombatPanel {...props} />)
  expect(await screen.findByRole('option', { name: '认输' })).toBeEnabled()
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'surrender' } })
  fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith(expect.stringContaining('/adventure-action'), expect.objectContaining({ body: expect.stringContaining('"choice":"surrender"') })))
  view.unmount()
  mocks.request.mockResolvedValue(state)
  render(<AdventureCombatPanel {...props} />)
  await screen.findByText('Opponent with a long known name')
  expect(screen.queryByRole('option', { name: '认输' })).not.toBeInTheDocument()
})

it('shows only the committed enemy technique and clears it when the next round has no technique', async () => {
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, battle: { ...state.actor.battle, last_enemy_skill_name: '已见过的回风护体诀' } } })
  render(<AdventureCombatPanel {...props} />)
  expect(await screen.findByTestId('enemy-last-skill')).toHaveTextContent('对手刚刚施展：已见过的回风护体诀')
  expect(screen.queryByRole('option', { name: /已见过的回风护体诀/ })).not.toBeInTheDocument()
  mocks.request.mockResolvedValue(state)
  fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
  await waitFor(() => expect(screen.queryByTestId('enemy-last-skill')).not.toBeInTheDocument())
})


it('renders an observed enemy technique in the selected English locale', async () => {
  setConfiguredLocale('en-US')
  try {
    mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, battle: { ...state.actor.battle, last_enemy_skill_name: 'Measured ward' } } })
    render(<AdventureCombatPanel {...props} />)
    expect(await screen.findByTestId('enemy-last-skill')).toHaveTextContent('Opponent used: Measured ward')
    expect(screen.queryByText(/对手刚刚施展/)).not.toBeInTheDocument()
  } finally {
    setConfiguredLocale('zh-CN')
  }
})

it('shows the frozen extended stage and paid story-driven refinement without sending actions', async () => {
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, battle: undefined }, progression: { stage_name: '超A级', rank: 41, max_rank: 60, experience: 120, required_experience: 13710, next_stage_name: '超神级', next_stage_rank: 51, foundation_name: '超限意志锚定', required_mastery: 2, experience_per_hour: 24, currency_per_hour: 2 } })
  await setConfiguredLocale('zh-CN')
  render(<AdventureCombatPanel {...props} />)
  expect(await screen.findByTestId('adventure-progression')).toHaveTextContent('超A级')
  expect(screen.getByTestId('adventure-progression')).toHaveTextContent('41 / 60')
  expect(screen.getByTestId('adventure-progression')).toHaveTextContent('超神级（51级）')
  expect(screen.getByTestId('adventure-progression')).toHaveTextContent('超限意志锚定')
  expect(screen.queryByRole('button', { name: '执行这一回合' })).not.toBeInTheDocument()
  expect(mocks.request.mock.calls.every(([, options]) => !options)).toBe(true)
})

it('keeps the final stage open-ended in English', async () => {
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, battle: undefined }, progression: { stage_name: 'Transcendent', rank: 60, max_rank: 60, experience: 0, required_experience: 0, required_mastery: 2, experience_per_hour: 24, currency_per_hour: 2 } })
  await setConfiguredLocale('en-US')
  const view = render(<AdventureCombatPanel {...props} />)
  expect(await screen.findByTestId('adventure-progression')).toHaveTextContent('Exploration and everyday life can continue.')
  expect(screen.getByTestId('adventure-progression')).not.toHaveTextContent('深修')
  view.unmount()
  await setConfiguredLocale('zh-CN')
})

it('uses the personal side of mixed-scale combat and respects paid upkeep availability', async () => {
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, energy: 30, battle: { ...state.actor.battle, scale: 'ship', player_scale: 'person', space_combat_energy_per_round: 20 }, assets: { parked: { health: 0, max_health: 1000, shield: 0, fuel: 0 } } }, skills: [{ id: 'shot', name: 'Personal strike', kind: 'attack', scale: 'person', cost: 15, usable: false }] })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByRole('option', { name: /Personal strike/ })
  expect(screen.getByText(/每轮另耗 20/)).toBeInTheDocument()
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:shot' } })
  expect(screen.getByRole('button', { name: '执行这一回合' })).toBeDisabled()
  expect(screen.queryByText(/载具已失去作战能力/)).not.toBeInTheDocument()
})

it('sends a paid movement skill as movement rather than an extra attack', async () => {
  mocks.request.mockResolvedValue({ ...state, family: 'wuxia', actor: { ...state.actor, skills: { footwork: 2 } }, skills: [{ id: 'footwork', name: '轻功', kind: 'pilot', scale: 'person', cost: 9, mobility: 15, mobility_usable: true }] })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByRole('option', { name: /撤离.*轻功/ })
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'escape:footwork' } })
  fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith(expect.stringContaining('/adventure-action'), expect.objectContaining({ body: JSON.stringify({ command_id: 'round-command', expected_head: 'event-1', branch: 'branch', action: { action: 'battle_round', choice: 'escape', skill_id: 'footwork' } }) })))
})

it('allows accepting only an already visible ceasefire offer', async () => {
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, energy: 0, battle: { ...state.actor.battle, enemy_ceasefire_offer: true } } })
  render(<AdventureCombatPanel {...props} />)
  await screen.findByRole('option', { name: '接受停战' })
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'accept_ceasefire' } })
  fireEvent.click(screen.getByRole('button', { name: '执行这一回合' }))
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith(expect.stringContaining('/adventure-action'), expect.objectContaining({ body: expect.stringContaining('"choice":"accept_ceasefire"') })))
})

it.each(['zh-CN', 'en-US'] as const)('shows a real mental effect and no invented control in %s', async (locale) => {
  await setConfiguredLocale(locale)
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, skills: { mind: 1 } }, skills: [{ id: 'mind', name: 'Mental pulse', kind: 'attack', scale: 'person', cost: 8, usable: true, effects: [{ kind: 'mental_attack', power: 14 }] }] })
  const view = render(<AdventureCombatPanel {...props} />)
  try {
    await screen.findByRole('option', { name: /Mental pulse/ })
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:mind' } })
    expect(screen.getAllByText(locale === 'zh-CN' ? /精神伤害14/ : /Mental damage 14/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(locale === 'zh-CN' ? /不叠武器，不附控制/ : /no weapon or control bonus/).length).toBeGreaterThan(0)
    expect(screen.queryByText('combatWindow.adventure.effect.mental_attack')).not.toBeInTheDocument()
  } finally {
    view.unmount()
    await setConfiguredLocale('zh-CN')
  }
})

it.each(['zh-CN', 'en-US'] as const)('shows an actual required equipped weapon class in %s', async (locale) => {
  await setConfiguredLocale(locale)
  mocks.request.mockResolvedValue({ ...state, actor: { ...state.actor, skills: { form: 1 } }, skills: [{ id: 'form', name: 'Sword form', kind: 'attack', scale: 'person', cost: 8, usable: false, required_weapon_class: 'sword' }] })
  const view = render(<AdventureCombatPanel {...props} />)
  try {
    await screen.findByRole('option', { name: /Sword form/ })
    expect(screen.getByRole('option', { name: /Sword form/ })).toBeDisabled()
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:form' } })
    expect(screen.getByRole('button', { name: locale === 'zh-CN' ? '执行这一回合' : 'Play this round' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: /Sword form/ }))
    expect(screen.getByText(locale === 'zh-CN' ? '必须装备的兵器类别' : 'Required equipped weapon class')).toBeInTheDocument()
    expect(screen.getByText(locale === 'zh-CN' ? '剑' : 'Sword')).toBeInTheDocument()
  } finally {
    view.unmount()
    await setConfiguredLocale('zh-CN')
  }
})
