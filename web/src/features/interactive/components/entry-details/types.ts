/** Presentation metadata never grants a capability or changes a settlement. */
export const QUALITY_GRADES = ['white', 'green', 'blue', 'purple', 'orange', 'red'] as const
export type { QualityGrade } from '../../types'
export type EntryMetadata = import('../../types').GameEntryMetadata
export interface DetailFact { label: string; value: string }
export type EntryCategory = 'techniques' | 'skills' | 'craft' | 'equipment' | 'inventory' | 'assets'
export type EntryItemGroup = 'weapons' | 'armor' | 'pills' | 'quest' | 'manuals' | 'tools' | 'materials' | 'assets' | 'other'
export interface EntryPresentation extends EntryMetadata {
  id: string
  name: string
  category: EntryCategory
  summary?: string
  grade_label?: string
  item_group?: EntryItemGroup
  visual_theme?: 'cultivation' | 'wuxia' | 'space' | 'fantasy'
  /** Only actual mastery or a declared definition stage, never an invented global rank. */
  visual_level?: { kind: 'mastery'; value: number; maximum: number; scale?: string } | { kind: 'stage'; value: string }
  requirements: DetailFact[]
  effects: DetailFact[]
  costs: DetailFact[]
  status: DetailFact[]
}

export interface AdventureSkillDetails extends EntryMetadata {
  id: string; name: string; kind?: string; scale?: string; cost?: number; power?: number
  learned?: boolean; trainable?: boolean
  required_rank?: number; prerequisite?: string; required_item?: string; training_hours?: number
  required_tool?: string; required_weapon?: string; required_weapon_class?: string; materials?: Record<string, number>
  effects?: { kind: string; power: number }[]
  enemy_effects?: { kind: string; power: number }[]
  conditional_effects?: { required_item: string; effects: { kind: string; power: number }[] }[]
  weapon_style?: boolean; rush?: boolean; once_per_target?: boolean
  backlash?: { health_percent: number; injury: number; mitigated_health_percent?: number; mitigated_injury?: number; foundation_skills?: string[]; foundation_rank?: number }
  usable?: boolean; reason_key?: string
}
export interface AdventureItemDetails extends EntryMetadata {
  id: string; name: string; kind?: string; scale?: string; effect?: string; power?: number
  /** Trusted template identity for a story-local item variant. */
  generated_from?: string
  weapon_class?: string; required_skill?: string; materials?: Record<string, number>; support_only?: boolean
  max_charges?: number; durability?: number
  equippable?: boolean; deployable?: boolean
  usable?: boolean; reason_key?: string
}
export interface AdventureDetailsResponse {
  head: string; family: 'wuxia' | 'space' | 'fantasy'
  actor: { skills?: Record<string, number>; inventory?: Record<string, number>; training?: Record<string, number>; equipment?: Record<string, string>; assets?: Record<string, { health: number; max_health: number; shield: number; fuel: number }>; item_charges?: Record<string, number> }
  skills: AdventureSkillDetails[]; items: AdventureItemDetails[]
  craft_methods?: (EntryMetadata & {
    id: string; name: string; skill_id: string; skill_rank: number; hours: number
    materials: Record<string, number>; output_id: string; output_count: number
    required_asset?: string; required_tool?: string; fuel_cost?: number
    /** Public references in this learned method, not additions to owned inventory. */
    item_names?: Record<string, string>; skill_name?: string
  })[]
}

/** Exact safe, owned-only projection; definitions are never read from author JSON. */
export interface CultivationEntry extends EntryMetadata {
  material_names?: Record<string, string>
  id: string; def_id?: string; record_key?: string; version?: number; name: string; name_key?: string
  definition?: Record<string, unknown>
  quantity?: number; slot?: string; mastered?: boolean; active?: boolean; learning_minutes?: number; required_learning_minutes?: number
  stage_eligible?: boolean; castable?: boolean; usable?: boolean; reason_key?: string
  mana_cost?: number; burden_cost?: number; operating_mana_cost?: number; effective_strain_reduction?: number
  restore_amount?: number; attack_bonus?: number; shield_amount?: number; duration_exchanges?: number
  recorded_grade?: string; recorded_rarity?: string; recorded_effect?: string; recorded_kind?: string; uncatalogued?: boolean
}
export interface CultivationDetailsResponse {
  head: string; skills: CultivationEntry[]; techniques: CultivationEntry[]; items: CultivationEntry[]; craft_methods?: CultivationEntry[]
}
