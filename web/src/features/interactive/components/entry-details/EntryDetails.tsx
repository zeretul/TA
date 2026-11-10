import { useWorldPresentation } from '../../world-presentation-context'
import { useTranslation } from 'react-i18next'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { EntryArtwork } from './EntryArtwork'
import { cn } from '@/lib/utils'
import { QUALITY_GRADES, type EntryMetadata, type EntryPresentation, type QualityGrade } from './types'

const qualityColors: Record<QualityGrade, string> = {
  white: 'bg-white border-slate-500', green: 'bg-green-600 border-green-800 dark:bg-green-400',
  blue: 'bg-blue-600 border-blue-800 dark:bg-blue-400', purple: 'bg-purple-600 border-purple-800 dark:bg-purple-400',
  orange: 'bg-orange-500 border-orange-800 dark:bg-orange-400', red: 'bg-red-600 border-red-800 dark:bg-red-400',
}

/** Colour is redundant with a readable grade. Missing/invalid grades are not inferred. */
export function QualityBadge({ grade, label }: { grade: EntryMetadata['quality_grade']; label?: string }) {
  const { t } = useTranslation()
  if (!grade || !QUALITY_GRADES.includes(grade)) return null
  return <span data-quality-grade={grade} className="inline-flex shrink-0 items-center gap-1.5 rounded-md border px-1.5 py-0.5 text-[11px] font-medium text-foreground">
    <span aria-hidden="true" className={cn('size-2.5 shrink-0 rounded-full border', qualityColors[grade])} />
    {label || t(`entryDetails.quality.${grade}`)}
  </span>
}

/** Read-only: opening, closing and inspecting an entry never sends a command. */
export function EntryDetails({ entry, compact = false, nameOnly = false, combat = false }: { entry: EntryPresentation; compact?: boolean; nameOnly?: boolean; combat?: boolean }) {
  const { t } = useTranslation()
  const { terms } = useWorldPresentation()
  return <Dialog>
    <DialogTrigger asChild>
      {nameOnly ? <button type="button" aria-label={t('entryDetails.inspect', { name: entry.name })} className="flex min-w-0 max-w-full items-center gap-2 rounded-md px-2 py-1 text-left text-xs hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring">
        <span className="break-words font-medium [overflow-wrap:anywhere]">{entry.name}</span>
        <QualityBadge grade={entry.quality_grade} label={entry.grade_label} />
      </button> : <button type="button" aria-label={t('entryDetails.inspect', { name: entry.name })} className={cn('flex w-full min-w-0 flex-wrap items-center gap-2 rounded-md border bg-background px-2.5 py-2 text-left text-xs transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring', compact && 'border-0 bg-transparent px-0 py-1')}>
        {!compact ? <EntryArtwork entry={entry} thumbnail /> : null}
        <span className="min-w-0 flex-1">
          {!compact ? <span className="mb-1 block break-words font-medium [overflow-wrap:anywhere]">{entry.name}</span> : null}
          <span className="flex flex-wrap items-center gap-2">
            <QualityBadge grade={entry.quality_grade} label={entry.grade_label} />
            {entry.summary ? <span className="text-muted-foreground">{entry.summary}</span> : null}
            <span className="text-muted-foreground">{t('entryDetails.details')}</span>
          </span>
        </span>
      </button>}
    </DialogTrigger>
    <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto [overflow-wrap:anywhere]">
      <DialogHeader className="pr-7">
        <DialogTitle className="break-words">{entry.name}</DialogTitle>
        <DialogDescription>{entry.category === 'techniques' ? terms.techniques : t(`entryDetails.category.${entry.category}`)}</DialogDescription>
        <div><QualityBadge grade={entry.quality_grade} label={entry.grade_label} /></div>
      </DialogHeader>
      <EntryArtwork key={`${entry.id}:${JSON.stringify(entry.art)}`} entry={entry} omitMissing />
      <EntryDetailsBody entry={entry} combat={combat} />
    </DialogContent>
  </Dialog>
}

/** combat=true keeps the tactical facts a battle needs; the archive view of a skill stays prose only. */
export function EntryDetailsBody({ entry, combat = false }: { entry: EntryPresentation; combat?: boolean }) {
  const { t } = useTranslation()
  const skillGuide = (entry.category === 'skills' || entry.category === 'techniques') && !combat
  const prose = skillGuide
    ? [['description', entry.instance_description?.trim() || entry.description], ['background', entry.background]] as const
    : [['instanceDescription', entry.instance_description], ['source', entry.source], ['description', entry.description], ['background', entry.background]] as const
  return <div className="min-w-0 space-y-4 text-sm">
    {prose.filter(([, value]) => value?.trim()).map(([key, value]) => <section key={key}>
      <h3 className="mb-1 text-xs font-semibold text-muted-foreground">{t(`entryDetails.${key}`)}</h3>
      <p className="whitespace-pre-wrap break-words leading-relaxed [overflow-wrap:anywhere]">{value}</p>
    </section>)}
    {!prose.some(([, value]) => value?.trim()) ? <p className="text-xs text-muted-foreground">{t('entryDetails.noDescription')}</p> : null}
    {!skillGuide && (['status', 'requirements', 'effects', 'costs'] as const).map(group => entry[group].length ? <section key={group}>
      <h3 className="mb-1 text-xs font-semibold text-muted-foreground">{t(`entryDetails.${group}`)}</h3>
      <dl className="space-y-1.5">{entry[group].map((fact, index) => <div key={`${fact.label}:${index}`} className="grid min-w-0 grid-cols-1 gap-x-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
        <dt className="text-muted-foreground">{fact.label}</dt><dd className="min-w-0 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{fact.value}</dd>
      </div>)}</dl>
    </section> : null)}
  </div>
}
