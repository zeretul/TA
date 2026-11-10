import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import type { CombatWindow, CombatWindowSide } from '../../types'
import { CombatWindowPanel } from './CombatWindowPanel'

const apiMocks = vi.hoisted(() => ({
  getCombatWindow: vi.fn(),
  playCombatRound: vi.fn(),
  narrateCombatRound: vi.fn(),
}))

vi.mock('../../api', () => apiMocks)

const scene = { node_id: 'node-1', title: '山道', location: '青云山道', world_time: '三月初七' }

function windowData(overrides: Partial<CombatWindow> = {}): CombatWindow {
  return {
    story_id: 'story-1',
    branch_id: 'branch-1',
    scene,
    player_actor_id: 'hero',
    session_id: 'battle-1',
    round: 2,
    ended: false,
    has_more: false,
    sides: [
      {
        actor_id: 'hero', name: '沈砚', side: 'player', exact: true, standing: 'standing',
        health: 40, health_max: 60, mana: 12, mana_max: 30, has_implement: true,
        skills: [{ id: 'condensed_blade', version: 1, castable: true, name_key: 'skill.condensed_blade.name' }],
      } as CombatWindowSide,
      {
        actor_id: 'wolf', name: '铁背狼', side: 'enemy', exact: false, standing: 'standing',
        observed: { condition: 'combatWindow.condition.hurt' },
        has_implement: false,
      } as CombatWindowSide,
    ],
    ...overrides,
  }
}

function renderPanel(overrides: Partial<CombatWindow> = {}, onContinueStory?: (message: string) => void) {
  apiMocks.getCombatWindow.mockResolvedValue(windowData(overrides))
  render(
    <CombatWindowPanel
      storyId="story-1"
      branchId="branch-1"
      active
      sessionId="battle-1"
      onSelectSession={() => {}}
      onClose={() => {}}
      onContinueStory={onContinueStory}
    />,
  )
}

beforeEach(() => {
  apiMocks.getCombatWindow.mockReset()
  apiMocks.playCombatRound.mockReset()
  apiMocks.narrateCombatRound.mockReset()
})

describe('CombatWindowPanel narration', () => {
  it('shows the frozen lethal warning and the committed fatality result', async () => {
    renderPanel({
      lethal_risk: true,
      last_round: {
        event_id: 'round-2',
        outcome: { round: 2, steps: [] },
        fatalities: [{
          actor_id: 'wolf', killer_actor_id: 'hero', source_turn_id: 'turn-risk',
          source_event_id: 'round-2', round: 2, step_index: 0,
        }],
      },
    })

    expect(await screen.findByText('对方已显露取命意图。这场交战可能致命。')).toBeInTheDocument()
    expect(screen.getByText('沈砚在正式交战中击杀了铁背狼。')).toBeInTheDocument()
  })

  it('shows the committed passage as the round’s own text', async () => {
    renderPanel({
      last_round: {
        event_id: 'round-2',
        narration: '沈砚凝刃在手，剑光贴着狼腹掠过。',
        outcome: { round: 2, steps: [{ actor_id: 'hero', target_id: 'wolf', kind: 'technique', skill_id: 'condensed_blade', cast: { allowed: true } }] },
      },
    })
    expect(await screen.findByText('沈砚凝刃在手，剑光贴着狼腹掠过。')).toBeInTheDocument()
  })

  it('names the technique a step performed, from the definition id', async () => {
    renderPanel({
      last_round: {
        event_id: 'round-2',
        outcome: { round: 2, steps: [{ actor_id: 'hero', target_id: 'wolf', kind: 'technique', skill_id: 'condensed_blade', cast: { allowed: true } }] },
      },
    })
    // The wording lives only in the interface's locale, so the panel has to
    // resolve the id rather than echo it. It appears twice — once in the learned
    // list, once on the step that performed it.
    expect(await screen.findAllByText(/凝刃术/)).toHaveLength(2)
    expect(screen.queryByText(/condensed_blade/)).toBeNull()
  })

  it('still names the technique when the cast was refused', async () => {
    renderPanel({
      last_round: {
        event_id: 'round-2',
        outcome: {
          round: 2,
          steps: [{
            actor_id: 'hero', kind: 'technique', skill_id: 'meridian_secret_blade',
            cast: { allowed: false }, reason_key: 'reason.insufficient_mana',
          }],
        },
      },
    })
    expect(await screen.findByText(/冲脉秘剑/)).toBeInTheDocument()
  })

  it('shows the settled round before any account exists', async () => {
    // §3 orders the committed result ahead of its expression: a round whose
    // passage is still being written must be readable as a settled round, with
    // its order and steps, rather than announced as missing text.
    renderPanel({
      last_round: {
        event_id: 'round-2',
        outcome: {
          round: 2,
          order: ['hero', 'wolf'],
          steps: [{ actor_id: 'hero', kind: 'technique', skill_id: 'strike', cast: { allowed: true }, resolution: { hit: true, damage: 7 } }],
        },
      },
    })
    expect((await screen.findAllByText('第 2 轮')).length).toBeGreaterThan(0)
    expect(screen.getByText(/出手顺序/)).toBeInTheDocument()
    expect(screen.queryByText('本轮尚未留下叙述。')).toBeNull()
  })

  it('renders each history round with its own passage', async () => {
    renderPanel({
      history: [
        {
          event_id: 'round-1',
          narration: '第一合，狼扑空。',
          outcome: { round: 1, steps: [{ actor_id: 'wolf', kind: 'technique', skill_id: 'strike', cast: { allowed: true }, resolution: { hit: false } }] },
        },
      ],
    })
    expect(await screen.findByText('第一合，狼扑空。')).toBeInTheDocument()
  })
})

