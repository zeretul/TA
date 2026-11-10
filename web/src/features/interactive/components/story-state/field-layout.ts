import type { ActorStateField } from '../../types'
import { builtinStateFieldDisplay } from '../../builtin-display-labels'

/** Renderer kinds understood by the stage state ledger. */
export type StateFieldRenderer = 'stat' | 'inline' | 'block' | 'list' | 'object' | 'injuries' | 'methods' | 'skills'

/** Built-in ledger groups used when a field declares no explicit group. */
export type StateFieldGroupKey = 'overview' | 'present' | 'world' | 'situation' | 'details' | 'holdings' | 'skills' | 'methods' | 'relations'

export const BUILTIN_STATE_FIELD_GROUPS: StateFieldGroupKey[] = ['world', 'present', 'situation', 'overview', 'skills', 'methods', 'holdings', 'relations', 'details']

export interface StateFieldLayout {
  renderer: StateFieldRenderer
  /** Builtin group key, or the template-declared group name. */
  group: string
  /** True when the group name came from ActorStateField.group. */
  customGroup: boolean
  groupLabelKey?: string
}

/** Strings at or above this length render as block paragraphs by default. */
const LONG_TEXT_THRESHOLD = 24

export function resolveStateFieldLayout(field: ActorStateField | undefined, value: unknown): StateFieldLayout {
  const renderer = resolveStateFieldRenderer(field, value)
  // The player-facing grouping comes first. The pack declares groups in its own
  // vocabulary — 行囊, 修为, 战斗与状态 — and those declarations are what the rules
  // and the saved layout key on, so they stay untouched. What the page shows is
  // a different question: a relation is not a belonging, and a learned technique
  // is not a second kind of inventory, so the four cases the design names are
  // re-grouped for display only.
  const facing = playerFacingGroupKey(field)
  if (facing) {
    return { renderer, group: facing, customGroup: false }
  }
  const declaredGroup = typeof field?.group === 'string' ? field.group.trim() : ''
  if (declaredGroup) {
    const labelKey = builtinStateFieldDisplay(field)?.groupLabelKey
    return { renderer, group: declaredGroup, customGroup: true, ...(labelKey ? { groupLabelKey: labelKey } : {}) }
  }
  switch (renderer) {
    case 'stat':
    case 'inline':
      return { renderer, group: 'overview', customGroup: false }
    case 'block':
      return { renderer, group: 'details', customGroup: false }
    default:
      return { renderer, group: 'holdings', customGroup: false }
  }
}

/**
 * playerFacingGroupKey is the display regrouping the design asks for.
 *
 * It answers only the four cases where the pack's own grouping reads wrong to a
 * player: what a character can do (技能), the methods they cultivate (功法), who
 * they are bound to (关系), and what they carry (行囊). Everything else keeps the
 * pack's declared group, because a group this interface invents would be a
 * second vocabulary the saved layout could not round-trip.
 */
export function isLegacyWorldOverview(field: ActorStateField | undefined, name: string): boolean {
  return ['cultivation_world.state', 'fantasy_world.order', 'apocalypse.situation', 'infinite_space.status'].includes(field?.path ?? '')
    || ['修行世界状态', '魔法、信仰与政治秩序', '灾变与生存局势', '轮回空间状态'].includes(name)
}

export function playerFacingGroupKey(field: ActorStateField | undefined): StateFieldGroupKey | null {
  if (!field) return null
  const path = typeof field.path === 'string' ? field.path.trim() : ''
  const name = typeof field.name === 'string' ? field.name.trim() : ''
  // Where someone is and what is pressing on them is the first thing a reader
  // wants, and the design files it under 概况 rather than under 战斗与状态: neither
  // row is a fight, and 战斗与状态 is where the numbers that decide a fight live.
  // The pack declares both rows in 战斗与状态, and that declaration is what the
  // rules and the saved layout key on, so this is a display regroup like the four
  // below it — which also fixes saves frozen with the old declaration.
  if (['identity.name', 'identity.profile', 'identity.appearance_style', 'identity.character_background',
    'current.situation', 'current.presence_location', 'current.goal_situation'].includes(path)) return 'overview'
  if (path === 'abilities.records' || path === 'abilities.learned') return 'skills'
  if (path === 'cultivation.techniques' || path === 'state.cultivation_method') return 'methods'
  if (path === 'relations.records' || path.startsWith('protagonist_relation.')) return 'relations'
  if (path.startsWith('equipment.') || path.startsWith('assets.') || path === 'state.currency') return 'holdings'
  // The world tab is not a dossier, so the pack files its rows by system —
  // 当前场景, 世界状态, 当前任务, 经济 — and read together those headings say
  // nothing about what the reader is looking at. The design splits them into
  // three: where and when the scene is, who is standing in it, and how the
  // wider situation stands. 商贾 stays with the pack's own group: it is the
  // shops' money, not the player's, and moving it beside 在场角色 would read as
  // one more thing in the player's purse.
  if (path === 'scene.current_time' || path === 'scene.location' || path === 'scene.current_event') return 'world'
  if (path === 'scene.present_actors') return 'present'
  if (path === 'world.situation' || path === 'world.locations' || path === 'world.factions') return 'situation'
  if (isLegacyWorldOverview(field, name)) return 'situation'
  // A save written before these paths existed still carries the pack's own field
  // names, and a template this interface has never seen may spell the same three
  // records differently. The names are the pack's, so matching them is reading
  // the declaration rather than guessing at one.
  if (name === '技能与能力' || name === '已学功法') return 'skills'
  if (['姓名', '基本身份', '外貌与装扮', '性格与背景', '当前处境', '出场与位置', '当前目标与处境'].includes(name)) return 'overview'
  if (name === '功法' || name === '功法与专精') return 'methods'
  if (['关系', '对主角好感度', '与主角关系', '所属势力'].includes(name)) return 'relations'
  if (name === '重要物品') return 'holdings'
  if (name === '当前时间' || name === '当前详细地点' || name === '当前事件') return 'world'
  if (name === '在场角色') return 'present'
  if (name === '世界局势' || name === '地点记录' || name === '势力记录') return 'situation'
  return null
}

