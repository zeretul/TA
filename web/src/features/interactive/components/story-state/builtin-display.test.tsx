import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { act, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import { createAgentTextMessage } from '@/lib/agent-ui-message'
import { buildAgentMessageViews, agentViewToRenderMessage } from '@/lib/agent-message-view'
import { PlayerCheckDetails } from '@/components/Chat/PlayerCheckDetails'
import type { ActorStateField, RuleResolution, Snapshot } from '../../types'
import { playerCheckEvidence } from '../story-stage/player-check-evidence'
import { useStoryStageMessages } from '../story-stage/use-story-stage-messages'
import { StoryStateDetails, StoryStateLedger } from './StoryStateLedger'
import { LedgerFieldView } from './ledger-fields'
import { playerFacingFieldLabelKey } from './field-layout'
import { builtinModuleDisplayName } from '../../builtin-display-labels'
import { WorldSettingsEditor, worldSettingsValue } from '@/components/Home/WorldSettingsEditor'
import { StoryTuningView } from '../director-console/StoryTuningView'
import { getActorStates, getRuleSystems } from '../../api'
import type { StorySummary } from '../../types'

vi.mock('../../api', () => ({ getActorStates: vi.fn(async () => []), getRuleSystems: vi.fn(async () => []) }))
vi.mock('@/features/settings/use-image-model-configured', () => ({ useImageModelConfigured: () => false }))

// Read actual source declarations, not translated test constants. Frozen
// schemas normalize away legacy path/id, so exercise that real wire shape too.
const source = readFileSync(resolve(process.cwd(), '../internal/interactive/actor_state_preset_fields.go'), 'utf8')
const names = ['姓名', '基本身份', '外貌与装扮', '性格与背景', '当前处境']
const declarations = [...source.matchAll(/textStateField\("([^"]+)", "([^"]+)", "([^"]+)", "([^"]+)", "([^"]+)"\)/g)]
const fields: ActorStateField[] = names.map(name => {
  const match = declarations.find(row => row[2] === name)
  if (!match) throw new Error(`source field missing: ${name}`)
  return { name: match[2], type: 'string', description: match[3], group: match[4], display: match[5] as ActorStateField['display'] }
})
const modules = readFileSync(resolve(process.cwd(), '../internal/interactive/director_modules.go'), 'utf8')
const ruleID = modules.match(/RuleSystemMysteryClueForwardID\s*=\s*"([^"]+)"/)![1]
const ruleLabel = modules.match(/ID:\s+RuleSystemMysteryClueForwardID,\s+Label:\s+"([^"]+)"/)![1]
function evidence(templateID = ruleID, label = ruleLabel) {
  return { id: 'committed', request: { rule: { template: 'dice_check', template_id: templateID, label } }, result: { label, dice: '1d20', rolls: [14], outcome: 'success' } } as unknown as RuleResolution
}
afterEach(async () => { await i18n.changeLanguage('zh-CN') })

it('localizes actual source field/group captions on locale flip without rewriting saved values or schema', async () => {
  await i18n.changeLanguage('en-US')
  const snapshot: Snapshot = { story_id: 'saved', branch_id: 'main', turns: [], state: { actors: { protagonist: { id: 'protagonist', template_id: 'protagonist', name: '玩家姓名', state: { 姓名: '玩家姓名', 基本身份: '玩家保存的中文身世', 当前处境: '已保存的中文处境' } } } }, actor_state_schema: { version: 1, revision: 1, system: { templates: [{ id: 'protagonist', name: '主角状态表', fields }] } } }
  const before = JSON.stringify(snapshot)
  render(<StoryStateLedger snapshot={snapshot} displayPreference="expanded" onDisplayPreferenceChange={vi.fn()} />)
  expect(screen.getByRole('heading', { name: 'Name' })).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Identity' })).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Current situation' })).toBeInTheDocument()
  expect(screen.getAllByRole('region', { name: 'Overview' }).length).toBeGreaterThan(0)
  expect(screen.getByText('玩家保存的中文身世')).toBeInTheDocument()
  await act(async () => { await i18n.changeLanguage('zh-CN') })
  expect(screen.getByRole('heading', { name: '姓名' })).toBeInTheDocument()
  expect(screen.getByRole('region', { name: '概况' })).toBeInTheDocument()
  expect(JSON.stringify(snapshot)).toBe(before)
})