describe('CombatWindowPanel prepared techniques', () => {
  it('shows the paid preparation and submits one explicit continuation action', async () => {
    apiMocks.playCombatRound.mockResolvedValue({})
    renderPanel({
      sides: [
        {
          actor_id: 'hero', name: '沈砚', side: 'player', standing: 'standing',
          health: 40, health_max: 60, mana: 12, mana_max: 30, has_implement: true,
          preparation: { skill_id: 'meridian_secret_blade', name_key: 'skill.meridian_secret_blade.name', started_round: 1, target_id: 'wolf' },
          skills: [{ id: 'meridian_secret_blade', version: 1, castable: false, name_key: 'skill.meridian_secret_blade.name' }],
        } as CombatWindowSide,
        {
          actor_id: 'wolf', name: '铁背狼', side: 'enemy', standing: 'standing',
          health: 22, health_max: 40, mana: 0, mana_max: 0, has_implement: false,
        } as CombatWindowSide,
      ],
    })

    expect(await screen.findAllByText('正在准备：冲脉秘剑')).toHaveLength(2)
    expect(screen.getByText(/准备已扣耗灵与负担/)).toBeInTheDocument()
    fireEvent.click(await screen.findByRole('button', { name: '结算本轮' }))

    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledWith('story-1', expect.objectContaining({
      sessionId: 'battle-1',
      actions: [{ actor_id: 'hero', kind: 'continue_preparation' }],
    })))
  })

  it('uses a fresh command after a rejected action so the player can correct it', async () => {
    apiMocks.playCombatRound
      .mockRejectedValueOnce(new APIError('Action rejected', { status: 400 }))
      .mockResolvedValueOnce({})
    renderPanel({
      sides: [
        {
          actor_id: 'hero', name: '沈砚', side: 'player', standing: 'standing',
          health: 40, health_max: 60, mana: 12, mana_max: 30, has_implement: true,
          preparation: { skill_id: 'meridian_secret_blade', name_key: 'skill.meridian_secret_blade.name', started_round: 1, target_id: 'wolf' },
          skills: [{ id: 'meridian_secret_blade', version: 1, castable: false, name_key: 'skill.meridian_secret_blade.name' }],
        } as CombatWindowSide,
        {
          actor_id: 'wolf', name: '铁背狼', side: 'enemy', standing: 'standing',
          health: 22, health_max: 40, mana: 0, mana_max: 0, has_implement: false,
        } as CombatWindowSide,
      ],
    })

    fireEvent.click(await screen.findByRole('button', { name: '结算本轮' }))
    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledTimes(1))
    fireEvent.click(await screen.findByRole('button', { name: /放弃招式/ }))
    fireEvent.click(await screen.findByRole('button', { name: '重试本轮' }))
    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledTimes(2))
    const first = apiMocks.playCombatRound.mock.calls[0][1]
    const second = apiMocks.playCombatRound.mock.calls[1][1]
    expect(first.commandId).toBeTruthy()
    expect(second.commandId).not.toBe(first.commandId)
    expect(second.actions).toEqual([{ actor_id: 'hero', kind: 'cancel_preparation' }])
  })

  it('offers explicit abandonment and lets an alternate main action carry the abandonment', async () => {
    apiMocks.playCombatRound.mockResolvedValue({})
    renderPanel({
      sides: [
        {
          actor_id: 'hero', name: '沈砚', side: 'player', standing: 'standing',
          health: 40, health_max: 60, mana: 12, mana_max: 30, has_implement: true,
          preparation: { skill_id: 'meridian_secret_blade', name_key: 'skill.meridian_secret_blade.name', started_round: 1, target_id: 'wolf' },
          skills: [{ id: 'meridian_secret_blade', version: 1, castable: false, name_key: 'skill.meridian_secret_blade.name' }],
        } as CombatWindowSide,
        {
          actor_id: 'wolf', name: '铁背狼', side: 'enemy', standing: 'standing',
          health: 22, health_max: 40, mana: 0, mana_max: 0, has_implement: false,
        } as CombatWindowSide,
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: /放弃招式/ }))
    fireEvent.click(screen.getByRole('button', { name: '结算本轮' }))

    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledWith('story-1', expect.objectContaining({
      actions: [{ actor_id: 'hero', kind: 'cancel_preparation' }],
    })))
  })
})

