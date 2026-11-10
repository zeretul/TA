/**
 * The realm entry's own shapes.
 *
 * Every field here is one the realm routes really return. The rules layer owns
 * the rooms, the connections and the rewards; this file only names them for the
 * panel, so nothing in the interface can invent a place the instance does not
 * have.
 */

/** One room of a realm the character has actually discovered. */
export interface RealmArea {
  id: string
  name: string
  name_key?: string
  description: string
  description_key?: string
}

/** One passage between two rooms, as the rules declared it. */
export interface RealmConnection {
  id: string
  from_area_id: string
  to_area_id: string
  bidirectional: boolean
  player_visible: boolean
}

/** One stated goal of the realm. */
export interface RealmObjective {
  id: string
  description: string
  description_key?: string
}

/** One reward the character may take right now, as the server resolved it. */
export interface RealmRewardOption {
  /** The realm definition's own reference, which is what a take names. */
  content_ref_id: string
  ref_id: string
  /** The catalogue's localisation key, so the panel never invents a name. */
  name_key: string
  name?: string
  quantity: number
}

/** One reward this instance has already handed over. */
export interface RealmGrantedReward {
  content_ref_id: string
  ref_id: string
  name_key: string
  name?: string
  quantity: number
}

/** One realm this branch has entered. */
export interface RealmInstance {
  instance_id: string
  realm_id: string
  name: string
  name_key?: string
  theme: string
  description_key?: string
  presence: 'inside' | 'outside'
  view: RealmInstanceView
  /** Where the character stood when they walked in, restored on the way out. */
  origin_location?: string
  /**
   * The declared rewards in the room the character is standing in that have not
   * been taken. Resolved by the server, so the panel never offers one the rules
   * would refuse.
   */
  rewards: RealmRewardOption[]
  /** What this instance has already handed over. */
  granted: RealmGrantedReward[]
  /** Current traversable doors; known-map connections may no longer be usable. */
  ways?: { connection_id: string; destination_area_id: string; destination_name: string; destination_name_key?: string }[]
  explore?: RealmExplore
}

/** Only the server can establish eligibility, costs and revealed approaches. */
export interface RealmExploreCost {
  kind: 'consume_item' | 'require_item' | 'use_skill'
  ref_id: string
  quantity: number
  name?: string
  name_key?: string
  energy_cost?: number
}

export interface RealmExploreApproach {
  id: string
  kind: 'survey' | 'prepare' | 'approach' | 'resolve'
  name: string
  name_key: string
  description: string
  description_key: string
  duration_seconds: number
  pressure_delta: number
  costs: RealmExploreCost[]
  available: boolean
  causes_setback: boolean
  blocked_reason?: string
}

export interface RealmExplore {
  pressure: number
  pressure_limit: number
  pressure_label: string
  pressure_label_key: string
  setback: boolean
  /** Absolute story time, never a wall-clock countdown. */
  return_due_seconds?: number
  return_available: boolean
  return_opened?: boolean
  approaches: RealmExploreApproach[]
}

/**
 * What this character really knows about the realm.
 *
 * Only discovered rooms and passages appear, so a hidden chamber is not on the
 * panel until it has been walked into.
 */
export interface RealmInstanceView {
  instance_id: string
  presence: 'inside' | 'outside'
  current_area?: RealmArea
  last_exit_area_id?: string
  objectives: RealmObjective[]
  areas: RealmArea[]
  connections: RealmConnection[]
  confirmed_facts: { id: string; text: string; text_key?: string; publicly_displayable: boolean }[]
  rumors: { id: string; text: string; text_key?: string; publicly_displayable: boolean }[]
  completed_objectives: string[]
  resolved_challenges: string[]
  granted_rewards: {
    id: string
    ref_id: string
    quantity: number
    source_ref: string
  }[]
}

/** One authored realm the entry can still offer. */
export interface RealmOption {
  realm_id: string
  realm_version: string
  name: string
  name_key?: string
  theme: string
  description_key?: string
  entry_area_id: string
}

/**
 * The realm entry's read.
 *
 * Reading it costs no world time and no generation, so opening the entry never
 * moves the character into a realm.
 */
export interface RealmState {
  story_id: string
  branch_id: string
  clock_seconds?: number
  /** The scene's committed location text, which is where the character stands. */
  location?: string
  instances: RealmInstance[]
  available: RealmOption[]
  /** Encountered partners and discovered instances only; read-only draft options. */
  cooperations?: RealmCooperation[]
}

/** One reward a committed action handed over, in the catalogue's wording. */
export interface RealmReward {
  ref_id: string
  /** The catalogue's localisation key, so the panel never invents a name. */
  name_key: string
  quantity: number
}

/** One committed realm action, as the write reports it. */
export interface RealmResult {
  story_id: string
  branch_id: string
  event_id: string
  instance_id: string
  realm_id: string
  action: 'enter' | 'move' | 'reward' | 'exit' | 'reenter'
  presence: 'inside' | 'outside'
  instance: unknown
  view: RealmInstanceView
  reward?: RealmReward
  /** True when this answer came from an action already committed under the same command. */
  replayed: boolean
}

/** The realm actions the entry can settle. */
export type RealmAction = RealmResult['action']

/** A bounded, server-projected preparation with a known partner. */
export interface RealmCooperation {
  project_id: string
  project_version: string
  realm_id: string
  instance_id: string
  name_key: string
  description_key: string
  player_role_key: string
  partner_role_key: string
  /** The already discovered clue the player may choose to share. */
  clue_text?: string
  clue_text_key?: string
  partner_id: string
  partner_name: string
  location_id: string
  location_name?: string
  phase: 'none' | 'agreed' | 'worked' | 'used' | 'reported'
  duration_seconds: number
  partner_energy_cost: number
  partner_energy_label_key?: string
  player_costs: RealmExploreCost[]
  can_ask: boolean
  can_work_if_agreed: boolean
  can_work: boolean
  can_report: boolean
  blocked_reason?: string
  report_facts: RealmCooperationReportFact[]
}

/** Only these committed, not-yet-shared outcomes may be selected for a report. */
export interface RealmCooperationReportFact {
  kind: 'ingress' | 'disposition' | 'setback' | 'side'
  source_ref: string
  description_key: string
  value_key?: string
}
