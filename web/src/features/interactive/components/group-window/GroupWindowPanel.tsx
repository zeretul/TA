import { useWorldPresentation } from '../../world-presentation-context'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranscriptScroll } from '../chat-window/use-transcript-scroll'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, Plus, Send, UserMinus, UserPlus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import type { CharacterExchange, CharacterWindowCharacter, GroupWindowGroup, GroupWindowMember } from '../../types'
import { PersonAvatar } from '../person-avatar'
import { SummaryBlock } from '../summary-block'
import { useGroupWindow } from './use-group-window'

interface GroupWindowPanelProps {
  storyId: string
  branchId: string
  /**
   * The project this story's world belongs to. A member's world-book picture is
   * stored as a project-relative path, so the roster addresses it through this
   * project instead of the read turning it into a URL.
   */
  projectId?: string
  /** True while the panel is on screen. It is what starts the read. */
  active: boolean
  /** The group whose conversation is open, or '' to show the group list. */
  groupId: string
  onSelectGroup: (groupId: string) => void
  onClose: () => void
  /**
   * Hands a plan the group agreed on to the main story's existing send chain.
   * A group can agree; only the story can carry it out.
   */
  onActOnPlan?: (message: string) => void
  windowActions?: ReactNode
  className?: string
  /**
   * True when the chat shell hosts this panel behind its own title bar. The
   * shell already says whose conversation is open and how to go back, so a
   * second header inside it would be two bars claiming the same job.
   */
  embedded?: boolean
  /**
   * The draft belongs to the group being written to and has to outlive the
   * container: closing the window unmounts this panel, and switching
   * conversations in the chat shell must not throw away what was half-written.
   * The host owns it for the same reason it owns which group is open.
   */
  draft: string
  onDraftChange: (value: string) => void
  /**
   * Called after a round or a membership change commits. The chat shell keeps a
   * list of every conversation beside this one, and that list previews the last
   * line — so it has to be told the last line moved.
   */
  onChanged?: () => void
}

/**
 * 群聊: the player's conversation spaces, their real membership, and what was
 * actually said in them.
 *
 * The roster, the contact condition and the transcript all come from the same
 * read, so the window cannot show a member who is not there or a line nobody
 * heard. A group is a place to talk: nothing said here makes anyone act.
 */
