import { BookOpen, GitBranch, Sparkles } from 'lucide-react'
import type { TFunction } from 'i18next'
import { Button } from '@/components/ui/button'
import type { TerminalOutcome } from '../../types'

interface StoryBranchEndedPanelProps {
  outcome: TerminalOutcome
  /** False when the stage cannot offer a fork, so the button is not shown as usable. */
  canFork: boolean
  /** True when there are earlier turns the reader has not loaded yet. */
  hasEarlier: boolean
  onReviewStory: () => void
  onForkBeforeEnding: () => void
  onStartNewStory: () => void
  t: TFunction
}

/**
 * §3 of 自制-死亡与分支结局: the branch ends, the story does not. The design asks
 * for a plain ending notice and the three ways on, and for entries that are not
 * implemented not to be shown as usable. Each button below maps to something
 * that really works — the history is still on the journal, a fork from an
 * earlier turn is the native branch path, and a new story is the native creation
 * path — so all three are offered; the fork is the only one that can be
 * unavailable, and then it is dropped rather than disabled.
 */
export function StoryBranchEndedPanel({
  outcome, canFork, hasEarlier, onReviewStory, onForkBeforeEnding, onStartNewStory, t,
}: StoryBranchEndedPanelProps) {
  return (
    <div
      data-testid="story-branch-ended"
      role="status"
      className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-3"
    >
      <p className="text-sm font-medium text-[var(--nova-text)]">{t('storyStage.ended.title')}</p>
      {outcome.reason_key || outcome.reason ? (
        <p className="mt-1 whitespace-pre-wrap break-words text-xs leading-5 text-[var(--nova-text-muted)]">{outcome.reason_key ? t(outcome.reason_key as never) : outcome.reason}</p>
      ) : null}
      <div className="mt-2 flex flex-wrap gap-1.5">
        <Button type="button" variant="outline" size="sm" className="h-7 gap-1 px-2 text-xs" onClick={onReviewStory}>
          <BookOpen className="h-3.5 w-3.5" />
          {t(hasEarlier ? 'storyStage.ended.reviewEarlier' : 'storyStage.ended.reviewStory')}
        </Button>
        {canFork ? (
          <Button type="button" variant="outline" size="sm" className="h-7 gap-1 px-2 text-xs" onClick={onForkBeforeEnding}>
            <GitBranch className="h-3.5 w-3.5" />
            {t('storyStage.ended.forkBefore')}
          </Button>
        ) : null}
        <Button type="button" variant="outline" size="sm" className="h-7 gap-1 px-2 text-xs" onClick={onStartNewStory}>
          <Sparkles className="h-3.5 w-3.5" />
          {t('storyStage.ended.startNew')}
        </Button>
      </div>
    </div>
  )
}
