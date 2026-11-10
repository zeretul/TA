import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, Plus, Search, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import type { CharacterWindowCharacter, GroupWindowGroup } from '../../types'
import { PersonAvatar } from '../person-avatar'
import { CharacterWindowPanel } from '../character-window/CharacterWindowPanel'
import { useCharacterWindow } from '../character-window/use-character-window'
import { CreateGroupForm, GroupWindowPanel } from '../group-window/GroupWindowPanel'
import { useGroupWindow } from '../group-window/use-group-window'

/** Which conversation the player is reading, if any. */
export interface ChatTarget {
  kind: 'person' | 'group'
  id: string
}

/**
 * One row of the conversation list, whichever kind of conversation it is.
 *
 * A private chat and a group are two different records with two different
 * services, but a player looking for "where was that conversation" does not
 * think in those terms. The row is the single shape the list sorts and draws.
 */
interface ConversationRow {
  kind: 'person' | 'group'
  id: string
  name: string
  preview: string
  worldTime: string
  unread: number
  /** Where the last line sits in the branch journal, so the two kinds compare. */
  sequence: number
  person?: CharacterWindowCharacter
  group?: GroupWindowGroup
}

interface ChatWindowPanelProps {
  storyId: string
  branchId: string
  projectId?: string
  /** True while the window is on screen. It is what starts the reads. */
  active: boolean
  /**
   * The conversation on screen, or null for the list alone. The stage owns it
   * because an incoming message elsewhere in the game has to be able to open
   * the chat straight onto the person who sent it.
   */
  target: ChatTarget | null
  onTargetChange: (target: ChatTarget | null) => void
  onClose: () => void
  /** Hands an arrangement or a group plan to the story's existing send chain. */
  onActOnPlan?: (message: string) => void
  /** Carries a conversation already on record into the story without repeating it. */
  onContinueStory?: (message: string, playerLetter?: { character_id: string; body: string }) => void
  /** Opens a person's profile without leaving the chat behind. */
  onOpenProfile?: (characterId: string) => void
  /**
   * Half-written messages, keyed by conversation. The stage owns them: closing
   * the window or stepping into another conversation must not throw away what
   * the player had typed.
   */
  drafts: Record<string, string>
  onDraftChange: (key: string, value: string) => void
  windowActions?: ReactNode
  className?: string
}

export function draftKey(kind: 'person' | 'group', id: string): string {
  return `${kind}:${id}`
}

/**
 * 聊天: every conversation this story has, in one window.
 *
 * It is a shell, not a second chat system. The list is read from the same two
 * reads the person window and the group window already make, and the open
 * conversation is the very panel that used to stand alone — so a message sent
 * here is the same message, stored in the same place, and nothing is copied
 * into a parallel history.
 *
 * Wide, the list and the conversation sit side by side; narrow, they take turns
 * with a way back. Selecting a conversation only changes what is being read: it
 * costs no world time and creates no turn.
 */