export function GroupWindowPanel({ storyId, branchId, projectId, active, groupId, onSelectGroup, onClose, onActOnPlan, windowActions, className, embedded, draft, onDraftChange, onChanged }: GroupWindowPanelProps) {
  const { t } = useTranslation()
  const entry = useGroupWindow({ storyId, branchId, groupId, active })
  const [plan, setPlan] = useState('')
  const [creating, setCreating] = useState(false)
  const { listRef, preserveEarlier } = useTranscriptScroll(JSON.stringify([storyId, branchId, groupId]), entry.data?.exchanges, entry.loadingEarlier)

  useEffect(() => {
    // A plan names an agreement in this group, not whichever group opens next.
    setPlan('')
    setCreating(false)
  }, [storyId, branchId, groupId])

  const groups = entry.data?.groups || []
  const cast = entry.data?.cast || []
  const selected = useMemo(() => groups.find((group) => group.id === groupId) || null, [groupId, groups])
  // The cast names the scene's present list and the roster; participants names
  // whoever spoke, including someone who has since left and is no longer cast.
  const names = useMemo(() => {
    const map: Record<string, string> = {}
    for (const character of cast) {
      map[character.id] = character.known_as || character.name || character.id
    }
    for (const [id, name] of Object.entries(entry.data?.participants || {})) {
      if (!map[id]) map[id] = name
    }
    return map
  }, [cast, entry.data?.participants])
  const participants = names

  const submit = async () => {
    const text = draft.trim()
    if (!text || entry.sending) return
    const sent = await entry.send(text)
    if (sent) {
      onDraftChange('')
      onChanged?.()
    }
  }

  // Membership changes move a group's place in the shell's list too: adding
  // someone writes a line, and a group with a newer line sorts above one
  // without.
  const addMembers = async (ids: string[]) => {
    const done = await entry.addMembers(ids)
    if (done) onChanged?.()
    return done
  }
  const removeMember = async (id: string) => {
    const done = await entry.removeMember(id)
    if (done) onChanged?.()
    return done
  }

  const openGroup = (id: string) => {
    setPlan('')
    onSelectGroup(id)
  }

  const actOnPlan = () => {
    if (!onActOnPlan) return
    const message = plan.trim()
    if (!message) return
    onActOnPlan(message)
  }

  return (
    <div data-testid="group-window-panel" className={cn('flex h-full min-h-0 flex-col bg-[var(--nova-surface)] text-[var(--nova-text)]', className)}>
      {embedded ? null : (
        <header className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2">
          {groupId ? (
            <Button type="button" variant="ghost" size="icon-sm" onClick={() => openGroup('')} aria-label={t('groupWindow.back')} title={t('groupWindow.back')}>
              <ArrowLeft className="h-4 w-4" />
            </Button>
          ) : null}
          <h2 className="min-w-0 flex-1 truncate text-[13px] font-medium">{selected ? selected.name : t('groupWindow.title')}</h2>
          {windowActions}
          <Button type="button" variant="ghost" size="icon-sm" onClick={onClose} aria-label={t('groupWindow.close')} title={t('groupWindow.close')}>
            <X className="h-4 w-4" />
          </Button>
        </header>
      )}

      <SceneBlock scene={entry.data?.scene} />

      {entry.error ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center">
          <p className="text-[12px] text-[var(--nova-danger)]">{t('groupWindow.loadFailed')}</p>
          <p className="text-[11px] text-[var(--nova-text-faint)]">{entry.error}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void entry.reload()}>{t('groupWindow.retry')}</Button>
        </div>
      ) : entry.loading ? (
        <div className="flex flex-1 items-center justify-center gap-2 text-[12px] text-[var(--nova-text-muted)]">
          <Spinner className="h-3.5 w-3.5" />
          {t('groupWindow.loading')}
        </div>
      ) : !groupId ? (
        creating ? (
          <CreateGroupForm
            cast={cast}
            busy={entry.creating}
            error={entry.createError}
            onCancel={() => setCreating(false)}
            onSubmit={async (name, memberIds) => {
              const made = await entry.create(name, memberIds)
              if (made) {
                setCreating(false)
                onChanged?.()
              }
              return Boolean(made)
            }}
          />
        ) : (
          <GroupList groups={groups} onSelect={openGroup} onCreate={() => setCreating(true)} />
        )
      ) : (
        <>
          <RosterBlock group={selected} projectId={projectId} busy={entry.changingMembers} error={entry.memberError} cast={cast} onAdd={addMembers} onRemove={removeMember} />
          <MatterList matters={entry.data?.matters || []} participants={participants} />
          <div ref={listRef} data-testid="group-window-transcript" className="flex min-h-[12rem] flex-1 flex-col gap-2 overflow-y-auto px-3 py-2">
            {entry.hasMore ? (
              <Button type="button" variant="ghost" size="sm" className="self-center text-[11px]" disabled={entry.loadingEarlier} onClick={() => void preserveEarlier(entry.loadEarlier)}>
                {entry.loadingEarlier ? t('groupWindow.loading') : t('groupWindow.loadEarlier')}
              </Button>
            ) : null}
            {(entry.data?.exchanges || []).length === 0 ? (
              <p className="py-6 text-center text-[12px] text-[var(--nova-text-faint)]">{t('groupWindow.noHistory')}</p>
            ) : (entry.data?.exchanges || []).map((exchange) => (
              <GroupLine key={exchange.id} exchange={exchange} participants={participants} />
            ))}
            {entry.sending ? (
              <p className="flex items-center gap-2 self-start text-[11px] text-[var(--nova-text-muted)]">
                <Spinner className="h-3 w-3" />
                {t('groupWindow.sending')}
              </p>
            ) : null}
          </div>
          {entry.silenced.length > 0 ? (
            <p className="shrink-0 px-3 pb-1 text-[11px] text-[var(--nova-text-muted)]">
              {t(selected?.channel === 'message' ? 'groupWindow.remoteSilenced' : 'groupWindow.silenced', { names: entry.silenced.map((id) => participants[id] || id).join('、') })}
            </p>
          ) : null}
          {entry.sendError ? <p className="shrink-0 px-3 pb-1 text-[11px] text-[var(--nova-danger)]">{t('groupWindow.sendFailed')} {entry.sendError}</p> : null}
          {selected && selected.members.some((member) => member.reachable) && onActOnPlan ? (
            <PlanBlock plan={plan} onPlanChange={setPlan} onAct={actOnPlan} />
          ) : null}
          <div className="flex shrink-0 items-end gap-2 border-t border-[var(--nova-border)] px-3 py-2">
            <Textarea
              value={draft}
              onChange={(event) => onDraftChange(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.shiftKey) {
                  event.preventDefault()
                  void submit()
                }
              }}
              minRows={1}
              maxRows={5}
              placeholder={t('groupWindow.inputPlaceholder')}
              className="min-h-8 flex-1 resize-none bg-[var(--nova-surface-2)] text-[12px]"
              disabled={entry.sending}
            />
            <Button type="button" size="icon-sm" className="h-8 w-8" disabled={!draft.trim() || entry.sending} onClick={() => void submit()} aria-label={t('groupWindow.send')} title={t('groupWindow.send')}>
              <Send className="h-4 w-4" />
            </Button>
          </div>
        </>
      )}
    </div>
  )
}

