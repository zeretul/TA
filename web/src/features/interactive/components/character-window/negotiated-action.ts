import type { CharacterExchange, CharacterMatter } from '../../types'

/**
 * The matter kinds that mean something was actually agreed. A disclosure, a
 * misunderstanding or a change of attitude is part of the situation, but none of
 * them is a commitment anyone can act on, so none of them produces an offer to
 * act.
 */
const AGREED_MATTER_KINDS = new Set(['permission', 'action', 'promise'])

/** The player's own move, and the commitments it is being made against. */
export interface NegotiatedAction {
  /**
   * What the other side is already committed to, exactly as the journal holds
   * it. It is shown but never editable: letting the player rewrite this half is
   * how an edited summary would hand an NPC an obligation they never accepted.
   */
  agreed: CharacterMatter[]
  /** The player's own plan, prefilled from what they last said. */
  plan: string
}

/**
 * Reads the window's bottom affordance out of committed facts.
 *
 * It returns null unless something was actually agreed, which is what keeps a
 * hypothesis, an invitation or a hesitation from being dressed up as a settled
 * plan. The agreement is not inferred from the conversation text: it is the
 * branch's own record of promises, permissions and undertakings, so the
 * affordance cannot appear for a conversation where nothing was concluded.
 */
export function buildNegotiatedAction(
  matters: CharacterMatter[] | undefined,
  exchanges: CharacterExchange[] | undefined,
): NegotiatedAction | null {
  const agreed = (matters || []).filter(
    (matter) => matter.status === 'open' && !matter.letter && AGREED_MATTER_KINDS.has(matter.kind),
  )
  if (agreed.length === 0) return null
  return { agreed, plan: lastPlayerLine(exchanges) }
}

/**
 * The text actually sent down the main story's own send chain.
 *
 * Only the player's own plan travels. The commitments on the other side are
 * already in the narrator's established-character block, read from the same
 * journal, so restating them here would be a second, editable copy of a record
 * the player does not own. When the player has cleared their plan, the agreed
 * statements stand in so the action still names what it is following through on.
 */
export function composeNegotiatedMessage(action: NegotiatedAction, plan: string): string {
  const trimmed = plan.trim()
  if (trimmed) return trimmed
  return action.agreed.map((matter) => matter.statement.trim()).filter(Boolean).join('；')
}

/**
 * A one-line prefill from the player's most recent line. It is a starting point
 * the player edits, not a decision made for them: the box is theirs to rewrite
 * or clear.
 */
function lastPlayerLine(exchanges: CharacterExchange[] | undefined): string {
  const lines = exchanges || []
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    const exchange = lines[index]
    if (exchange.speaker !== 'player') continue
    const line = exchange.text.replace(/\s+/g, ' ').trim()
    if (line) return line
  }
  return ''
}
