/**
 * An arrangement made in a window, waiting for the player's own send.
 *
 * A plan agreed with a person or a group is not an act. Adding it puts words in
 * the main composer's draft; the story only sees them when the player presses
 * send, together with whatever else they wrote and whichever option they pick.
 * Keeping it here rather than in the window is what lets the window close while
 * the player keeps talking, and what keeps a half-made plan out of the journal.
 */
export interface StagedAction {
  id: string
  /** The player's own words, exactly as they will join the main request. */
  message: string
  /**
   * A letter is a formal record, not a sentence: the story needs the recipient
   * and the body as their own fields, so they travel beside the words the turn
   * reads rather than inside them.
   */
  playerLetter?: { character_id: string; body: string }
}

/** Two entries that would make the same request are the same entry. */
function actionKey(action: StagedAction): string {
  return `${action.playerLetter?.character_id || ''}\u0000${action.message.replace(/\s+/g, ' ').trim()}`
}

let stagedActionSequence = 0

/** An id that only has to be unique within one stage's staged list. */
export function newStagedActionID(): string {
  stagedActionSequence += 1
  return `staged-${stagedActionSequence}-${Date.now()}`
}

/**
 * Adds one arrangement, ignoring a repeat.
 *
 * Pressing the same button twice is one intention, not two, and the story must
 * not be handed the same plan twice in one request.
 */
export function addStagedAction(current: StagedAction[], action: StagedAction): StagedAction[] {
  if (!action.message.trim() && !action.playerLetter) return current
  const key = actionKey(action)
  if (current.some((item) => actionKey(item) === key)) return current
  return [...current, action]
}

/**
 * The single request a staged list and the player's own box make together.
 *
 * Each entry keeps its own line so the player can see in the transcript what
 * they sent, and the typed text comes last because it is the part they wrote
 * most recently. The letter, when there is one, is the request's own field.
 */
export function composeStagedRequest(
  staged: StagedAction[],
  typed: string,
): { message: string; playerLetter?: StagedAction['playerLetter'] } {
  const parts = [...staged.map((item) => item.message.trim()), typed.trim()].filter(Boolean)
  const letter = staged.find((item) => item.playerLetter)?.playerLetter
  return { message: parts.join('\n'), ...(letter ? { playerLetter: letter } : {}) }
}