/**
 * The group's scene line: where the story stands and when. It does not list who
 * else is here — a group's members are the only people who know what is said in
 * it, and a "present" roster beside the conversation would read as an audience
 * the group does not have.
 */
function SceneBlock({ scene }: { scene?: { title?: string; location?: string; world_time?: string; present?: string[] } }) {
  const { t } = useTranslation()
  if (!scene || (!scene.location && !scene.world_time)) return null
  return (
    <div className="shrink-0 border-b border-[var(--nova-border)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
      {scene.world_time ? <span>{t('groupWindow.sceneTime')} · {scene.world_time}</span> : null}
      {scene.location ? <span className={cn(scene.world_time && 'ml-2')}>{scene.location}</span> : null}
    </div>
  )
}

function GroupList({ groups, onSelect, onCreate }: { groups: GroupWindowGroup[]; onSelect: (id: string) => void; onCreate: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-2 py-2">
        {groups.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-1 px-6 text-center">
            <p className="text-[12px] text-[var(--nova-text-muted)]">{t('groupWindow.empty')}</p>
            <p className="text-[11px] text-[var(--nova-text-faint)]">{t('groupWindow.emptyHint')}</p>
          </div>
        ) : groups.map((group) => (
          <button
            key={group.id}
            type="button"
            data-testid="group-window-row"
            data-group-id={group.id}
            onClick={() => onSelect(group.id)}
            className="flex w-full flex-col gap-0.5 rounded-[var(--nova-radius)] px-2 py-2 text-left transition-colors hover:bg-[var(--nova-hover)]"
          >
            <span className="flex items-center gap-1.5">
              <span className="min-w-0 flex-1 truncate text-[12px] text-[var(--nova-text)]">{group.name}</span>
              <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">
                {t('groupWindow.reachableCount', { reachable: group.reachable_members, total: group.members.length })}
              </span>
            </span>
            {group.last_exchange ? (
              <span className="truncate text-[11px] text-[var(--nova-text-muted)]">{group.last_exchange}</span>
            ) : (
              <span className="truncate text-[11px] text-[var(--nova-text-faint)]">{t('groupWindow.noHistoryShort')}</span>
            )}
          </button>
        ))}
      </div>
      <div className="shrink-0 border-t border-[var(--nova-border)] px-3 py-2">
        <Button type="button" variant="outline" size="sm" className="h-7 w-full gap-1.5 text-[11px]" onClick={onCreate}>
          <Plus className="h-3.5 w-3.5" />
          {t('groupWindow.create')}
        </Button>
      </div>
    </div>
  )
}

/**
 * Building a group from people the story already knows.
 *
 * The list is the story's own cast, so the player cannot name a stranger into
 * existence. The group is only a place to talk: creating it does not make
 * anyone a party member or give the player a rank.
 */
