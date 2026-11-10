/**
 * The task entry's own shapes.
 *
 * Every field here is one the task routes really return. The rules layer owns the
 * objectives, the facts that satisfy them and the reward; this file only names
 * them for the panel, so nothing in the interface can invent a goal or a tick.
 */

/** Which layer of 自制-事件与任务 a task belongs to. */
export type QuestKind = 'errand' | 'hunt' | 'personal'

/**
 * A task's state. An offer is not an acceptance.
 *
 * 'expired' is written by the formal clock, never by the panel: an agreement
 * whose instant the committed time crossed closes even when nobody was looking.
 */
export type QuestStatus = 'offered' | 'accepted' | 'completed' | 'abandoned' | 'expired'

/** One objective and whether a committed fact has satisfied it. */
export interface QuestObjective {
  id: string
  description: string
  completed: boolean
  /** The committed fact that satisfied it, empty while open. */
  source_ref?: string
}

/** One promised payment, stated before acceptance. */
export interface QuestReward {
  kind: 'item' | 'currency'
  ref_id: string
  /** The catalogue's localisation key for an item, so the panel invents no name. */
  name_key?: string
  quantity: number
}

/** One task really in play on this branch. */
export interface QuestTask {
  task_id: string
  task_version: string
  kind: QuestKind
  title: string
  brief: string
  giver?: string
  status: QuestStatus
  /** The task's own wording plus its real state, so a tick means it happened. */
  objectives: QuestObjective[]
  rewards: QuestReward[]
  /** True once the payment was really handed over. */
  settled: boolean
  accepted_at?: string
  settled_at?: string
  /**
   * The absolute formal instant the agreement runs to, absent when none was
   * agreed. It is the same number the rules expire on, so the panel cannot
   * count down to a moment the arithmetic would not honour.
   */
  deadline_seconds?: number
  /**
   * The same instant in the story's own wording, absent when no deadline was
   * agreed. The panel says 未约定期限 rather than inventing a countdown.
   */
  deadline_text?: string
  /** Committed time left when this view was read; negative once the instant passed. */
  remaining_seconds?: number
  /** True once the deadline passed before the work was finished. */
  expired?: boolean
}

/** One authored task the entry can still offer. */
export interface QuestOption {
  task_id: string
  task_version: string
  kind: QuestKind
  title: string
  brief: string
  giver?: string
  /**
   * The interval the offer runs once accepted, absent when none is agreed. It is
   * a duration rather than an instant because the limit starts at acceptance.
   */
  deadline_seconds?: number
}

/**
 * The task entry's read.
 *
 * Reading it costs no world time and no generation, so opening the entry never
 * accepts anything on the player's behalf.
 */
export interface QuestState {
  story_id: string
  branch_id: string
  /** The scene's committed location text. */
  location?: string
  tasks: QuestTask[]
  available: QuestOption[]
}

/** One payment a committed action really handed over. */
export interface QuestSettledReward {
  kind: 'item' | 'currency'
  ref_id: string
  name_key?: string
  quantity: number
}

/** One committed task action, as the write reports it. */
export interface QuestResult {
  story_id: string
  branch_id: string
  event_id: string
  task_id: string
  action: QuestAction
  status: QuestStatus
  objectives: QuestObjective[]
  /** What this action paid, empty for any other action. */
  rewards: QuestSettledReward[]
  settled: boolean
  /** True when this answer came from an action already committed under the same command. */
  replayed: boolean
}

/** The task actions the entry can settle. */
export type QuestAction = 'offer' | 'accept' | 'abandon' | 'complete' | 'settle'
