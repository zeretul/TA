import { useState } from 'react'
import { Landmark, Plus, FolderClock, ArrowRight, LoaderCircle, X, RotateCw, ScrollText, Wheat, Coins, MessageCircle, BookOpenText } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { useHistoryGame, type HistoryGameController } from './use-history-game'
import { useHistoryDraft } from './use-history-draft'
import { useHistoryLabels } from './presentation'
import { HistoryEdict } from './HistoryEdict'
import { HistoryAffairs } from './HistoryAffairs'
import { HistoryCourt } from './HistoryCourt'
import { HistoryRecords, HistoryOutcomes } from './HistoryRecords'
import { HistoryNewGame, HistorySaves } from './HistorySaves'
import './history.css'
import './history-port.css'
import { HistoryWorldDesk } from './HistoryWorldDesk'

export function HistoryGame({ active }: { active: boolean }) {
  const game = useHistoryGame(active)
  const { t } = useHistoryLabels()
  const [newOpen, setNewOpen] = useState(false)
  const [savesOpen, setSavesOpen] = useState(false)
  return <div className="history-game" data-testid="history-game">
    <header className="history-toolbar"><div><Landmark aria-hidden="true" /><span>{t('history.title')}</span><span className="history-candidate">{t('history.candidate')}</span></div><div className="history-toolbar-actions"><Button variant="ghost" size="sm" disabled={game.busy || !game.bootstrap} onClick={() => setNewOpen(true)}><Plus />{t('history.newGame')}</Button><Button variant="ghost" size="sm" disabled={game.busy || !game.bootstrap} onClick={() => setSavesOpen(true)}><FolderClock />{t('history.saves')}</Button></div></header>
{game.failure && <div role="alert" className="history-feedback history-feedback-error history-global-feedback"><p>{t(game.failure.key)}</p><div><Button variant="outline" size="sm" disabled={game.busy} onClick={() => void game.retry()}><RotateCw />{t('history.retry')}</Button>{game.state && <Button variant="ghost" size="sm" disabled={game.busy} onClick={() => void game.refresh()}>{t('history.refresh')}</Button>}</div></div>}
    <div className="history-scroll">
      <div className="history-content">
        {game.bootstrap?.campaigns.some((item) => item.unavailable) && <div className="history-feedback" role="status"><p>{t('history.save.unavailableNotice')}</p><Button variant="outline" size="sm" onClick={() => setSavesOpen(true)}>{t('history.saves')}</Button></div>}
        {game.notice && <div role="status" className="history-feedback"><p>{t(game.notice)}</p><Button variant="ghost" size="icon-sm" aria-label={t('history.dismiss')} onClick={game.clearNotice}><X /></Button></div>}
        {game.state ? game.state.world ? <HistoryWorldDesk key={`${game.bootstrap!.project_id}:${game.state.id}`} game={game} /> : <HistoryDesk key={`${game.bootstrap!.project_id}:${game.state.id}`} game={game} /> : game.busy ? <div className="history-loading" role="status"><LoaderCircle className="animate-spin" />{t('history.loading')}</div> : <section className="history-opening"><div className="history-opening-seal" aria-hidden="true"><Landmark /></div><p className="history-eyebrow">{t('history.scenario')}</p><h1>{t('history.startTitle')}</h1><p className="history-opening-text">{t('history.startText')}</p><Button className="history-primary" size="lg" disabled={!game.bootstrap} onClick={() => setNewOpen(true)}>{t('history.start')}<ArrowRight /></Button><p className="history-opening-note">{t('history.scenarioNote')}</p></section>}
      </div>
    </div>
    {game.pending && <div className="history-pending" role="status" aria-live="polite"><LoaderCircle className="animate-spin" /><span>{t(`history.busy.${game.pending}`)}</span>{game.canCancel && <Button variant="outline" size="sm" onClick={() => void game.cancel()}>{t('history.cancel')}</Button>}</div>}
    <HistoryNewGame game={game} open={newOpen} onOpenChange={setNewOpen} />
    <HistorySaves game={game} open={savesOpen} onOpenChange={setSavesOpen} />
  </div>
}