describe('CombatWindowPanel narration recovery', () => {
  it('offers to write the missing passage without replaying the round', async () => {
    apiMocks.narrateCombatRound.mockResolvedValue({})
    renderPanel({
      last_round: { event_id: 'round-2', outcome: { round: 2, steps: [] } },
      narration_error: 'model call failed',
      narration_state: 'failed',
    })
    const retry = await screen.findByRole('button', { name: '补写这段文字' })
    expect(screen.getByText('model call failed')).toBeInTheDocument()
    fireEvent.click(retry)
    await waitFor(() => expect(apiMocks.narrateCombatRound).toHaveBeenCalledTimes(1))
    expect(apiMocks.narrateCombatRound).toHaveBeenCalledWith('story-1', expect.objectContaining({
      sessionId: 'battle-1',
      roundEventId: 'round-2',
    }))
    // §8: the settled round stands. Recovery writes prose, it does not fight again.
    expect(apiMocks.playCombatRound).not.toHaveBeenCalled()
  })

  it('retries a lost passage response under the identity of the first attempt', async () => {
    apiMocks.narrateCombatRound.mockRejectedValueOnce(new Error('response lost')).mockResolvedValueOnce({})
    renderPanel({
      last_round: { event_id: 'round-2', outcome: { round: 2, steps: [] } },
      narration_error: 'initial narration failed',
      narration_state: 'failed',
    })
    fireEvent.click(await screen.findByRole('button', { name: '补写这段文字' }))
    expect(await screen.findByText('response lost')).toBeInTheDocument()
    fireEvent.click(await screen.findByRole('button', { name: '补写这段文字' }))
    await waitFor(() => expect(apiMocks.narrateCombatRound).toHaveBeenCalledTimes(2))
    const [first, second] = apiMocks.narrateCombatRound.mock.calls.map(call => call[1].commandId)
    expect(first).toBeTruthy()
    expect(second).toBe(first)
    expect(apiMocks.playCombatRound).not.toHaveBeenCalled()
  })

  it('does not show a failure reason for a passage that was never attempted', async () => {
    // §3 requires "待生成" and "失败" to look different. A round settled but not
    // yet written has no reason to report, so it says it is still being written
    // rather than borrowing the failure wording.
    renderPanel({
      last_round: { event_id: 'round-2', outcome: { round: 2, steps: [] } },
      narration_error: 'no passage yet',
      narration_state: 'pending',
    })
    expect(await screen.findByText('本轮已经结算，文字还在写。')).toBeInTheDocument()
    expect(screen.queryByText('no passage yet')).toBeNull()
    expect(screen.queryByText('本回合的结果已经记下，只是这一段的文字没有写成。')).toBeNull()
  })

  it('reports a failed passage attempt as a failure, not as one nobody made', async () => {
    // The two-phase commit settles the round and then asks for its prose. When
    // that second call fails, the journal holds a committed round with no prose
    // — exactly the shape a round nobody has written yet has. The panel must not
    // let the honest "still being written" wording cover for a call that failed,
    // so the failure is remembered against the round it belongs to.
    apiMocks.getCombatWindow.mockResolvedValue(windowData({
      last_round: { event_id: 'round-2', outcome: { round: 2, steps: [] } },
      narration_error: '这一段叙述尚未写入。',
      narration_state: 'pending',
    }))
    apiMocks.playCombatRound.mockResolvedValue(windowData({
      last_round: { event_id: 'round-2', outcome: { round: 2, steps: [] } },
      narration_state: 'pending',
    }))
    apiMocks.narrateCombatRound.mockRejectedValue(new Error('the model did not answer'))
    render(
      <CombatWindowPanel
        storyId="story-1" branchId="branch-1" active sessionId="battle-1"
        onSelectSession={() => {}} onClose={() => {}}
      />,
    )
    fireEvent.click(await screen.findByRole('button', { name: '结算本轮' }))
    expect(await screen.findByText('the model did not answer')).toBeInTheDocument()
    expect(screen.getByText('本回合的结果已经记下，只是这一段的文字没有写成。')).toBeInTheDocument()
    expect(screen.queryByText('本轮已经结算，文字还在写。')).toBeNull()
  })

  it('keeps the recovery offer off a round whose passage arrived', async () => {
    renderPanel({
      last_round: { event_id: 'round-2', narration: '已写成的经过。', outcome: { round: 2, steps: [] } },
    })
    expect(await screen.findByText('已写成的经过。')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '补写这段文字' })).toBeNull()
  })

  // A round whose passage failed must stay fillable after the player fights on.
  // The prose belongs to the round, not to the fight, so an earlier round that
  // lost its passage to a failed call cannot become permanently mute just because
  // a later round was played — that is the shape the delivery review reproduced.
  it('lets an earlier round have its missing passage written later', async () => {
    apiMocks.narrateCombatRound.mockResolvedValue({})
    renderPanel({
      last_round: { event_id: 'round-2', narration: '第二合的经过。', outcome: { round: 2, steps: [] } },
      history: [
        { event_id: 'round-1', outcome: { round: 1, steps: [] } },
        { event_id: 'round-2', narration: '第二合的经过。', outcome: { round: 2, steps: [] } },
      ],
    })
    const retries = await screen.findAllByRole('button', { name: '补写这段文字' })
    // Only round 1 is missing its passage; the newest round already has one.
    expect(retries).toHaveLength(1)
    fireEvent.click(retries[0])
    await waitFor(() => expect(apiMocks.narrateCombatRound).toHaveBeenCalledTimes(1))
    expect(apiMocks.narrateCombatRound).toHaveBeenCalledWith('story-1', expect.objectContaining({
      sessionId: 'battle-1',
      roundEventId: 'round-1',
    }))
    expect(apiMocks.playCombatRound).not.toHaveBeenCalled()
  })
})

