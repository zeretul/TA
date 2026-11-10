import { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { setConfiguredLocale } from '../../src/i18n'
import { AdventureCombatPanel } from '../../src/features/interactive/components/combat-window/AdventureCombatPanel'
import { StoryStateLedger } from '../../src/features/interactive/components/story-state/StoryStateLedger'
import type { Snapshot } from '../../src/features/interactive/types'
import fixtures from './adventure-progression-data.json'
import '../../src/index.css'

// Synthetic owned state over a snapshot of authored progression definitions.
// This exercises the actual UI components without a model or a player's save.
const params = new URLSearchParams(location.search)
const language = params.get('language') || 'zh-CN'
const english = language.startsWith('en')
const family = params.get('family') === 'wuxia' ? 'wuxia' : 'space'
const catalog = fixtures[family]
const stages = catalog.progression.stages
const stage = stages[stages.length - (params.get('ceiling') === '1' ? 1 : 2)]
const rank = params.get('ceiling') === '1' ? catalog.progression.max_rank : stage.min_rank
const actor = { family, rank, experience: 120, health: 100+20*(rank-1), max_health: 100+20*(rank-1), energy: 100+10*(rank-1), max_energy: 100+10*(rank-1), injury: 0, currency: 12000, skills: Object.fromEntries(catalog.skills.map(skill => [skill.id, 2])), training: {}, inventory: {}, equipment: {}, assets: {}, acquired: {}, defeated: {} }
const required = rank === catalog.progression.max_rank ? 0 : rank*100+Math.max(0,rank-10)**2*10
const following = stages.find(candidate => candidate.min_rank > rank)
const name = (id: string) => english ? (catalog.english_names as Record<string, string>)[id] || id : catalog.skills.find(skill => skill.id === id)?.name || id
const response = { head: 'fixture-head', family, clock_seconds: 20000, actor, enemy_condition: '', skills: catalog.skills.map(skill => ({ ...skill, name: name(skill.id), learned: true, trainable: true })), items: [], progression: { stage_name: english ? stage.english_name : stage.name, rank, max_rank: catalog.progression.max_rank, experience: actor.experience, required_experience: required, next_stage_name: following ? (english ? following.english_name : following.name) : undefined, next_stage_rank: following?.min_rank, foundation_name: catalog.skills.map(skill => name(skill.id)).join(' / '), required_mastery: 2, experience_per_hour: catalog.progression.experience_per_hour, currency_per_hour: catalog.progression.currency_per_hour } }
const attemptedWrites: string[] = []
;(window as Window & { progressionFixtureWrites?: string[] }).progressionFixtureWrites = attemptedWrites
const actualFetch = window.fetch.bind(window)
window.fetch = async (input, init) => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
  if (!url.includes('/api/interactive/stories/progression-fixture/')) return actualFetch(input, init)
  const method = init?.method || (input instanceof Request ? input.method : 'GET')
  if (method !== 'GET') {
    attemptedWrites.push(method)
    return new Response('Fixture does not allow writes', { status: 405 })
  }
  return new Response(JSON.stringify(response), { headers: { 'Content-Type': 'application/json' } })
}
setConfiguredLocale(language)
document.documentElement.dataset.theme = params.get('theme') || 'dark'
const snapshot: Snapshot = { story_id: 'progression-fixture', branch_id: 'main', turns: [], state: { actors: { protagonist: { name: english ? 'Fixture traveler' : '夹具行者', template_id: 'protagonist', state: { 历练状态: actor } } } }, actor_state_schema: { version: 1, revision: 1, system: { templates: [{ id: 'protagonist', name: 'Player', fields: [{ name: '历练状态', type: 'object' }] }], adventure_rules: { family, progression: catalog.progression, skills: catalog.skills, items: [] } } } }
function Fixture() {
  const [view, setView] = useState<'combat' | 'ledger' | 'closed'>('combat')
  return <main className="bg-background text-foreground" style={{ width: '100%', maxWidth: 680, minHeight: '100dvh', margin: 'auto' }}>
    <nav className="flex gap-3 border-b p-2 text-sm"><button onClick={() => setView('combat')}>Combat view</button><button onClick={() => setView('ledger')}>Character ledger</button></nav>
    {view === 'combat' ? <div style={{ height: 'calc(100dvh - 42px)' }}><AdventureCombatPanel storyId="progression-fixture" branchId="main" active onClose={() => setView('closed')} /></div> : view === 'ledger' ? <StoryStateLedger snapshot={snapshot} displayPreference="expanded" onDisplayPreferenceChange={() => {}} /> : <button onClick={() => setView('combat')}>Reopen</button>}
  </main>
}
createRoot(document.getElementById('root')!).render(<Fixture />)
