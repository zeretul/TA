import { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StoryStage } from '../../src/features/interactive/components/StoryStage'
import { OwnedEntryDetails } from '../../src/features/interactive/components/entry-details/OwnedEntryDetails'
import { WorldPresentationProvider } from '../../src/features/interactive/world-presentation-context'
import { setConfiguredLocale } from '../../src/i18n'
import type { Snapshot, StorySummary } from '../../src/features/interactive/types'
import '../../src/index.css'

// Browser-only deterministic fixtures; no user story, model or backend is used.
const params = new URLSearchParams(location.search)
const initialTheme = params.get('theme') || 'dark'
document.documentElement.dataset.theme = initialTheme
document.documentElement.classList.toggle('dark', initialTheme === 'dark')
setConfiguredLocale(params.get('locale') || 'zh-CN')
const cases = ['cultivation', 'wuxia', 'space', 'fantasy', 'author', 'cultivation-author']
function fixture(world: string) {
  const family = world === 'author' ? 'fantasy' : world === 'cultivation-author' ? 'cultivation' : world
  const cultivation = family === 'cultivation'
  const communication = family === 'space' ? 'radio' : cultivation ? 'cultivation-signal' : 'letter'
  const statusTemplate = cultivation ? 'xiuxian-state' : family + '-state-v1'
  const gameplay = { status_template: statusTemplate, growth: family, combat: family, communication, opening: 'scene',
    ...(world === 'author' ? { presentation: { 'zh-CN': { combat: '学院决斗', exploration: '魔法调查', techniques: '咒语', partner: '伴侣', energy: '魔力' }, 'en-US': { combat: 'School duels', exploration: 'Magical investigation', techniques: 'Spells', partner: 'Partner', energy: 'Magic' } } } : {}),
  }
  const story = { id: 'world-ui-' + world, title: 'World presentation acceptance', protagonist: { mode: 'default' }, story_teller_id: 'rhythm', planning_mode: 'disabled', planning_template_id: 'adventure', module_refs: { actor_state_id: world === 'cultivation-author' ? 'world-actor-state-custom' : statusTemplate, rule_system_id: 'fixture-rule' }, reply_target_chars: 1000, choice_count: 3, opening: { mode: 'ai' }, branches: 1, events: 0, turn_count: 1, pack_binding: { pack_id: 'fixture-' + world, scope: 'pack:fixture-' + world, gameplay } } as StorySummary
  const actor = { family, rank: 1, experience: 10, health: 80, max_health: 100, energy: 30, max_energy: 50, injury: 0, currency: 80, skills: { basic: 1 }, training: {}, inventory: {}, assets: {}, acquired: {}, defeated: {} }
  const turn = { id: 'first', parent_id: null, branch_id: 'main', ts: '2026-11-06T00:00:00Z', user: '观察周围', narrative: '你停在路口，正在等候下一步行动。' }
  const fields = cultivation ? [{ name: '境界', type: 'string', path: 'cultivation.stage', value_format: 'stage' }] : [{ name: '历练状态', type: 'object' }]
  const snapshot = { story_id: story.id, branch_id: 'main', turn_count: 1, turns: [turn], current_turn: turn, state: { actors: { protagonist: { id: 'protagonist', name: '旅人', role: 'protagonist', template_id: 'protagonist', state: cultivation ? { 境界: 'mortal' } : { 历练状态: actor } } } }, actor_state_schema: { system: { templates: [{ id: 'protagonist', name: '旅人', fields }], ...(!cultivation ? { adventure_rules: { family, skills: [{ id: 'basic', name: '基本技艺' }], items: [] } } : {}) } } } as unknown as Snapshot
  return { story, snapshot }
}
function Harness() {
  const [world, setWorld] = useState(params.get('world') || 'fantasy')
  const [locale, setLocale] = useState(params.get('locale') || 'zh-CN')
  const [mode, setMode] = useState(params.get('mode') || 'stage')
  const { story, snapshot } = fixture(world)
  const change = (key: string, value: string) => {
    const url = new URL(location.href); url.searchParams.set(key, value); history.replaceState({}, '', url)
    if (key === 'world') setWorld(value)
    if (key === 'locale') { setConfiguredLocale(value); setLocale(value) }
    if (key === 'mode') setMode(value)
  }
  return <div className="bg-background text-foreground" style={{ height: '100dvh', minWidth: 0, display: 'flex', flexDirection: 'column' }}>
    <nav aria-label="Acceptance controls" className="flex flex-wrap gap-2 border-b p-2 text-xs">
      <label>World <select aria-label="Fixture world" value={world} onChange={event => change('world', event.target.value)}>{cases.map(name => <option key={name}>{name}</option>)}</select></label>
      <label>Language <select aria-label="Fixture language" value={locale} onChange={event => change('locale', event.target.value)}><option value="zh-CN">中文</option><option value="en-US">English</option></select></label>
      <button onClick={() => change('mode', 'stage')}>Main game</button><button onClick={() => change('mode', 'details')}>Owned details</button>
    </nav>
    <div style={{ flex: 1, minHeight: 0, minWidth: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
      {mode === 'stage' ? <StoryStage active projectId="world-ui-project" workspace="isolated-world-ui" story={story} stories={[story]} storyId={story.id} branchId="main" tellers={[]} snapshot={snapshot} stateDisplayPreference="collapsed" onDone={async () => snapshot} /> : <WorldPresentationProvider story={story} snapshot={snapshot}><div className="p-3"><OwnedEntryDetails snapshot={snapshot} /></div></WorldPresentationProvider>}
    </div>
  </div>
}
createRoot(document.getElementById('root')!).render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } } })}><Harness /></QueryClientProvider>)