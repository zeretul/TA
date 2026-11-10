import { useState, type CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'
import { projectFileAssetURL } from '@/lib/api-client/project-files'
import { realmLabel } from '../story-state/realm-label'
import atlas from '@/assets/collectibles/archetypes-v1.png'
import { QUALITY_GRADES, type EntryPresentation, type QualityGrade } from './types'
import './entry-artwork.css'

const presets: Record<string, [number, number]> = {
  'cultivation-technique': [0, 0], 'cultivation-skill': [1, 0], 'cultivation-equipment': [2, 0],
  'wuxia-technique': [0, 1], 'wuxia-skill': [1, 1], 'wuxia-equipment': [2, 1],
  'space-technique': [0, 2], 'space-skill': [1, 2], 'space-equipment': [2, 2],
}
const colors: Record<QualityGrade, string> = { white: '#cbd5e1', green: '#34d399', blue: '#60a5fa', purple: '#c084fc', orange: '#fb923c', red: '#fb4b62' }
const layers: Record<QualityGrade, number> = { white: 1, green: 2, blue: 3, purple: 4, orange: 5, red: 6 }

/** This is only an intensity within the declared rules' scale. No comparison of
 * a person's mastery, a machine, a ship or cultivation realms is exposed. */
export function artworkIntensity(level: EntryPresentation['visual_level']): number {
  if (!level) return 0
  if (level.kind === 'mastery') return Number.isFinite(level.value) && level.maximum > 1 ? Math.max(0, Math.min(1, (level.value - 1) / (level.maximum - 1))) : 0
  const qi = /^qi_([1-9]|1[0-3])$/.exec(level.value)
  if (qi) return (Number(qi[1]) - 1) / 33
  const stage = /^(foundation|core|nascent|spirit|void|integration|mahayana)_(early|middle|late)$/.exec(level.value)
  if (!stage) return 0
  // Major realm order is the cultivation domain's existing stage table only.
  return (['foundation', 'core', 'nascent', 'spirit', 'void', 'integration', 'mahayana'].indexOf(stage[1]) * 3 + ['early', 'middle', 'late'].indexOf(stage[2]) + 13) / 33
}

export function artworkSource(art: EntryPresentation['art']): { url: string; cell?: [number, number] } | null {
  if (art?.source === 'preset' && art.preset_id && presets[art.preset_id]) return { url: atlas, cell: presets[art.preset_id] }
  if (art?.source === 'catalog' && art.catalog_id && /^[a-f0-9]{64}$/.test(art.catalog_id) && !art.project_id && !art.image_path && !art.preset_id) return { url: `/api/catalog-art/${art.catalog_id}.png` }
  // Only same-project relative assets are allowed, never model-supplied URLs.
  if (art?.source === 'generated' && art.project_id && art.image_path && /^assets\/image\/generated\/[a-zA-Z0-9_./-]+\.(png|jpe?g|webp)$/i.test(art.image_path) && !art.image_path.split('/').some(p => !p || p === '..' || p === '.')) return { url: projectFileAssetURL(art.project_id, art.image_path) }
  return null
}

export function EntryArtwork({ entry, thumbnail = false, omitMissing = false }: { entry: EntryPresentation; thumbnail?: boolean; omitMissing?: boolean }) {
  const { t } = useTranslation()
  const source = artworkSource(entry.art)
  const [failedSource, setFailedSource] = useState('')
  const valid = source && failedSource !== source.url
  if (!valid && omitMissing) return null
  const grade = entry.quality_grade && QUALITY_GRADES.includes(entry.quality_grade) ? entry.quality_grade : undefined
  const intensity = artworkIntensity(entry.visual_level)
  const style = { '--entry-aura': grade ? colors[grade] : 'transparent', '--entry-layers': grade ? layers[grade] : 0, '--entry-intensity': intensity } as CSSProperties
  const level = entry.visual_level
  const skillGuide = entry.category === 'skills' || entry.category === 'techniques'
  const stage = level?.kind === 'stage' ? realmLabel(level.value, t) : ''
  const caption = level?.kind === 'mastery'
    ? t('entryDetails.visualMastery', { level: level.value, maximum: level.maximum })
    : stage ? t('entryDetails.visualStage', { stage }) : ''
  const Container = thumbnail ? 'span' : 'figure'
  return <Container className={`entry-artwork ${thumbnail ? 'entry-artwork--thumbnail' : ''} ${valid ? '' : 'entry-artwork--empty'}`} data-art-theme={entry.visual_theme} data-art-grade={grade} data-art-intensity={intensity.toFixed(3)} style={style} aria-hidden={thumbnail || undefined}>
    <span className="entry-artwork__frame">
      {valid ? <span className="entry-artwork__image" role={thumbnail ? undefined : 'img'} aria-label={thumbnail ? undefined : t('entryDetails.illustration', { name: entry.name })} style={source.cell ? { backgroundImage: `url(${source.url})`, backgroundSize: '300% 300%', backgroundPosition: `${source.cell[0] * 50}% ${source.cell[1] * 50}%` } : undefined}>
        <img src={source.url} alt="" className={source.cell ? 'entry-artwork__preload' : undefined} onError={() => setFailedSource(source.url)} loading="lazy" />
      </span> : <span className="entry-artwork__empty">{thumbnail ? null : t('entryDetails.emptyArt')}</span>}
      <span className="entry-artwork__tracery" aria-hidden="true" />
      <span className="entry-artwork__halo" aria-hidden="true" />
      {grade && layers[grade] >= 4 ? <span className="entry-artwork__corners" aria-hidden="true" /> : null}
      {grade && layers[grade] >= 5 ? <span className="entry-artwork__orbit" aria-hidden="true" /> : null}
    </span>
    {!thumbnail && !skillGuide && caption ? <figcaption className="pt-2 text-xs text-muted-foreground">{caption}</figcaption> : null}
  </Container>
}
