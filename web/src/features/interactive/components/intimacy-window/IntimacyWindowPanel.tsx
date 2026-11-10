import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { HeartHandshake, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { IntimacySurface } from './IntimacySurface'

interface IntimacyWindowPanelProps {
  storyId: string
  branchId: string
  projectId?: string
  /** True while the panel is on screen. It is what starts the read. */
  active: boolean
  /**
   * A person the window was opened for, when it was opened from that person's
   * conversation. The window then invites that one person; opened from the
   * toolbar it builds a roster instead.
   */
  focusCharacterId?: string
  /** Carries the activity into the story without restating it. */
  onContinueStory?: (message: string) => void
  /**
   * Collapses the panel. Closing is only a display act (T27): a pending
   * invitation stays pending and a running activity keeps running, so this
   * cannot end or consume anything.
   */
  onClose: () => void
  /** Dock/float/minimize controls, when the stage hosts this in a movable window. */
  windowActions?: ReactNode
  className?: string
}

/**
 * The dual-cultivation window.
 *
 * T27 gives this activity its own entry in the toolbar and its own window: the
 * chat window is about conversation, and folding this into it made the chat
 * offer a second kind of act it does not otherwise perform. The window is a
 * host, nothing more — it draws a title bar and hands the body to the same
 * surface the activity has always used, so the records, the invitation flow and
 * the phase rules are the ones the game already has.
 */
export function IntimacyWindowPanel({ storyId, branchId, projectId, active, focusCharacterId, onContinueStory, onClose, windowActions, className }: IntimacyWindowPanelProps) {
  const { t } = useTranslation()
  return (
    <div data-testid="intimacy-window-panel" className={cn('flex h-full min-h-0 flex-col bg-[var(--nova-surface)] text-[var(--nova-text)]', className)}>
      <header className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2">
        <HeartHandshake className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
        <h2 className="min-w-0 flex-1 truncate text-[13px] font-medium">{t('intimacyWindow.title')}</h2>
        {windowActions}
        <Button type="button" variant="ghost" size="icon-sm" onClick={onClose} aria-label={t('intimacyWindow.close')} title={t('intimacyWindow.closeHint')}>
          <X className="h-4 w-4" />
        </Button>
      </header>
      <IntimacySurface
        storyId={storyId}
        branchId={branchId}
        projectId={projectId}
        active={active}
        focusCharacterId={focusCharacterId}
        onContinueStory={onContinueStory}
      />
    </div>
  )
}
