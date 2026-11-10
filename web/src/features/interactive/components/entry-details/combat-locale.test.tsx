import { act, cleanup, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import { AdventureCombatPanel } from '../combat-window/AdventureCombatPanel'
import { useCombatWindow } from '../combat-window/use-combat-window'
import type { CombatWindow } from '../../types'
import adventureZH from './__fixtures__/adventure-window-zh-CN.json'
import adventureEN from './__fixtures__/adventure-window-en-US.json'
const mocks = vi.hoisted(() => ({ request: vi.fn(), combat: vi.fn(), play: vi.fn(), narrate: vi.fn() }))
vi.mock('@/lib/api-client', () => ({ requestJSON: mocks.request, jsonHeaders: {}, createAgentCommandID: () => 'not-used', APIError: class APIError extends Error {} }))
vi.mock('../../api', () => ({ getCombatWindow: mocks.combat, playCombatRound: mocks.play, narrateCombatRound: mocks.narrate }))
afterEach(async () => { cleanup(); vi.clearAllMocks(); await i18n.changeLanguage('zh-CN') })
it('refreshes locale-resolved adventure prose in an already open panel without submitting an action', async () => {
  await i18n.changeLanguage('zh-CN')
  mocks.request.mockImplementation(async () => i18n.language === 'en-US' ? adventureEN : adventureZH)
  render(<AdventureCombatPanel storyId="story" branchId="fork" active onClose={() => {}} />)
  await screen.findByRole('option', { name: /Known ability/ })
  await act(async () => { await i18n.changeLanguage('en-US') })
  await screen.findByRole('option', { name: /Known ability/ })
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'skill:owned' } })
  fireEvent.click(screen.getByRole('button', { name: 'Inspect Known ability' }))
  expect(screen.getByText('Fixture description')).toBeVisible()
  expect(screen.queryByText(adventureZH.skills[0].description)).not.toBeInTheDocument()
  expect(mocks.request.mock.calls.every(call => call.length === 1)).toBe(true)
})
it('re-reads the cultivation window on locale changes, without replaying or narrating a round', async () => {
  await i18n.changeLanguage('zh-CN')
  mocks.combat.mockImplementation(async (): Promise<CombatWindow> => ({ story_id: 'story', branch_id: 'fork', scene: { node_id: 'fixture' }, session_id: 'battle', round: 1, ended: false, has_more: false, sides: [{ actor_id: 'protagonist', name: 'Player', side: 'player', exact: true, standing: 'standing', has_implement: false, route_exists: true, skills: [{ id: 'fire_bolt', version: 1, castable: true, description: i18n.language === 'en-US' ? 'English intro' : '中文简介' }] }] }))
  const { result } = renderHook(() => useCombatWindow({ storyId: 'story', branchId: 'fork', sessionId: 'battle', active: true }))
  await waitFor(() => expect(result.current.data?.sides?.[0].skills?.[0].description).toBe('中文简介'))
  await act(async () => { await i18n.changeLanguage('en-US') })
  await waitFor(() => expect(result.current.data?.sides?.[0].skills?.[0].description).toBe('English intro'))
  expect(mocks.combat).toHaveBeenCalledTimes(2)
  expect(mocks.play).not.toHaveBeenCalled()
  expect(mocks.narrate).not.toHaveBeenCalled()
})
