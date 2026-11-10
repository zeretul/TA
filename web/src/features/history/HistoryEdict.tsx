import { Feather, ScrollText, Check, ChevronRight, Clock3, Coins, Users } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import type { HistoryGameController } from './use-history-game'
import { HistoryEdictReference } from './HistoryEdictReference'
import { useHistoryLabels } from './presentation'

interface Props { game: HistoryGameController; text: string; onChange: (value: string) => void }
export function HistoryEdict({ game, text, onChange }: Props) {
  const { t, number, region, minister, kind, reason, status } = useHistoryLabels(game.state)
  const { text: localText } = useHistoryLabels(game.state)
  const prepared = game.prepared
  const orders = prepared?.plan.orders || []
  const instructions = prepared?.plan.instructions || []
  const savedAdvice = game.state?.advice?.find(item=>item.id===prepared?.plan.source_advice_id)
  const source = prepared?.plan.source_chat_id ? game.state?.chats.find((chat) => chat.id === prepared.plan.source_chat_id && !chat.discarded) : undefined
  const issue = async () => {
    if (!prepared) return
    if (await game.act('edict', { plan: prepared.plan, expected_revision: prepared.expected_revision })) {
      if (!prepared.plan.source_chat_id) onChange('')
    }
  }
  return <div className={game.state?.world ? "history-edict-layout" : undefined}><section className="history-manuscript" aria-labelledby="history-edict-heading">
    <div className="history-section-heading"><Feather aria-hidden="true" /><h2 id="history-edict-heading">{t(prepared ? 'history.edict.review' : 'history.edict.title')}</h2></div>
    <p className="history-muted">{t(prepared ? 'history.edict.reviewHint' : 'history.edict.hint')}</p>
    {prepared ? <>
      {savedAdvice&&<blockquote className="history-condition"><strong>{t('history.review.selectedAdvice')}</strong><p>{savedAdvice.text}</p></blockquote>}
      {source && <details className="history-details"><summary>{t('history.edict.source', { turn: source.turn + 1, names: source.minister_ids.map(minister).join(' · ') })}</summary><div className="history-prose mb-4"><strong>{t('history.court.speaker')}</strong><p>{source.user_message}</p>{source.replies.map((reply, index) => <div className="mt-3" key={`${reply.minister_id}-${index}`}><strong>{minister(reply.minister_id)}</strong><p>{reply.text}</p></div>)}</div></details>}
      {source && text.trim() && <p className="history-muted mt-3">{t('history.edict.draftRetained')}</p>}
      <details className="history-details"><summary>{t(source ? 'history.edict.draftingRequest' : 'history.edict.original')}</summary><p className="history-prose">{prepared.plan.text}</p></details>
      {!!orders.length && <ol className="history-orders">{orders.map((order,index)=><li key={index} className="history-order"><div className="history-order-title"><span className="history-order-number">{index+1}</span><strong>{t(`history.world.action.${order.action}`)}</strong>{order.disclosure === 'sealed' && <span className="history-tag">{t('history.port.sealed')}</span>}{order.kind && <span className="history-tag">{t(`history.world.order.${order.kind}`)}</span>}</div><p className="history-prose">{order.text || game.state?.world?.orders.find(o=>o.id===order.order_id)?.text}</p>{order.minister_id && <p className="history-muted">{minister(order.minister_id)}</p>}{order.condition && <p className="history-condition">{order.condition}</p>}{order.targets?.length ? <p className="history-muted">{order.targets.map(id=>localText(game.state?.world?.entities.find(e=>e.id===id)?.name)).join(' · ')}</p> : null}{Object.entries(order.limits || {}).map(([resource,amount])=><p key={resource}>{t('history.world.spendingCap')}: {number(amount)} {localText(game.state?.scenario?.resources.find(r=>r.id===resource)?.unit)}</p>)}{prepared.outcomes?.[index]?.status==='unsupported' && <p role="alert">{t('history.world.invalidOrder')}</p>}</li>)}</ol>}
      <ol className="history-orders">
        {instructions.map((instruction, index) => <li key={instruction.id || index} className={instruction.kind === 'unsupported' ? 'history-order history-order-unsupported' : 'history-order'}>
          <div className="history-order-title"><span className="history-order-number">{index + 1}</span><h3>{kind(instruction.kind)}</h3>{instruction.kind === 'unsupported' && <span className="history-tag">{t('history.edict.unsupported')}</span>}</div>
          {instruction.kind !== 'unsupported' && <div className="history-order-facts">
            {instruction.target && <span><ChevronRight aria-hidden="true" />{region(instruction.target)}</span>}
            {instruction.minister_id && <span><Users aria-hidden="true" />{minister(instruction.minister_id)}</span>}
            {(instruction.budget || 0) > 0 && <span><Coins aria-hidden="true" />{number(instruction.budget!)} {t('history.silverUnit')}</span>}
            {(instruction.grain || 0) > 0 && <span>{number(instruction.grain!)} {t('history.grainUnit')}</span>}
            {(instruction.quarters || 0) > 0 && <span><Clock3 aria-hidden="true" />{t('history.order.duration', { count: instruction.quarters })}</span>}
          </div>}
          {instruction.project_id && <p className="history-muted">{t('history.order.project')}: {(() => { const project = game.state?.projects.find((item) => item.id === instruction.project_id); return project ? `${region(project.target)} · ${kind(project.kind)}` : t('history.order.unknownProject') })()}</p>}
          {!!instruction.depends_on?.length && <p className="history-condition">{t('history.order.after', { items: instruction.depends_on.map((id) => { const at = instructions.findIndex((item) => item.id === id); if (at >= 0) return `${at + 1}. ${kind(instructions[at].kind)}`; const project = game.state?.projects.find((item) => item.id === id); return project ? `${region(project.target)} · ${kind(project.kind)}` : t('history.order.unknownProject') }).join(' · ') })}</p>}
          {(instruction.not_before_turn || 0) > (game.state?.turn || 0) && <p className="history-condition">{t('history.order.notBefore', { turn: instruction.not_before_turn! + 1 })}</p>}
          {instruction.reason && <p className="history-prose history-muted">{reason(instruction.reason)}</p>}
          {prepared.outcomes?.filter((outcome) => outcome.instruction_id === instruction.id && outcome.status !== 'ready').map((outcome, at) => <p className="history-condition" key={at}>{status(outcome.status)}{outcome.reason !== instruction.reason && <> · {reason(outcome.reason)}</>}</p>)}
        </li>)}
      </ol>
      <div className="history-actions"><Button className="history-primary" disabled={game.busy || (instructions.length === 0 && orders.length===0) || (orders.length>0 && prepared.outcomes?.some(o=>o.status!=="ready"))} onClick={() => void issue()}><Check />{t('history.edict.confirm')}</Button><Button variant="ghost" disabled={game.busy} onClick={game.discardPrepared}>{t('history.edict.edit')}</Button></div>
    </> : <>
      <label className="sr-only" htmlFor="history-edict-text">{t('history.edict.label')}</label>
      <Textarea id="history-edict-text" className="history-edict-input" value={text} onChange={(event) => onChange(event.target.value)} placeholder={t('history.edict.placeholder')} disabled={game.busy} maxLength={12000} />
      <div className="history-actions"><Button className="history-primary" disabled={game.busy || !text.trim()} onClick={() => void game.act('prepare-edict', { text: text.trim() })}><ScrollText />{t('history.edict.prepare')}</Button>{game.state?.world && <Button variant="outline" disabled={game.busy || !text.trim()} onClick={()=>void game.act('direct-edict',{text:text.trim()}).then(ok=>{if(ok)onChange('')})}>{t('history.world.direct')}</Button>}</div>
    </>}
  </section>{game.state?.world && <HistoryEdictReference game={game} draft={text} onChange={onChange} />}</div>
}
