// Explicit component-browser fixture, never used by the shipped application.
import React from 'react'
import { createRoot } from 'react-dom/client'
import { AdventureCombatPanel } from '../../src/features/interactive/components/combat-window/AdventureCombatPanel'
import { setConfiguredLocale } from '../../src/i18n'
import '../../src/index.css'

const params = new URLSearchParams(location.search)
const locale = params.get('locale') || 'zh-CN'
const english = locale === 'en-US'
const theme = params.get('theme') || 'dark'
const width = Number(params.get('width') || 620)
document.documentElement.dataset.theme = theme
document.documentElement.classList.toggle('dark', theme === 'dark')
setConfiguredLocale(locale)
const skill = english ? 'Measured ward of the north gate, as actually seen in this exchange' : '北门回风护体诀·这一回合实际观察到的招式名称'
const state = { head: 'head-1', family: 'wuxia', enemy_condition: 'wounded', actor: { health: 70, max_health: 100, energy: 80, max_energy: 100, injury: 0, skills: {}, inventory: {}, assets: {}, battle: { id: 'battle', scale: 'person', opponent_name: english ? 'North gatekeeper' : '北门守卫', round: 0, status: 'active', last_enemy_skill_name: '' } }, skills: [], items: [] }
let submissions = 0
window.fetch = async (input, init) => {
  const pathname = new URL(typeof input === 'string' ? input : input instanceof Request ? input.url : input.toString(), location.href).pathname
  if (pathname.endsWith('/adventure-window')) return Response.json(state)
  if (pathname.endsWith('/adventure-action')) {
    const command = JSON.parse(String(init?.body || '{}'))
    if (command.expected_head !== state.head) return Response.json({ error: 'Stale fixture head' }, { status: 409 })
    submissions++
    state.head = `head-${submissions + 1}`
    state.actor.battle.round = submissions
    state.actor.battle.last_enemy_skill_name = submissions === 1 ? skill : ''
    return Response.json({ event_id: `round-${submissions}` })
  }
  return Response.json({})
}
createRoot(document.getElementById('root')!).render(<div style={{ height: '100dvh', width: '100%', maxWidth: width, margin: 'auto' }}><AdventureCombatPanel storyId="enemy-tactics-fixture" branchId="main" active onClose={() => {}} /></div>)
