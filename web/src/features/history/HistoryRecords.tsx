import { BookOpenText, SearchCheck, ChevronDown } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { HistoryGameController } from './use-history-game'
import type { HistoryOutcome, HistoryState } from './types'
import { useHistoryLabels } from './presentation'

export function HistoryOutcomes({ outcomes, state }: { outcomes: HistoryOutcome[]; state: HistoryState }) {
  const { status, reason, region, kind, t, number } = useHistoryLabels(state)
  return <ul className="history-outcomes">{outcomes.map((outcome, index) => {
    const project = state.projects?.find((item) => item.id === outcome.project_id)
    return <li key={`${outcome.instruction_id}-${outcome.project_id}-${index}`}><div><strong>{project ? `${region(project.target)} · ${kind(project.kind)}` : t('history.order.index', { index: index + 1 })}</strong><span className={`history-status history-status-${outcome.status}`}>{status(outcome.status)}</span></div><p>{reason(outcome.reason)}</p>{(outcome.spent || 0) > 0 && <small>{t('history.records.spent')}: {number(outcome.spent!)} {t('history.silverUnit')}</small>}</li>
  })}</ul>
}

export function HistoryRecords({ game }: { game: HistoryGameController }) {
  const state = game.state!
  const { t, season, number, region, kind } = useHistoryLabels(state)
  const reports = (state.reports || []).filter((report) => !report.discarded)
  const inspection = state.inspections?.filter((item) => !item.discarded).at(-1)
  const financeFacts = inspection && inspection.target === 'treasury' ? Object.entries(inspection.facts).filter(([key,value]) => key !== 'turn' && typeof value === 'number') : []
  return <section className="history-records" aria-labelledby="history-records-heading">
    <div className="history-section-heading"><BookOpenText aria-hidden="true" /><h2 id="history-records-heading">{t('history.records.title')}</h2><Button variant="outline" size="sm" disabled={game.busy} onClick={() => void game.act('inspect', { target: 'treasury' })}><SearchCheck />{t('history.inspect')}</Button></div>
    {inspection && financeFacts.length > 0 && <details className="history-details" open><summary>{t('history.inspect.result')} · {t('history.turn', { turn: inspection.turn + 1 })}</summary><dl className="history-accounting">{financeFacts.map(([key,value]) => <div key={key}><dt>{t(`history.inspect.${key}`, { defaultValue: key })}</dt><dd>{number(value as number)}</dd></div>)}</dl></details>}
    {reports.length === 0 && <p className="history-empty-inline">{t('history.records.empty')}</p>}
    {[...reports].reverse().map((report, index) => <details key={report.id} className="history-report" open={index === 0}>
      <summary><span>{t('history.records.report', { season: season(report.year, report.quarter) })}</span><ChevronDown className="history-disclosure" aria-hidden="true" /></summary>
      <p className="history-prose history-report-prose">{report.narrative || t('history.records.noNarrative')}</p>
      <dl className="history-accounting">{(['income','upkeep','reserved','spent','upkeep_arrears','closing_treasury'] as const).map((field) => <div key={field}><dt>{t(`history.records.${field === 'upkeep_arrears' ? 'arrears' : field === 'closing_treasury' ? 'close' : field}`)}</dt><dd>{number(report[field])} <small>{t('history.silverUnit')}</small></dd></div>)}</dl>
      <p className="history-muted">{t('history.records.financeHint')}</p>
      {!!report.events?.length && <div className="history-report-events"><h3>{t('history.records.events')}</h3>{report.events.map((event, at) => <p key={at}><strong>{region(event.region_id)}</strong> · {event.reason}</p>)}</div>}
      {!!report.applied_adjustments?.length && <div className="history-report-events"><h3>{t('history.records.execution')}</h3>{report.applied_adjustments.map((adjustment, at) => { const project = state.projects?.find((item) => item.id === adjustment.project_id); return <p key={at}><strong>{project ? `${region(project.target)} · ${kind(project.kind)}` : t('history.order.index', { index: at + 1 })}</strong> · {adjustment.delay ? t('history.status.delayed') : t('history.records.efficiency', { percent: adjustment.efficiency_pct })}<br />{adjustment.reason}</p> })}</div>}
      {!!report.outcomes?.length && <><h3>{t('history.records.details')}</h3><HistoryOutcomes outcomes={report.outcomes} state={state} /></>}
    </details>)}
    <details className="history-details"><summary>{t('history.edict.original')}<ChevronDown className="history-disclosure" aria-hidden="true" /></summary>{state.edicts?.length ? [...state.edicts].reverse().map((edict) => <article key={edict.id} className="history-edict-record"><small>{t('history.turn', { turn: edict.turn + 1 })}</small><p className="history-prose">{edict.plan.text}</p><HistoryOutcomes outcomes={edict.outcomes || []} state={state} /></article>) : <p className="history-muted">{t('history.edict.empty')}</p>}</details>
  </section>
}
