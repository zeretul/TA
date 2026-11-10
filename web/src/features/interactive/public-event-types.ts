/**
 * The public event entry's own shapes.
 *
 * Every field here is one the event routes really return. The rules layer owns
 * the stages, the lines a character may be shown and the reward; this file only
 * names them for the panel, so nothing in the interface can invent a stage, a
 * secret or a payment.
 */

/** Whether a stage is behind the branch's committed instant, current, or still ahead. */
export type PublicEventStageStatus = 'past' | 'current' | 'ahead'

/** One payment, stated in the event's own wording. */
export interface PublicEventReward {
  kind: 'currency' | 'item'
  ref_id: string
  quantity: number
}

/** One stage and what this character may be told about it. */
export interface PublicEventStage {
  id: string
  title: string
  status: PublicEventStageStatus
  /**
   * The public summary always, plus the participant lines only for the stage the
   * character was actually present at. A secret is never handed out by the
   * server, so it cannot appear here.
   */
  lines: string[]
  /** The branch clock instant this stage becomes current. */
  opens_at_seconds: number
}

/** One party the region knows about. Their secret motive is not in this view. */
export interface PublicEventParticipant {
  id: string
  name: string
  role: string
}

/** One way to take part, in the event's own wording. */
export interface PublicEventActionOption {
  kind: string
  label: string
  brief: string
  /** What this choice really pays, absent when it pays nothing. */
  reward?: PublicEventReward
}

/** One public event as this branch knows it. */
export interface PublicEvent {
  event_id: string
  event_version: string
  title: string
  brief: string
  region: string
  /** Empty before the event opens, which is how the panel knows to say "not yet". */
  current_stage_id?: string
  current_stage_title?: string
  closed: boolean
  stages: PublicEventStage[]
  participants: PublicEventParticipant[]
  actions: PublicEventActionOption[]
  /** Whether this character took part, and how. */
  joined: boolean
  participation?: string
  withdrawn?: boolean
  joined_at_seconds?: number
  /** The stage they were present at, which is not the current stage once it moves on. */
  joined_stage?: string
}

/**
 * The event entry's read.
 *
 * Reading it costs no world time and no generation: each stage is derived from
 * the branch's committed clock, so opening the entry cannot advance the event.
 */
export interface PublicEventState {
  story_id: string
  branch_id: string
  /** The scene's committed location text, which decides which events are known. */
  location?: string
  clock_seconds: number
  clock_text?: string
  events: PublicEvent[]
}

/** One committed participation action, as the write reports it. */
export interface PublicEventResult {
  story_id: string
  branch_id: string
  event_id: string
  action: PublicEventAction
  /** The choice recorded on the participation. */
  participation: string
  joined_stage?: string
  withdrawn: boolean
  /** What this action really paid, empty for a withdrawal. */
  rewards: PublicEventReward[]
  /** True when this answer came from an action already committed under the same command. */
  replayed: boolean
}

/** The participation actions the entry can settle. */
export type PublicEventAction = 'join' | 'withdraw'