export function ChatWindowPanel({
  storyId, branchId, projectId, active, target, onTargetChange, onClose, onActOnPlan, onContinueStory, onOpenProfile, drafts, onDraftChange, windowActions, className,
}: ChatWindowPanelProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [composing, setComposing] = useState<'none' | 'menu' | 'person' | 'group'>('none')

  // The list is a plain read of both windows with no conversation open, which is
  // exactly what those reads already return: the cast, and the groups.
  const people = useCharacterWindow({ storyId, branchId, characterId: '', active })
  const groups = useGroupWindow({ storyId, branchId, groupId: '', active })

  const cast = people.data?.characters || []

  const rows = useMemo(() => {
    const list: ConversationRow[] = []
    for (const character of cast) {
      list.push({
        kind: 'person',
        id: character.id,
        name: character.known_as || character.name || character.id,
        preview: character.last_exchange || '',
        worldTime: character.last_world_time || '',
        unread: character.unread_messages || 0,
        sequence: character.last_sequence || 0,
        person: character,
      })
    }
    for (const group of groups.data?.groups || []) {
      list.push({
        kind: 'group',
        id: group.id,
        name: group.name,
        preview: group.last_exchange || '',
        worldTime: group.last_world_time || '',
        unread: 0,
        sequence: group.last_sequence || 0,
        group,
      })
    }
    // Most recent first, and a conversation nobody has spoken in yet sorts below
    // every one that has: silence is not newer than a line.
    list.sort((left, right) => {
      if (left.sequence !== right.sequence) return right.sequence - left.sequence
      return left.name.localeCompare(right.name)
    })
    return list
  }, [cast, groups.data?.groups])

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return rows
    return rows.filter((row) => row.name.toLowerCase().includes(needle) || row.preview.toLowerCase().includes(needle))
  }, [query, rows])

  const loading = people.loading || groups.loading
  const error = people.error || groups.error

  const open = target ? rows.find((row) => row.kind === target.kind && row.id === target.id) || null : null
  const openKey = target ? draftKey(target.kind, target.id) : ''
  const openName = open?.name || ''

  // A line sent from the open conversation changes what the list should preview,
  // but the list has its own read and would otherwise keep showing the previous
  // last line until the window reopened.
  const refreshList = () => {
    void people.reload()
    void groups.reload()
  }

  const select = (next: ChatTarget) => {
    setComposing('none')
    setQuery('')
    onTargetChange(next)
  }

  return (
    <div
      data-testid="chat-window-panel"
      className={cn('@container flex h-full min-h-0 flex-col bg-[var(--nova-surface)] text-[var(--nova-text)]', className)}
    >
      <header className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2">
        {/* Narrow, the list and the conversation take turns, so the way back has
            to be where the title is. Wide, both are on screen and the button
            would lead somewhere already visible. */}
        {target ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="@lg:hidden"
            onClick={() => onTargetChange(null)}
            aria-label={t('chatWindow.back')}
            title={t('chatWindow.back')}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
        ) : null}
        <h2 className="min-w-0 flex-1 truncate text-[13px] font-medium">{open ? openName : t('chatWindow.title')}</h2>
        {open && open.kind === 'person' && onOpenProfile ? (
          <Button type="button" variant="ghost" size="sm" className="h-6 shrink-0 px-1.5 text-[11px]" onClick={() => onOpenProfile(open.id)}>
            {t('chatWindow.viewProfile')}
          </Button>
        ) : null}
        {windowActions}
        <Button type="button" variant="ghost" size="icon-sm" onClick={onClose} aria-label={t('chatWindow.close')} title={t('chatWindow.close')}>
          <X className="h-4 w-4" />
        </Button>
      </header>

      <div className="flex min-h-0 flex-1">
        {/* The list. Wide it is a fixed column beside the conversation; narrow it
            is the whole window until one is chosen. */}
        <div
          className={cn(
            'min-h-0 w-full flex-col border-[var(--nova-border)] @lg:w-[248px] @lg:shrink-0 @lg:border-r',
            target ? 'hidden @lg:flex' : 'flex',
          )}
        >
          <div className="flex shrink-0 items-center gap-1.5 border-b border-[var(--nova-border)] px-2 py-1.5">
            <div className="relative min-w-0 flex-1">
              <Search aria-hidden className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--nova-text-faint)]" />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t('chatWindow.searchPlaceholder')}
                aria-label={t('chatWindow.searchPlaceholder')}
                className="h-7 w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] pl-7 pr-2 text-[11px] text-[var(--nova-text)] outline-none placeholder:text-[var(--nova-text-faint)]"
              />
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t('chatWindow.newChat')}
              title={t('chatWindow.newChat')}
              aria-expanded={composing !== 'none'}
              onClick={() => setComposing((current) => (current === 'none' ? 'menu' : 'none'))}
            >
              <Plus className="h-4 w-4" />
            </Button>
          </div>

          {composing === 'menu' ? (
            <div className="flex shrink-0 flex-col gap-1 border-b border-[var(--nova-border)] px-2 py-1.5">
              <p className="px-0.5 text-[10px] text-[var(--nova-text-faint)]">{t('chatWindow.newChatHint')}</p>
              <Button type="button" variant="outline" size="sm" className="h-7 justify-start text-[11px]" onClick={() => setComposing('person')}>
                {t('chatWindow.startPerson')}
              </Button>
              <Button type="button" variant="outline" size="sm" className="h-7 justify-start text-[11px]" onClick={() => setComposing('group')}>
                {t('chatWindow.createGroup')}
              </Button>
            </div>
          ) : null}

          {composing === 'person' ? (
            <PersonPicker
              characters={cast}
              projectId={projectId}
              onPick={(characterId) => select({ kind: 'person', id: characterId })}
              onCancel={() => setComposing('none')}
            />
          ) : composing === 'group' ? (
            <CreateGroupForm
              cast={cast}
              busy={groups.creating}
              error={groups.createError}
              onCancel={() => setComposing('none')}
              onSubmit={async (name, memberIds) => {
                const made = await groups.create(name, memberIds)
                if (!made) return false
                select({ kind: 'group', id: made.id })
                return true
              }}
            />
          ) : error ? (
            <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center">
              <p className="text-[12px] text-[var(--nova-danger)]">{t('chatWindow.loadFailed')}</p>
              <p className="text-[11px] text-[var(--nova-text-faint)]">{error}</p>
              <Button type="button" variant="outline" size="sm" onClick={refreshList}>{t('chatWindow.retry')}</Button>
            </div>
          ) : loading ? (
            <div className="flex flex-1 items-center justify-center gap-2 text-[12px] text-[var(--nova-text-muted)]">
              <Spinner className="h-3.5 w-3.5" />
              {t('chatWindow.loading')}
            </div>
          ) : visible.length === 0 ? (
            <div className="flex flex-1 flex-col items-center justify-center gap-1 px-6 text-center">
              <p className="text-[12px] text-[var(--nova-text-muted)]">{query.trim() ? t('chatWindow.searchEmpty') : t('chatWindow.empty')}</p>
              {query.trim() ? null : <p className="text-[11px] text-[var(--nova-text-faint)]">{t('chatWindow.emptyHint')}</p>}
            </div>
          ) : (
            <ul className="flex min-h-0 flex-1 flex-col overflow-y-auto py-1" aria-label={t('chatWindow.listLabel')}>
              {visible.map((row) => (
                <li key={draftKey(row.kind, row.id)}>
                  <ConversationRowButton
                    row={row}
                    projectId={projectId}
                    selected={target?.kind === row.kind && target.id === row.id}
                    onSelect={() => select({ kind: row.kind, id: row.id })}
                  />
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* The conversation. It is the existing panel, hosted rather than
            reimplemented, so both kinds keep one reading surface and one set of
            services. */}
        <div className={cn('min-h-0 min-w-0 flex-1', target ? 'flex flex-col' : 'hidden @lg:flex @lg:flex-col')}>
          {target?.kind === 'person' ? (
            <CharacterWindowPanel
              storyId={storyId}
              branchId={branchId}
              projectId={projectId}
              active={active}
              characterId={target.id}
              onSelectCharacter={(id) => onTargetChange(id ? { kind: 'person', id } : null)}
              onClose={onClose}
              onActOnPlan={onActOnPlan}
              onContinueStory={onContinueStory}
              draft={drafts[openKey] || ''}
              onDraftChange={(value) => onDraftChange(openKey, value)}
              onChanged={refreshList}
              embedded
            />
          ) : target?.kind === 'group' ? (
            <GroupWindowPanel
              storyId={storyId}
              branchId={branchId}
              projectId={projectId}
              active={active}
              groupId={target.id}
              onSelectGroup={(id) => onTargetChange(id ? { kind: 'group', id } : null)}
              onClose={onClose}
              onActOnPlan={onActOnPlan}
              draft={drafts[openKey] || ''}
              onDraftChange={(value) => onDraftChange(openKey, value)}
              onChanged={refreshList}
              embedded
            />
          ) : (
            <div className="flex flex-1 flex-col items-center justify-center gap-1 px-6 text-center">
              <p className="text-[12px] text-[var(--nova-text-muted)]">{t('chatWindow.emptyConversation')}</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

/**
 * One conversation in the list: who, the last thing said, when, and how much of
 * it the player has not looked at.
 *
 * The time is the story's own clock, never a wall-clock date. A world where a
 * day has no fixed length has no "yesterday" to print, and a real date would
 * claim a calendar the world never agreed to.
 */
function ConversationRowButton({ row, projectId, selected, onSelect }: {
  row: ConversationRow
  projectId?: string
  selected: boolean
  onSelect: () => void
}) {
  const { t } = useTranslation()
  const preview = row.preview || (row.kind === 'group' ? t('chatWindow.noPreviewGroup') : t('chatWindow.noPreview'))
  return (
    <button
      type="button"
      data-testid="chat-window-row"
      data-chat-kind={row.kind}
      data-chat-id={row.id}
      aria-current={selected ? 'true' : undefined}
      onClick={onSelect}
      className={cn(
        'flex w-full items-start gap-2 px-2.5 py-2 text-left transition-colors',
        selected ? 'bg-[var(--nova-accent)]/10' : 'hover:bg-[var(--nova-hover)]',
      )}
    >
      {row.kind === 'person' ? (
        <PersonAvatar
          avatar={row.person?.avatar}
          lorePath={row.person?.avatar_lore_path}
          projectId={projectId}
          name={row.name}
          size="md"
        />
      ) : (
        <span
          aria-hidden="true"
          className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[13px] text-[var(--nova-text-muted)]"
        >
          {row.name.trim().slice(0, 1) || '#'}
        </span>
      )}
      <span className="min-w-0 flex-1">
        <span className="flex items-baseline gap-1.5">
          <span className="min-w-0 flex-1 truncate text-[12px] text-[var(--nova-text)]">{row.name}</span>
          {row.worldTime ? <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">{row.worldTime}</span> : null}
        </span>
        <span className="mt-0.5 flex items-center gap-1.5">
          <span className={cn('min-w-0 flex-1 truncate text-[11px]', row.preview ? 'text-[var(--nova-text-muted)]' : 'text-[var(--nova-text-faint)]')}>
            {row.kind === 'group' ? `${t('chatWindow.groupMembers', { count: row.group?.members.length || 0 })} · ` : ''}
            {preview}
          </span>
          {row.unread > 0 ? (
            <span className="flex h-4 min-w-4 shrink-0 items-center justify-center rounded-full bg-[var(--nova-accent)] px-1 text-[10px] font-medium text-[var(--nova-accent-foreground,white)]">
              {row.unread}
            </span>
          ) : null}
        </span>
      </span>
    </button>
  )
}

/**
 * Who to start talking to.
 *
 * It lists the story's own cast and nothing else: the author's catalogue of a
 * hundred people is not a contact list, and offering an uncontacted stranger
 * would be inventing an acquaintance the story never made.
 */
function PersonPicker({ characters, projectId, onPick, onCancel }: {
  characters: CharacterWindowCharacter[]
  projectId?: string
  onPick: (characterId: string) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 items-center gap-1.5 border-b border-[var(--nova-border)] px-2.5 py-1.5">
        <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--nova-text-muted)]">{t('chatWindow.pickPersonHint')}</span>
        <Button type="button" variant="ghost" size="sm" className="h-6 shrink-0 px-1.5 text-[11px]" onClick={onCancel}>
          {t('groupWindow.cancel')}
        </Button>
      </div>
      {characters.length === 0 ? (
        <p className="px-3 py-4 text-center text-[11px] text-[var(--nova-text-faint)]">{t('chatWindow.noCast')}</p>
      ) : (
        <ul className="flex min-h-0 flex-1 flex-col overflow-y-auto py-1">
          {characters.map((character) => {
            const name = character.known_as || character.name || character.id
            return (
              <li key={character.id}>
                <button
                  type="button"
                  data-testid="chat-window-pick-row"
                  data-character-id={character.id}
                  onClick={() => onPick(character.id)}
                  className="flex w-full items-center gap-2 px-2.5 py-1.5 text-left transition-colors hover:bg-[var(--nova-hover)]"
                >
                  <PersonAvatar
                    avatar={character.avatar}
                    lorePath={character.avatar_lore_path}
                    projectId={projectId}
                    name={name}
                    size="sm"
                  />
                  <span className="min-w-0 flex-1 truncate text-[12px] text-[var(--nova-text)]">{name}</span>
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
