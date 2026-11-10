import type { HistoryState, WorldAccount } from './types'

// Reconstruct only visible account facts from the frozen opening and canonical
// reports. An unknown opening is not silently treated as a zero balance.
export function reportAccounts(state: HistoryState, reportID: string) {
  const accounts = new Map<string, WorldAccount>((state.scenario?.initial?.accounts || []).map(account => [account.id, { ...account }]))
  for (const report of state.reports.filter(item => !item.discarded)) {
    const opening = new Map([...accounts].map(([id, account]) => [id, account.balance]))
    for (const account of report.world?.new_accounts || []) { accounts.set(account.id, { ...account }); opening.set(account.id, account.balance) }
    const movement = new Map<string, number>()
    for (const transfer of report.world?.transfers || []) {
      for (const [id, delta] of [[transfer.from, -transfer.amount], [transfer.to, transfer.amount]] as const) {
        const account = accounts.get(id)
        if (account) { account.balance += delta; movement.set(id, (movement.get(id) || 0) + delta) }
      }
    }
    if (report.id === reportID) return [...accounts.values()].filter(account => movement.has(account.id) || account.owner === (state.scenario?.player_entity_id || state.world?.accounts[0]?.owner)).map(account => ({ ...account, opening: opening.get(account.id), change: movement.get(account.id) || 0 }))
  }
  return []
}