function HistoryDesk({ game }: { game: HistoryGameController }) {
  const state = game.state!
  const { t, number, season, region } = useHistoryLabels(state)
  const [tab, setTab] = useState<'edict' | 'court' | 'records'>('edict')
  const [edict, setEdict] = useHistoryDraft(`${game.bootstrap!.project_id}:${state.id}`, 'edict')
  const [advanceOpen, setAdvanceOpen] = useState(false)
  const issue = [...state.regions].sort((left, right) => right.disaster - left.disaster)[0]
  const latestEdict = state.edicts?.at(-1)
  const advance = async () => { if (await game.act('advance')) { setAdvanceOpen(false); setTab('records') } }
  return <>
    <section className="history-desk-heading"><div><p className="history-eyebrow">{t('history.scenario')}</p><h1>{state.name}</h1><p className="history-muted">{t('history.subtitle')}</p></div><div className="history-calendar"><span>{season(state.year, state.quarter)}</span><small>{t('history.turn', { turn: state.turn + 1 })}</small></div></section>
    <dl className="history-resources">
      <div><dt><Coins aria-hidden="true" />{t('history.treasury')}</dt><dd>{number(state.treasury)} <small>{t('history.silverUnit')}</small></dd></div>
      <div title={t('history.privyHint')}><dt><Landmark aria-hidden="true" />{t('history.privy')}</dt><dd>{number(state.privy_purse)} <small>{t('history.silverUnit')}</small></dd></div>
      <div><dt><Wheat aria-hidden="true" />{t('history.grain')}</dt><dd>{number(state.grain)} <small>{t('history.grainUnit')}</small></dd></div>
    </dl>
    <div className="history-current-issue"><ScrollText aria-hidden="true" /><div><strong>{t('history.currentIssue')}</strong><p>{issue?.disaster > 0 ? t('history.issue', { region: region(issue.id), severity: issue.disaster, refugees: number(issue.refugees) }) : t('history.calm')}</p></div></div>
    <div className="history-desk-nav"><div className="history-tabs" role="tablist" aria-label={t('history.title')}>
      {([{ id: 'edict', icon: ScrollText }, { id: 'court', icon: MessageCircle }, { id: 'records', icon: BookOpenText }] as const).map(({ id, icon: Icon }) => <button type="button" key={id} role="tab" id={`history-tab-${id}`} aria-selected={tab === id} aria-controls={`history-panel-${id}`} onClick={() => setTab(id)}><Icon aria-hidden="true" />{t(`history.tab.${id}`)}</button>)}
    </div><Button className="history-advance" disabled={game.busy} onClick={() => setAdvanceOpen(true)}>{t('history.advance')}<ArrowRight /></Button></div>
    <div id="history-panel-edict" role="tabpanel" aria-labelledby="history-tab-edict" hidden={tab !== 'edict'}>
      <div className="history-desk-grid"><div><HistoryEdict game={game} text={edict} onChange={setEdict} />{latestEdict && !game.prepared && <details className="history-details history-latest-edict" open={game.notice === 'history.done.edict'}><summary>{t('history.edict.sent')}</summary><HistoryOutcomes state={state} outcomes={latestEdict.outcomes || []} /></details>}</div><HistoryAffairs state={state} /></div>
    </div>
    <div id="history-panel-court" role="tabpanel" aria-labelledby="history-tab-court" hidden={tab !== 'court'}><HistoryCourt game={game} onDraft={(chat) => { game.discardPrepared(); setTab('edict'); void game.act('prepare-edict', { source_chat_id: chat.id, text: t('history.court.draftInstruction') }) }} /></div>
    <div id="history-panel-records" role="tabpanel" aria-labelledby="history-tab-records" hidden={tab !== 'records'}><HistoryRecords game={game} /></div>
    <p className="history-autosave"><span aria-hidden="true" />{t('history.autoSaved')}</p>
    <Dialog open={advanceOpen} onOpenChange={(value) => { if (!game.busy) setAdvanceOpen(value) }}><DialogContent><DialogHeader><DialogTitle>{t('history.advance.title')}</DialogTitle><DialogDescription>{t('history.advance.hint')}</DialogDescription></DialogHeader>{game.failure && <p role="alert" className="text-sm text-destructive">{t(game.failure.key)}</p>}{(edict.trim() || game.prepared) && <p className="text-sm text-amber-700 dark:text-amber-300">{t('history.advance.draftWarning')}</p>}<div className="flex flex-wrap justify-end gap-2"><Button variant="ghost" disabled={game.busy} onClick={() => setAdvanceOpen(false)}>{t('history.back')}</Button><Button disabled={game.busy} onClick={() => void advance()}>{game.pending === 'advance' && <LoaderCircle className="animate-spin" />}{t('history.advance.confirm')}</Button></div>{game.pending === 'advance' && <Button variant="outline" onClick={() => void game.cancel().then(() => setAdvanceOpen(false))}>{t('history.cancel')}</Button>}</DialogContent></Dialog>
  </>
}