describe('CombatWindowPanel opponent information boundary', () => {
  it('shows a stranger as a visible condition rather than as numbers', async () => {
    renderPanel()
    expect(await screen.findByText('受了些伤')).toBeInTheDocument()
    // The enemy's stored pools were never sent, so nothing can be rendered from
    // them — and the panel must not fall back to a zero.
    expect(screen.queryByText('生命 22 / 40')).toBeNull()
    expect(screen.queryByText('灵力 0 / 0')).toBeNull()
    expect(screen.getByText('随身物品：未知')).toBeInTheDocument()
    expect(screen.getByText('已见手段：尚未见识')).toBeInTheDocument()
  })

  it('names only the arts the player could actually recognise', async () => {
    renderPanel({
      sides: [
        {
          actor_id: 'hero', name: '沈砚', side: 'player', exact: true, standing: 'standing',
          health: 40, health_max: 60, mana: 12, mana_max: 30, has_implement: true, skills: [], route_exists: true,
        } as CombatWindowSide,
        {
          actor_id: 'wolf', name: '铁背狼', side: 'enemy', exact: false, standing: 'standing',
          observed: {
            condition: 'combatWindow.condition.hurt',
            techniques: [
              { round: 1, recognized: false },
              { round: 2, recognized: true, skill_id: 'condensed_blade', name_key: 'skill.condensed_blade.name' },
            ],
            doses: 1,
          },
          has_implement: false,
        } as CombatWindowSide,
      ],
    })
    expect(await screen.findByText(/一门认不出的手段/)).toBeInTheDocument()
    expect(screen.getByText(/凝刃术/)).toBeInTheDocument()
    expect(screen.getByText('已见使用消耗品 1 次')).toBeInTheDocument()
    // An unidentified art must not leak its definition id through a label.
    expect(screen.queryByText(/condensed_blade/)).toBeNull()
  })

  it('shows an opponent\'s lasting effects as presence rather than as counts', async () => {
    renderPanel({
      sides: [
        {
          actor_id: 'hero', name: '沈砚', side: 'player', exact: true, standing: 'standing',
          health: 40, health_max: 60, mana: 12, mana_max: 30, has_implement: true, skills: [], route_exists: true,
        } as CombatWindowSide,
        {
          actor_id: 'wolf', name: '铁背狼', side: 'enemy', exact: false, standing: 'standing',
          observed: {
            condition: 'combatWindow.condition.hurt',
            shield: true, burn_ticks: true, slow_actions: true,
          },
          has_implement: false,
        } as CombatWindowSide,
      ],
    })
    expect(await screen.findByText('有东西挡着劲力')).toBeInTheDocument()
    expect(screen.getByText('身上还带着火')).toBeInTheDocument()
    expect(screen.getByText('动作迟滞')).toBeInTheDocument()
    // The observation carries no number, so nothing can render one.
    expect(screen.queryByText(/护盾 \d/)).toBeNull()
    expect(screen.queryByText(/灼烧 \d/)).toBeNull()
  })
})

