import { fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import { WorldPresentationProvider } from '../../world-presentation-context'
import type { Snapshot } from '../../types'
import { StateDetailsDialog } from './StateDetailsDialog'
import { EntryDetails } from '../entry-details/EntryDetails'

vi.mock('@/lib/api-client', async (importOriginal) => ({
  ...await importOriginal<object>(),
  requestJSON: vi.fn().mockResolvedValue({ techniques: [], skills: [], items: [] }),
}))
const frozen = {
  module_refs: { actor_state_id: 'world-actor-state-authored' },
  pack_binding: { pack_id: 'custom', scope: 'pack:custom', gameplay: {
    status_template: 'xiuxian-state', growth: 'cultivation', combat: 'cultivation', opening: 'scene',
    presentation: { 'en-US': { energy: 'Essence', techniques: 'Arts' } },
  } },
}
const snapshot: Snapshot = {
  story_id: 'story', branch_id: 'main', turns: [],
  state: { actors: { protagonist: { name: 'Hero', template_id: 'protagonist', state: { '境界': 'mortal', '灵力': 12, '灵力上限': 30 } } } },
  actor_state_schema: { version: 1, revision: 1, system: { templates: [{ id: 'protagonist', name: 'Hero', fields: [
    { name: '境界', type: 'string', value_format: 'stage' },
    { name: '灵力', type: 'number' }, { name: '灵力上限', type: 'number' },
  ] }] } },
}
beforeEach(async () => { await i18n.changeLanguage('en-US') })
afterEach(async () => { await i18n.changeLanguage('zh-CN') })
it('keeps frozen energy and ability labels inside the expanded state portal', () => {
  render(<WorldPresentationProvider story={frozen} snapshot={snapshot}><StateDetailsDialog snapshot={snapshot} /></WorldPresentationProvider>)
  fireEvent.click(screen.getByRole('button'))
  const dialog = screen.getByRole('dialog')
  expect(within(dialog).getByText('Essence', { exact: true })).toBeVisible()
  expect(within(dialog).getByRole('button', { name: 'Your Arts, skills and items' })).toBeVisible()
  expect(within(dialog).queryByText('灵力', { exact: true })).not.toBeInTheDocument()
})
it('keeps the authored technique category in an individual detail dialog', () => {
  render(<WorldPresentationProvider story={frozen} snapshot={snapshot}><EntryDetails entry={{
    id: 'method', name: 'Test method', category: 'techniques', requirements: [], costs: [], effects: [], status: [],
  }} /></WorldPresentationProvider>)
  fireEvent.click(screen.getByRole('button', { name: 'Inspect Test method' }))
  expect(within(screen.getByRole('dialog')).getByText('Arts', { exact: true })).toBeVisible()
})
