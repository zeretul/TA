import type { TFunction } from 'i18next'
import type { ActorStateField } from './types'

// Display metadata for exact builtin source declarations. Frozen schemas can
// omit legacy path/id, so name, type and description identify that source row.
// Custom names/descriptions/identities and all saved values remain untouched.
const sourceFields = [
  {"path": "knowledge.location", "name": "已知位置", "type": "object", "description": "主角最后实际获知的位置及该条消息的来源、获知时刻和可信度。", "group": "经历见闻", "labelKey": "loreOverview.row.lastKnownLocation", "groupKey": "knownHistory"},
  {"path": "identity.affiliations", "name": "所属势力", "type": "list", "description": "主角已经确认的本人物当前所属势力，使用势力的精确名称或ID；未知留空。", "group": "关系", "labelKey": "loreOverview.row.affiliations", "groupKey": "relations"},
  {"path": "identity.affiliations", "name": "所属势力", "type": "list", "description": "主角已经获知的本人物势力归属及其状态；身份或状态未知时不推断。", "group": "关系", "labelKey": "loreOverview.row.affiliations", "groupKey": "relations"},
  {"path": "knowledge.relationships", "name": "已知人物关系", "type": "object", "description": "只记录主角已经获知的该人物对其他已知人物的有方向关系。", "group": "关系", "labelKey": "loreOverview.relationships.title", "groupKey": "relations"},
  {"path": "identity.name", "name": "姓名", "type": "string", "description": "主角姓名；尚未确定时保持空白，确认后只在正式改名时更新。", "group": "概况", "labelKey": "storyStage.state.source.identity.name", "groupKey": "overview"},
  {"path": "identity.profile", "name": "基本身份", "type": "string", "description": "合并记录年龄、性别、职业、种族、社会身份等当前故事确实使用的信息。", "group": "概况", "labelKey": "storyStage.state.source.identity.profile", "groupKey": "overview"},
  {"path": "identity.appearance_style", "name": "外貌与装扮", "type": "string", "description": "合并记录有辨识度的外貌、衣着、装备外观和当前伪装；忽略无承接价值的临时描写。", "group": "概况", "labelKey": "storyStage.state.source.identity.appearance_style", "groupKey": "overview"},
  {"path": "identity.character_background", "name": "性格与背景", "type": "string", "description": "记录会持续影响选择的性格倾向与已确认背景，不复述回合经历。", "group": "概况", "labelKey": "storyStage.state.source.identity.character_background", "groupKey": "overview"},
  {"path": "current.situation", "name": "当前处境", "type": "string", "description": "记录当前优势、危险、限制和直接压力；具体地点由故事状态维护。", "group": "战斗与状态", "labelKey": "storyStage.state.source.current.situation", "groupKey": "condition"},
  {"path": "identity.profile", "name": "基本身份", "type": "string", "description": "合并记录年龄、性别、职业、种族、组织与公开身份等有用信息。", "group": "概况", "labelKey": "storyStage.state.source.identity.profile", "groupKey": "overview"},
  {"path": "identity.appearance_style", "name": "外貌与装扮", "type": "string", "description": "合并记录辨识特征、常见装束、装备外观和当前伪装。", "group": "概况", "labelKey": "storyStage.state.source.identity.appearance_style", "groupKey": "overview"},
  {"path": "identity.character_background", "name": "性格与背景", "type": "string", "description": "记录稳定性格、关键来历和会持续影响行为的背景。", "group": "概况", "labelKey": "storyStage.state.source.identity.character_background", "groupKey": "overview"},
  {"path": "current.presence_location", "name": "出场与位置", "type": "string", "description": "用一句话记录在场、离场、失联、死亡等存续状态及当前位置或最后确认位置。", "group": "战斗与状态", "labelKey": "storyStage.state.source.current.presence_location", "groupKey": "condition"},
  {"path": "current.goal_situation", "name": "当前目标与处境", "type": "string", "description": "记录已经由行为、对话或设定确认的近期目标、压力、优势与限制，不猜测幕后计划。", "group": "战斗与状态", "labelKey": "storyStage.state.source.current.goal_situation", "groupKey": "condition"},
  {"path": "protagonist_relation.summary", "name": "与主角关系", "type": "string", "description": "记录关系类型、当前态度、关系阶段、重要承诺、边界和主要矛盾。", "group": "关系", "labelKey": "storyStage.state.source.protagonist_relation.summary", "groupKey": "relations"},
  {"path": "knowledge.about_protagonist", "name": "对主角的已知信息", "type": "list", "description": "只列会影响该角色判断或行动、且该角色确实已经得知的信息，最多 6 项。", "group": "经历见闻", "labelKey": "storyStage.state.source.knowledge.about_protagonist", "groupKey": "knownHistory"},
  {"path": "identity.profile", "name": "类型与外观", "type": "string", "description": "合并记录敌人、怪物、Boss、陷阱或异常实体的类型、身份与辨识特征。", "group": "概况", "labelKey": "storyStage.state.source.identity.profile.opponent", "groupKey": "overview"},
  {"path": "current.lifecycle_location", "name": "存续与位置", "type": "string", "description": "用一句话记录活跃、撤退、被俘、失效、消灭等状态及所在或最后确认位置。", "group": "概况", "labelKey": "storyStage.state.source.current.lifecycle_location", "groupKey": "overview"},
  {"path": "threat.assessment", "name": "威胁态势", "type": "string", "description": "合并记录威胁级别、警戒或追击状态和当前目标。", "group": "威胁", "labelKey": "storyStage.state.source.threat.assessment", "groupKey": "threat"},
  {"path": "behavior.pattern", "name": "行动模式", "type": "string", "description": "记录已经观察或可靠确认的攻击倾向、触发方式和行为规律。", "group": "经历见闻", "labelKey": "storyStage.state.source.behavior.pattern", "groupKey": "knownHistory"},
  {"path": "weakness.exit_condition", "name": "弱点与退场条件", "type": "string", "description": "合并记录已确认弱点，以及击败、驱散、封印、谈判或摆脱条件。", "group": "经历见闻", "labelKey": "storyStage.state.source.weakness.exit_condition", "groupKey": "knownHistory"},
  {"path": "scene.current_time", "name": "当前时间", "type": "string", "description": "按世界观记录当前日期、时段或阶段；只保留对行动有意义的精度。", "group": "当前场景", "labelKey": "storyStage.state.source.scene.current_time", "groupKey": "world"},
  {"path": "scene.location", "name": "当前详细地点", "type": "string", "description": "用一个字段表达当前具体地点及必要的上级范围，不再拆分大区、区域和房间。", "group": "当前场景", "labelKey": "storyStage.state.source.scene.location", "groupKey": "world"},
  {"path": "scene.current_event", "name": "当前事件", "type": "string", "description": "合并记录正在发生的事件、直接压力和下一步必须面对的问题。", "group": "当前场景", "labelKey": "storyStage.state.source.scene.current_event", "groupKey": "world"},
  {"path": "scene.present_actors", "name": "在场角色", "type": "list", "description": "使用 Actor 状态中已有的精确 ID；新建 Actor 直接使用故事语言中的角色名称，Actor 名称即 ID。只保留当前可感知或正在互动的角色。", "group": "当前场景", "labelKey": "storyStage.state.source.scene.present_actors", "groupKey": "world"},
  {"path": "scene.continuation_hook", "name": "可承接钩子", "type": "string", "description": "保留一个下一段可以直接承接的未完成动作、对话、发现或迫近威胁；它不是剧情总结或选项列表。", "group": "当前场景", "labelKey": "storyStage.state.source.scene.continuation_hook", "groupKey": "world"},
  {"path": "world.situation", "name": "世界局势", "type": "string", "description": "记眼下的世界概况和正在发生的主要事件。写清哪些地方或势力发生了什么，哪些变化仍在持续。", "group": "世界状态", "labelKey": "storyStage.state.source.world.situation", "groupKey": "situation"},
  {"path": "tasks.current", "name": "当前任务", "type": "object", "description": "记录已经成立且仍需推进的任务。", "group": "当前任务", "labelKey": "storyStage.state.source.tasks.current", "groupKey": "tasks"},
  {"path": "world.locations", "name": "地点记录", "type": "object", "description": "记录仍有探索、通行、资源或剧情价值的重要地点。", "group": "地点", "labelKey": "storyStage.state.source.world.locations", "groupKey": "situation"},
  {"path": "world.factions", "name": "势力记录", "type": "object", "description": "记录仍在影响主角、任务或世界局势的重要势力。", "group": "势力", "labelKey": "storyStage.state.source.world.factions", "groupKey": "situation"},
  {"path": "abilities.records", "name": "技能与能力", "type": "object", "description": "记录属于当前 Actor、会再次使用或仍有可变状态的能力。", "group": "行囊", "labelKey": "storyStage.state.source.abilities.records", "groupKey": "holdings"},
  {"path": "assets.important_items", "name": "重要物品", "type": "object", "description": "记录由当前 Actor 持有、会影响后续行动的物品。", "group": "行囊", "labelKey": "storyStage.state.source.assets.important_items", "groupKey": "holdings"},
  {"path": "relations.records", "name": "关系", "type": "object", "description": "记录当前 Actor 对目标 Actor 或势力的有方向关系。", "group": "行囊", "labelKey": "storyStage.state.source.relations.records", "groupKey": "holdings"},
] as const

