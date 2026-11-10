import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import type { RealmCooperation } from '../../realm-types'
import { RealmCooperationBlock } from './RealmCooperationBlock'

const project: RealmCooperation = {
  project_id: 'internal-project', project_version: '1.0', realm_id: 'internal-realm', instance_id: 'instance-a',
  name_key: 'game.cooperation.projects.servicebay.name', description_key: 'game.cooperation.projects.servicebay.description',
  player_role_key: 'game.cooperation.projects.servicebay.playerRole', partner_role_key: 'game.cooperation.projects.servicebay.partnerRole',
  clue_text: '旧默认文案不优先', clue_text_key: 'game.realm.explore.serviceBay.facts.clue',
  partner_id: 'internal-actor', partner_name: '已认识的工友', location_id: 'internal-place', location_name: '47号减压长廊',
  phase: 'none', duration_seconds: 600, partner_energy_cost: 6, partner_energy_label_key: 'game.cooperation.energy.stamina',
  player_costs: [{ kind: 'consume_item', ref_id: 'internal-wire', name: '合金丝', quantity: 1 }],
  can_ask: true, can_work_if_agreed: true, can_work: false, can_report: false,
  report_facts: [],
}
const facts: RealmCooperation['report_facts'] = [
  { kind: 'ingress', source_ref: 'private-ingress-ref', description_key: 'game.cooperation.reportFacts.ingress', value_key: 'game.realm.explore.serviceBay.assisted_slow.name' },
  { kind: 'disposition', source_ref: 'private-disposition-ref', description_key: 'game.cooperation.reportFacts.disposition', value_key: 'game.realm.explore.serviceBay.preserve.name' },
  { kind: 'setback', source_ref: 'private-setback-ref', description_key: 'game.cooperation.reportFacts.setback' },
  { kind: 'side', source_ref: 'private-side-ref', description_key: 'game.cooperation.reportFacts.side' },
]
afterEach(() => setConfiguredLocale('zh-CN'))

