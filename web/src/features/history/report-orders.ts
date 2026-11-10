import type { HistoryState } from './types'

// Reconstruct the wording in force when the selected season was committed.
// Edicts belong to the live branch; later amendments never rewrite past reports.
export function reportOrderText(state: HistoryState, turn: number, orderID: string) {
 let text: string | undefined
 for (const edict of state.edicts) {
  if (edict.turn > turn) continue
  for (const [index, intent] of (edict.plan.orders || []).entries()) {
   const id = intent.action === 'create' ? edict.outcomes[index]?.project_id : intent.order_id
   if (id === orderID && (intent.action === 'create' || intent.action === 'revise') && intent.text) text = intent.text
  }
 }
 return text
}
