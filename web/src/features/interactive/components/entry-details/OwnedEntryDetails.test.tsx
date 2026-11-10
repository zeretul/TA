import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import type { Snapshot } from '../../types'
import { OwnedEntryDetails } from './OwnedEntryDetails'
const api = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/lib/api-client', () => ({ requestJSON: api.request }))
const snapshot = (branch = 'main'): Snapshot => ({ story_id: 'story', branch_id: branch, turns: [], context_revision: 1, state: {}, actor_state_schema: { version: 1, revision: 1, system: { adventure_rules: { family: 'wuxia', skills: [], items: [] } } } })
const response = { head: 'h1', family: 'wuxia', actor: { skills: { learned: 1 }, inventory: {}, training: {} }, skills: [{ id: 'learned', name: 'A known skill', quality_grade: 'orange', description: 'An introduction.', background: 'A background.', required_rank: 5, training_hours: 24, cost: 17, learned: true, trainable: false, usable: false, reason_key: 'api.adventure.rejected' }], items: [] }
beforeEach(async () => { api.request.mockReset(); await i18n.changeLanguage('en-US') })
afterEach(async () => { cleanup(); await i18n.changeLanguage('zh-CN') })
const expand = () => fireEvent.click(screen.getByRole('button', { name: 'Your Martial arts, skills and items' }))
it('fetches only when opened and renders a skill guide without gameplay commands', async () => {
  api.request.mockResolvedValue(response)
  const original = JSON.stringify(response), originalSnapshot = snapshot()
  const saved = JSON.stringify(originalSnapshot)
  render(<OwnedEntryDetails snapshot={originalSnapshot} />)
  expect(api.request).not.toHaveBeenCalled()
  expand()
  fireEvent.click(await screen.findByRole('button', { name: 'Inspect A known skill' }))
  expect(screen.getByRole('dialog')).toBeInTheDocument()
  expect(screen.getByText('An introduction.')).toBeVisible()
  expect(screen.getByText('A background.')).toBeVisible()
  expect(screen.queryByText('17')).not.toBeInTheDocument()
  expect(api.request.mock.calls).toEqual([['/api/interactive/stories/story/adventure-window?branch=main']])
  expect(JSON.stringify(response)).toBe(original)
  expect(JSON.stringify(originalSnapshot)).toBe(saved)
})
it('drops an old branch read, reloads the current language, and reports an actual empty result', async () => {
  let old!: (value: unknown) => void
  api.request.mockImplementation((url: string) => url.includes('branch=main') ? new Promise(resolve => { old = resolve }) : Promise.resolve({ ...response, skills: [] }))
  const view = render(<OwnedEntryDetails snapshot={snapshot()} />)
  expand()
  view.rerender(<OwnedEntryDetails snapshot={snapshot('fork')} />)
  await screen.findByText('No registered Martial arts, skills or owned items yet.')
  await act(async () => old(response))
  expect(screen.queryByText('A known skill')).not.toBeInTheDocument()
  await act(async () => { await i18n.changeLanguage('zh-CN') })
  await screen.findByText('暂无已登记的武学、技能或持有物品。')
  expect(api.request.mock.calls.filter(([url]) => url.includes('branch=fork'))).toHaveLength(2)
})
it('reports failed details without hiding canonical state and supports a read-only retry', async () => {
  api.request.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ ...response, skills: [] })
  render(<OwnedEntryDetails snapshot={snapshot()} />)
  expand()
  await screen.findByRole('alert')
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
  await screen.findByText('No registered Martial arts, skills or owned items yet.')
  await waitFor(() => expect(api.request).toHaveBeenCalledTimes(2))
})
it('refetches optional completed art on reopening even when the game head did not move', async () => {
  api.request.mockResolvedValueOnce(response).mockResolvedValueOnce({ ...response, skills: [{ ...response.skills[0], art: { source: 'preset', preset_id: 'wuxia-skill' } }] })
  const view = render(<OwnedEntryDetails snapshot={snapshot()} />)
  expand()
  fireEvent.click(await screen.findByRole('button', { name: 'Inspect A known skill' }))
  expect(screen.queryByRole('img')).toBeNull()
  fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
  expand(); expand()
  fireEvent.click(await screen.findByRole('button', { name: 'Inspect A known skill' }))
  await screen.findByRole('img', { name: 'Illustration of A known skill' })
  expect(view.container.querySelector('.entry-artwork__image')).toBeNull()
  expect(api.request).toHaveBeenCalledTimes(2)
  expect(api.request.mock.calls.every(call => call.length === 1 && call[0] === '/api/interactive/stories/story/adventure-window?branch=main')).toBe(true)
})
it('shows a learned method’s missing materials and deployed-asset costs without listing them as possessions', async () => {
  api.request.mockResolvedValue({ ...response, family: 'space', skills: [], craft_methods: [{
    id: 'gen.craft.receiver', name: 'Signal analysis', skill_id: 'analysis', skill_name: 'Signal engineering', skill_rank: 2,
    hours: 4, materials: { recording: 1 }, output_id: 'fragment', output_count: 1, required_asset: 'processor', fuel_cost: 10,
    item_names: { recording: 'Recorded transmission', fragment: 'Signal fragments', processor: 'Signal processor' },
  }] })
  render(<OwnedEntryDetails snapshot={snapshot()} />)
  expand()
  fireEvent.click(await screen.findByRole('button', { name: 'Inspect Signal analysis' }))
  expect(screen.getByText('Recorded transmission × 1')).toBeVisible()
  expect(screen.getByText('Signal fragments × 1')).toBeVisible()
  expect(screen.getByText('Signal processor')).toBeVisible()
  expect(screen.getByText('Required deployed asset')).toBeVisible()
  expect(screen.getByText('Crafting time (hours)')).toBeVisible()
  expect(screen.getByText('Minimum skill mastery')).toBeVisible()
  expect(screen.getByText('Fuel consumed')).toBeVisible()
  fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
  expect(screen.queryByRole('button', { name: 'Inspect Signal processor' })).not.toBeInTheDocument()
  expect(api.request.mock.calls).toEqual([['/api/interactive/stories/story/adventure-window?branch=main']])
})
