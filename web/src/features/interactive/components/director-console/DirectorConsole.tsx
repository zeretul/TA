import { useEffect, useMemo, useState } from 'react'
import type { BranchSummary, Snapshot, StorySummary } from '../../types'
import { BranchPreview } from './BranchPreview'
import { DirectorConsoleHeader } from './DirectorConsoleHeader'
import { DirectorConsoleTabs } from './DirectorConsoleTabs'
import { OverviewView } from './OverviewView'
import { readStoredDirectorConsoleTab, writeStoredDirectorConsoleTab } from './persistence'
import type { DirectorConsoleTab } from './types'

export interface DirectorConsoleProps {
  storyId?: string
  story?: StorySummary
  branchId: string
  branches: BranchSummary[]
  snapshot: Snapshot | null
  stateError?: string
  onSwitchBranch: (branchId: string) => void | Promise<void>
  onOpenBranchTimeline: () => void
  /** Jump the stage to a turn's own prose, from a history entry. */
  onOpenTurn?: (turnId: string) => void
}

/**
 * The right-hand handbook while a story is playing: what just changed, and what
 * has happened so far. Narrative requirements, style, rules and model choices
 * are not here — they belong to game settings, which owns the single editable
 * copy, and the branch plan stays backstage where the story keeps it.
 */
export function DirectorConsole({
  storyId,
  story,
  branchId,
  branches,
  snapshot,
  stateError,
  onSwitchBranch,
  onOpenBranchTimeline,
  onOpenTurn,
}: DirectorConsoleProps) {
  const [activeTab, setActiveTab] = useState<DirectorConsoleTab>(() => readStoredDirectorConsoleTab(storyId) || 'recent')

  useEffect(() => {
    setActiveTab(readStoredDirectorConsoleTab(storyId) || 'recent')
  }, [storyId])

  const changeTab = (tab: DirectorConsoleTab) => {
    setActiveTab(tab)
    writeStoredDirectorConsoleTab(storyId, tab)
  }

  // The header names the route from the same set the list covers, so a branch
  // that only the snapshot knows still reads as its own name.
  const knownBranches = useMemo(() => {
    const byID = new Map<string, BranchSummary>()
    for (const branch of [...branches, ...(snapshot?.graph?.branches || [])]) {
      if (!byID.has(branch.id)) byID.set(branch.id, branch)
    }
    return Array.from(byID.values())
  }, [branches, snapshot?.graph?.branches])
  const turnCount = story?.turn_count ?? ((snapshot?.turns || []).length || (snapshot?.current_turn ? 1 : 0))
  const activeView = activeTab === 'history' ? (
    <BranchPreview
      branches={branches}
      currentBranchId={branchId}
      snapshot={snapshot}
      onSwitchBranch={onSwitchBranch}
      onOpenTimeline={onOpenBranchTimeline}
      onOpenTurn={onOpenTurn}
    />
  ) : (
    <OverviewView
      key={branchId}
      snapshot={snapshot}
      stateError={stateError}
    />
  )

  return (
    <aside className="director-console flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--director-canvas)] text-[var(--nova-text)]">
      <DirectorConsoleHeader
        branchId={branchId}
        branches={knownBranches}
        turnCount={turnCount}
        story={story}
      />
      <DirectorConsoleTabs activeTab={activeTab} onChange={changeTab} />
      <div className="min-h-0 flex-1 overflow-hidden">{activeView}</div>
    </aside>
  )
}