it('localizes only an exact builtin check identity and preserves the committed evidence', async () => {
  await i18n.changeLanguage('en-US')
  const saved = evidence()
  const before = JSON.stringify(saved)
  const message = createAgentTextMessage({ id: 'saved-turn', role: 'assistant', text: 'Saved body', metadata: { public_rule_roll: playerCheckEvidence(saved) } })
  const rendered = agentViewToRenderMessage(buildAgentMessageViews([message])[0])!
  render(<PlayerCheckDetails evidence={rendered.public_rule_roll!} />)
  expect(screen.getByText(/Mystery investigation: clues keep moving/)).toBeInTheDocument()
  await act(async () => { await i18n.changeLanguage('zh-CN') })
  expect(screen.getByText(new RegExp(ruleLabel))).toBeInTheDocument()
  expect(JSON.stringify(saved)).toBe(before)
})

it('renders the actual Go player snapshot DTO through evidence metadata without requiring the private request', async () => {
  await i18n.changeLanguage('en-US')
  // The Go player-projection regression verifies these exact JSON bytes against
  // projectInteractiveSnapshotForPlayer; do not replace this with a raw journal.
  const snapshot = JSON.parse(readFileSync(resolve(process.cwd(), 'src/features/interactive/components/story-stage/__fixtures__/player-rule-snapshot.json'), 'utf8')) as Snapshot
  const before = JSON.stringify(snapshot)
  const { result } = renderHook(() => useStoryStageMessages({
    snapshot, liveMessages: [], streaming: false, stageKey: 'saved-story:main',
    liveTurnNavigationAnchorId: 'live', optimisticInteractiveImages: {}, belongsToStage: () => true, renderKeyFor: () => undefined,
  }))
  const rendered = buildAgentMessageViews(result.current.agentMessages).map(view => agentViewToRenderMessage(view)).find(message => message?.public_rule_roll)!
  render(<PlayerCheckDetails evidence={rendered.public_rule_roll!} />)
  expect(screen.getByText(/Mystery investigation: clues keep moving/)).toBeInTheDocument()
  expect(screen.getByText(/Failure/)).toBeInTheDocument()
  expect(screen.getByText(/1d20/)).toBeInTheDocument()
  await act(async () => { await i18n.changeLanguage('zh-CN') })
  expect(screen.getByText(new RegExp(ruleLabel))).toBeInTheDocument()
  expect(JSON.stringify(snapshot)).toBe(before)
})

it('keeps custom field lookalikes and unknown/custom check labels literal', async () => {
  await i18n.changeLanguage('en-US')
  for (const field of [{ ...fields[0], description: 'A player-authored field' }, { ...fields[0], path: 'custom.identity' }, { ...fields[0], name: '自定义称谓' }]) {
    expect(playerFacingFieldLabelKey(field)).toBeUndefined()
  }
  const field = { ...fields[0], description: 'A player-authored field' }
  const { unmount } = render(<LedgerFieldView item={{ id: 'custom', label: field.name, field, renderer: 'block', value: '保留原文', change: null, resolveSibling: () => undefined }} />)
  expect(screen.getByRole('heading', { name: '姓名' })).toBeInTheDocument()
  unmount()
  const custom = evidence(ruleID, '玩家自写的检定标题')
  const unknown = evidence('custom-rule', ruleLabel)
  render(<><PlayerCheckDetails evidence={playerCheckEvidence(custom)!} /><PlayerCheckDetails evidence={playerCheckEvidence(unknown)!} /></>)
  expect(screen.getByText(/玩家自写的检定标题/)).toBeInTheDocument()
  expect(screen.getByText(new RegExp(ruleLabel))).toBeInTheDocument()
})