export function builtinStateFieldDisplay(field: ActorStateField | undefined): { labelKey: string; groupLabelKey?: string } | undefined {
  if (!field) return undefined
  const source = sourceFields.find(entry => entry.name === field.name && entry.type === field.type && entry.description === field.description
    && (!field.path || field.path === entry.path) && (!field.id || field.id === entry.path || field.id === entry.name))
  return source ? { labelKey: source.labelKey, groupLabelKey: field.group === source.group ? `storyStage.state.sourceGroup.${source.groupKey}` : undefined } : undefined
}

const sourceRules: Record<string, { label: string; key: string }> = {
  "balanced-dice-check": {"label": "均衡骰子检定", "key": "snapshot.checkEvidence.rule.balanced-dice-check"},
  "dm-fail-forward": {"label": "推进型 DM：失败也前进", "key": "snapshot.checkEvidence.rule.dm-fail-forward"},
  "dm-osr-player-skill": {"label": "OSR 型 DM：玩家技巧优先", "key": "snapshot.checkEvidence.rule.dm-osr-player-skill"},
  "dm-cinematic-heroic": {"label": "电影英雄型 DM：高光优先", "key": "snapshot.checkEvidence.rule.dm-cinematic-heroic"},
  "dm-gritty-survival": {"label": "硬核生存型 DM：资源与后果", "key": "snapshot.checkEvidence.rule.dm-gritty-survival"},
  "dm-mystery-clue-forward": {"label": "悬疑调查型 DM：线索不断线", "key": "snapshot.checkEvidence.rule.dm-mystery-clue-forward"},
  "dm-drama-stakes": {"label": "戏剧张力型 DM：只为重大赌注掷骰", "key": "snapshot.checkEvidence.rule.dm-drama-stakes"},
}

