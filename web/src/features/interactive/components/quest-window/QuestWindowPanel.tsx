import { useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight, Eye, EyeOff, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import type { QuestTask } from '../../quest-types'
import { readFollowedTasks, writeFollowedTasks } from './followed'
import { useQuests } from './use-quests'
import { usePlayerAttention } from '../use-player-attention'

interface QuestWindowPanelProps {
  storyId: string
  branchId: string
  /** True while the panel is on screen. It is what starts the read. */
  active: boolean
  /** Collapses the panel. It settles nothing. */
  onClose: () => void
  /**
   * Writes an intent into the main story's own input. It only fills the box: the
   * player still edits and sends it, and accepting, handing in or collecting
   * still go through the formal path.
   */
  onFillIntent: (intent: string) => void
  className?: string
}

/**
 * Statuses that are still in play. Everything else is history.
 *
 * `offered` belongs here: the world has raised the matter and the character
 * knows about it, but the player has not taken it on. Filing it under history
 * said the opposite — that it was already closed — and hid the one action the
 * player can still take on it.
 */
const IN_HAND: QuestTask['status'][] = ['offered', 'accepted']

/**
 * 见闻与事务: the matters this character knows about, as committed facts.
 *
 * It is a view, not an operator console. Every line comes from the task record
 * the rules already wrote — the objectives, the ticks a committed fact produced,
 * the agreed reward and the agreed deadline. The panel has no accept, hand-in,
 * collect or abandon control, because those are the player's intentions and go
 * through the main story's own input and the formal path; viewing a matter must
 * never take it on.
 *
 * Following a matter is a reading preference: it floats the matter to the top
 * and nothing more. Unfollowing neither cancels it nor stops its deadline.
 */
export function QuestWindowPanel({ storyId, branchId, active, onClose, onFillIntent, className }: QuestWindowPanelProps) {
  const { t } = useTranslation()
  const entry = useQuests({ storyId, branchId, active })
  const attention = usePlayerAttention({ storyId, branchId, active })
  const followed = attention.data?.quests || []
  const [legacy, setLegacy] = useState<string[]>([])
  const [importing, setImporting] = useState(false)
  const target = useRef('')
  target.current = JSON.stringify([storyId, branchId, active])
  const [opportunitiesOpen, setOpportunitiesOpen] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)

  useEffect(() => {
    setLegacy(readFollowedTasks(storyId, branchId))
    setImporting(false)
  }, [storyId, branchId])

  const data = entry.data
  const tasks = data?.tasks || []
  const available = data?.available || []

  const { inHand, history } = useMemo(() => splitTasks(tasks), [tasks])
  const followedSet = useMemo(() => new Set(followed), [followed])

  const toggleFollow = (taskID: string) => {
    void attention.setFollowing('quest', taskID, !followedSet.has(taskID))
  }

  const importLegacy = async () => {
    const captured = target.current
    let remaining = [...legacy]
    setImporting(true)
    for (const id of legacy.slice(0, 32)) {
      if (target.current !== captured) return
      if (!await attention.setFollowing('quest', id, true)) continue
      remaining = remaining.filter((value) => value !== id)
      // Remove only IDs confirmed in this branch's journal. Unknown IDs and
      // failed writes stay in local storage so migration remains retryable.
      writeFollowedTasks(storyId, branchId, remaining)
      if (target.current !== captured) return
      setLegacy(remaining)
    }
    if (target.current === captured) setImporting(false)
  }

  const intentFor = (task: QuestTask): string => {
    switch (task.status) {
      case 'offered':
        return t('questWindow.intentAccept', { title: task.title })
      case 'accepted':
        return t('questWindow.intentContinue', { title: task.title })
      case 'completed':
        return task.settled
          ? t('questWindow.intentAlreadySettled', { title: task.title })
          : t('questWindow.intentCollect', { title: task.title })
      case 'expired':
        return t('questWindow.intentExpired', { title: task.title })
      default:
        return t('questWindow.intentAsk', { title: task.title })
    }
  }

  return (
    <div data-testid="quest-panel" className={cn('flex h-full min-h-0 flex-col bg-[var(--nova-surface)] text-[var(--nova-text)]', className)}>
      <header className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2">
        <h2 className="min-w-0 flex-1 truncate text-[13px] font-medium">{t('questWindow.title')}</h2>
        <Button type="button" variant="ghost" size="icon-sm" onClick={onClose} aria-label={t('questWindow.close')} title={t('questWindow.closeHint')}>
          <X className="h-4 w-4" />
        </Button>
      </header>

      {attention.error ? <div role="alert" className="px-3 py-2 text-xs text-destructive">{attention.error}<Button variant="ghost" size="sm" onClick={() => void attention.reload()}>{t('questWindow.retry')}</Button></div> : null}
      {legacy.length > 0 ? <div className="px-3 py-2 text-xs text-muted-foreground"><p>{t('questWindow.legacyAttention', { count: legacy.length })}</p><Button variant="outline" size="sm" disabled={attention.busy || !attention.data || importing} onClick={() => void importLegacy()}>{t('questWindow.importAttention')}</Button></div> : null}

      {entry.error ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center">
          <p className="text-[12px] text-[var(--nova-danger)]">{t('questWindow.loadFailed')}</p>
          <p className="text-[11px] text-[var(--nova-text-faint)]">{entry.error}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void entry.reload()}>{t('questWindow.retry')}</Button>
        </div>
      ) : entry.loading && !data ? (
        <div className="flex flex-1 items-center justify-center gap-2 text-[12px] text-[var(--nova-text-muted)]">
          <Spinner className="h-3.5 w-3.5" />
          {t('questWindow.loading')}
        </div>
      ) : (
        <>
          <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-3 py-2">
            <PlaceBlock location={data?.location} />

            {inHand.length === 0 && available.length === 0 && history.length === 0 ? (
              <p className="text-[11px] text-[var(--nova-text-faint)]">{t('questWindow.empty')}</p>
            ) : null}

            {inHand.length > 0 ? (
              <section className="flex flex-col gap-1">
                <span className="text-[11px] text-[var(--nova-text-muted)]">{t('questWindow.inHand')}</span>
                {sortInHand(inHand, followedSet).map((task) => (
                  <TaskBlock
                    key={task.task_id}
                    task={task}
                    followed={followedSet.has(task.task_id)}
                    followDisabled={entry.loading || attention.busy || !attention.data || importing}
                    onToggleFollow={() => toggleFollow(task.task_id)}
                    onFillIntent={() => onFillIntent(intentFor(task))}
                  />
                ))}
              </section>
            ) : null}

            {available.length > 0 ? (
              <CollapsibleSection
                open={opportunitiesOpen}
                onToggle={() => setOpportunitiesOpen((open) => !open)}
                label={t('questWindow.opportunities', { count: available.length })}
              >
                {available.map((option) => (
                  <div key={option.task_id} className="flex flex-col gap-0.5 rounded border border-[var(--nova-border)] px-2 py-1.5 text-[11px]">
                    <div className="flex items-baseline gap-2">
                      <span className="min-w-0 flex-1 truncate text-[12px] text-[var(--nova-text)]">{option.title}</span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => toggleFollow(option.task_id)}
                        disabled={entry.loading || attention.busy || !attention.data || importing}
                        aria-label={followedSet.has(option.task_id) ? t('questWindow.unfollow') : t('questWindow.follow')}
                        title={followedSet.has(option.task_id) ? t('questWindow.unfollow') : t('questWindow.follow')}
                      >
                        {followedSet.has(option.task_id) ? <Eye className="h-3.5 w-3.5" /> : <EyeOff className="h-3.5 w-3.5" />}
                      </Button>
                    </div>
                    <span className="text-[var(--nova-text-muted)]">{option.brief}</span>
                    {option.giver ? <span className="text-[var(--nova-text-muted)]">{t('questWindow.giver', { giver: option.giver })}</span> : null}
                    <span className="text-[var(--nova-text-faint)]">
                      {option.deadline_seconds
                        ? t('questWindow.offerDeadline', { duration: formatDuration(option.deadline_seconds, t) })
                        : t('questWindow.noDeadline')}
                    </span>
                  </div>
                ))}
              </CollapsibleSection>
            ) : null}

            {history.length > 0 ? (
              <CollapsibleSection
                open={historyOpen}
                onToggle={() => setHistoryOpen((open) => !open)}
                label={t('questWindow.history', { count: history.length })}
              >
                {history.map((task) => (
                  <TaskBlock
                    key={task.task_id}
                    task={task}
                    followed={followedSet.has(task.task_id)}
                    followDisabled={entry.loading || attention.busy || !attention.data || importing}
                    onToggleFollow={() => toggleFollow(task.task_id)}
                    onFillIntent={() => onFillIntent(intentFor(task))}
                  />
                ))}
              </CollapsibleSection>
            ) : null}
          </div>

          <footer className="shrink-0 border-t border-[var(--nova-border)] px-3 py-2">
            <p className="text-[11px] text-[var(--nova-text-faint)]">{t('questWindow.hint')}</p>
          </footer>
        </>
      )}
    </div>
  )
}