describe('CombatWindowPanel carried consumables', () => {
  it('shows and consumes a ward by its inventory identity', async () => {
    const data = windowData()
    data.sides![0].items = [{ instance_id: 'talisman.ward.lower', def_id: 'talisman.ward.lower', name_key: 'item.talisman.ward_lower.name', quantity: 1, purpose: 'shield', shield_amount: 12, duration_exchanges: 3, usable: true }]
    apiMocks.getCombatWindow.mockResolvedValue(data)
    apiMocks.playCombatRound.mockResolvedValue(windowData())
    render(<CombatWindowPanel storyId="story-1" branchId="branch-1" active sessionId="battle-1" onSelectSession={() => {}} onClose={() => {}} />)
    const ward = (await screen.findAllByRole('button', { name: /护身符/ }))[0]
    expect(screen.getByText('护盾 12，持续 3 轮')).toBeInTheDocument()
    fireEvent.click(ward)
    fireEvent.click(screen.getByRole('button', { name: '结算本轮' }))
    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledTimes(1))
    expect(apiMocks.playCombatRound.mock.calls[0][1].actions).toEqual([{ actor_id: 'hero', kind: 'use_item', item_instance_id: 'talisman.ward.lower' }])
  })

  // A hero holding one restoring pill with two doses, plus one already used up.
  function withItems() {
    const data = windowData()
    data.sides = data.sides?.map((side) => side.actor_id === 'hero'
      ? {
        ...side,
        items: [
          {
            instance_id: '回春丹', def_id: 'pill.restore.health.lower',
            name_key: 'item.pill.restore_health_lower.name',
            quantity: 2, purpose: 'restore_health', restore_amount: 12, usable: true,
          },
          {
            instance_id: '空瓶丹', def_id: 'pill.restore.health.lower',
            name_key: 'item.pill.restore_health_lower.name',
            quantity: 0, purpose: 'restore_health', restore_amount: 12,
            usable: false, reason_key: 'reason.item_exhausted',
          },
        ],
      }
      : side)
    return data
  }

  it('shows each carried copy with its effect and remaining doses', async () => {
    apiMocks.getCombatWindow.mockResolvedValue(withItems())
    render(
      <CombatWindowPanel
        storyId="story-1" branchId="branch-1" active sessionId="battle-1"
        onSelectSession={() => {}} onClose={() => {}}
      />,
    )
    // §3: the name, the effect and the count are shown together, and the name
    // comes from the catalogue's own key rather than from the definition id.
    expect(await screen.findByText('随身消耗品')).toBeInTheDocument()
    expect(screen.getAllByText('回春丹')).toHaveLength(2)
    expect(screen.getAllByText('回复生命 12')).toHaveLength(2)
    expect(screen.getByText('余 2 份')).toBeInTheDocument()
    expect(screen.queryByText(/pill\.restore\.health\.lower/)).toBeNull()
  })

  it('names the reason an exhausted copy cannot be taken', async () => {
    apiMocks.getCombatWindow.mockResolvedValue(withItems())
    render(
      <CombatWindowPanel
        storyId="story-1" branchId="branch-1" active sessionId="battle-1"
        onSelectSession={() => {}} onClose={() => {}}
      />,
    )
    // §8: a refused use costs nothing, so the window says why rather than
    // hiding the entry or letting the click fail later.
    expect(await screen.findByText('这一份已经用完了。')).toBeInTheDocument()
    expect(screen.getByText('余 0 份')).toBeInTheDocument()
  })

  it('sends only the copy, never an effect the client priced itself', async () => {
    apiMocks.getCombatWindow.mockResolvedValue(withItems())
    apiMocks.playCombatRound.mockResolvedValue(windowData())
    render(
      <CombatWindowPanel
        storyId="story-1" branchId="branch-1" active sessionId="battle-1"
        onSelectSession={() => {}} onClose={() => {}}
      />,
    )

    const dose = (await screen.findAllByRole('button', { name: /^回春丹/ }))[0]
    fireEvent.click(dose)
    fireEvent.click(screen.getByRole('button', { name: '结算本轮' }))

    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledTimes(1))
    const actions = apiMocks.playCombatRound.mock.calls[0][1].actions
    expect(actions).toEqual([{ actor_id: 'hero', kind: 'use_item', item_instance_id: '回春丹' }])
  })

  it('will not offer a copy the rules would refuse', async () => {
    apiMocks.getCombatWindow.mockResolvedValue(withItems())
    render(
      <CombatWindowPanel
        storyId="story-1" branchId="branch-1" active sessionId="battle-1"
        onSelectSession={() => {}} onClose={() => {}}
      />,
    )
    const buttons = await screen.findAllByRole('button', { name: /^回春丹/ })
    // The spent copy is the second entry and is disabled, so a click cannot
    // select a dose the round would refuse for being exhausted.
    expect(buttons[1]).toBeDisabled()
    expect(buttons[0]).toBeEnabled()
  })

  it('says which choice went stale instead of leaving the button dark', async () => {
    apiMocks.getCombatWindow.mockResolvedValue(withItems())
    render(
      <CombatWindowPanel
        storyId="story-1" branchId="branch-1" active sessionId="battle-1"
        onSelectSession={() => {}} onClose={() => {}}
      />,
    )
    fireEvent.click((await screen.findAllByRole('button', { name: /^回春丹/ }))[0])
    expect(screen.getByRole('button', { name: '结算本轮' })).toBeEnabled()
    expect(screen.queryByText('这个选择已经不作数了，请重新选一项。')).toBeNull()
  })
})

