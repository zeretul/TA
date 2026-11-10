import { useMemo, useState } from 'react'
import { ArrowRight, GitBranch, Loader2, Map as MapIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { BranchSummary, Snapshot, TurnEvent } from '../../types'
import { branchDisplayName } from '../branching/model'
import { formatStoryElapsed, storyInstantsByTurn } from './story-time'

interface BranchPreviewProps {
  branches: BranchSummary[]
  currentBranchId: string
  snapshot: Snapshot | null
  onSwitchBranch: (branchId: string) => void | Promise<void>
  onOpenTimeline: () => void
  /** Read one turn's own prose again, where it was written. */
  onOpenTurn?: (turnId: string) => void
}

/**
 * The story so far, one entry per turn already played.
 *
 * Each entry is the turn's own recollection of what it published. A turn without
 * one — every turn played before the recollection existed, and any turn whose
 * model left it out — shows a leading excerpt of its prose instead, which reads
 * as an excerpt and never claims to be a summary.
 *
 * Switching branches is still here, because a route change is how the player
 * reads a different past. What is not here is anything the story has not played
 * yet: the branch plan stays backstage.
 */
export function BranchPreview({ branches, currentBranchId, snapshot, onSwitchBranch, onOpenTimeline, onOpenTurn }: BranchPreviewProps) {
  const { t } = useTranslation()
  const [switchingBranchId, setSwitchingBranchId] = useState('')
  const [switchError, setSwitchError] = useState('')
  const resolvedBranches = useMemo(
    () => resolvePreviewBranches(branches, currentBranchId, snapshot),
    [branches, currentBranchId, snapshot],
  )
  const turns = useMemo(() => [...(snapshot?.turns || [])].reverse(), [snapshot?.turns])
  // Only turns that actually spent in-fiction time carry an instant, so the list
  // never repeats a time for a turn that did not move the story's clock.
  const instants = useMemo(() => storyInstantsByTurn(snapshot?.turns || []), [snapshot?.turns])
  const labels = {
    main: t('branchTimeline.mainBranch'),
    unknown: t('branchTimeline.unknownBranch'),
  }

  const switchBranch = async (branch: BranchSummary) => {
    if (branch.id === currentBranchId || switchingBranchId) return
    setSwitchingBranchId(branch.id)
    setSwitchError('')
    try {
      await onSwitchBranch(branch.id)
    } catch (error) {
      console.error('[director-console] switch story branch from preview failed', { branchId: branch.id, error })
      setSwitchError(t('directorPanel.branches.switchFailed'))
    } finally {
      setSwitchingBranchId('')
    }
  }

  return (
    <section data-testid="director-story-history" className="flex h-full min-h-0 flex-col">
      <div className="director-console__scroll min-h-0 flex-1 overflow-y-auto px-4 py-4">
        {resolvedBranches.length > 1 ? (
          <div className="mb-4">
            <div className="flex items-center justify-between gap-2">
              <h3 className="director-console__display text-xs font-semibold text-[var(--nova-text)]">{t('directorPanel.branches.title')}</h3>
              <Button variant="ghost" size="xs" className="gap-1.5 text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]" onClick={onOpenTimeline}>
                <MapIcon className="h-3.5 w-3.5" />
                <span className="truncate">{t('directorPanel.branches.openTimeline')}</span>
                <ArrowRight className="h-3.5 w-3.5" />
              </Button>
            </div>
            <div className="mt-2 flex flex-wrap gap-1.5">
              {resolvedBranches.map((branch) => {
                const current = branch.id === currentBranchId
                const name = branchDisplayName(branch, labels)
                const busy = switchingBranchId === branch.id
                return (
                  <button
                    key={branch.id}
                    type="button"
                    aria-current={current ? 'true' : undefined}
                    aria-label={current ? t('directorPanel.branches.currentNamed', { name }) : t('directorPanel.branches.switchTo', { name })}
                    disabled={Boolean(switchingBranchId)}
                    onClick={() => void switchBranch(branch)}
                    className={cn(
                      'flex min-w-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] outline-none transition-colors focus-visible:ring-2 focus-visible:ring-[var(--director-brass)]/45 disabled:cursor-wait',
                      current
                        ? 'border-[color-mix(in_srgb,var(--director-brass)_45%,var(--nova-border))] bg-[color-mix(in_srgb,var(--director-brass)_8%,var(--nova-surface))] text-[var(--nova-text)]'
                        : 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)]',
                    )}
                  >
                    {busy ? <Loader2 className="h-3 w-3 animate-spin" /> : <GitBranch className="h-3 w-3 shrink-0" />}
                    <span className="max-w-40 truncate">{name}</span>
                  </button>
                )
              })}
            </div>
          </div>
        ) : null}

        {turns.length > 0 ? (
          <ol className="space-y-1.5">
            {turns.map((turn) => (
              <HistoryEntry key={turn.id} turn={turn} instantSeconds={instants.get(turn.id)} onOpenTurn={onOpenTurn} />
            ))}
          </ol>
        ) : (
          <div className="mt-4 flex min-h-40 flex-col items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 text-center">
            <GitBranch className="h-5 w-5 text-[var(--nova-text-faint)]" />
            <p className="mt-2 text-xs font-medium text-[var(--nova-text-muted)]">{t('directorPanel.history.none')}</p>
            <p className="mt-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('directorPanel.history.noneHint')}</p>
          </div>
        )}

        {switchError ? <div role="alert" className="mt-3 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] p-2 text-xs text-[var(--nova-danger)]">{switchError}</div> : null}
      </div>

      {resolvedBranches.length <= 1 ? (
        <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--director-canvas)] p-3">
          <Button variant="outline" className="w-full justify-between border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]" onClick={onOpenTimeline}>
            <span className="flex min-w-0 items-center gap-2">
              <MapIcon className="h-3.5 w-3.5" />
              <span className="truncate">{t('directorPanel.branches.openTimeline')}</span>
            </span>
            <ArrowRight className="h-3.5 w-3.5" />
          </Button>
        </div>
      ) : null}
    </section>
  )
}

