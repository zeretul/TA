import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import type { Snapshot } from '../../types'
import { StoryStateDetails } from '../story-state/StoryStateLedger'
import cultivationEN from './__fixtures__/cultivation-entry-details-en-US.json'
import cultivationZH from './__fixtures__/cultivation-entry-details-zh-CN.json'
import adventureEN from './__fixtures__/adventure-window-en-US.json'
import adventureZH from './__fixtures__/adventure-window-zh-CN.json'
const api = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/lib/api-client', () => ({ requestJSON: api.request }))

// These bytes come from real Store.CreateStory -> freeze/normalise -> branch
// cold reopen -> App DTO serialization. Only transport is stubbed here; no
// invented frontend DTO bypasses the server's actual field/serialization path.
const fixtures = { 'en-US': { cultivation: cultivationEN, adventure: adventureEN }, 'zh-CN': { cultivation: cultivationZH, adventure: adventureZH } }
function snapshot(adventure: boolean): Snapshot {
  return { story_id: 'fixture-story', branch_id: 'fork', turns: [], state: { actors: { protagonist: { name: 'Player', template_id: 'hero', state: { 境界: 'qi_3', 重要物品: { 'Unknown handmade memento': { 描述: 'My own unfinished carving.', 来源: 'Made at camp.' } } } } } }, actor_state_schema: { version: 1, revision: 1, system: { templates: [{ id: 'hero', name: 'Hero', fields: [{ name: '境界', type: 'string' }, { name: '重要物品', type: 'object' }] }], ...(adventure ? { adventure_rules: { family: 'wuxia' as const, skills: [], items: [] } } : {}) } } }
}
afterEach(async () => { cleanup(); api.request.mockReset(); document.documentElement.classList.remove('dark'); await i18n.changeLanguage('zh-CN') })
for (const language of ['zh-CN', 'en-US'] as const) for (const dark of [false, true]) for (const width of [375, 1188]) {
  it(`renders real cultivation DTO in the full state flow: ${language}, ${dark ? 'dark' : 'light'}, ${width}px`, async () => {
    await i18n.changeLanguage(language)
    document.documentElement.classList.toggle('dark', dark)
    const dto = fixtures[language].cultivation
    const before = JSON.stringify(dto), state = snapshot(false), stateBefore = JSON.stringify(state)
    api.request.mockResolvedValue({ ...dto, items: [...dto.items, { id: 'memento', name: 'Unknown handmade memento', quantity: 1, uncatalogued: true, instance_description: 'My own unfinished carving.' }] })
    render(<div style={{ width }}><StoryStateDetails snapshot={state} /></div>)
    fireEvent.click(within(screen.getByTestId('owned-entry-details')).getByRole('button'))
    expect(await screen.findAllByRole('button', { name: i18n.t('entryDetails.inspect', { name: 'Unknown handmade memento' }) })).toHaveLength(1)
    expect(screen.getAllByText('Unknown handmade memento')).toHaveLength(1)
    const skillName = i18n.t('skill.fire_bolt.name')
    fireEvent.click(await screen.findByRole('button', { name: i18n.t('entryDetails.inspect', { name: skillName }) }))
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText(dto.skills[0].background)).toBeVisible()
    expect(within(dialog).getByText('own skill wording')).toBeVisible()
    expect(within(dialog).queryByText('actual teacher')).not.toBeInTheDocument()
    expect(within(dialog).getByText(i18n.t('entryDetails.quality.white'))).toBeVisible()
    expect(within(dialog).queryByText(i18n.t('entryDetails.fact.mana'))).not.toBeInTheDocument()
    fireEvent.keyDown(dialog, { key: 'Escape' })
    fireEvent.click(await screen.findByRole('button', { name: i18n.t('entryDetails.inspect', { name: 'Owned blade' }) }))
    const itemDialog = screen.getByRole('dialog')
    expect(within(itemDialog).getByText('real worn hilt')).toBeVisible()
    expect(within(itemDialog).getByText('actual gift')).toBeVisible()
    expect(within(itemDialog).getByText(i18n.t('entryDetails.fact.stageEligibility'))).toBeVisible()
    expect(within(itemDialog).queryByText(i18n.t('entryDetails.fact.useEligibility'))).not.toBeInTheDocument()
    expect(within(itemDialog).queryByText(/rare|upper|enemy_inventory|localizations/)).not.toBeInTheDocument()
    expect(JSON.stringify(dto)).toBe(before)
    expect(JSON.stringify(state)).toBe(stateBefore)
    expect(api.request.mock.calls.every(call => call.length === 1 && call[0].includes('/cultivation-entry-details?branch=fork'))).toBe(true)
  })
}
for (const language of ['zh-CN', 'en-US'] as const) it(`renders real adventure DTO in state inspection without source mutation: ${language}`, async () => {
  await i18n.changeLanguage(language)
  const dto = fixtures[language].adventure
  api.request.mockResolvedValue(dto)
  const before = JSON.stringify(dto)
  render(<StoryStateDetails snapshot={snapshot(true)} />)
  fireEvent.click(within(screen.getByTestId('owned-entry-details')).getByRole('button'))
  fireEvent.click(await screen.findByRole('button', { name: i18n.t('entryDetails.inspect', { name: dto.skills[0].name }) }))
  const dialog = screen.getByRole('dialog')
  expect(within(dialog).getByText(dto.skills[0].description)).toBeVisible()
  expect(within(dialog).getByText(dto.skills[0].background)).toBeVisible()
  expect(within(dialog).queryByText(i18n.t('entryDetails.fact.requiredRank'))).not.toBeInTheDocument()
  expect(within(dialog).queryByText(i18n.t('entryDetails.fact.innerEnergy'))).not.toBeInTheDocument()
  expect(JSON.stringify(dto)).toBe(before)
  expect(document.body.textContent).not.toMatch(/enemy_inventory|hidden reward|private-check/)
})
for (const language of ['zh-CN', 'en-US'] as const) it(`reads actual equipped and consumable entries, with server-calculated effects: ${language}`, async () => {
  await i18n.changeLanguage(language)
  api.request.mockResolvedValue(fixtures[language].cultivation)
  render(<StoryStateDetails snapshot={snapshot(false)} />)
  fireEvent.click(within(screen.getByTestId('owned-entry-details')).getByRole('button'))
  const weapons = await screen.findByRole('tab', { name: i18n.t('entryDetails.collection.weapons') })
  fireEvent.mouseDown(weapons, { button: 0, ctrlKey: false })
  fireEvent.click(screen.getByRole('button', { name: i18n.t('entryDetails.inspect', { name: 'Equipped focus' }) }))
  let dialog = screen.getByRole('dialog')
  expect(within(dialog).getByText(i18n.t('combatWindow.slot.weapon'))).toBeVisible()
  expect(within(dialog).getByText('actual school issue')).toBeVisible()
  fireEvent.keyDown(dialog, { key: 'Escape' })
  fireEvent.mouseDown(screen.getByRole('tab', { name: i18n.t('entryDetails.collection.pills') }), { button: 0, ctrlKey: false })
  fireEvent.click(await screen.findByRole('button', { name: i18n.t('entryDetails.inspect', { name: 'Restoring pill' }) }))
  dialog = screen.getByRole('dialog')
  expect(within(dialog).getByText(i18n.t('entryDetails.fact.restore_amount'))).toBeVisible()
  expect(within(dialog).getByText('20')).toBeVisible()
  expect(within(dialog).getByText('actual apothecary')).toBeVisible()
})
