import { HistoryOpenEvents } from './HistoryOpenEvents'
import { reportOrderText } from './report-orders'
import { useState } from 'react'
import { ArrowRight, LoaderCircle, Coins, MessageCircle, ScrollText, Map, BookOpenText, ListChecks } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { HistoryMap } from './HistoryMap'
import { HistoryPortrait } from './HistoryPortrait'
import { HistoryChronicle } from './HistoryChronicle'
import { HistoryReportAccounts } from './HistoryReportAccounts'
import { HistoryCourt } from './HistoryCourt'
import { HistoryEdict } from './HistoryEdict'
import { useHistoryLabels } from './presentation'
import { useHistoryDraft } from './use-history-draft'
import type { HistoryGameController } from './use-history-game'
import type { HistoryState, WorldEntity, WorldOrder } from './types'

type Tab = 'overview' | 'edict' | 'court' | 'orders' | 'records' | 'chronicle'
export function HistoryWorldDesk({ game }: { game: HistoryGameController }) {
  const state = game.state!
  const world = state.world!
  const pack = state.scenario!
  const { t, text, number, season, minister } = useHistoryLabels(state)
  const [tab, setTab] = useState<Tab>('overview')
  const [edict, setEdict] = useHistoryDraft(`${game.bootstrap!.project_id}:${state.id}`, 'edict')
  const [advanceOpen, setAdvanceOpen] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const [courtSeed, setCourtSeed] = useState<{ id: number; ministerId?: string; message: string; reportId?: string }>()
  const [selectedReport, setSelectedReport] = useState('')
  const reports = state.reports.filter(item => !item.discarded)
  const report = reports.find(item => item.id === selectedReport) || reports.at(-1)
  const ask = (ministerId?: string, reportId?: string, message?: string) => {
    setCourtSeed({ id: Date.now(), ministerId, reportId, message: message || t(reportId ? 'history.world.askReport' : 'history.world.askSituation') })
    setTab('court')
  }
  const draft = (value: string) => { game.discardPrepared(); setEdict(edict.trim() ? `${edict.trim()}\n\n${value}` : value); setTab('edict') }
  const amend = (order: WorldOrder) => draft(t('history.world.reviseText', { text: order.text, turn: order.created_turn + 1, version: order.version }))
  const action = async (order: WorldOrder, value: string) => {
    await game.act('edict', { plan: { text: t(`history.world.action.${value}`), instructions: [], orders: [{ action: value, order_id: order.id }] } })
  }
  return <>
    <section className="history-desk-heading history-illustrated-heading" style={pack.presentation?.backdrop ? { backgroundImage: `linear-gradient(90deg, var(--history-paper), transparent 95%), url("${pack.presentation.backdrop}")` } : undefined}><div><p className="history-eyebrow">{text(pack.name)}</p><h1>{state.name}</h1><p className="history-muted">{t('history.world.subtitle')}</p></div><div className="history-calendar"><span>{text(pack.calendar.name)} · {season(state.year - pack.calendar.era_offset, state.quarter)}</span><small>{t('history.turn', { turn: state.turn + 1 })}</small></div></section>
    <dl className="history-resources history-world-resources">{world.accounts.filter(account => account.owner === (pack.player_entity_id || world.accounts[0]?.owner)).map(account => <div key={account.id}><dt><Coins />{text(account.name)}</dt><dd>{number(account.balance)} <small>{text(pack.resources.find(item => item.id === account.resource)?.unit)}</small></dd></div>)}</dl>
    {world.ending && <div role="status" className="history-current-issue"><strong>{t(`history.world.ending.${world.ending}`)}</strong><p>{t('history.world.endingHint')}</p></div>}
    <div className="history-desk-nav"><div className="history-tabs history-world-tabs" role="tablist" aria-label={t('history.title')}>
      {([{ id: 'overview', icon: Map }, { id: 'edict', icon: ScrollText }, { id: 'court', icon: MessageCircle }, { id: 'orders', icon: ListChecks }, { id: 'records', icon: BookOpenText }, { id: 'chronicle', icon: BookOpenText }] as const).map(({ id, icon: Icon }) => <button type="button" role="tab" key={id} aria-selected={tab === id} aria-controls={`history-world-${id}`} onClick={() => setTab(id)}><Icon />{t(`history.world.tab.${id}`)}</button>)}
    </div><Button disabled={game.busy || !!world.ending} className="history-advance" onClick={() => setAdvanceOpen(true)}>{t('history.advance')}<ArrowRight /></Button></div>
    <div id="history-world-overview" role="tabpanel" hidden={tab !== 'overview'}>
      <HistoryMap state={state} onDraft={draft} onAsk={(id,message) => ask(id,undefined,message)} />
      <div className="history-world-summary"><HistoryOpenEvents state={state} onDraft={draft} />
      <section className="history-manuscript"><h2>{t('history.world.objectives')}</h2>{pack.objectives.map(goal => <p className="history-objective" key={goal.id}><span aria-hidden="true">{world.achieved?.includes(goal.id) ? '✓' : '○'}</span>{goal.ending === 'defeat' ? `${t('history.world.danger')}: ` : ''}{text(goal.name)}<small>{number(world.entities.find(e => e.id === goal.entity_id)?.metrics[goal.metric] || 0)} · {goal.operator === 'lte' ? '≤' : '≥'} {number(goal.value)}</small></p>)}<p className="history-muted">{text(pack.description)}</p></section></div>
      <WorldAtlas state={state} onAsk={ask} onDraft={draft} />
    </div>
    <div id="history-world-edict" role="tabpanel" hidden={tab !== 'edict'}><HistoryEdict game={game} text={edict} onChange={setEdict} /></div>
    <div id="history-world-court" role="tabpanel" hidden={tab !== 'court'}><HistoryCourt game={game} seed={courtSeed} onDraft={chat => { game.discardPrepared(); setTab('edict'); void game.act('prepare-edict', { source_chat_id: chat.id, text: t('history.court.draftInstruction') }) }} /></div>
    <div id="history-world-orders" role="tabpanel" hidden={tab !== 'orders'}><section className="history-manuscript"><h2>{t('history.world.ordersTitle')}</h2><p className="history-muted">{t('history.world.ordersHint')}</p>{!world.orders.length && <p className="history-empty-inline">{t('history.world.noOrders')}</p>}
      {[...world.orders].reverse().map(order => <article className="history-world-order" key={order.id}><div className="history-order-title"><span className="history-tag">{t(`history.world.order.${order.kind}`)}</span>{order.disclosure === 'sealed' && <span className="history-tag">{t('history.port.sealed')}</span>}<strong>{t(`history.world.status.${order.status}`)}</strong><small>{t('history.world.revision', { version: order.version })}</small></div><p className="history-prose">{order.text}</p><p className="history-muted">{minister(order.minister_id)} · {t('history.world.progress', { value: order.progress })}</p>{order.condition && <p className="history-condition">{order.condition}</p>}<p className="history-prose">{order.last_result}</p>
        <dl className="history-world-costs">{pack.resources.filter(r => order.spent?.[r.id] || order.limits?.[r.id] !== undefined).map(r => <div key={r.id}><dt>{text(r.name)}</dt><dd>{number(order.spent?.[r.id] || 0)} / {order.limits?.[r.id] !== undefined ? number(order.limits[r.id]) : t('history.world.delegated')} {text(r.unit)}</dd></div>)}</dl>
        {!['completed', 'revoked'].includes(order.status) && <div className="history-actions"><Button variant="outline" size="sm" disabled={game.busy} onClick={() => void action(order, order.status === 'paused' ? 'resume' : 'pause')}>{t(`history.world.action.${order.status === 'paused' ? 'resume' : 'pause'}`)}</Button><Button variant="outline" size="sm" disabled={game.busy} onClick={() => amend(order)}>{t('history.world.action.revise')}</Button><Button variant="ghost" size="sm" disabled={game.busy} onClick={() => void action(order, 'revoke')}>{t('history.world.action.revoke')}</Button><Button variant="ghost" size="sm" disabled={game.busy} onClick={() => ask(order.minister_id)}>{t('history.world.askOwner')}</Button></div>}</article>)}
    </section></div>
    <div id="history-world-records" role="tabpanel" hidden={tab !== 'records'}><section className="history-manuscript"><h2>{t('history.world.reportsTitle')}</h2>{!report ? <p className="history-empty-inline">{t('history.world.noReports')}</p> : <><label className="history-muted" htmlFor="history-world-report">{t('history.world.chooseReport')}</label><select id="history-world-report" className="history-world-select" value={report.id} onChange={e => setSelectedReport(e.target.value)}>{[...reports].reverse().map(r => <option key={r.id} value={r.id}>{season(r.year - pack.calendar.era_offset, r.quarter)}</option>)}</select><HistoryReportAccounts state={state} reportID={report.id} /><p className="history-prose history-world-report">{report.narrative}</p><div className="history-actions"><Button variant="outline" disabled={game.busy} onClick={() => ask(undefined, report.id)}>{t('history.world.discussReport')}</Button><Button variant="outline" disabled={game.busy} onClick={() => draft(t('history.world.reportDraft', { turn: report.turn + 1 }))}>{t('history.world.adjustOrders')}</Button><Button variant="ghost" onClick={() => setTab('orders')}>{t('history.world.tab.orders')}</Button></div>
      <details className="history-details"><summary>{t('history.world.ledger')}</summary><div className="history-ledger">{report.world?.transfers?.map((entry, index) => <article key={index}><strong>{number(entry.amount)} {text(pack.resources.find(r => r.id === entry.resource)?.unit)}</strong><p>{entry.from ? text(world.accounts.find(a => a.id === entry.from)?.name) : t(entry.counterparty_hidden ? 'history.port.undisclosedAccount' : 'history.world.income')} → {entry.to ? text(world.accounts.find(a => a.id === entry.to)?.name) : t(entry.counterparty_hidden ? 'history.port.undisclosedAccount' : 'history.world.expense')}</p><p className="history-muted">{entry.reason}</p></article>)}</div></details>
      <details className="history-details"><summary>{t('history.world.results')}</summary>{report.world?.results?.map(result => <article key={result.order_id} className="history-event"><strong>{t(`history.world.status.${result.status}`)}</strong><p>{reportOrderText(state, report.turn, result.order_id) || t('history.review.unavailableOrder')}</p><p>{result.reason}</p></article>)}</details></>}</section></div>
    <div id="history-world-chronicle" role="tabpanel" hidden={tab !== 'chronicle'}><HistoryChronicle state={state} /></div>
    <p className="history-autosave"><span aria-hidden="true" />{t('history.autoSaved')}</p>
    <Dialog open={advanceOpen} onOpenChange={value => { if (!game.busy) setAdvanceOpen(value) }}><DialogContent><DialogHeader><DialogTitle>{t('history.advance.title')}</DialogTitle><DialogDescription>{t('history.world.advanceHint')}</DialogDescription></DialogHeader>{game.failure && <p role="alert" className="text-sm text-destructive">{t(game.failure.key)}</p>}{(edict.trim() || game.prepared) && <p className="history-condition">{t('history.advance.draftWarning')}</p>}<div className="history-actions"><Button variant="ghost" disabled={game.busy} onClick={() => setAdvanceOpen(false)}>{t('history.back')}</Button><Button disabled={game.busy} onClick={() => void game.act('advance').then(ok => { if (ok) { setAdvanceOpen(false); setSelectedReport(''); setTab('records') } })}>{game.pending === 'advance' && <LoaderCircle className="animate-spin" />}{t('history.advance.confirm')}</Button></div>{game.pending === 'advance' && <Button variant="outline" disabled={cancelling} onClick={() => { setCancelling(true); void game.cancel().then(() => setAdvanceOpen(false)).finally(() => setCancelling(false)) }}>{t('history.cancel')}</Button>}</DialogContent></Dialog>
  </>
}

