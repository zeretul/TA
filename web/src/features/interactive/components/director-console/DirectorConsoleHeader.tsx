import { BookOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { BranchSummary, StorySummary } from '../../types'
import { branchDisplayName } from '../branching/model'

interface DirectorConsoleHeaderProps {
  branchId: string
  branches: BranchSummary[]
  turnCount: number
  story?: StorySummary
}

/** Identity and scope only; editable settings live in game settings. */
export function DirectorConsoleHeader({ branchId, branches, turnCount, story }: DirectorConsoleHeaderProps) {
  const { t } = useTranslation()
  // The route's own name, the one the timeline shows. The id is storage's
  // business: `br_...` says nothing about where the player is.
  const branchName = branchId
    ? branchDisplayName(branches.find((item) => item.id === branchId), {
        main: t('branchTimeline.mainBranch'),
        unknown: t('branchTimeline.unknownBranch'),
      })
    : t('branchTimeline.mainBranch')
  // The story's own name is what the player opened. An unnamed workspace still
  // has a branch and a turn count, so the second line stays honest either way.
  const storyName = story?.title?.trim() || ''

  return (
    <header className="shrink-0 border-b border-[var(--nova-border)] bg-[color-mix(in_srgb,var(--director-canvas)_92%,transparent)] px-4 py-3.5 backdrop-blur-xl">
      <div className="flex min-w-0 items-center gap-3">
        <div data-testid="director-panel-icon" className="flex size-9 shrink-0 items-center justify-center rounded-[11px] border border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--director-brass)]" aria-hidden="true">
          <BookOpen className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="director-console__display truncate text-sm font-semibold leading-5 text-[var(--nova-text)]">{t('directorPanel.handbookTitle')}</h2>
          <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[10px] text-[var(--nova-text-faint)]">
            {storyName ? (
              <>
                <span className="truncate">{storyName}</span>
                <span aria-hidden="true">/</span>
              </>
            ) : null}
            <span className="truncate">{t('directorPanel.branch', { branch: branchName })}</span>
            <span aria-hidden="true">/</span>
            <span className="shrink-0">{t('directorPanel.turnCount', { count: turnCount })}</span>
          </div>
        </div>
      </div>
    </header>
  )
}