it('localizes exact source module identities in world defaults and locked story settings without changing IDs', async () => {
  await i18n.changeLanguage('en-US')
  const ruleID = modules.match(/RuleSystemGrittySurvivalID\s*=\s*"([^"]+)"/)![1]
  const ruleName = modules.match(/ID:\s+RuleSystemGrittySurvivalID,\s+Label:\s+"([^"]+)"/)![1]
  const states = readFileSync(resolve(process.cwd(), '../internal/interactive/actor_state_preset_genres.go'), 'utf8')
  const stateName = states.match(/Name:\s+"([^"]+)"/)![1]
  const state = { id: 'xiuxian-state', name: stateName }
  const rule = { id: ruleID, name: ruleName }
  vi.mocked(getActorStates).mockResolvedValue([state] as never)
  vi.mocked(getRuleSystems).mockResolvedValue([rule] as never)
  const pack = { id: 'fanren-jie', name: 'Custom world name', title: 'Custom world title', world_count: 1, character_count: 1, builtin: true }
  const value = worldSettingsValue(undefined)
  const story: StorySummary = { id: 'saved', title: '玩家保存的标题', origin: '', protagonist: { mode: 'default' }, story_teller_id: '', planning_template_id: '', choice_count: 3, reply_target_chars: 2000, opening: { mode: 'ai' }, created_at: '', updated_at: '', branches: 1, events: 1, turn_count: 1, module_refs: { rule_system_id: ruleID, actor_state_id: state.id } }
  const before = JSON.stringify({ pack, value, story, rule, state })
  const change = vi.fn()
  render(<><WorldSettingsEditor pack={pack} value={value} onChange={change} tellers={[]} planningTemplates={[]} modules={[{ ...rule, kind: 'rule_system' }, { ...state, kind: 'actor_state' }]} saving={false} /><StoryTuningView story={story} planningTemplates={[]} tellers={[]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={change} group="rules" /></>)
  await waitFor(() => expect(screen.getAllByText(/Gritty survival: resources and consequences/).length).toBeGreaterThanOrEqual(2))
  expect(screen.getAllByText(/Cultivation state/).length).toBeGreaterThanOrEqual(2)
  await act(async () => { await i18n.changeLanguage('zh-CN') })
  expect(screen.getAllByText(new RegExp(ruleName)).length).toBeGreaterThanOrEqual(2)
  expect(JSON.stringify({ pack, value, story, rule, state })).toBe(before)
  expect(change).not.toHaveBeenCalled()
})

it('keeps overridden or unknown module names literal and recognizes only the source-generated world ID shape', async () => {
  await i18n.changeLanguage('en-US')
  expect(builtinModuleDisplayName('actor_state', { id: 'xiuxian-state', name: '玩家的自定状态' }, i18n.t)).toBe('玩家的自定状态')
  expect(builtinModuleDisplayName('actor_state', { id: 'custom-state', name: '修仙状态系统' }, i18n.t)).toBe('修仙状态系统')
  expect(builtinModuleDisplayName('actor_state', { id: 'xiuxian-state', name: '修仙状态系统', builtin_overridden: true }, i18n.t)).toBe('修仙状态系统')
  expect(builtinModuleDisplayName('actor_state', { id: 'world-actor-state-' + 'a'.repeat(20), name: '世界专属行动状态' }, i18n.t)).toBe('World-specific action state')
  expect(builtinModuleDisplayName('rule_system', { id: 'world-rule-system-' + 'b'.repeat(20), name: '世界专属行动检定' }, i18n.t)).toBe('World-specific action checks')
  expect(builtinModuleDisplayName('actor_state', { id: 'world-actor-state-player', name: '世界专属行动状态' }, i18n.t)).toBe('世界专属行动状态')
  expect(builtinModuleDisplayName('actor_state', { id: 'unknown' }, i18n.t)).toBe('unknown')
})

