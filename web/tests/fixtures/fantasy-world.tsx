import { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { setConfiguredLocale } from '../../src/i18n'
import { CharacterCreationDialog } from '../../src/components/Home/CharacterCreationDialog'
import { AdventureCombatPanel } from '../../src/features/interactive/components/combat-window/AdventureCombatPanel'
import { EntryArtwork } from '../../src/features/interactive/components/entry-details/EntryArtwork'
import type { EntryPresentation } from '../../src/features/interactive/components/entry-details/types'
import { StoryStateLedger } from '../../src/features/interactive/components/story-state/StoryStateLedger'
import type { StoryPackIdentity, StoryPackSummary } from '../../src/lib/api-client'
import type { Snapshot } from '../../src/features/interactive/types'
import '../../src/index.css'

// Real product components over an exported Go projection. The battle placement
// and HTTP responses are deterministic fixtures, not a live model or save.
const params = new URLSearchParams(location.search)
const language = params.get('language') || 'zh-CN'
const english = language.startsWith('en')
const data = await (await fetch('./fantasy-world-data.json')).json()
const artwork: Record<string, string> = await (await fetch('./fantasy-world-art.json')).json()
const projection = data.locales[language] as { summary: StoryPackSummary; identities: Record<string, StoryPackIdentity> }
const actor = structuredClone(data.templates.find((template: { id: string }) => template.id === 'sg-start-learner').state['历练状态'])
actor.battle = { id: 'fixture-battle', scale: 'person', opponent_name: english ? 'Bridge Yard Practice Target' : '桥工的练习木靶', round: 1, status: 'active', can_surrender: true }
const localize = (definition: { name: string; localizations?: Record<string, { name?: string }> }) => english ? definition.localizations?.['en-US']?.name || definition.name : definition.name
const response = { head: 'fixture-head', family: 'fantasy', clock_seconds: 0, actor, enemy_condition: '', skills: data.rules.skills.map((skill: { id: string; name: string }) => ({ ...skill, name: localize(skill), usable: Boolean(actor.skills[skill.id]) })), items: data.rules.items.map((item: { name: string }) => ({ ...item, name: localize(item) })) }
const attemptedWrites: string[] = []
;(window as Window & { fantasyFixtureWrites?: string[] }).fantasyFixtureWrites = attemptedWrites
const actualFetch = window.fetch.bind(window)
window.fetch = async (input, init) => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
  if (url.includes('/api/story-packs/sui-guan-realms/identity-draft')) {
    attemptedWrites.push('identity-preview')
    const choice = JSON.parse(String(init?.body)).choice
    const identity = { ...projection.identities[choice.origin_id], name: choice.name, age: choice.age }
    return new Response(JSON.stringify({ identity, contact_candidates: [] }), { headers: { 'Content-Type': 'application/json' } })
  }
  if (!url.includes('/api/interactive/stories/fantasy-fixture/')) return actualFetch(input, init)
  if ((init?.method || 'GET') !== 'GET') {
    attemptedWrites.push('forbidden-write')
    return new Response('Rendering fixture does not save game actions', { status: 405 })
  }
  return new Response(JSON.stringify(response), { headers: { 'Content-Type': 'application/json' } })
}
setConfiguredLocale(language)
document.documentElement.dataset.theme = params.get('theme') || 'dark'
const snapshot = { story_id: 'fantasy-fixture', branch_id: 'main', turns: [], state: { actors: { protagonist: { name: 'Rowan', template_id: 'protagonist', state: { 历练状态: actor, 精力: actor.energy, 精力上限: actor.max_energy } } } }, actor_state_schema: { version: 1, revision: 1, system: { templates: [{ id: 'protagonist', name: 'Player', fields: [{ name: '历练状态', type: 'object' }, { name: '精力', type: 'number' }, { name: '精力上限', type: 'number' }] }], adventure_rules: data.rules } } } as Snapshot
function Fixture() {
  const [view, setView] = useState(params.get('mode') || 'combat')
  return <main className="bg-background text-foreground" style={{ width: '100%', maxWidth: 680, minHeight: '100dvh', margin: 'auto' }}>
    <nav className="flex flex-wrap gap-3 border-b p-2 text-sm"><button onClick={() => setView('combat')}>Combat view</button><button onClick={() => setView('ledger')}>Character ledger</button><button onClick={() => setView('creation')}>Create character</button><button onClick={() => setView('art')}>Runtime artwork</button></nav>
    {view === 'combat' ? <div style={{ height: 'calc(100dvh - 42px)' }}><AdventureCombatPanel storyId="fantasy-fixture" branchId="main" active onClose={() => setView('closed')} /></div> : view === 'ledger' ? <StoryStateLedger snapshot={snapshot} displayPreference="expanded" onDisplayPreferenceChange={() => {}} /> : view === 'art' ? <section data-testid="runtime-artwork" className="grid gap-3 p-3 sm:grid-cols-2">{['sg-blade', 'sg-spark', 'sg-bow', 'sg-shield'].map(id => { const definition = data.rules.skills.find((entry: { id: string }) => entry.id === id); const entry = { ...definition, name: localize(definition), art: artwork[id] ? { source: 'catalog', catalog_id: artwork[id] } : undefined, visual_theme: 'fantasy', visual_level: { kind: 'mastery', value: 1, maximum: 10, scale: 'person' } } as EntryPresentation; return <article key={id}><h2 className="mb-1 text-sm">{entry.name}</h2><EntryArtwork entry={entry} /></article> })}</section> : null}
    <CharacterCreationDialog open={view === 'creation'} pack={projection.summary} onOpenChange={open => { if (!open) setView('combat') }} onStart={() => { attemptedWrites.push('start-story') }} />
  </main>
}
createRoot(document.getElementById('root')!).render(<Fixture />)
