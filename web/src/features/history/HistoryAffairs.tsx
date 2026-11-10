import { ChevronDown, Landmark } from 'lucide-react'
import type { HistoryState } from './types'
import { useHistoryLabels } from './presentation'

export function HistoryAffairs({ state }: { state: HistoryState }) {
  const { t, number, region, minister, kind, status, reason } = useHistoryLabels(state)
  const projects = state.projects || []
  return <aside className="history-affairs" aria-label={t('history.projects')}>
    <div className="history-section-heading"><Landmark aria-hidden="true" /><h2>{t('history.projects')}</h2><span className="history-count">{projects.filter((project) => project.status !== 'completed').length}</span></div>
    {projects.length === 0 ? <p className="history-empty-inline">{t('history.projects.empty')}</p> : <div className="history-projects">{[...projects].reverse().map((project) => <details key={project.id} className="history-project" open={project.status === 'blocked'}>
      <summary><span className="history-project-heading"><span>{region(project.target)} · {kind(project.kind)}</span><span className={`history-status history-status-${project.status}`}>{status(project.status)}</span></span><ChevronDown className="history-disclosure" aria-hidden="true" /></summary>
      <div className="history-project-detail">
        <p>{minister(project.minister_id)}</p>
        <progress aria-label={t('history.projects.progress', { progress: project.progress, required: project.required })} value={project.progress} max={Math.max(1, project.required)} />
        <p className="history-muted">{t('history.projects.progress', { progress: project.progress, required: project.required })} · {t(project.reserved ? 'history.projects.funding' : 'history.projects.unfunded')}</p>
        <p className="history-muted">{t('history.projects.cost', { spent: number(project.spent), budget: number(project.budget) })}</p>
        {project.reason && <p className="history-condition">{reason(project.reason)}</p>}
        <code className="history-record-id">{project.id}</code>
      </div>
    </details>)}</div>}
    <details className="history-details history-region-details">
      <summary>{t('history.regions')}<ChevronDown className="history-disclosure" aria-hidden="true" /></summary>
      {(state.regions || []).map((item) => <section key={item.id} className="history-region"><h3>{region(item.id)}</h3><dl>{(['heart', 'disaster', 'refugees', 'irrigation', 'training'] as const).map((field) => <div key={field}><dt>{t(`history.region.${field}`)}</dt><dd>{number(item[field])}</dd></div>)}</dl></section>)}
    </details>
  </aside>
}
