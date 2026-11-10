import { useHistoryLabels } from './presentation'
import { reportAccounts } from './report-accounts'
import type { HistoryState } from './types'

export function HistoryReportAccounts({ state, reportID }: { state: HistoryState; reportID: string }) {
  const { t, text, number } = useHistoryLabels(state)
  const rows = reportAccounts(state, reportID)
  if (!rows.length) return null
  return <details className="history-details" open><summary>{t('history.port.accountChanges')}</summary><div className="history-account-table"><table><thead><tr>{['account', 'opening', 'change', 'closing'].map(key => <th key={key}>{t(`history.port.balance.${key}`)}</th>)}</tr></thead><tbody>{rows.map(account => <tr key={account.id}><th scope="row">{text(account.name)}<small>{text(state.scenario!.resources.find(resource => resource.id === account.resource)?.unit)}</small></th><td>{account.opening === undefined ? '—' : number(account.opening)}</td><td>{account.change > 0 ? '+' : ''}{number(account.change)}</td><td>{number(account.balance)}</td></tr>)}</tbody></table></div></details>
}