/**
 * The name a field carries for the player.
 *
 * 主法器 is the pack's word for the one weapon that participates in the main
 * attack, and the design asks the page to call it 主武器 so it reads as a weapon
 * rather than as a treasure. The stored field keeps its name: the rules read the
 * slot by that exact key, so renaming it in state would make every existing save
 * lose its equipment. Only the label changes, which is why this is a display
 * mapping rather than a rename.
 *
 * 经脉负担 and 余伤负担下限 follow the same rule. The first is what a technique
 * costs a body to perform, not damage to a meridian, so the page calls it 施招负担
 * and the word 经脉 stops appearing on every technique. The second is not a
 * parameter the player should have to read at all: it is the number a wound
 * holds the burden above, so the page shows the wound itself — see
 * stateInjuries — and this label is the fallback for a template that still
 * carries the raw field.
 *
 * Shared builtin source captions also follow the interface language, but only
 * when their exact source declaration matches. Custom fields and their prose
 * keep their own wording; translation never renames a stored state key.
 *
 * The Chinese wording stays here as the fallback a caller with no translator at
 * hand still gets; a component that renders asks for the key instead.
 */
const FIELD_DISPLAY_NAMES: Record<string, { name: string; key: string }> = {
  '主法器': { name: '主武器', key: 'storyStage.state.fieldLabel.primaryWeapon' },
  '经脉负担': { name: '施招负担', key: 'storyStage.state.fieldLabel.castBurden' },
  '余伤负担下限': { name: '伤势', key: 'storyStage.state.injury.label' },
}

/** The i18n key for a field label this interface renamed, if it renamed it. */
export function playerFacingFieldLabelKey(field: ActorStateField | undefined): string | undefined {
  const name = typeof field?.name === 'string' ? field.name.trim() : ''
  return builtinStateFieldDisplay(field)?.labelKey || FIELD_DISPLAY_NAMES[name]?.key
}

export function playerFacingFieldName(field: ActorStateField | undefined, fallback: string): string {
  const name = typeof field?.name === 'string' ? field.name.trim() : ''
  return FIELD_DISPLAY_NAMES[name]?.name || fallback
}

function resolveStateFieldRenderer(field: ActorStateField | undefined, value: unknown): StateFieldRenderer {
  const display = field?.display
  if (display === 'inline' || display === 'block' || display === 'list') return display
  if (display === 'stat') return isBoundedNumberField(field, value) ? 'stat' : 'inline'

  if (isBoundedNumberField(field, value)) return 'stat'

  const type = field?.type || inferredType(value)
  switch (type) {
    case 'list':
      return containsObjectItem(value) ? 'object' : 'list'
    case 'object':
      return 'object'
    case 'number':
    case 'bool':
    case 'enum':
      return 'inline'
    default:
      return isLongText(value) ? 'block' : 'inline'
  }
}

export function isBoundedNumberField(field: ActorStateField | undefined, value: unknown): boolean {
  return field?.type === 'number'
    && typeof value === 'number'
    && Number.isFinite(value)
    && hasDeclaredBound(field)
}

/**
 * Reports whether a field declares any bound at all.
 *
 * A fixed `max` counts, and so does a declared `max_field`: the pool fields carry
 * no fixed maximum because their bound is another settled field, but they are
 * still bounded numbers and belong in the stat renderer rather than the inline
 * one. Whether the sibling actually resolves is a separate question, asked where
 * the sibling's value is available.
 */
function hasDeclaredBound(field: ActorStateField): boolean {
  if (typeof field.max === 'number' && Number.isFinite(field.max)) {
    return typeof field.min !== 'number' || !Number.isFinite(field.min) || field.max > field.min
  }
  return typeof field.max_field === 'string' && field.max_field.trim() !== ''
}

function isLongText(value: unknown) {
  return typeof value === 'string' && value.trim().length >= LONG_TEXT_THRESHOLD
}

function containsObjectItem(value: unknown) {
  return Array.isArray(value) && value.some((item) => typeof item === 'object' && item !== null)
}

function inferredType(value: unknown): string {
  if (Array.isArray(value)) return 'list'
  if (value === null) return 'object'
  return typeof value
}
