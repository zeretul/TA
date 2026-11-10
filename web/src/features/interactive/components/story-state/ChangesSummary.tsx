import { Sparkle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { ActorStateSchemaSnapshot } from '../../types'
import { buildChangeDigest, type DigestEntry } from './changes-digest'
import type { ActorStateEntry, StoryStateChange } from './model'

/**
 * ChangesSummary is the digest of what the current turn really changed.
 *
 * It is not a list of the writes the turn made: a turn rewrites most of its
 * fields every time, and printing them all would bury the one fact the player
 * needs — that they were wounded, learned an art, or spent their last spirit
 * stones. changes-digest keeps the differences that matter and ranks them;
 * this component only decides how each one reads.
 *
 * At the opening the summary says 初始状态 instead of itemising the character
 * sheet. A new story writes seventy-odd fields at once, and none of them is a
 * change the player made: they are the state the story starts in.
 */
export function ChangesSummary({
  changes,
  actors,
  schema,
  turnCount,
  standalone = false,
}: {
  changes: StoryStateChange[]
  actors: ActorStateEntry[]
  schema?: ActorStateSchemaSnapshot
  turnCount?: number
  standalone?: boolean
}) {
  const { t } = useTranslation()
  const opening = (turnCount ?? 0) === 0
  const digest = opening ? [] : buildChangeDigest(changes, actors, schema)
  // A quiet turn still says something. The field-level detail is on each field
  // and the journal still holds every write, but a summary that vanishes leaves
  // the player unsure whether the turn landed or the panel broke.
  const quiet = !opening && digest.length === 0

  const title = opening ? t('storyStage.state.digest.initial') : t('storyStage.state.digest.title')

  return (
    <div className="story-state-ledger__summary" data-standalone={standalone || undefined} aria-label={title}>
      <span className="story-state-ledger__summary-title">
        <Sparkle aria-hidden="true" className="size-3" />
        {title}
      </span>
      {opening ? null : quiet ? (
        <span className="min-w-0 break-words text-[var(--nova-text-faint)]">{t('storyStage.state.digest.quiet')}</span>
      ) : (
        <span className="min-w-0 whitespace-normal [overflow-wrap:anywhere]">
          {digest.map((entry, index) => (
            <span key={entry.id}>
              {index > 0 ? <span className="text-[var(--nova-text-faint)]"> · </span> : null}
              <DigestItem entry={entry} />
            </span>
          ))}
        </span>
      )}
    </div>
  )
}

/**
 * DigestItem reads one difference as a sentence fragment.
 *
 * "学会火弹术" rather than "已学功法 火弹术 +": the field name is the rules'
 * bookkeeping, and the player already knows which art they learned. Where the
 * digest recorded no verb the entry falls back to the field name, because
 * inventing a verb for a change this interface does not understand would be
 * worse than naming the field.
 */
function DigestItem({ entry }: { entry: DigestEntry }) {
  const { t } = useTranslation()
  const { change } = entry
  // A field the pack names is already a word; a caption this interface wrote
  // ("伤势") has to be translated, or an English summary carries one Chinese
  // word in the middle of it.
  const label = entry.labelKey ? t(entry.labelKey) : entry.label
  const named = entry.verb ? t(entry.verb, { name: label }) : label
  // "王婆子 灵石 +4": another person's number needs its owner, or a trade reads
  // as the same field moving in both directions at once.
  const subject = entry.actor
    ? <><span className="text-[var(--nova-text-faint)]">{entry.actor} </span>{named}</>
    : named

  if (change.kind === 'delta' && change.delta !== null) {
    return (
      <span>
        <span className="text-[var(--nova-text-muted)]">{subject} </span>
        <span className={cn('font-mono tabular-nums', change.tone === 'positive' ? 'text-[var(--story-state-positive)]' : 'text-[var(--story-state-negative)]')}>
          {change.delta >= 0 ? `+${change.delta}` : change.delta}
        </span>
      </span>
    )
  }
  if (change.kind === 'added') {
    return (
      <span className="text-[var(--story-state-positive)]">
        {entry.verb || entry.actor ? subject : t('storyStage.state.digest.gained', { name: label })}
      </span>
    )
  }
  if (change.kind === 'removed') {
    return (
      <span className="text-[var(--story-state-negative)]">
        {entry.actor ? <span className="text-[var(--nova-text-faint)]">{entry.actor} </span> : null}
        {t('storyStage.state.digest.lost', { name: label })}
      </span>
    )
  }
  if (change.kind === 'archived' || change.kind === 'restored') {
    return (
      <span className={change.tone === 'positive' ? 'text-[var(--story-state-positive)]' : 'text-[var(--story-state-negative)]'}>
        <span className="text-[var(--nova-text-muted)]">{subject} </span>
        {change.kind === 'archived' ? t('storyStage.state.change.archived') : t('storyStage.state.change.restored')}
      </span>
    )
  }
  if (change.text) {
    return (
      <span>
        <span className="text-[var(--nova-text-muted)]">{subject} </span>
        <span className="text-[var(--nova-text)]">{change.text}</span>
      </span>
    )
  }
  // A verb already finished the sentence ("抵达青石坊市"). Adding "已更新"
  // after it would read as a second, vaguer fact about the same thing.
  if (entry.verb) return <span className="text-[var(--nova-text)]">{subject}</span>
  return (
    <span>
      <span className="text-[var(--nova-text-muted)]">{subject} </span>
      <span className="text-[var(--nova-text-faint)]">
        {change.kind === 'cleared' ? t('storyStage.state.change.cleared') : t('storyStage.state.change.updated')}
      </span>
    </span>
  )
}
