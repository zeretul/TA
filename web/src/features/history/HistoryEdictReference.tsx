import { HistoryAdviceLibrary } from './HistoryAdviceLibrary'
import { Button } from '@/components/ui/button'
import { useHistoryLabels } from './presentation'
import type { HistoryGameController } from './use-history-game'

export function HistoryEdictReference({ game, draft, onChange }: { game: HistoryGameController; draft: string; onChange: (text: string) => void }) {
  const { t, minister } = useHistoryLabels(game.state)
  const state = game.state!
  const report = state.reports.filter(item => !item.discarded).at(-1)
  const chats = state.chats.filter(item => !item.discarded).slice(-8).reverse()
  const orders = state.world!.orders.filter(order => ['active', 'waiting', 'paused'].includes(order.status))
  return <aside className="history-edict-reference" aria-label={t('history.port.reference')}>
    <h3>{t('history.port.reference')}</h3><p className="history-muted">{t('history.port.referenceHint')}</p>
    <HistoryAdviceLibrary game={game} draft={draft} />
    <details className="history-details"><summary>{t('history.port.examples')}</summary><div className="history-actions">{['focus','audit','appointment','industry','diplomacy','research','succession','petition'].map(key => <Button key={key} size="sm" variant="outline" disabled={game.busy} onClick={() => onChange(`${draft.trim()}${draft.trim() ? '\n\n' : ''}${t(`history.port.template.${key}`)}`)}>{t(`history.port.example.${key}`)}</Button>)}</div></details>
    <details className="history-details"><summary>{t('history.port.latestReport')}</summary><p className="history-prose">{report?.narrative || t('history.world.noReports')}</p></details>
    <details className="history-details"><summary>{t('history.port.savedAdvice', { count: chats.length })}</summary>{chats.map(chat => <article className="history-event" key={chat.id}><strong>{chat.minister_ids.map(minister).join(' · ')}</strong><p className="history-prose">{chat.user_message}</p>{chat.replies.map((reply, index) => <p key={index} className="history-prose">{minister(reply.minister_id)}：{reply.text}</p>)}<Button size="sm" variant="outline" disabled={game.busy} onClick={() => void game.act('prepare-edict', { source_chat_id: chat.id, text: draft.trim() || t('history.court.draftInstruction') })}>{t('history.port.useAdvice')}</Button></article>)}{!chats.length && <p className="history-muted">{t('history.court.empty')}</p>}</details>
    <details className="history-details"><summary>{t('history.world.tab.orders')} · {orders.length}</summary>{orders.map(order => <article className="history-event" key={order.id}><span className="history-tag">{t(`history.world.status.${order.status}`)}</span><p className="history-prose">{order.text}</p><small>{order.last_result}</small></article>)}{!orders.length && <p className="history-muted">{t('history.world.noOrders')}</p>}</details>
  </aside>
}