export function builtinCheckLabelKey(id: string | undefined, label: string | undefined): string | undefined {
  const source = id ? sourceRules[id] : undefined
  return source && source.label === label ? source.key : undefined
}

const sourceModules: Record<string, { name: string; key: string }> = {
  "rule_system:default": {"name": "均衡 DM 检定", "key": "snapshot.sourceModule.rule_system.default"},
  "rule_system:dm-fail-forward": {"name": "推进型 DM：失败也前进", "key": "snapshot.sourceModule.rule_system.dm-fail-forward"},
  "rule_system:dm-osr-player-skill": {"name": "OSR 型 DM：玩家技巧优先", "key": "snapshot.sourceModule.rule_system.dm-osr-player-skill"},
  "rule_system:dm-cinematic-heroic": {"name": "电影英雄型 DM：高光优先", "key": "snapshot.sourceModule.rule_system.dm-cinematic-heroic"},
  "rule_system:dm-gritty-survival": {"name": "硬核生存型 DM：资源与后果", "key": "snapshot.sourceModule.rule_system.dm-gritty-survival"},
  "rule_system:dm-mystery-clue-forward": {"name": "悬疑调查型 DM：线索不断线", "key": "snapshot.sourceModule.rule_system.dm-mystery-clue-forward"},
  "rule_system:dm-mystery-investigation-stats-v1": {"name": "侦探调查检定", "key": "snapshot.sourceModule.rule_system.dm-mystery-investigation-stats-v1"},
  "rule_system:dm-drama-stakes": {"name": "戏剧张力型 DM：只为重大赌注掷骰", "key": "snapshot.sourceModule.rule_system.dm-drama-stakes"},
  "actor_state:default": {"name": "默认状态系统", "key": "snapshot.sourceModule.actor_state.default"},
  "actor_state:xiuxian-state": {"name": "修仙状态系统", "key": "snapshot.sourceModule.actor_state.xiuxian-state"},
  "actor_state:detective-investigation-v1": {"name": "侦探调查状态", "key": "snapshot.sourceModule.actor_state.detective-investigation-v1"},
  "actor_state:western-fantasy-state": {"name": "西幻状态系统", "key": "snapshot.sourceModule.actor_state.western-fantasy-state"},
  "actor_state:apocalypse-state": {"name": "末世状态系统", "key": "snapshot.sourceModule.actor_state.apocalypse-state"},
  "actor_state:infinite-flow-state": {"name": "无限流状态系统", "key": "snapshot.sourceModule.actor_state.infinite-flow-state"},
}

/** Localize source-owned module captions only; renamed/custom IDs stay literal. */
export function builtinModuleDisplayName(kind: 'rule_system' | 'actor_state', module: { id: string; name?: string; builtin_overridden?: boolean } | undefined, t: TFunction): string {
  if (!module) return ''
  if (!module.builtin_overridden) {
    const source = sourceModules[`${kind}:${module.id}`]
    if (source && source.name === module.name) return t(source.key)
    const prefix = kind === 'actor_state' ? 'world-actor-state-' : 'world-rule-system-'
    const name = kind === 'actor_state' ? '世界专属行动状态' : '世界专属行动检定'
    if (module.name === name && module.id.startsWith(prefix) && /^[a-f0-9]{20}$/.test(module.id.slice(prefix.length))) {
      return t(`snapshot.sourceModule.${kind}.world`)
    }
  }
  return module.name || module.id
}
