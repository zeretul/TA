import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'

interface SummaryBlockProps {
  /** What the list holds, e.g. "Unfinished" or "Members". */
  label: string
  count: number
  /**
   * One line of the list's own content, shown while it is closed. It is what
   * keeps the closed block informative rather than a bare count.
   */
  summary?: string
  /** A control that belongs beside the label and must stay reachable when closed. */
  action?: ReactNode
  children: ReactNode
  className?: string
}

/**
 * A list above a conversation that is closed to one line until asked for.
 *
 * Arrangements, letters in transit and a group's roster all grow with the story,
 * and each of them used to take its full height out of the conversation below —
 * enough of them and the messages were pushed off the panel entirely. Closed to a
 * single line each, the transcript keeps the remaining height and the composer
 * stays reachable, while the content is one click away.
 *
 * Open, the list scrolls inside a capped height of its own rather than pushing
 * the conversation down again, so a long arrangement is still readable without
 * costing the messages their room. Several of them open at once share that room
 * instead of each claiming its full cap: an open block gives height back before
 * the conversation does, so the messages keep a readable floor and the composer
 * stays where it is.
 */
export function SummaryBlock({ label, count, summary, action, children, className }: SummaryBlockProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className={cn(
        'flex flex-col border-b border-[var(--nova-border)]',
        open ? 'min-h-0 shrink' : 'shrink-0',
        className,
      )}
    >
      <div className="flex shrink-0 items-center gap-1.5 px-3 py-1.5">
        <CollapsibleTrigger asChild>
          <button type="button" className="flex min-w-0 flex-1 items-center gap-1.5 text-left">
            {open
              ? <ChevronDown className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" />
              : <ChevronRight className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" />}
            <span className="shrink-0 text-[10px] uppercase tracking-wide text-[var(--nova-text-faint)]">{label}</span>
            <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">{t('common.items', { count })}</span>
            {open || !summary ? <span className="min-w-0 flex-1" /> : (
              <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--nova-text-muted)]">{summary}</span>
            )}
          </button>
        </CollapsibleTrigger>
        {action}
      </div>
      <CollapsibleContent className="flex min-h-0 flex-col overflow-hidden">
        <div className="min-h-0 max-h-[9.5rem] flex-1 overflow-y-auto px-3 pb-2">{children}</div>
      </CollapsibleContent>
    </Collapsible>
  )
}
