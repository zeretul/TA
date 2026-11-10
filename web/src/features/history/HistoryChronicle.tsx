import { useState } from 'react'
import { BookOpenText, Award } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { useHistoryLabels } from './presentation'
import type { HistoryState } from './types'

export function HistoryChronicle({ state }: { state: HistoryState }) {
  const { t, text, season } = useHistoryLabels(state)
  const [query, setQuery] = useState('')
  const achieved = state.scenario!.objectives.filter(goal => state.world!.achieved.includes(goal.id))
  const reports = state.reports.filter(report => !report.discarded).filter(report => !query || `${report.narrative} ${state.edicts.filter(edict => edict.turn === report.turn).map(edict => edict.plan.text).join(' ')}`.toLocaleLowerCase().includes(query.toLocaleLowerCase()))
  return <section className="history-manuscript history-chronicle">
    <div className="history-section-heading"><BookOpenText /><h2>{t('history.port.chronicle')}</h2></div>
    <p className="history-muted">{t('history.port.chronicleHint')}</p>
    <div className="history-achievements"><Award />{achieved.length ? achieved.map(goal => <span className="history-tag" key={goal.id}>{text(goal.name)}</span>) : <span className="history-muted">{t('history.port.noAchievement')}</span>}</div>
    <Input aria-label={t('history.port.searchChronicle')} placeholder={t('history.port.searchChronicle')} value={query} onChange={event => setQuery(event.target.value)} />
    {!reports.length && <p className="history-empty-inline">{t('history.port.noChronicle')}</p>}
    {[...reports].reverse().map(report => <details className="history-details history-chronicle-season" key={report.id}><summary><strong>{season(report.year - state.scenario!.calendar.era_offset, report.quarter)}</strong><span>{t('history.port.chronicleSeason', { count: state.edicts.filter(e => e.turn === report.turn).length })}</span></summary>
      {state.edicts.filter(edict => edict.turn === report.turn).map(edict => <blockquote className="history-prose" key={edict.id}>{edict.plan.text}</blockquote>)}
      <p className="history-prose">{report.narrative}</p>
      {report.world?.events.map((event,index) => <article className="history-event" key={`${event.id || event.title}-${index}`}><strong>{event.title}</strong><p>{event.text}</p></article>)}
    </details>)}
  </section>
}
