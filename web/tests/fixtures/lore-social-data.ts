import type { CharacterWindow, Snapshot } from '../../src/features/interactive/types'

export const cast: CharacterWindow = {
  story_id: 'social-fixture', branch_id: 'main', scene: { location: '本次相见的渡口', world_time: '第三日午时' }, has_more: false,
  faction_memberships: [{ id: 'membership-1', faction_id: '青山书院', faction_name: '青山书院', status: 'formal', statement: '已通过当面接纳。', source_exchange_id: 'fixture-membership-accepted', world_time: '第二日' }],
  characters: Array.from({ length: 9 }, (_, i) => ({
    id: `known-${i}`, name: i === 0 ? '很长的姓名与已经公开的称呼以检查窄屏换行' : `同行者${i}`, role: '已经相识的人',
    present: i === 1, channel: i === 1 ? 'scene' : 'message', unread_messages: 0, open_matters: 0, affinity: 40, affinity_band: 'friendly',
    relationship: { id: `formal-${i}`, character_id: `known-${i}`, kind: 'friend', statement: '一起办过一件事，明确以朋友相称。', world_time: '第二日', expressed_by: `fixture-expression-${i}`, answered_by: `fixture-answer-${i}` },
  })),
}
export const snapshot: Snapshot = {
  story_id: 'social-fixture', branch_id: 'main', turns: [], state: { actors: {
    protagonist: { name: '试玩者', template_id: 'protagonist', state: { 所属势力: ['青山书院'] } },
    world: { name: '世界', template_id: 'world_entities', state: { 势力记录: { 青山书院: { 类型: '书院', 核心特征: '一处已经拜访的地方。' } } } },
    ...Object.fromEntries(cast.characters.map((person, i) => [person.id, { name: person.name, template_id: 'important_character', state: {
      所属势力: i < 3 ? [{ faction_id: '青山书院', name: '青山书院', status: 'formal' }] : [], 已知位置: { location: '最后听说在城外的旧茶铺歇脚，此后没有新的消息。', learned_at: '第二日酉时', source: 'conversation', nature: 'hearsay', confidence: 'uncertain', evidence_refs: [`fixture-location-${i}`] }, 出场与位置: '最后听说在城外的旧茶铺歇脚，此后没有新的消息。',
    } }])),
  } },
}
