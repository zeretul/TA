import { describe, expect, it } from 'vitest'
import type { CharacterWindow, Snapshot } from '../../types'
import { buildKnownPeople } from './known-people'
import { buildLoreOverview } from './model'

const actor = (name: string, state: Record<string, unknown>) => ({ name, template_id: 'important_character', state })
function fixture(): { snapshot: Snapshot; characters: CharacterWindow } {
  return {
    snapshot: { story_id: 's', branch_id: 'main', turns: [], state: { actors: {
      protagonist: actor('玩家', { 所属势力: ['黄枫谷'], 关系: { a: { 类型: '同伴', 阶段: '初识' }, b: '竞争', secret: '不可泄漏' } }),
      a: actor('同名', { 所属势力: ['黄枫谷', '黄枫谷'], 已知位置: { location: '昨天离开药铺，去向未明', source: 'observation' }, 出场与位置: '昨天离开药铺，去向未明', 已知人物关系: { b: { 类型: '同门', source: 'conversation', nature: 'told', confidence: 'uncertain', learned_at: '第二日午时', evidence_refs: ['exchange-known'] }, secret: '秘密亲属', 同名: '不得按重名合并' }, 关系: { b: '私密结怨' } }),
      b: actor('同名', { 所属势力: ['天星宗'], 出场与位置: '旧仓库', written_at_seconds: 999 }),
      secret: actor('尚未登场', { 所属势力: ['隐藏教派'] }),
      world: actor('世界', { 势力记录: { 黄枫谷: { 类型: '宗门' }, 天星宗: { 类型: '宗门' } } }),
    } } },
    characters: { story_id: 's', branch_id: 'main', scene: { location: '本次码头', world_time: '第三日午时' }, has_more: false,
      characters: [
        { id: 'a', name: '同名', present: false, channel: 'message', unread_messages: 0, open_matters: 0, affinity: 100, affinity_band: 'deep', tags: ['不能猜为势力'], relationship: { id: 'bond-a', character_id: 'a', kind: 'estranged', statement: '已明确分道扬镳', world_time: '第二日' } },
        { id: 'b', name: '同名', present: true, channel: 'scene', unread_messages: 0, open_matters: 0, affinity: 80, affinity_band: 'deep' },
      ],
    },
  }
}