describe('CombatWindowPanel round submission', () => {
  it('resolves one exchange for a double click', async () => {
    let settle: (value: unknown) => void = () => {}
    apiMocks.playCombatRound.mockImplementation(() => new Promise((resolve) => { settle = resolve }))
    renderPanel()

    const play = await screen.findByRole('button', { name: '结算本轮' })
    fireEvent.click(play)
    fireEvent.click(play)

    // The button disables on re-render, but the in-flight guard is what actually
    // keeps two clicks from resolving two exchanges.
    expect(apiMocks.playCombatRound).toHaveBeenCalledTimes(1)
    settle(windowData())
    await waitFor(() => expect(apiMocks.getCombatWindow).toHaveBeenCalledTimes(2))
  })

  it('reports a failed read and offers a retry', async () => {
    apiMocks.getCombatWindow.mockRejectedValue(new Error('offline'))
    render(
      <CombatWindowPanel
        storyId="story-1" branchId="branch-1" active sessionId="battle-1"
        onSelectSession={() => {}} onClose={() => {}}
      />,
    )
    expect(await screen.findByText('offline')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重试' })).toBeInTheDocument()
  })

  it('shows the empty state when the branch has no live battle', async () => {
    apiMocks.getCombatWindow.mockResolvedValue(windowData({ session_id: undefined, sides: [] }))
    render(
      <CombatWindowPanel
        storyId="story-1" branchId="branch-1" active sessionId=""
        onSelectSession={() => {}} onClose={() => {}}
      />,
    )
    expect(await screen.findByText('当前分支上没有正在进行的战斗。')).toBeInTheDocument()
  })
})

describe('CombatWindowPanel handoff back to the story', () => {
  it('offers to continue the story once the battle has a result', async () => {
    const onContinueStory = vi.fn()
    renderPanel({ ended: true, result: 'enemy_fled' }, onContinueStory)

    const button = await screen.findByRole('button', { name: '继续故事' })
    // §5: one click is the authorization, so it must not be gated behind a
    // second confirmation.
    fireEvent.click(button)
    expect(onContinueStory).toHaveBeenCalledTimes(1)
    expect(onContinueStory.mock.calls[0][0]).toBeTruthy()
    // The battle is already settled, so continuing must not resolve a round.
    expect(apiMocks.playCombatRound).not.toHaveBeenCalled()
  })

  it('offers no way to end a battle that is still running', async () => {
    // §3: a live fight may only be collapsed, actually retreated from, or
    // surrendered — there is no button that ends it as a display act.
    renderPanel({ ended: false }, vi.fn())
    expect(await screen.findByRole('button', { name: '结算本轮' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '继续故事' })).toBeNull()
  })

  it('keeps the affordance inert when no continuation is wired', async () => {
    renderPanel({ ended: true, result: 'player_won' })
    const button = await screen.findByRole('button', { name: '继续故事' })
    expect(button).toBeDisabled()
  })
})

describe('CombatWindowPanel technique targets', () => {
  // A technique is aimed at someone. §4.2 prices a preparation when it starts
  // and binds its completion to the opponent it named, so the window must fix a
  // legal target before the cost is paid rather than leaving the round to be
  // refused for having named nobody.
  it('aims an ordinary attack at the only standing opponent', async () => {
    apiMocks.playCombatRound.mockResolvedValue({})
    renderPanel()

    fireEvent.click(await screen.findByRole('button', { name: '结算本轮' }))
    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledWith('story-1', expect.objectContaining({
      actions: [{ actor_id: 'hero', kind: 'technique', skill_id: 'strike', target_id: 'wolf' }],
    })))
  })

  it('aims a prepared technique at the same opponent it was begun against', async () => {
    apiMocks.playCombatRound.mockResolvedValue({})
    renderPanel({
      sides: [
        {
          actor_id: 'hero', name: '沈砚', side: 'player', exact: true, standing: 'standing',
          health: 40, health_max: 60, mana: 12, mana_max: 30, has_implement: true,
          skills: [{ id: 'meridian_secret_blade', version: 1, castable: true, name_key: 'skill.meridian_secret_blade.name' }],
        } as CombatWindowSide,
        {
          actor_id: 'wolf', name: '铁背狼', side: 'enemy', standing: 'standing',
          observed: { condition: 'combatWindow.condition.hurt' }, has_implement: false,
        } as CombatWindowSide,
      ],
    })

    fireEvent.click(await screen.findByRole('button', { name: /^冲脉秘剑/ }))
    fireEvent.click(await screen.findByRole('button', { name: '结算本轮' }))
    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledWith('story-1', expect.objectContaining({
      actions: [{ actor_id: 'hero', kind: 'technique', skill_id: 'meridian_secret_blade', target_id: 'wolf' }],
    })))
  })

  it('asks which opponent when the choice is genuinely ambiguous', async () => {
    apiMocks.playCombatRound.mockResolvedValue({})
    renderPanel({
      sides: [
        {
          actor_id: 'hero', name: '沈砚', side: 'player', exact: true, standing: 'standing',
          health: 40, health_max: 60, mana: 12, mana_max: 30, has_implement: true,
          skills: [{ id: 'condensed_blade', version: 1, castable: true, name_key: 'skill.condensed_blade.name' }],
        } as CombatWindowSide,
        {
          actor_id: 'wolf', name: '铁背狼', side: 'enemy', standing: 'standing',
          observed: { condition: 'combatWindow.condition.hurt' }, has_implement: false,
        } as CombatWindowSide,
        {
          actor_id: 'bandit', name: '山匪', side: 'enemy', standing: 'standing',
          observed: { condition: 'combatWindow.condition.unharmed' }, has_implement: false,
        } as CombatWindowSide,
      ],
    })

    const play = await screen.findByRole('button', { name: '结算本轮' })
    // Two opponents and no choice made: the round waits rather than guessing.
    expect(play).toBeDisabled()
    expect(screen.getByText('请先选择这一招要打向谁。')).toBeInTheDocument()

    fireEvent.click(await screen.findByRole('button', { name: '山匪' }))
    expect(screen.getByRole('button', { name: '结算本轮' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: '结算本轮' }))
    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledWith('story-1', expect.objectContaining({
      actions: [{ actor_id: 'hero', kind: 'technique', skill_id: 'strike', target_id: 'bandit' }],
    })))
  })

  it('plays no damaging technique when no opponent is left standing', async () => {
    renderPanel({
      sides: [
        {
          actor_id: 'hero', name: '沈砚', side: 'player', exact: true, standing: 'standing',
          health: 40, health_max: 60, mana: 12, mana_max: 30, has_implement: true,
          skills: [{ id: 'condensed_blade', version: 1, castable: true, name_key: 'skill.condensed_blade.name' }],
        } as CombatWindowSide,
      ],
    })

    expect(await screen.findByRole('button', { name: '结算本轮' })).toBeDisabled()
    expect(screen.getByText('场上没有可攻击的对手。')).toBeInTheDocument()
  })
})

