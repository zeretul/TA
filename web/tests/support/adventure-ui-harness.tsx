import React from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AdventureCombatPanel } from '../../src/features/interactive/components/combat-window/AdventureCombatPanel'
import { CombatWindowPanel } from '../../src/features/interactive/components/combat-window/CombatWindowPanel'
import { GameSettingsView } from '../../src/features/interactive/components/game-settings/GameSettingsView'
import { setConfiguredLocale } from '../../src/i18n'
import { PlayerCheckDetails } from '../../src/components/Chat/PlayerCheckDetails'
import { StoryStateDetails } from '../../src/features/interactive/components/story-state/StoryStateLedger'
import type { Snapshot } from '../../src/features/interactive/types'
import '../../src/index.css'

const params = new URLSearchParams(location.search)
const theme = params.get('theme') || 'dark'
document.documentElement.dataset.theme = theme
document.documentElement.classList.toggle('dark', theme === 'dark')
setConfiguredLocale(params.get('locale') || 'zh-CN')
const mode = params.get('mode') || 'adventure'
const common = { storyId: 'ui-story', branchId: 'main', active: true, onClose: () => {}, onContinueStory: () => {} }
const adventureBefore = { family: 'space', rank: 1, experience: 10, health: 80, max_health: 100, energy: 20, max_energy: 50, injury: 0, currency: 100, skills: { engineering: 1 }, training: { engineering: 3 }, inventory: { part: 2 }, assets: {}, acquired: {}, defeated: {} }
const adventureAfter = { ...adventureBefore, experience: 16, training: { engineering: 6 }, acquired: { secret: true }, defeated: { boss: true } }
const stateSnapshot = { story_id: 'ui-story', branch_id: 'main', turn_count: 2, state: { actors: { protagonist: { name: params.get('locale') === 'en-US' ? 'Traveler' : '旅人', role: 'protagonist', template_id: 'protagonist', state: { 历练状态: adventureAfter } } } }, actor_state_schema: { system: { adventure_rules: { family: 'space', skills: [{ id: 'engineering', name: '工程技艺' }], items: [{ id: 'part', name: '维修零件' }] }, templates: [{ id: 'protagonist', name: 'Traveler', fields: [{ name: '历练状态', type: 'object' }] }] } }, current_turn: { id: 'turn2', state_delta: { actor_ops: [{ actor_id: 'protagonist', field_id: '历练状态', op: 'set', value: adventureAfter, previous_value: adventureBefore }] } } } as unknown as Snapshot
createRoot(document.getElementById('root')!).render(<QueryClientProvider client={new QueryClient()}><div style={{ height: '100dvh', width: '100%', maxWidth: mode === 'settings' ? undefined : 620, margin: 'auto' }}>
  {mode === 'state' ? <StoryStateDetails snapshot={stateSnapshot} /> : mode === 'evidence' ? <PlayerCheckDetails evidence={{ label: 'D20', dice: '1d20', rolls: [14], outcome: 'success', bonus_total: 2 }} /> : mode === 'settings' ? <GameSettingsView projectId="project-ui" visible onOpenBooks={() => {}} /> : mode === 'talisman' ? <CombatWindowPanel {...common} sessionId="fight" onSelectSession={() => {}} /> : <AdventureCombatPanel {...common} />}
</div></QueryClientProvider>)