function WorldAtlas({ state, onAsk, onDraft }: { state: HistoryState; onAsk: (ministerId?: string) => void; onDraft: (text: string) => void }) {
  const { t, text, number } = useHistoryLabels(state)
  const [kind, setKind] = useState('region')
  const [query, setQuery] = useState('')
  const [page,setPage]=useState(0)
  const world = state.world!
  const kinds = [...new Set(world.entities.map(item => item.kind))]
  const entities = world.entities.filter(item => item.kind === kind && text(item.name).toLocaleLowerCase().includes(query.toLocaleLowerCase()))
  const currentPage = Math.min(page, Math.max(0, Math.ceil(entities.length / 60) - 1))
  const name = (id?: string) => text(world.entities.find(item => item.id === id)?.name)
  const details = (entity: WorldEntity) => <><dl className="history-world-metrics">{Object.entries(entity.metrics || {}).map(([key, value]) => <div key={key}><dt>{text(state.scenario!.metrics.find(m => m.id === key)?.name)}</dt><dd>{number(value)}</dd></div>)}</dl>{entity.location && <p className="history-muted">{t('history.world.location')}: {name(entity.location)}</p>}{entity.owner && <p className="history-muted">{t('history.world.owner')}: {name(entity.owner)}</p>}{world.accounts.filter(account => account.owner === entity.id).map(account => <p className="history-muted" key={account.id}>{text(account.name)} · {number(account.balance)} {text(state.scenario!.resources.find(resource => resource.id === account.resource)?.unit)}</p>)}{Object.entries(entity.attributes || {}).map(([key, value]) => <p key={key} className="history-prose history-muted">{text(value)}</p>)}<div className="history-actions">{state.ministers.some(m => m.id === entity.id) && <Button variant="outline" size="sm" onClick={() => onAsk(entity.id)}>{t('history.world.summon')}</Button>}<Button variant="ghost" size="sm" onClick={() => onDraft(t('history.world.entityDraft', { name: text(entity.name) }))}>{t('history.world.handle')}</Button></div></>
  return <section className="history-atlas"><div className="history-section-heading"><Map /><h2>{t('history.world.atlas')}</h2></div><div className="history-atlas-controls"><label className="sr-only" htmlFor="history-entity-kind">{t('history.world.category')}</label><select id="history-entity-kind" value={kind} className="history-world-select" onChange={event => {setKind(event.target.value);setPage(0)}}>{kinds.map(value => <option key={value} value={value}>{t(`history.world.entity.${value}`, { defaultValue: t('history.world.entity.other') })}</option>)}</select><Input aria-label={t('history.world.search')} placeholder={t('history.world.search')} value={query} onChange={event => {setQuery(event.target.value);setPage(0)}} /></div><div className="history-atlas-grid">{entities.slice(currentPage*60,(currentPage+1)*60).map(entity => <details className={`history-territory history-territory-${entity.kind}`} key={entity.id}><summary>{entity.kind === 'person' && <HistoryPortrait state={state} id={entity.id} name={text(entity.name)} />}<strong>{text(entity.name)}</strong><small>{entity.kind === 'region' ? `${t('history.world.publicSupport')} ${number(entity.metrics.heart || 0)}` : entity.kind === 'technology' ? `${number(entity.metrics.progress || 0)}%` : text(entity.attributes?.office)}</small></summary>{details(entity)}</details>)}</div>{entities.length>60&&<div className="history-actions"><Button variant="outline" disabled={currentPage===0} onClick={()=>setPage(currentPage-1)}>{t('history.bundle.previous')}</Button><span>{t('history.bundle.page',{page:currentPage+1,pages:Math.ceil(entities.length/60),count:entities.length})}</span><Button variant="outline" disabled={(currentPage+1)*60>=entities.length} onClick={()=>setPage(currentPage+1)}>{t('history.bundle.next')}</Button></div>}{!entities.length && <p className="history-empty-inline">{t('history.world.noMatches')}</p>}</section>
}
