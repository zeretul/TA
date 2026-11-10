import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ProvenanceDetails } from './ProvenanceDetails'
import type { KnownPerson, KnownRelation } from './known-people'

interface RelationshipProps {
  people: KnownPerson[]
  relations: KnownRelation[]
  onOpenPerson: (id: string) => void
}

export function RelationshipList({ people, relations, onOpenPerson }: RelationshipProps) {
  const { t } = useTranslation()
  const names = new Map(people.map(person => [person.id, person.name]))
  return <div className="flex min-w-0 flex-col gap-2" data-testid="relationship-list">
    {relations.length === 0 ? <p className="text-xs text-muted-foreground">{t('loreOverview.relationships.empty')}</p> : relations.map(relation => (
      <div key={relation.id} className="min-w-0 rounded border border-[var(--nova-border)] p-2 text-xs">
        <div className="flex flex-wrap items-center gap-1">
          <button type="button" className="break-all text-[var(--nova-accent)] hover:underline" onClick={() => onOpenPerson(relation.sourceId)}>{names.get(relation.sourceId)}</button>
          <span aria-hidden>{relation.undirected ? '·' : relation.mutual ? '↔' : '→'}</span>
          <button type="button" className="break-all text-[var(--nova-accent)] hover:underline" onClick={() => onOpenPerson(relation.targetId)}>{names.get(relation.targetId)}</button>
          <span className="break-words text-muted-foreground">{relation.labelKey ? t(relation.labelKey) : relation.label}</span>
        </div>
        <ProvenanceDetails value={relation.provenance} />
        {relation.statement ? <p className="mt-1 whitespace-pre-wrap break-words text-muted-foreground">{relation.statement}</p> : null}
      </div>
    ))}
  </div>
}

/** A bounded ego graph keeps long-lived casts readable. Every edge remains
 * available in the list; paging changes only the drawing, never the facts. */
