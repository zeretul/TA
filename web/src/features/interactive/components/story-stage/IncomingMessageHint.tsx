import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, Mail } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { CharacterWindowMessage } from '../../types'

interface IncomingMessageHintProps {
  messages: CharacterWindowMessage[]
  /** Opens the character window on the sender. */
  onOpen: (characterId: string) => void
}

/**
 * The stage's quiet hint for incoming messages.
 *
 * It is quiet on purpose: a message is a person writing to the protagonist, not
 * a task that must be taken up, so nothing here is a modal, a badge that counts
 * up, or a prompt that blocks reading. The player can look at the list, open the
 * sender's window to answer, or leave it alone entirely, and leaving it alone
 * accepts nothing — only a committed reply in the window does that.
 */
export function IncomingMessageHint({ messages, onOpen }: IncomingMessageHintProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  if (messages.length === 0) return null

  const newest = messages[0]
  return (
    <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-1.5">
      <button
        type="button"
        className="flex w-full items-center gap-2 text-left"
        onClick={() => setExpanded((open) => !open)}
        aria-expanded={expanded}
      >
        <Mail className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
        <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--nova-text-muted)]">
          {messages.length === 1
            ? t('characterWindow.hintSingle', { name: newest.name, time: newest.world_time })
            : t('characterWindow.hintMany', { count: messages.length, name: newest.name })}
        </span>
        <ChevronDown className={cn('h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition-transform', expanded && 'rotate-180')} />
      </button>
      {expanded ? (
        <ul className="mt-1.5 flex flex-col gap-1 pb-1">
          {messages.map((message) => (
            <li key={message.id}>
              <button
                type="button"
                className="w-full rounded-[var(--nova-radius)] px-2 py-1.5 text-left transition-colors hover:bg-[var(--nova-hover)]"
                onClick={() => onOpen(message.character_id)}
              >
                <span className="flex items-baseline justify-between gap-2">
                  <span className="truncate text-[11px] text-[var(--nova-text)]">{message.name}</span>
                  <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">
                    {message.lapsed ? t('characterWindow.messageLapsed') : message.world_time}
                  </span>
                </span>
                <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-muted)]">{message.text}</span>
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <div className="mt-1 flex items-center gap-2">
          <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--nova-text-faint)]">{newest.text}</span>
          <Button type="button" variant="ghost" size="sm" className="h-6 shrink-0 px-2 text-[10px]" onClick={() => onOpen(newest.character_id)}>
            {t('characterWindow.hintOpen')}
          </Button>
        </div>
      )}
    </div>
  )
}