// The window used to lay the whole cast out and make the player side every name,
// which turned a bystander into an enemy. The scene settles the encounter now, so
// these tests cover what the panel does with that answer: it opens the fight the
// scene states, and it does not invent one when the scene states none.
describe('CombatWindowPanel encounter', () => {
  function renderEntry(overrides: Partial<CombatWindow> = {}, onSelectSession: (id: string) => void = () => {}) {
    apiMocks.getCombatWindow.mockResolvedValue(windowData({
      session_id: undefined, round: undefined, sides: undefined, ...overrides,
    }))
    render(
      <CombatWindowPanel
        storyId="story-1"
        branchId="branch-1"
        active
        sessionId=""
        onSelectSession={onSelectSession}
        onClose={() => {}}
      />,
    )
  }

  it('lists the participants the scene settles, with no side to choose', async () => {
    renderEntry({
      encounter: {
        combatants: [
          { actor_id: 'hero', name: '沈砚', side: 'player' },
          { actor_id: 'wolf', name: '铁背狼', side: 'enemy' },
        ],
        bystanders: ['沈青'],
      },
    })

    fireEvent.click(await screen.findByRole('button', { name: '开始一场战斗' }))

    expect(await screen.findByText('沈砚')).toBeInTheDocument()
    expect(screen.getByText('铁背狼')).toBeInTheDocument()
    // A bystander is reported as standing there rather than given a side.
    expect(screen.getByText(/在场旁观/)).toHaveTextContent('沈青')
    expect(screen.queryByText('所属')).toBeNull()
  })

  it('opens the fight from the derived encounter without sending a roster', async () => {
    apiMocks.playCombatRound.mockResolvedValue(windowData())
    renderEntry({
      encounter: {
        combatants: [
          { actor_id: 'hero', name: '沈砚', side: 'player' },
          { actor_id: 'wolf', name: '铁背狼', side: 'enemy' },
        ],
      },
    })

    fireEvent.click(await screen.findByRole('button', { name: '开始一场战斗' }))
    fireEvent.click(await screen.findByRole('button', { name: '建立这场遭遇' }))

    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledWith('story-1', expect.objectContaining({
      combatants: [
        { actor_id: 'hero', side: 'player' },
        { actor_id: 'wolf', side: 'enemy' },
      ],
    })))
  })

  it('selects the battle it just opened instead of falling back to the picker', async () => {
    const onSelectSession = vi.fn()
    apiMocks.playCombatRound.mockResolvedValue(windowData({ session_id: 'battle-new' }))
    renderEntry({
      encounter: {
        combatants: [
          { actor_id: 'hero', name: '沈砚', side: 'player' },
          { actor_id: 'wolf', name: '铁背狼', side: 'enemy' },
        ],
      },
    }, onSelectSession)

    fireEvent.click(await screen.findByRole('button', { name: '开始一场战斗' }))
    fireEvent.click(await screen.findByRole('button', { name: '建立这场遭遇' }))

    await waitFor(() => expect(onSelectSession).toHaveBeenCalledWith('battle-new'))
  })

  it('refuses to open a fight the scene does not have, and says why', async () => {
    renderEntry({ encounter: { reason_key: 'combatWindow.encounter.noEnemy' } })

    fireEvent.click(await screen.findByRole('button', { name: '开始一场战斗' }))

    expect(await screen.findByText('当前场景没有敌对的对象，不必开战。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '建立这场遭遇' })).toBeDisabled()
    expect(apiMocks.playCombatRound).not.toHaveBeenCalled()
  })
})

// R1 keeps one person in one unfinished battle. A second opening is refused, and
// the panel has to carry the player into the fight that already holds them: a
// refusal with no way in would be a dead end rather than a rule.
describe('CombatWindowPanel running battle', () => {
  function renderEntry(onSelectSession: (id: string) => void) {
    apiMocks.getCombatWindow.mockResolvedValue(windowData({
      session_id: undefined, round: undefined, sides: undefined,
      encounter: {
        combatants: [
          { actor_id: 'hero', name: '沈砚', side: 'player' },
          { actor_id: 'wolf', name: '铁背狼', side: 'enemy' },
        ],
      },
    }))
    render(
      <CombatWindowPanel
        storyId="story-1" branchId="branch-1" active sessionId=""
        onSelectSession={onSelectSession} onClose={() => {}}
      />,
    )
  }

  it('offers to enter the battle the refusal named, and enters it', async () => {
    const onSelectSession = vi.fn()
    apiMocks.playCombatRound.mockRejectedValue(new APIError('already fighting', {
      status: 409,
      code: 'api.combatWindow.actorAlreadyFighting',
      details: { session_id: 'battle-old' },
    }))
    renderEntry(onSelectSession)

    fireEvent.click(await screen.findByRole('button', { name: '开始一场战斗' }))
    fireEvent.click(await screen.findByRole('button', { name: '建立这场遭遇' }))

    // The refusal still says what happened, and now also offers the way in.
    expect(await screen.findByText(/已经在一场没有结束的交手里/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '进入那场战斗' }))

    expect(onSelectSession).toHaveBeenCalledWith('battle-old')
    // Entering is a selection, never a second opening.
    expect(apiMocks.playCombatRound).toHaveBeenCalledTimes(1)
  })

  it('does not turn an unrelated failure into an offer to enter another battle', async () => {
    apiMocks.playCombatRound.mockRejectedValue(new APIError('server is down', { status: 500 }))
    renderEntry(vi.fn())

    fireEvent.click(await screen.findByRole('button', { name: '开始一场战斗' }))
    fireEvent.click(await screen.findByRole('button', { name: '建立这场遭遇' }))

    expect(await screen.findByText(/server is down/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '进入那场战斗' })).toBeNull()
  })
})