describe('known people projection', () => {
  it('groups explicit affiliations without guessing from tags or the author cast', () => {
    const { snapshot, characters } = fixture()
    const known = buildKnownPeople(snapshot, characters)
    expect(known.people.map(person => [person.id, person.affiliations])).toEqual([
      ['protagonist', ['黄枫谷']], ['a', ['黄枫谷']], ['b', ['天星宗']],
    ])
    expect(JSON.stringify(known)).not.toMatch(/隐藏教派|尚未登场|不能猜为势力/)
  })

  it('uses learned whereabouts, never bare off-screen state, and dates only the current observation', () => {
    const { snapshot, characters } = fixture()
    const known = buildKnownPeople(snapshot, characters)
    expect(known.people[1]).toMatchObject({ lastKnownLocation: '昨天离开药铺，去向未明', seenNow: false, observedAt: undefined })
    expect(known.people[2]).toMatchObject({ lastKnownLocation: '本次码头', seenNow: true, observedAt: '第三日午时' })
    characters.characters[1].present = false
    expect(buildKnownPeople(snapshot, characters).people[2]).toMatchObject({ lastKnownLocation: undefined, observedAt: undefined })
  })

  it('retains directed known NPC relations, exact IDs and authoritative formal bonds', () => {
    const { snapshot, characters } = fixture()
    const known = buildKnownPeople(snapshot, characters)
    expect(known.relations).toMatchObject([
      { id: 'known:protagonist:b', sourceId: 'protagonist', targetId: 'b', label: '竞争', statement: undefined, mutual: false },
      { id: 'known:a:b', sourceId: 'a', targetId: 'b', label: '同门', statement: undefined, mutual: false, provenance: { learnedAt: '第二日午时', sourceKey: 'loreOverview.provenance.source.conversation', confidence: 'uncertain', references: [{ labelKey: 'loreOverview.provenance.exchange', id: 'exchange-known' }] } },
      { id: 'formal:a', sourceId: 'protagonist', targetId: 'a', label: 'estranged', labelKey: 'characterWindow.relationship.estranged', statement: '已明确分道扬镳', mutual: false, undirected: true },
    ])
    expect(JSON.stringify(known.relations)).not.toMatch(/私密结怨|秘密亲属|不可泄漏|不得按重名合并/)
  })

  it('keeps kinship and mentorship alongside the separately settled social bond', () => {
    const { snapshot, characters } = fixture()
    const player = (snapshot.state.actors as Record<string, { state: Record<string, unknown> }>).protagonist
    player.state['关系'] = { a: { 类型: '师徒', 阶段: '入门' } }
    const relations = buildKnownPeople(snapshot, characters).relations
    expect(relations.some(relation => relation.sourceId === 'protagonist' && relation.targetId === 'a' && relation.label === '师徒')).toBe(true)
    expect(relations.some(relation => relation.id === 'formal:a' && relation.label === 'estranged')).toBe(true)
  })

  it('does not turn high affinity or schema defaults into facts', () => {
    const { snapshot, characters } = fixture()
    ;(snapshot.state.actors as Record<string, unknown>).b = actor('同名', {})
    snapshot.actor_state_schema = { system: { templates: [{ id: 'important_character', fields: [
      { path: 'identity.affiliations', name: '所属势力', type: 'list', default: ['假设宗门'] },
      { path: 'knowledge.relationships', name: '已知人物关系', type: 'object', visibility: 'private' },
    ] }] } } as Snapshot['actor_state_schema']
    const known = buildKnownPeople(snapshot, characters)
    expect(known.people[2].affiliations).toEqual([])
    expect(known.relations.some(relation => relation.sourceId === 'b')).toBe(false)
    expect(buildKnownPeople(null, null)).toEqual({ people: [], relations: [] })
  })

  it('shows known faction members and player relations from the same model', () => {
    const { snapshot, characters } = fixture()
    const model = buildLoreOverview({ snapshot, characters, quests: null, events: null, journey: null })
    expect(model.byTab.factions.map(faction => [faction.title, faction.members?.map(person => person.id)])).toEqual([
      ['黄枫谷', ['a']], ['天星宗', ['b']],
    ])
    expect(model.byTab.player[0].relations?.map(relation => relation.id)).toEqual(['known:protagonist:b', 'formal:a'])
    expect(model.byTab.people[0].links).toContainEqual({ tab: 'factions', entryId: 'world:势力记录:黄枫谷', label: '黄枫谷' })
    expect(JSON.stringify(model)).not.toContain('隐藏教派')
  })
  it('retains explicit learned times and evidence without inventing them or copying free evidence', () => {
    const { snapshot, characters } = fixture()
    const actor = (snapshot.state.actors as Record<string, { state: Record<string, unknown> }>).a
    actor.state['已知位置'] = { location: '渡口', learned_at: '第二日酉时', source: 'conversation', nature: 'hearsay', confidence: 'confirmed', evidence_refs: ['public-1'], evidence: '秘密原文不能复制' }
    const person = buildKnownPeople(snapshot, characters).people.find(person => person.id === 'a')!
    expect(person.lastKnownLocation).toBe('渡口')
    expect(person.locationProvenance).toMatchObject({ learnedAt: '第二日酉时', nature: 'hearsay', confidence: 'uncertain', references: [{ id: 'public-1' }] })
    expect(JSON.stringify(person.locationProvenance)).not.toContain('秘密原文')
    characters.characters[0].relationship!.expressed_by = 'expression-1'
    characters.characters[0].relationship!.answered_by = 'answer-1'
    expect(buildKnownPeople(snapshot, characters).relations.find(relation => relation.id === 'formal:a')?.provenance).toMatchObject({ learnedAt: '第二日', nature: 'formal', references: [{ id: 'expression-1' }, { id: 'answer-1' }] })
  })

  it('keeps same-named factions separate and distinguishes pending and ended membership', () => {
    const { snapshot, characters } = fixture()
    const actors = snapshot.state.actors as Record<string, { state: Record<string, unknown> }>
    actors.world.state['势力记录'] = { east: { name: '同名宗门' }, west: { name: '同名宗门' } }
    actors.a.state['所属势力'] = [{ faction_id: 'east', name: '同名宗门', status: 'left' }, { faction_id: 'west', name: '同名宗门', status: 'pending' }]
    actors.b.state['所属势力'] = [{ faction_id: 'west', name: '同名宗门', status: 'formal' }]
    actors.protagonist.state['所属势力'] = [{ faction_id: 'east', name: '旧称', status: 'formal' }, '同名宗门']
    characters.faction_memberships = [{ id: 'm1', faction_id: 'east', faction_name: '同名宗门', status: 'left', statement: '申请已被拒绝', world_time: '第三日', source_exchange_id: 'refused' }]
    const model = buildLoreOverview({ snapshot, characters, quests: null, events: null, journey: null })
    expect(model.byTab.factions).toHaveLength(2)
    expect(model.byTab.factions.find(entry => entry.factionId === 'east')).toMatchObject({ title: '同名宗门', pastMembers: [{ id: 'a' }] })
    expect(model.byTab.factions.find(entry => entry.factionId === 'west')).toMatchObject({ title: '同名宗门', members: [{ id: 'b' }], pendingMembers: [{ id: 'a' }] })
    expect(model.knownPeople.people[0].affiliations).toEqual([])
    expect(model.knownPeople.people[0].memberships).toMatchObject([{ factionId: 'east', status: 'left', statement: '申请已被拒绝' }, { name: '同名宗门', status: 'unresolved' }])
    expect(model.byTab.people[0].links).toContainEqual({ tab: 'factions', entryId: 'world:势力记录:east', label: '同名宗门' })
    expect(model.byTab.people[1].links).toContainEqual({ tab: 'factions', entryId: 'world:势力记录:west', label: '同名宗门' })
  })

  it('will not combine a snapshot with a cast from a different branch', () => {
    const { snapshot, characters } = fixture()
    characters.branch_id = 'fork'
    expect(buildKnownPeople(snapshot, characters)).toEqual({ people: [], relations: [] })
  })

})