/**
 * One played turn: a readable title, the turn's own recollection of it, and the
 * story time it reached when it really spent some.
 *
 * The title is what the player asked for, because that is how they will look for
 * the turn. A turn the host wrote for the story itself has no player line, so it
 * is named for what it is instead of showing an empty heading.
 */
function HistoryEntry({ turn, instantSeconds, onOpenTurn }: { turn: TurnEvent; instantSeconds?: number; onOpenTurn?: (turnId: string) => void }) {
  const { t } = useTranslation()
  const title = turn.user_context_only
    ? t('storyStage.turnNavigator.autonomousContinuation')
    : firstLine(turn.user) || t('storyStage.turnNavigator.emptyUser')
  const recollection = turn.recap?.trim() || excerpt(turn.narrative, 120)
  // The instant the story's own clock reached here. A turn that spent no
  // in-fiction time has none, and saying "same time as before" every row would
  // be noise.
  const storyTime = instantSeconds === undefined ? '' : formatStoryElapsed(instantSeconds, t)
  return (
    <li>
      <button
        type="button"
        onClick={() => onOpenTurn?.(turn.id)}
        className="group flex w-full min-w-0 flex-col gap-1 rounded-[var(--nova-radius)] border border-transparent px-2.5 py-2 text-left outline-none transition-colors hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] focus-visible:ring-2 focus-visible:ring-[var(--director-brass)]/45"
      >
        <span className="flex min-w-0 items-baseline gap-2">
          <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-[var(--nova-text)]">{title}</span>
          {storyTime ? (
            <span className="shrink-0 text-[10px] tabular-nums text-[var(--nova-text-faint)]">{storyTime}</span>
          ) : null}
        </span>
        <span className="whitespace-pre-line break-words text-[11px] leading-5 text-[var(--nova-text-muted)]">
          {recollection || t('directorPanel.history.noRecollection')}
        </span>
      </button>
    </li>
  )
}

function firstLine(value: string) {
  return value.trim().split(/\r?\n/).find(Boolean) || ''
}

function excerpt(value: string, limit: number) {
  const text = value.trim().replace(/\s+/g, ' ')
  if (!text) return ''
  return text.length > limit ? `${text.slice(0, limit - 1)}…` : text
}

function resolvePreviewBranches(branches: BranchSummary[], currentBranchId: string, snapshot: Snapshot | null) {
  const merged = new Map<string, BranchSummary>()
  for (const branch of branches) merged.set(branch.id, branch)
  for (const branch of snapshot?.graph?.branches || []) merged.set(branch.id, branch)

  if (currentBranchId && !merged.has(currentBranchId)) {
    merged.set(currentBranchId, {
      id: currentBranchId,
      head: snapshot?.current_turn?.id || snapshot?.turns.at(-1)?.id || '',
      title: currentBranchId,
      created_at: snapshot?.turns[0]?.ts || '',
      current: true,
    })
  }

  return Array.from(merged.values()).sort((left, right) => {
    if (left.id === 'main') return -1
    if (right.id === 'main') return 1
    return left.created_at.localeCompare(right.created_at) || left.id.localeCompare(right.id)
  })
}