export function RelationshipView({ people, relations, onOpenPerson }: RelationshipProps) {
  const { t } = useTranslation()
  const marker = useId().replace(/:/g, '')
  const [mode, setMode] = useState<'graph' | 'list'>('graph')
  const [selected, setSelected] = useState('protagonist')
  const [page, setPage] = useState(0)
  const center = people.find(person => person.id === selected) || people[0]
  const related = center ? relations.filter(relation => relation.sourceId === center.id || relation.targetId === center.id) : []
  const neighborIds = [...new Set(related.map(relation => relation.sourceId === center?.id ? relation.targetId : relation.sourceId))]
  const pages = Math.max(1, Math.ceil(neighborIds.length / 6))
  const currentPage = Math.min(page, pages - 1)
  const neighbors = neighborIds.slice(currentPage * 6, currentPage * 6 + 6)
    .map(id => people.find(person => person.id === id)).filter((person): person is KnownPerson => Boolean(person))
  const positions = new Map(neighbors.map((person, index) => {
    const angle = -Math.PI / 2 + index * 2 * Math.PI / Math.max(neighbors.length, 1)
    return [person.id, { x: 200 + 140 * Math.cos(angle), y: 220 + 150 * Math.sin(angle) }]
  }))
  if (center) positions.set(center.id, { x: 200, y: 220 })
  return <section className="flex min-w-0 flex-col gap-2" aria-label={t('loreOverview.relationships.title')}>
    <div className="flex flex-wrap gap-1">
      <Button size="sm" variant={mode === 'graph' ? 'secondary' : 'ghost'} aria-pressed={mode === 'graph'} onClick={() => setMode('graph')}>{t('loreOverview.relationships.graph')}</Button>
      <Button size="sm" variant={mode === 'list' ? 'secondary' : 'ghost'} aria-pressed={mode === 'list'} onClick={() => setMode('list')}>{t('loreOverview.relationships.list')}</Button>
    </div>
    <p className="text-[11px] text-muted-foreground">{t('loreOverview.relationships.hint')} {t('loreOverview.relationships.uncertainHint')}</p>
    {mode === 'list' ? <RelationshipList people={people} relations={relations} onOpenPerson={onOpenPerson} /> : <>
      {center ? <label className="flex min-w-0 items-center gap-2 text-xs">
        <span className="shrink-0">{t('loreOverview.relationships.center')}</span>
        <select className="min-w-0 flex-1 rounded border bg-[var(--nova-surface)] px-2 py-1" value={center.id} onChange={event => { setSelected(event.target.value); setPage(0) }}>
          {people.map(person => <option key={person.id} value={person.id}>{person.name}</option>)}
        </select>
      </label> : null}
      {center && related.length ? <>
        <svg viewBox="0 0 400 440" className="w-full min-w-0 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)]" role="group" aria-label={t('loreOverview.relationships.graphLabel', { name: center.name })}>
          <defs><marker id={marker} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="currentColor" /></marker></defs>
          {related.filter(relation => positions.has(relation.sourceId) && positions.has(relation.targetId)).map(relation => {
            const source = positions.get(relation.sourceId)!, target = positions.get(relation.targetId)!
            const dx = target.x - source.x, dy = target.y - source.y, length = Math.hypot(dx, dy)
            const label = relation.labelKey ? t(relation.labelKey) : relation.label
            const reciprocal = related.some(other => other.sourceId === relation.targetId && other.targetId === relation.sourceId)
            const offsetX = reciprocal ? -dy / length * 12 : 0, offsetY = reciprocal ? dx / length * 12 : 0
            return <g key={relation.id} className="text-[var(--nova-text-muted)]">
              <title>{label} · {relation.provenance?.confidence ? t(`loreOverview.provenance.confidence.${relation.provenance.confidence}`) : t('loreOverview.notRecorded')}</title>
              <line x1={source.x + dx / length * 32 + offsetX} y1={source.y + dy / length * 32 + offsetY} x2={target.x - dx / length * 32 + offsetX} y2={target.y - dy / length * 32 + offsetY} stroke="currentColor" strokeDasharray={relation.provenance?.confidence === 'confirmed' ? undefined : '5 4'} markerEnd={relation.undirected ? undefined : `url(#${marker})`} markerStart={relation.mutual ? `url(#${marker})` : undefined} />
              <text x={(source.x + target.x) / 2 + offsetX * 2} y={(source.y + target.y) / 2 - 9 + offsetY * 2} textAnchor="middle" fontSize="13" fill="currentColor">{short(label, 12)}</text>
            </g>
          })}
          {[center, ...neighbors].map(person => {
            const position = positions.get(person.id)!
            return <g key={person.id} transform={`translate(${position.x} ${position.y})`} role="button" tabIndex={0} aria-label={t('loreOverview.relationships.openPerson', { name: person.name })} className="cursor-pointer text-[var(--nova-text)] focus:outline-[var(--nova-accent)]" onClick={() => onOpenPerson(person.id)} onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); onOpenPerson(person.id) } }}>
              <title>{person.name}</title>
              <circle r="30" fill="var(--nova-surface)" stroke={person.id === center.id ? 'var(--nova-accent)' : 'var(--nova-border)'} strokeWidth="2" />
              <text textAnchor="middle" dy="5" fontSize="15" fill="currentColor">{short(person.name, 3)}</text>
              <text textAnchor="middle" y="49" fontSize="14" fill="currentColor">{short(person.name, 8)}</text>
            </g>
          })}
        </svg>
        {pages > 1 ? <div className="flex items-center justify-center gap-2 text-xs">
          <Button size="sm" variant="ghost" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>{t('loreOverview.relationships.previous')}</Button>
          <span>{currentPage + 1} / {pages}</span>
          <Button size="sm" variant="ghost" disabled={currentPage + 1 === pages} onClick={() => setPage(currentPage + 1)}>{t('loreOverview.relationships.next')}</Button>
        </div> : null}
        <RelationshipList people={people} relations={related.filter(relation => neighbors.some(person => person.id === relation.sourceId || person.id === relation.targetId))} onOpenPerson={onOpenPerson} />
      </> : <p className="text-xs text-muted-foreground">{t('loreOverview.relationships.empty')}</p>}
    </>}
  </section>
}

function short(text: string, limit: number) {
  const characters = Array.from(text)
  return characters.length > limit ? `${characters.slice(0, limit).join('')}…` : text
}