/** Where the character stands, in the scene's own committed wording. */
function PlaceBlock({ location }: { location?: string }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-between rounded border border-[var(--nova-border)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
      <span>{t('questWindow.currentPlace')}</span>
      <span className="min-w-0 truncate text-[var(--nova-text)]">{location || t('questWindow.unknownPlace')}</span>
    </div>
  )
}

/**
 * One matter, as the record really has it.
 *
 * The objectives are the task's own wording with the ticks a committed fact
 * produced; the reward is what was agreed, not a guess; the deadline is the
 * instant the arithmetic expires on, or an honest 未约定期限.
 */
function TaskBlock({ task, followed, followDisabled, onToggleFollow, onFillIntent }: {
  task: QuestTask
  followed: boolean
  followDisabled: boolean
  onToggleFollow: () => void
  onFillIntent: () => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const done = task.objectives.filter((objective) => objective.completed).length
  const next = task.objectives.find((objective) => !objective.completed)

  return (
    <section className="flex flex-col gap-0.5 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5">
      <div className="flex items-baseline gap-1">
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-1 text-left"
          onClick={() => setOpen((value) => !value)}
          aria-expanded={open}
          aria-label={t('questWindow.toggleDetails', { title: task.title })}
        >
          {open ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}
          <span className="min-w-0 truncate text-[12px] text-[var(--nova-text)]">{task.title}</span>
        </button>
        <span className="shrink-0 text-[11px] text-[var(--nova-text-muted)]">{t(`questWindow.status.${task.status}`)}</span>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={onToggleFollow}
          disabled={followDisabled}
          aria-label={followed ? t('questWindow.unfollow') : t('questWindow.follow')}
          title={followed ? t('questWindow.unfollow') : t('questWindow.follow')}
        >
          {followed ? <Eye className="h-3.5 w-3.5" /> : <EyeOff className="h-3.5 w-3.5" />}
        </Button>
      </div>

      <div className="flex flex-wrap items-baseline gap-x-2 text-[11px] text-[var(--nova-text-muted)]">
        <span>{t('questWindow.progress', { done, total: task.objectives.length })}</span>
        {task.giver ? <span>{t('questWindow.giver', { giver: task.giver })}</span> : null}
        {task.expired || task.status === 'accepted' ? (
          <span className={cn(task.expired && 'text-[var(--nova-danger)]')}>
            {task.expired ? t('questWindow.expired') : task.deadline_text ? t('questWindow.deadline', { when: task.deadline_text }) : t('questWindow.noDeadline')}
          </span>
        ) : null}
      </div>

      {next && !open ? (
        <p className="truncate text-[11px] text-[var(--nova-text-muted)]">{t('questWindow.nextClue', { clue: next.description })}</p>
      ) : null}

      {open ? (
        <div className="flex flex-col gap-0.5">
          <p className="text-[11px] text-[var(--nova-text-muted)]">{task.brief}</p>
          <div className="flex flex-col gap-0.5">
            <span className="text-[11px] text-[var(--nova-text-muted)]">{t('questWindow.objectives')}</span>
            {task.objectives.map((objective) => (
              <p key={objective.id} className="text-[11px] text-[var(--nova-text)]">
                {objective.completed ? '✓ ' : '· '}
                {objective.description}
              </p>
            ))}
          </div>
          <div className="flex flex-col gap-0.5">
            <span className="text-[11px] text-[var(--nova-text-muted)]">{t('questWindow.rewards')}</span>
            {task.rewards.length === 0 ? (
              <p className="text-[11px] text-[var(--nova-text-faint)]">{t('questWindow.noReward')}</p>
            ) : (
              task.rewards.map((reward) => (
                <p key={`${reward.kind}:${reward.ref_id}`} className="text-[11px] text-[var(--nova-text)]">
                  {reward.kind === 'item' && reward.name_key ? t(reward.name_key) : reward.ref_id} × {reward.quantity}
                </p>
              ))
            )}
            {task.settled ? <p className="text-[11px] text-[var(--nova-text-faint)]">{t('questWindow.settled')}</p> : null}
          </div>
          <Button type="button" variant="outline" size="sm" className="mt-1 self-start" onClick={onFillIntent}>
            {t('questWindow.fillIntent')}
          </Button>
        </div>
      ) : null}
    </section>
  )
}

function CollapsibleSection({ open, onToggle, label, children }: {
  open: boolean
  onToggle: () => void
  label: string
  children: ReactNode
}) {
  return (
    <section className="flex flex-col gap-1">
      <button type="button" className="flex items-center gap-1 self-start text-[11px] text-[var(--nova-text-muted)]" onClick={onToggle} aria-expanded={open}>
        {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
        {label}
      </button>
      {open ? children : null}
    </section>
  )
}

/** Splits the branch's tasks into matters still in play and matters closed. */
function splitTasks(tasks: QuestTask[]) {
  const inHand: QuestTask[] = []
  const history: QuestTask[] = []
  for (const task of tasks) {
    if (IN_HAND.includes(task.status)) {
      inHand.push(task)
    } else {
      history.push(task)
    }
  }
  return { inHand, history }
}

/**
 * Followed matters first, then the nearest agreed deadline, then the rest in
 * stable order. Following only reorders; it never changes a status.
 */
function sortInHand(tasks: QuestTask[], followed: Set<string>): QuestTask[] {
  return [...tasks].sort((left, right) => {
    const followedDelta = Number(followed.has(right.task_id)) - Number(followed.has(left.task_id))
    if (followedDelta !== 0) return followedDelta
    const leftDeadline = left.deadline_seconds ?? Number.POSITIVE_INFINITY
    const rightDeadline = right.deadline_seconds ?? Number.POSITIVE_INFINITY
    if (leftDeadline !== rightDeadline) return leftDeadline - rightDeadline
    return left.task_id.localeCompare(right.task_id)
  })
}

/**
 * Renders an authored offer interval in real days, hours and minutes.
 *
 * The interval is a plain duration, so it is stated in the units it is
 * measured in. The story's committed clock words elapsed time in 时辰 and 刻
 * (see the Go DescribeElapsed), but those are 7200 and 900 seconds: restating
 * a 3600-second hour as one 时辰 would halve it, and a 60-second minute as one
 * 刻 would cut it fifteen-fold.
 */
function formatDuration(seconds: number, t: (key: string, options?: Record<string, unknown>) => string): string {
  // Round to whole minutes first so a remainder can never report 60 minutes.
  const totalMinutes = Math.max(1, Math.round(seconds / 60))
  const days = Math.floor(totalMinutes / (24 * 60))
  const hours = Math.floor((totalMinutes % (24 * 60)) / 60)
  const minutes = totalMinutes % 60
  const parts: string[] = []
  if (days > 0) parts.push(t('questWindow.durationDays', { count: days }))
  if (hours > 0) parts.push(t('questWindow.durationHours', { count: hours }))
  if (minutes > 0) parts.push(t('questWindow.durationMinutes', { count: minutes }))
  return parts.join(' ')
}