// R3: opening a battle establishes the encounter and settles nothing. The first
// exchange is the player's own decision, made after seeing the state both sides
// are actually in, so the panel must not send an action with the opening and
// must not ask for prose about a round that does not exist.
describe('CombatWindowPanel opening a battle', () => {
  const encounter = {
    combatants: [
      { actor_id: 'hero', name: '沈砚', side: 'player' as const },
      { actor_id: 'wolf', name: '铁背狼', side: 'enemy' as const },
    ],
  }

  it('opens the encounter without sending an action or writing a passage', async () => {
    const onSelectSession = vi.fn()
    apiMocks.getCombatWindow.mockResolvedValue(windowData({
      session_id: undefined, round: undefined, sides: undefined, encounter,
    }))
    apiMocks.playCombatRound.mockResolvedValue(windowData({ session_id: 'battle-new', round: 1, sides: undefined }))
    render(
      <CombatWindowPanel
        storyId="story-1" branchId="branch-1" active sessionId=""
        onSelectSession={onSelectSession} onClose={() => {}}
      />,
    )

    fireEvent.click(await screen.findByRole('button', { name: '开始一场战斗' }))
    fireEvent.click(await screen.findByRole('button', { name: '建立这场遭遇' }))

    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledTimes(1))
    const [, payload] = apiMocks.playCombatRound.mock.calls[0]
    expect(payload.sessionId).toBeUndefined()
    expect(payload.actions).toEqual([])
    expect(payload.combatants).toEqual([
      { actor_id: 'hero', side: 'player' },
      { actor_id: 'wolf', side: 'enemy' },
    ])
    // No round was settled, so there is no round whose passage could be written.
    expect(apiMocks.narrateCombatRound).not.toHaveBeenCalled()
    expect(onSelectSession).toHaveBeenCalledWith('battle-new')
  })

  it('says the encounter is waiting for an action, then settles round 1 from it', async () => {
    const entry = windowData({ session_id: undefined, round: undefined, sides: undefined, encounter })
    const opened = windowData({ session_id: 'battle-new', round: 1, history: [] })
    const played = windowData({
      session_id: 'battle-new', round: 1,
      history: [{ event_id: 'round-1', outcome: { round: 1, steps: [] } }],
      last_round: { event_id: 'round-1', outcome: { round: 1, steps: [] } },
    })
    apiMocks.getCombatWindow.mockImplementation((_storyId: string, options?: { sessionId?: string }) =>
      Promise.resolve(options?.sessionId ? opened : entry))
    apiMocks.playCombatRound.mockImplementation((_storyId: string, payload: { sessionId?: string }) =>
      Promise.resolve(payload.sessionId ? played : opened))

    function Host() {
      const [sessionId, setSessionId] = useState('')
      return (
        <CombatWindowPanel
          storyId="story-1" branchId="branch-1" active sessionId={sessionId}
          onSelectSession={setSessionId} onClose={() => {}}
        />
      )
    }
    render(<Host />)

    fireEvent.click(await screen.findByRole('button', { name: '开始一场战斗' }))
    fireEvent.click(await screen.findByRole('button', { name: '建立这场遭遇' }))

    // The opened fight shows both sides at their current numbers and asks for
    // the first action instead of leaving an empty history to read as a loss.
    expect(await screen.findByText(/双方已经照面，还没有交手/)).toBeInTheDocument()
    expect(screen.queryByText('还没有交手的记录。')).toBeNull()

    fireEvent.click(await screen.findByRole('button', { name: '结算本轮' }))

    await waitFor(() => expect(apiMocks.playCombatRound).toHaveBeenCalledTimes(2))
    const [, second] = apiMocks.playCombatRound.mock.calls[1]
    expect(second.sessionId).toBe('battle-new')
    expect(second.actions).toHaveLength(1)
    // Only now does a round exist, so only now is a passage asked for.
    await waitFor(() => expect(apiMocks.narrateCombatRound).toHaveBeenCalledTimes(1))
  })
})


describe('protagonist control with companions', () => {
  it('keeps an observed ally visible but never takes it over when the protagonist falls', async () => {
    const initial = windowData()
    renderPanel({ sides: [
      { actor_id: 'beast', name: '青岚', side: 'player', exact: false, standing: 'standing', observed: { condition: 'combatWindow.condition.healthy' } } as CombatWindowSide,
      { ...initial.sides![0], standing: 'fallen' } as CombatWindowSide,
      initial.sides![1],
    ] })
    expect(await screen.findByText('青岚')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '结算本轮' })).toBeNull()
    expect(apiMocks.playCombatRound).not.toHaveBeenCalled()
  })
  it('does not infer the controlled actor from side or precision if the identity is absent', async () => {
    renderPanel({ player_actor_id: undefined })
    expect(await screen.findByText('沈砚')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '结算本轮' })).toBeNull()
  })
})


it.each(['', 'protagonist'])('labels only an unnamed canonical protagonist without exposing its internal id', async name => {
  const initial = windowData()
  renderPanel({ player_actor_id: 'protagonist', sides: [
    { ...initial.sides![0], actor_id: 'protagonist', name }, initial.sides![1],
  ] })
  expect(await screen.findByText('主角')).toBeInTheDocument()
  expect(screen.queryByText('protagonist')).toBeNull()
})
