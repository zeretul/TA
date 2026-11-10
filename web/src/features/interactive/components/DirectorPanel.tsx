import type { BranchSummary, Snapshot, StorySummary } from '../types'
import { DirectorConsole } from './director-console/DirectorConsole'

interface DirectorPanelProps {
  storyId?: string
  story?: StorySummary
  branchId?: string
  branches: BranchSummary[]
  snapshot: Snapshot | null
  onSwitchBranch: (branchId: string) => void | Promise<void>
  onOpenBranchTimeline: () => void
  onOpenTurn?: (turnId: string) => void
}

export function DirectorPanel({
  storyId,
  story,
  branchId,
  branches,
  snapshot,
  onSwitchBranch,
  onOpenBranchTimeline,
  onOpenTurn,
}: DirectorPanelProps) {
  return (
    <DirectorConsole
      storyId={storyId}
      story={story}
      branchId={branchId || snapshot?.branch_id || ''}
      branches={branches}
      snapshot={snapshot}
      stateError={snapshot?.current_turn?.state_error || ''}
      onSwitchBranch={onSwitchBranch}
      onOpenBranchTimeline={onOpenBranchTimeline}
      onOpenTurn={onOpenTurn}
    />
  )
}
