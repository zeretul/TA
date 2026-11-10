import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { AdventureCombatPanel } from './AdventureCombatPanel'
import i18n from '@/i18n'
const mocks = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/lib/api-client', () => ({ requestJSON: mocks.request, jsonHeaders: {}, createAgentCommandID: () => 'fantasy-round', APIError: class APIError extends Error { status = 409 } }))
afterEach(async () => { localStorage.clear(); await i18n.changeLanguage('zh-CN') })

it('uses fantasy labels and sends a real learned skill rather than inventing a combat', async () => {
  await i18n.changeLanguage('en-US')
  mocks.request.mockResolvedValue({ head: 'saved-head', family: 'fantasy', enemy_condition: 'wounded', actor: { health: 100, max_health: 100, energy: 80, max_energy: 100, skills: { spark: 1 }, inventory: { focus: 1, charcoal: 2, staff: 1 }, equipment: { 'person:weapon': 'staff' }, assets: {}, battle: { id: 'battle', scale: 'person', opponent_name: 'Bridge Practice Target', round: 1, status: 'active' } }, skills: [{ id: 'spark', name: 'Candleflame Edge', kind: 'attack', scale: 'person', cost: 7, required_tool: 'focus', materials: { charcoal: 1 }, usable: true }], items: [{ id: 'focus', name: 'Portable Focus' }, { id: 'charcoal', name: 'Charcoal' }, { id: 'staff', name: 'Focus Staff', kind: 'weapon', scale: 'person', weapon_class: 'staff' }] })
  render(<AdventureCombatPanel storyId="fantasy-story" branchId="main" active onClose={vi.fn()} />)
  await screen.findByText('Bridge Practice Target')
  expect(screen.getByText('Realms encounter')).toBeInTheDocument()
  expect(screen.getByText('Effort')).toBeInTheDocument()
  expect(screen.queryByText('Inner energy')).not.toBeInTheDocument()
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:spark' } })
  fireEvent.click(screen.getByRole('button', { name: i18n.t('combatWindow.adventure.confirm') }))
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith(expect.stringContaining('/adventure-action'), expect.objectContaining({ body: JSON.stringify({ command_id: 'fantasy-round', expected_head: 'saved-head', branch: 'main', action: { action: 'battle_round', choice: 'skill', skill_id: 'spark' } }) })))
})
