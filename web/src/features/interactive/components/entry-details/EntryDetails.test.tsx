import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import i18n from '@/i18n'
import { EntryDetails, QualityBadge } from './EntryDetails'
import { adventurePresentations, cultivationPresentation } from './presentations'
import { QUALITY_GRADES, type AdventureDetailsResponse, type EntryPresentation } from './types'

const entry: EntryPresentation = { id: 'copy-1', name: 'A very long named blade '.repeat(12), quality_grade: 'red', category: 'equipment', description: 'Definition introduction.', background: 'The old forge had no windows. '.repeat(80), instance_description: 'A notch I made yesterday.', source: 'Given by the ferryman.', requirements: [{ label: 'Actual minimum', value: 'Foundation' }], costs: [{ label: 'Actual mana', value: '13' }], effects: [], status: [] }
afterEach(async () => { cleanup(); await i18n.changeLanguage('zh-CN'); document.documentElement.classList.remove('dark') })

describe('entry inspection', () => {
  it.each(['zh-CN', 'en-US'])('has six readable grades in %s with a redundant decorative swatch', async language => {
    await i18n.changeLanguage(language)
    for (const theme of ['', 'dark']) {
      document.documentElement.className = theme
      const view = render(<>{QUALITY_GRADES.map(grade => <QualityBadge key={grade} grade={grade} />)}</>)
      expect(view.container.querySelectorAll('[data-quality-grade]')).toHaveLength(6)
      for (const grade of QUALITY_GRADES) expect(screen.getByText(i18n.t(`entryDetails.quality.${grade}`))).toBeVisible()
      expect(view.container.querySelectorAll('[aria-hidden="true"]')).toHaveLength(6)
      view.unmount()
    }
  })
  it('opens by keyboard, preserves custom facts separately, scrolls long content and closes without a command', async () => {
    await i18n.changeLanguage('en-US')
    const user = userEvent.setup()
    render(<EntryDetails entry={entry} />)
    const trigger = screen.getByRole('button', { name: `Inspect ${entry.name.trim()}` })
    trigger.focus(); await user.keyboard('{Enter}')
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText(entry.instance_description!)).toBeVisible()
    expect(within(dialog).getByText(entry.source!)).toBeVisible()
    expect(within(dialog).getByText(entry.description!)).toBeVisible()
    expect(within(dialog).getByText(entry.background!.trim())).toBeVisible()
    expect(dialog).toHaveClass('overflow-y-auto')
    expect(dialog.className).toContain('100dvh')
    expect(within(dialog).getByText('13')).toBeVisible()
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()
  })
  it('does not infer missing quality, expose private fields, or turn unknown prose into mechanics', () => {
    const raw = { ...entry, quality_grade: undefined, definition: { grade: 'top', rarity: 'unique', hidden_inventory: { secret: 9 } }, localizations: { 'en-US': { background: 'not resolved' } } }
    const result = cultivationPresentation(raw, 'equipment', i18n.t)
    const view = render(<EntryDetails entry={result} />)
    expect(view.container.querySelector('[data-quality-grade]')).toBeNull()
    fireEvent.click(screen.getByRole('button'))
    expect(screen.queryByText(/secret|top|unique|not resolved/)).not.toBeInTheDocument()
    expect(JSON.stringify(result)).not.toMatch(/hidden_inventory|localizations|secret/)
    expect(result.effects).toEqual([])
  })
  it('keeps adventure details restricted to actually learned or owned records, never strike or a hidden catalog', () => {
    const data: AdventureDetailsResponse = { head: 'head', family: 'wuxia', actor: { skills: { learned: 1, strike: 1 }, inventory: { owned: 1, gone: 0 } }, skills: [{ id: 'learned', name: 'Learned', required_rank: 7, cost: 41, power: 3, kind: 'attack', quality_grade: 'white' }, { id: 'strike', name: 'Strike' }, { id: 'secret', name: 'Secret' }], items: [{ id: 'owned', name: 'Owned', kind: 'material', power: 0, equippable: true, deployable: false }, { id: 'gone', name: 'Gone' }] }
    const before = JSON.stringify(data)
    const rows = adventurePresentations(data, i18n.t)
    expect(rows.map(row => row.id)).toEqual(['learned', 'owned'])
    expect(rows[1].effects).toEqual([])
    expect(rows[1].status).toContainEqual({ label: i18n.t('entryDetails.fact.equippable'), value: i18n.t('entryDetails.available') })
    expect(rows[1].status).toContainEqual({ label: i18n.t('entryDetails.fact.deployable'), value: i18n.t('entryDetails.unavailable') })
    expect(rows[0].requirements).toContainEqual({ label: i18n.t('entryDetails.fact.requiredRank'), value: '7' })
    expect(rows[0].costs).toContainEqual({ label: i18n.t('entryDetails.fact.innerEnergy'), value: '41' })
    expect(JSON.stringify(data)).toBe(before)
  })
  it('shows only the backend stage check, never claims a stage-eligible item is fully usable', () => {
    const row = cultivationPresentation({ id: 'item', name: 'Item', stage_eligible: true, quantity: 1, operating_mana_cost: 9 }, 'inventory', i18n.t)
    expect(row.status).toContainEqual({ label: i18n.t('entryDetails.fact.stageEligibility'), value: i18n.t('entryDetails.yes') })
    expect(row.status.find(fact => fact.label === i18n.t('entryDetails.fact.useEligibility'))).toBeUndefined()
  })
  it('reports real unmitigated plasma heat without inventing a protective foundation', () => {
    const data: AdventureDetailsResponse = { head: 'source', family: 'space', actor: { skills: { arc: 1 } }, skills: [{ id: 'arc', name: 'Personal arc', kind: 'control', scale: 'person', cost: 9, effects: [{ kind: 'electronic_disrupt', power: 1 }], backlash: { health_percent: 2, injury: 3 } }], items: [] }
    const rows = adventurePresentations(data, i18n.t)
    expect(rows[0].costs).toContainEqual({ label: i18n.t('entryDetails.fact.backlashHealth'), value: '2%' })
    expect(rows[0].costs).toContainEqual({ label: i18n.t('entryDetails.fact.backlashInjury'), value: '3' })
    expect(rows[0].costs).toHaveLength(3)
    expect(JSON.stringify(rows)).not.toMatch(/undefined|NaN/)
  })

})