describe('bounded cooperation drafts', () => {
  it('shows separate contributions and story time, and asking shares nothing or starts no work', async () => {
    const draft = vi.fn(), user = userEvent.setup()
    render(<RealmCooperationBlock project={project} onDraftAction={draft} />)
    expect(screen.getByText('共同作业：10 分钟游戏世界时间，不是现实等待。')).toBeInTheDocument()
    expect(screen.getByText('已认识的工友自己支付 6 点气力，不从你身上扣除。')).toBeInTheDocument()
    expect(screen.getByText('你的投入：消耗：合金丝 × 1')).toBeInTheDocument()
    expect(screen.getByText(/不想分享或对方不愿意，也可以独行/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '先问是否愿意，不透露线索' }))
    expect(draft).toHaveBeenCalledExactlyOnceWith(expect.stringContaining('现在只问意愿，不透露我发现的线索，也不开始工作'))
    expect(document.body.textContent).not.toMatch(/internal-|instance-a/)
    expect(draft.mock.calls[0][0]).not.toContain('隔离标识表明')
  })

  it('requires explicit sharing and the partner’s consent in the work-if-agreed draft', async () => {
    const draft = vi.fn()
    render(<RealmCooperationBlock project={project} onDraftAction={draft} />)
    await userEvent.setup().click(screen.getByRole('button', { name: '分享这条线索，若对方同意就一起准备' }))
    const message = draft.mock.calls[0][0]
    expect(message).toContain('隔离标识表明逐段断电可从维护通道安全慢行。')
    expect(message).toContain('其他未说的发现仍保留')
    expect(message).toContain('只有对方明确答应且各自条件具备时')
    expect(message).not.toMatch(/internal-|instance-a/)
    expect(screen.getByText('还没一起准备')).toBeInTheDocument()
  })

  it('shows exactly the discovered clue and uses author text only when no localization key is supplied', async () => {
    const view = render(<RealmCooperationBlock project={project} onDraftAction={vi.fn()} />)
    expect(screen.getByText('这次调查记录：隔离标识表明逐段断电可从维护通道安全慢行。')).toBeInTheDocument()
    expect(screen.queryByText(/旧默认文案不优先/)).not.toBeInTheDocument()
    view.rerender(<RealmCooperationBlock project={{ ...project, clue_text_key: undefined, clue_text: '作者自定义的已知观察。' }} onDraftAction={vi.fn()} />)
    expect(screen.getByText('这次调查记录：作者自定义的已知观察。')).toBeInTheDocument()
    view.rerender(<RealmCooperationBlock project={{ ...project, clue_text_key: undefined, clue_text: undefined }} onDraftAction={vi.fn()} />)
    expect(screen.getByRole('button', { name: '分享这条线索，若对方同意就一起准备' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '先问是否愿意，不透露线索' })).toBeEnabled()
  })

  it('does not invent materials from the partner; asking can remain available while work is blocked', () => {
    render(<RealmCooperationBlock project={{ ...project, can_work_if_agreed: false, blocked_reason: 'missing_material' }} onDraftAction={vi.fn()} />)
    expect(screen.getByRole('button', { name: '先问是否愿意，不透露线索' })).toBeEnabled()
    expect(screen.getByRole('button', { name: '分享这条线索，若对方同意就一起准备' })).toBeDisabled()
    expect(screen.getByText('你的材料还不够；不会挪用对方的私人物件。')).toBeInTheDocument()
  })

  it('does not present an unreadable NPC actor as free work', () => {
    render(<RealmCooperationBlock project={{ ...project, partner_energy_cost: 0, blocked_reason: 'partner_unavailable', can_work_if_agreed: false }} onDraftAction={vi.fn()} />)
    expect(screen.getByText('当前无法核实对方的工作条件，不能认定这项分工不耗能量。')).toBeInTheDocument()
    expect(screen.queryByText('这项分工不另扣对方能量。')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '分享这条线索，若对方同意就一起准备' })).toBeDisabled()
  })

  it('uses existing consent without re-sharing; completed work does not pretend the site was crossed', async () => {
    const draft = vi.fn(), user = userEvent.setup()
    const view = render(<RealmCooperationBlock project={{ ...project, phase: 'agreed', can_work: true }} onDraftAction={draft} />)
    await user.click(screen.getByRole('button', { name: '按已答应的分工一起准备' }))
    expect(draft).toHaveBeenCalledExactlyOnceWith(expect.stringContaining('不扩大已经分享的内容'))
    view.rerender(<RealmCooperationBlock project={{ ...project, phase: 'worked' }} onDraftAction={draft} />)
    expect(screen.getByText('共同准备已完成；你还没有在现场使用方案')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('starts with no reported facts selected, and drafts only the player’s selected real outcomes', async () => {
    const draft = vi.fn(), user = userEvent.setup()
    render(<RealmCooperationBlock project={{ ...project, phase: 'used', can_report: true, report_facts: facts }} onDraftAction={draft} />)
    expect(screen.getAllByRole('checkbox')).toHaveLength(4)
    screen.getAllByRole('checkbox').forEach(box => expect(box).not.toBeChecked())
    const report = screen.getByRole('button', { name: '只把所选结果加入告知打算' })
    expect(report).toBeDisabled()
    await user.click(screen.getByRole('checkbox', { name: '核心选择了“保电完成离线诊断”' }))
    await user.click(report)
    const message = draft.mock.calls[0][0]
    expect(message).toContain('核心选择了“保电完成离线诊断”')
    expect(message).not.toMatch(/实际采用了|已经受挫|亲自走到了|private-|internal-/)
    expect(message).toContain('不分享其他发现')
  })

  it('keeps fact categories separately selectable and clears them for a different instance or result set', async () => {
    const user = userEvent.setup()
    const sameRef = [facts[0], { ...facts[2], source_ref: facts[0].source_ref }]
    const view = render(<RealmCooperationBlock project={{ ...project, phase: 'used', can_report: true, report_facts: sameRef }} onDraftAction={vi.fn()} />)
    await user.click(screen.getAllByRole('checkbox')[0])
    expect(screen.getAllByRole('checkbox')[1]).not.toBeChecked()
    view.rerender(<RealmCooperationBlock project={{ ...project, instance_id: 'instance-b', phase: 'used', can_report: true, report_facts: sameRef }} onDraftAction={vi.fn()} />)
    screen.getAllByRole('checkbox').forEach(box => expect(box).not.toBeChecked())
    await user.click(screen.getAllByRole('checkbox')[0])
    view.rerender(<RealmCooperationBlock project={{ ...project, instance_id: 'instance-b', phase: 'reported', can_report: true, report_facts: [facts[3]] }} onDraftAction={vi.fn()} />)
    expect(screen.getByRole('checkbox')).not.toBeChecked()
    expect(screen.getByRole('button', { name: '只把所选结果加入告知打算' })).toBeDisabled()
  })

  it('obeys busy and server eligibility for all drafts without any write path', () => {
    const view = render(<RealmCooperationBlock project={project} onDraftAction={vi.fn()} draftingDisabled />)
    screen.getAllByRole('button').forEach(button => expect(button).toBeDisabled())
    view.rerender(<RealmCooperationBlock project={{ ...project, phase: 'used', can_report: false, report_facts: facts, blocked_reason: 'not_present' }} onDraftAction={vi.fn()} />)
    screen.getAllByRole('checkbox').forEach(box => expect(box).toBeDisabled())
    expect(screen.getByText('对方现在不在场；知道去哪里找人不等于已经会面。')).toBeInTheDocument()
  })

  it('uses independent English descriptions, costs and plans without leaking source identifiers', async () => {
    act(() => setConfiguredLocale('en-US'))
    const draft = vi.fn()
    render(<RealmCooperationBlock project={{ ...project, partner_name: 'Known mechanic', location_name: 'Corridor 47', player_costs: [{ ...project.player_costs[0], name: 'Alloy wire' }] }} onDraftAction={draft} />)
    expect(screen.getByText('Prepare offline markers')).toBeInTheDocument()
    expect(screen.getByText('Known mechanic pays 6 stamina from their own reserves, not yours.')).toBeInTheDocument()
    expect(document.body.textContent).not.toMatch(/[\u4e00-\u9fff]|game\.|realmWindow\.|internal-/)
    await userEvent.setup().click(screen.getByRole('button', { name: 'Ask first, without sharing the clue' }))
    expect(draft).toHaveBeenCalledExactlyOnceWith(expect.stringContaining('I am not sharing my clue or starting the work'))
  })
})