it('falls back to the saved caption for unknown or mismatched caption metadata', async () => {
  await i18n.changeLanguage('en-US')
  render(<><PlayerCheckDetails evidence={{ label: '玩家自写标题', label_key: 'snapshot.checkEvidence.rule.dm-mystery-clue-forward' }} /><PlayerCheckDetails evidence={{ label: 'Unknown source caption', label_key: 'snapshot.checkEvidence.rule.unknown' }} /></>)
  expect(screen.getByText(/玩家自写标题/)).toBeInTheDocument()
  expect(screen.getByText(/Unknown source caption/)).toBeInTheDocument()
})


it('uses a localized generic title rather than a private request caption for an unlabeled public result', async () => {
  await i18n.changeLanguage('en-US')
  const saved = evidence()
  saved.result.label = ''
  const before = JSON.stringify(saved)
  const publicEvidence = playerCheckEvidence(saved)!
  expect(publicEvidence.label).toBeUndefined()
  expect(publicEvidence.label_key).toBeUndefined()
  render(<PlayerCheckDetails evidence={publicEvidence} />)
  expect(screen.getByText(`${i18n.t('snapshot.ruleRoll.title')} · ${i18n.t('snapshot.checkEvidence.outcome.success')}`)).toBeInTheDocument()
  expect(screen.queryByText(new RegExp(ruleLabel))).not.toBeInTheDocument()
  expect(JSON.stringify(saved)).toBe(before)
})

it('shows one current world situation and falls back to the legacy overview for older saves', () => {
  const snapshot: Snapshot = {
    story_id: 'old-save', branch_id: 'main', turns: [],
    state: { 世界局势: '河水上涨，渡口今晨停渡。', 修行世界状态: '旧概况只记录在存档里。' },
  }
  const view = render(<StoryStateLedger snapshot={snapshot} displayPreference="expanded" onDisplayPreferenceChange={vi.fn()} />)
  expect(screen.getByText('河水上涨，渡口今晨停渡。')).toBeInTheDocument()
  expect(screen.queryByText('旧概况只记录在存档里。')).not.toBeInTheDocument()
  view.rerender(<StoryStateLedger snapshot={{ ...snapshot, state: { 修行世界状态: '旧概况只记录在存档里。' } }}
    displayPreference="expanded" onDisplayPreferenceChange={vi.fn()} />)
  expect(screen.getByText('旧概况只记录在存档里。')).toBeInTheDocument()
})

it.each(['stage', 'console'])('keeps owned details inside the protagonist tab in the %s', async (surface) => {
  const snapshot: Snapshot = {
    story_id: 'saved', branch_id: 'main', turns: [],
    state: {
      世界局势: '渡口今晨停渡。',
      actors: {
        protagonist: { id: 'protagonist', template_id: 'protagonist', name: '沈砚', state: { 姓名: '沈砚' } },
        companion: { id: 'companion', role: 'supporting', template_id: 'companion', name: '陶平', state: { 姓名: '陶平' } },
      },
    },
    actor_state_schema: {
      version: 1, revision: 1,
      system: { templates: [{ id: 'protagonist', name: '主角状态表', fields: [{ name: '境界', type: 'string' }] }] },
    },
  }
  render(surface === 'stage'
    ? <StoryStateLedger snapshot={snapshot} displayPreference="expanded" onDisplayPreferenceChange={vi.fn()} />
    : <StoryStateDetails snapshot={snapshot} />)
  const details = screen.getByTestId('owned-entry-details')
  expect(details).toBeVisible()
  expect(details.closest('[role="tabpanel"]')).toHaveAttribute('aria-labelledby', screen.getByRole('tab', { name: /沈砚/ }).id)
  await act(async () => { fireEvent.mouseDown(screen.getByRole('tab', { name: /陶平/ }), { button: 0, ctrlKey: false }) })
  expect(details).not.toBeVisible()
  await act(async () => { fireEvent.mouseDown(screen.getByRole('tab', { name: /世界/ }), { button: 0, ctrlKey: false }) })
  expect(details).not.toBeVisible()
})