export function CreateGroupForm({ cast, busy, error, onCancel, onSubmit }: {
  cast: CharacterWindowCharacter[]
  busy: boolean
  error: string | null
  onCancel: () => void
  onSubmit: (name: string, memberIds: string[]) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const { communication } = useWorldPresentation()
  const [name, setName] = useState('')
  const [picked, setPicked] = useState<string[]>([])
  const toggle = (id: string) => {
    setPicked((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id])
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-3 py-2">
        <input
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder={t('groupWindow.namePlaceholder')}
          className="h-8 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[12px] text-[var(--nova-text)] outline-none"
          maxLength={40}
        />
        <p className="text-[11px] text-[var(--nova-text-faint)]">{t(communication === 'radio' ? 'groupWindow.radioPickHint' : 'groupWindow.pickHint')}</p>
        {cast.length === 0 ? (
          <p className="py-4 text-center text-[11px] text-[var(--nova-text-faint)]">{t('groupWindow.noCast')}</p>
        ) : cast.map((character) => {
          const checked = picked.includes(character.id)
          return (
            <button
              key={character.id}
              type="button"
              data-testid="group-window-cast-row"
              data-character-id={character.id}
              aria-pressed={checked}
              onClick={() => toggle(character.id)}
              className={cn(
                'flex w-full items-center gap-2 rounded-[var(--nova-radius)] border px-2 py-1.5 text-left text-[12px]',
                checked
                  ? 'border-[var(--nova-accent)]/40 bg-[var(--nova-accent)]/10 text-[var(--nova-text)]'
                  : 'border-[var(--nova-border)] bg-transparent text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)]',
              )}
            >
              <span className="min-w-0 flex-1 truncate">{character.known_as || character.name || character.id}</span>
            </button>
          )
        })}
        {error ? <p className="text-[11px] text-[var(--nova-danger)]">{error}</p> : null}
      </div>
      <div className="flex shrink-0 gap-2 border-t border-[var(--nova-border)] px-3 py-2">
        <Button type="button" variant="ghost" size="sm" className="h-7 flex-1 text-[11px]" onClick={onCancel} disabled={busy}>{t('groupWindow.cancel')}</Button>
        <Button
          type="button"
          size="sm"
          className="h-7 flex-1 text-[11px]"
          disabled={busy || !name.trim() || picked.length === 0}
          onClick={() => void onSubmit(name, picked)}
        >
          {busy ? t('groupWindow.creating') : t('groupWindow.createConfirm')}
        </Button>
      </div>
    </div>
  )
}

/**
 * The group's roster, with who can hear right now.
 *
 * A member who is in the group but not reachable is still listed. Saying so is
 * the point: the player is told who is out of earshot rather than being shown a
 * group that quietly pretends everyone is present.
 */
function RosterBlock({ group, projectId, busy, error, cast, onAdd, onRemove }: {
  group: GroupWindowGroup | null
  projectId?: string
  busy: boolean
  error: string | null
  cast: CharacterWindowCharacter[]
  onAdd: (ids: string[]) => Promise<boolean>
  onRemove: (id: string) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const [adding, setAdding] = useState(false)
  if (!group) return null
  const memberIDs = new Set(group.members.map((member) => member.id))
  const candidates = cast.filter((character) => !memberIDs.has(character.id))
  const reachable = group.members.filter((member) => member.reachable).length
  const names = group.members.map((member) => member.name || member.id).join('、')
  return (
    <SummaryBlock
      label={t('groupWindow.members')}
      count={group.members.length}
      summary={t(group.channel === 'message' ? 'groupWindow.remoteMembersSummary' : 'groupWindow.membersSummary', { names, reachable })}
      action={(
        <button
          type="button"
          className="flex shrink-0 items-center gap-0.5 text-[10px] text-[var(--nova-text-muted)] hover:underline"
          onClick={() => setAdding((current) => !current)}
        >
          <UserPlus className="h-3 w-3" />
          {t('groupWindow.addMember')}
        </button>
      )}
    >
      <ul className="flex flex-col gap-0.5">
        {group.members.map((member) => (
          <MemberRow key={member.id} member={member} remote={group.channel === 'message'} projectId={projectId} busy={busy} onRemove={onRemove} />
        ))}
      </ul>
      {adding ? (
        <div className="mt-1.5 flex flex-col gap-1 border-t border-[var(--nova-border)] pt-1.5">
          {candidates.length === 0 ? (
            <p className="text-[10px] text-[var(--nova-text-faint)]">{t('groupWindow.noMoreCandidates')}</p>
          ) : candidates.map((character) => (
            <button
              key={character.id}
              type="button"
              disabled={busy}
              onClick={() => void onAdd([character.id])}
              className="flex items-center justify-between gap-2 rounded-[var(--nova-radius)] px-1.5 py-1 text-left text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)]"
            >
              <span className="truncate">{character.known_as || character.name || character.id}</span>
              <Plus className="h-3 w-3 shrink-0" />
            </button>
          ))}
        </div>
      ) : null}
      {error ? <p className="mt-1 text-[10px] text-[var(--nova-danger)]">{error}</p> : null}
    </SummaryBlock>
  )
}

function MemberRow({ member, remote, projectId, busy, onRemove }: { member: GroupWindowMember; remote: boolean; projectId?: string; busy: boolean; onRemove: (id: string) => Promise<boolean> }) {
  const { t } = useTranslation()
  return (
    <li className="group flex items-center gap-1.5 text-[11px]">
      <PersonAvatar
        avatar={member.avatar}
        lorePath={member.avatar_lore_path}
        projectId={projectId}
        name={member.name || member.id}
        size="xs"
      />
      <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full', member.reachable ? 'bg-[var(--nova-accent-green)]' : 'bg-[var(--nova-border)]')} aria-hidden="true" />
      <span className="min-w-0 flex-1 truncate text-[var(--nova-text-muted)]">{member.name || member.id}</span>
      {!member.reachable ? (
        <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">{t(remote ? 'groupWindow.disconnected' : 'groupWindow.outOfEarshot')}</span>
      ) : null}
      <button
        type="button"
        disabled={busy}
        aria-label={t('groupWindow.removeMember', { name: member.name || member.id })}
        title={t('groupWindow.removeMember', { name: member.name || member.id })}
        className="shrink-0 text-[var(--nova-text-faint)] opacity-0 transition-opacity hover:text-[var(--nova-danger)] group-hover:opacity-100"
        onClick={() => void onRemove(member.id)}
      >
        <UserMinus className="h-3 w-3" />
      </button>
    </li>
  )
}

function MatterList({ matters, participants }: { matters: { id: string; character_id: string; kind: string; statement: string; deadline?: string }[]; participants: Record<string, string> }) {
  const { t } = useTranslation()
  if (matters.length === 0) return null
  // The closed line names who settled the first one and what it was, so the block
  // still carries its content while collapsed.
  const first = matters[0]
  const summary = `${participants[first.character_id] || first.character_id} · ${first.statement}`
  return (
    <SummaryBlock label={t('groupWindow.matters')} count={matters.length} summary={summary}>
      <ul className="flex flex-col gap-1">
        {matters.map((matter) => (
          <li key={matter.id} className="flex items-start gap-1.5 text-[11px] text-[var(--nova-text-muted)]">
            <span className="shrink-0 text-[var(--nova-text-faint)]">{participants[matter.character_id] || matter.character_id}</span>
            <span className="min-w-0 flex-1">{matter.statement}</span>
            {matter.deadline ? <span className="shrink-0 text-[var(--nova-text-faint)]">{matter.deadline}</span> : null}
          </li>
        ))}
      </ul>
    </SummaryBlock>
  )
}

/**
 * What the player wants the group to do next.
 *
 * It is an occasional action rather than part of reading the conversation, so it
 * stays folded to a single line until the player opens it. Group windows were
 * losing their messages to the roster, the arrangements and this field together.
 */
function PlanBlock({ plan, onPlanChange, onAct }: { plan: string; onPlanChange: (value: string) => void; onAct: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] py-1.5">
      <SummaryBlock
        label={t('groupWindow.planTitle')}
        count={plan.trim() ? 1 : 0}
        summary={plan.trim() || t('groupWindow.planHint')}
        className="border-b-0 px-3"
      >
        <Textarea
          value={plan}
          onChange={(event) => onPlanChange(event.target.value)}
          minRows={1}
          maxRows={3}
          placeholder={t('groupWindow.planPlaceholder')}
          className="mb-1.5 min-h-8 resize-none bg-[var(--nova-surface)] text-[12px]"
        />
        <Button type="button" size="sm" className="h-7 w-full text-[11px]" disabled={!plan.trim()} onClick={onAct}>
          {t('groupWindow.planAct')}
        </Button>
      </SummaryBlock>
    </div>
  )
}

/** One line in the group, attributed to the person who actually said it. */
function GroupLine({ exchange, participants }: { exchange: CharacterExchange; participants: Record<string, string> }) {
  const { t } = useTranslation()
  const mine = exchange.speaker === 'player'
  const author = exchange.author_id || exchange.character_id
  const name = mine ? t('groupWindow.speaker.you') : (participants[author] || author || t('groupWindow.speaker.unknown'))
  return (
    <div className={cn('flex flex-col gap-0.5', mine ? 'items-end' : 'items-start')}>
      <span className="text-[10px] text-[var(--nova-text-faint)]">
        {name}
        {exchange.world_time ? ` · ${exchange.world_time}` : ''}
      </span>
      <p
        className={cn(
          'max-w-[92%] whitespace-pre-wrap break-words rounded-[var(--nova-radius)] border px-2 py-1.5 text-[12px] leading-relaxed',
          mine
            ? 'border-[var(--nova-user-message-border)] bg-[var(--nova-user-message-bg-from)] text-[var(--nova-user-message-text)]'
            : 'border-[var(--nova-assistant-message-border)] bg-[var(--nova-assistant-message-bg)] text-[var(--nova-text)]',
        )}
      >
        {exchange.text}
      </p>
    </div>
  )
}
