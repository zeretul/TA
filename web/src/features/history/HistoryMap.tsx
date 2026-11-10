import { useEffect, useRef, useState } from 'react'
import { Flag, Map as MapIcon, ScrollText, MessageCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { resolveMapRegion } from './world-map'
import { useHistoryLabels } from './presentation'
import type { HistoryState } from './types'

export function HistoryMap({ state, onDraft, onAsk }: { state: HistoryState; onDraft: (text: string) => void; onAsk: (minister?: string, message?: string) => void }) {
  const { t, text, number } = useHistoryLabels(state)
  const [selected, setSelected] = useState('')
  const [layer, setLayer] = useState<'heart' | 'disaster' | 'military' | 'owner'>('disaster')
  const [destination, setDestination] = useState('')
  const [zoom, setZoom] = useState(1)
  const canvas = useRef<HTMLDivElement>(null)
  const presentation = state.scenario?.presentation
  const world = state.world!
  const regions = presentation?.regions?.filter(region => world.entities.some(entity => entity.id === region.entity_id)) || []
  const focus = regions.find(region => region.entity_id === selected) || regions[0]
  useEffect(() => {
    if (canvas.current && focus && presentation) canvas.current.scrollLeft = ((focus.center[0] - presentation.view_box[0]) / presentation.view_box[2]) * canvas.current.scrollWidth - canvas.current.clientWidth / 2
  }, [selected, zoom, presentation])
  if (!presentation || !focus) return null
  const entity = world.entities.find(item => item.id === focus.entity_id)!
  const entityIndex = new Map(world.entities.map(item => [item.id,item]))
  const regionIDs = new Set(regions.map(region => region.entity_id))
  const regionOf = (item: typeof entity) => resolveMapRegion(item.location,entityIndex,regionIDs)
  const armies = world.entities.filter(item => item.kind === 'army' && regionOf(item) === entity.id && item.status !== 'dead')
  const width = presentation.view_box[2]
  const byID = (id: string) => world.entities.find(item => item.id === id)
  const color = (id: string) => {
    const item = byID(id)!
    if (layer === 'owner') return item.owner === (state.scenario?.player_entity_id || world.accounts[0]?.owner) ? 'var(--history-map-calm)' : 'var(--history-map-warning)'
    if (layer === 'military') return world.entities.some(e => e.kind === 'army' && regionOf(e) === id && e.status !== 'dead') ? 'var(--history-map-army)' : 'var(--history-map-calm)'
    const danger = layer === 'heart' ? 100 - (item.metrics.heart ?? 50) : item.metrics.disaster ?? 0
    return danger >= 60 ? 'var(--history-map-danger)' : danger >= 30 ? 'var(--history-map-warning)' : 'var(--history-map-calm)'
  }
  return <section className="history-strategy-map" aria-labelledby="history-map-heading">
    <div className="history-section-heading"><MapIcon /><h2 id="history-map-heading">{t('history.port.map')}</h2></div>
    <div className="history-map-toolbar" role="group" aria-label={t('history.port.mapLayer')}>
      {(['heart', 'disaster', 'military', 'owner'] as const).map(value => <Button key={value} size="sm" variant={layer === value ? 'secondary' : 'ghost'} aria-pressed={layer === value} onClick={() => setLayer(value)}>{t(`history.port.layer.${value}`)}</Button>)}
      <Button size="sm" variant="ghost" aria-label={t('history.port.zoomOut')} disabled={zoom <= 0.75} onClick={() => setZoom(value => Math.max(0.75,value - 0.25))}>−</Button><Button size="sm" variant="ghost" aria-label={t('history.port.zoomIn')} disabled={zoom >= 2} onClick={() => setZoom(value => Math.min(2,value + 0.25))}>+</Button><span className="history-muted">{t('history.port.mapHint')}</span>
    </div>
    <div className="history-map-layout">
      <div className="history-map-canvas" ref={canvas}><svg style={{minWidth:720 * zoom}} className="history-geography" viewBox={presentation.view_box.join(' ')} role="group" aria-label={t('history.port.map')}>
        <defs><pattern id="history-map-paper" width="16" height="16" patternUnits="userSpaceOnUse"><path d="M16 0H0V16" fill="none" stroke="currentColor" opacity="0.05" strokeWidth="0.5" /></pattern></defs>
        <rect x={presentation.view_box[0]} y={presentation.view_box[1]} width={presentation.view_box[2]} height={presentation.view_box[3]} fill="url(#history-map-paper)" />
        {presentation.routes?.map(([a, b]) => {
          const start = regions.find(r => r.entity_id === a), end = regions.find(r => r.entity_id === b)
          return start && end ? <line className="history-map-route" key={`${a}-${b}`} x1={start.center[0]} y1={start.center[1]} x2={end.center[0]} y2={end.center[1]} /> : null
        })}
        {regions.map(region => {
          const item = byID(region.entity_id)!
          const name = text(item.name)
          return <g className={`history-map-region ${focus.entity_id === item.id ? 'is-selected' : ''}`} key={item.id} tabIndex={0} role="button" aria-label={name} aria-pressed={focus.entity_id === item.id} onClick={() => setSelected(item.id)} onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); setSelected(item.id) } }}>
            <title>{name}</title>
            {region.rings?.length ? <path d={region.rings.map(ring => `M${ring.map(point => point.join(',')).join('L')}Z`).join(' ')} fill={color(item.id)} fillRule="evenodd" /> : <circle cx={region.center[0]} cy={region.center[1]} r={width * 0.035} fill={color(item.id)} />}
          </g>
        })}
        {regions.map(region => <text className="history-map-label" key={region.entity_id} x={region.center[0]} y={region.center[1]} fontSize={width / 55} textAnchor="middle" pointerEvents="none">{text(region.label || byID(region.entity_id)!.name)}</text>)}
        {layer === 'military' && regions.map(region => {
          const count = world.entities.filter(item => item.kind === 'army' && item.status !== 'dead' && regionOf(item) === region.entity_id).length
          return count ? <text key={region.entity_id} className="history-map-army-count" x={region.center[0]} y={region.center[1] + width / 40} fontSize={width / 55} textAnchor="middle" pointerEvents="none">⚑ {count}</text> : null
        })}
      </svg></div>
      <aside className="history-map-detail">
        <p className="history-eyebrow">{t('history.port.selectedRegion')}</p><h3>{text(entity.name)}</h3><p className="history-muted">{t('history.world.owner')}: {text(byID(entity.owner || '')?.name) || t('history.port.unknownOwner')}</p>
        <dl className="history-world-metrics">{Object.entries(entity.metrics).filter(([key]) => ['heart', 'disaster', 'refugees', 'control', 'tax_base'].includes(key)).map(([key, value]) => <div key={key}><dt>{text(state.scenario!.metrics.find(metric => metric.id === key)?.name)}</dt><dd>{number(value)}</dd></div>)}</dl>
        <div className="history-actions"><Button size="sm" variant="outline" onClick={() => onDraft(t('history.port.regionDraft', { name: text(entity.name) }))}><ScrollText />{t('history.port.localDecree')}</Button><Button size="sm" variant="ghost" onClick={() => onAsk(state.ministers.find(m => byID(m.id)?.location === entity.id)?.id, t('history.port.regionQuestion', { name: text(entity.name) }))}><MessageCircle />{t('history.port.askRegion')}</Button></div>
        <h4 className="history-map-armies-title"><Flag />{t('history.port.stationed')}</h4>
        {!armies.length && <p className="history-muted">{t('history.port.noArmy')}</p>}
        {armies.map(army => <article key={army.id} className="history-map-army"><strong>{text(army.name)}</strong><small>{text(byID(army.location || '')?.name)}</small><small>{t('history.port.armySummary', { size: number(army.metrics.size || 0), morale: number(army.metrics.morale || 0) })}</small><Button size="sm" variant="ghost" onClick={() => onDraft(t('history.port.marchDraft', { army: text(army.name), target: text(byID(destination || focus.entity_id)?.name) }))}>{t('history.port.planMarch')}</Button></article>)}
        {!!armies.length && <label className="history-map-destination">{t('history.port.destination')}<select className="history-world-select" value={destination || focus.entity_id} onChange={event => setDestination(event.target.value)}>{regions.map(region => <option key={region.entity_id} value={region.entity_id}>{text(byID(region.entity_id)!.name)}</option>)}</select></label>}
      </aside>
    </div>
  </section>
}
