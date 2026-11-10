import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import type { RealmExplore, RealmExploreApproach } from '../../realm-types'
import { RealmExploreBlock } from './RealmExploreBlock'

const approach: RealmExploreApproach = {
  id: 'server-only-id', kind: 'approach', name: '本地化前名称', name_key: 'game.realm.explore.serviceBay.repair.name',
  description: '本地化前描述', description_key: 'game.realm.explore.serviceBay.repair.description',
  duration_seconds: 600, pressure_delta: 0, costs: [
    { kind: 'use_skill', ref_id: 'secret-skill-id', name: '已学电路短接', quantity: 1, energy_cost: 8 },
    { kind: 'consume_item', ref_id: 'secret-item-id', name: '合金丝', quantity: 1 },
    { kind: 'require_item', ref_id: 'secret-tool-id', name: '扳手', quantity: 1 },
  ], available: false, causes_setback: false, blocked_reason: 'capability',
}
const explore: RealmExplore = {
  pressure: 2, pressure_limit: 4, pressure_label: '供电负荷', pressure_label_key: 'game.realm.explore.serviceBay.pressure',
  setback: false, return_available: false, approaches: [approach],
}
afterEach(() => { setConfiguredLocale('zh-CN') })

describe('projected exploration choices', () => {
  it('keeps a blocked method visible with real cost names but cannot draft it', async () => {
    const draft = vi.fn(), user = userEvent.setup()
    render(<RealmExploreBlock explore={explore} realmName="检修舱" inside onDraftAction={draft} />)
    expect(screen.getByText('消耗：合金丝 × 1')).toBeInTheDocument()
    expect(screen.getByText(/技能消耗 8/)).toBeInTheDocument()
    expect(screen.getByText('需持有（不消耗）：扳手 × 1')).toBeInTheDocument()
    expect(screen.getByText(/请先准备/)).toBeInTheDocument()
    const button = screen.getByRole('button', { name: '加入打算：飞线抢修后通过' })
    expect(button).toBeDisabled()
    await user.click(button)
    expect(draft).not.toHaveBeenCalled()
    expect(document.body.textContent).not.toMatch(/secret-|server-only-id/)
  })

  it('puts the available recovery first without removing blocked alternatives or mutating the server view', () => {
    const state = { ...explore, setback: true, approaches: [approach, { ...approach, id: 'residue', name_key: 'game.realm.explore.oldBoat.residue.name', available: true, costs: [] }] }
    render(<RealmExploreBlock explore={state} realmName="旧舟" inside onDraftAction={vi.fn()} />)
    const buttons = screen.getAllByRole('button')
    expect(buttons[0]).toHaveTextContent('回收散落铁件')
    expect(buttons[0]).toBeEnabled()
    expect(buttons[1]).toBeDisabled()
    expect(screen.getByText(/请先准备/)).toBeInTheDocument()
    expect(state.approaches[0]).toBe(approach)
  })

  it('explains the unused cooperation requirement without revealing an unknown partner', () => {
    render(<RealmExploreBlock explore={{ ...explore, approaches: [{ ...approach,
      name_key: 'game.realm.explore.serviceBay.assisted_slow.name',
      description_key: 'game.realm.explore.serviceBay.assisted_slow.description',
      blocked_reason: 'cooperation_required', costs: [],
    }] }} realmName="检修舱" inside onDraftAction={vi.fn()} />)
    expect(screen.getByRole('button', { name: '加入打算：按标记逐段断电穿行' })).toBeDisabled()
    expect(screen.getByText(/实际完成这处场地的共同准备/)).toBeInTheDocument()
    expect(screen.getByText(/也可以继续独行/)).toBeInTheDocument()
    expect(document.body.textContent).not.toMatch(/赵灵|宋老橹|席尔瓦|npc_|xinghai-p/)
  })

  it('only stages a localized intention, without changing pressure or rewards', async () => {
    const draft = vi.fn(), user = userEvent.setup()
    const state = { ...explore, approaches: [{ ...approach, available: true, causes_setback: true, pressure_delta: 2 }] }
    render(<RealmExploreBlock explore={state} realmName="检修舱" inside onDraftAction={draft} />)
    expect(screen.getByText('这次行动将达到压力上限并导致受挫。')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '加入打算：飞线抢修后通过' }))
    expect(draft).toHaveBeenCalledExactlyOnceWith('我想在检修舱采取“飞线抢修后通过”的办法。')
    expect(screen.getByText('2 / 4')).toBeInTheDocument()
    expect(state).toMatchObject({ pressure: 2, setback: false })
  })

  it('reports remaining story time without running a wall-clock timer', () => {
    const view = render(<RealmExploreBlock explore={{ ...explore, return_due_seconds: 7200 }} realmName="检修舱" inside clockSeconds={3600} />)
    expect(screen.getByText(/还需 60 分钟 世界时间/)).toBeInTheDocument()
    expect(screen.getByText(/停留在此页面不会计时/)).toBeInTheDocument()
    view.rerender(<RealmExploreBlock explore={{ ...explore, return_due_seconds: 7200, return_available: true }} realmName="检修舱" inside clockSeconds={7200} />)
    expect(screen.getByText(/回访所需的世界时间已到/)).toBeInTheDocument()
  })

  it('distinguishes an opened return route from a later setback, without a false zero wait', () => {
    const view = render(<RealmExploreBlock explore={{ ...explore, return_due_seconds: 7200, return_opened: true }} realmName="检修舱" inside clockSeconds={8000} />)
    expect(screen.getByText('回访侧路已经打开。已取走的物资不会补回。')).toBeInTheDocument()
    expect(screen.queryByText(/还需 0/)).not.toBeInTheDocument()
    view.rerender(<RealmExploreBlock explore={{ ...explore, return_due_seconds: 7200, return_opened: true, setback: true }} realmName="检修舱" inside clockSeconds={8000} />)
    expect(screen.getByText(/受挫已使这次回访机会失去/)).toBeInTheDocument()
    expect(screen.queryByText('回访侧路已经打开。已取走的物资不会补回。')).not.toBeInTheDocument()
  })

  it('does not enable inside actions when outside or when the story is busy', () => {
    const state = { ...explore, approaches: [{ ...approach, available: true }] }
    const view = render(<RealmExploreBlock explore={state} realmName="检修舱" inside={false} onDraftAction={vi.fn()} />)
    expect(screen.getByRole('button')).toBeDisabled()
    view.rerender(<RealmExploreBlock explore={state} realmName="检修舱" inside draftingDisabled onDraftAction={vi.fn()} />)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('uses independent English text for known names, costs and deterministic consequences', () => {
    act(() => setConfiguredLocale('en-US'))
    render(<RealmExploreBlock explore={{ ...explore, approaches: [{ ...approach, name_key: 'game.realm.explore.herbCave.ward.name', description_key: 'game.realm.explore.herbCave.ward.description', costs: [], causes_setback: true }] }} realmName="Herb Cave" inside />)
    expect(screen.getByText('Cross under a ward')).toBeInTheDocument()
    expect(screen.getByText('Power strain')).toBeInTheDocument()
    expect(screen.getByText('This action will reach the pressure limit and cause a setback.')).toBeInTheDocument()
    expect(document.body.textContent).not.toMatch(/game\.realm|realmWindow\./)
  })
})
