import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, HeartHandshake, Send } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import type {
  CharacterExchange, CharacterMediaClip, CharacterWindowCharacter, IntimacyInvitationThread, IntimacyResponseStatus,
  IntimacyWindowActivity,
} from '../../types'
import { useIntimacyWindow } from './use-intimacy-window'
import { IntimacyModelLine } from './IntimacyModelLine'
import { PersonAvatar, personAvatarSource } from '../person-avatar'
import { PersonClip } from '../character-window/PersonClip'

interface IntimacySurfaceProps {
  storyId: string
  branchId: string
  /**
   * The project this story's world belongs to. A person's world-book picture is
   * stored as a project-relative path, so the roster addresses it through this
   * project instead of the read turning it into a URL.
   */
  projectId?: string
  /** True while the container that hosts this surface is on screen. */
  active: boolean
  /**
   * The person whose chat hosts this surface, when it is a person's chat. The
   * surface then reads and invites for that one person, and an invitation is the
   * one-to-one kind. Empty in a group chat, where the roster is multi-person.
   */
  focusCharacterId?: string
  /**
   * Hands the fact that this happened to the main story's existing send chain.
   * The activity is recorded here; carrying it on is a story turn, not a second
   * action path, and nothing about cultivation is granted by this surface.
   */
  onContinueStory?: (message: string) => void
  className?: string
}

const RESPONSE_STATUSES: IntimacyResponseStatus[] = ['pending', 'accepted', 'declined', 'conditional']

/**
 * The intimacy activity itself: who is taking part, what each person answered,
 * what has been said, and the few operations that actually change it.
 *
 * T27 gives it a window of its own. It is still not a second chat platform — it
 * reads and writes the same journal the rest of the game uses, and it grants
 * nothing: no cultivation, no recovery, no breakthrough. Ordinary conversation
 * stays in the chat window; this window is only about this one activity.
 *
 * Being open is not consent and it does not end anything: a pending invitation
 * stays pending, a running activity stays running, and only an invitation, an
 * answer or an explicit end changes what is recorded. Closing or minimizing the
 * window is likewise a display act, so it never ends or consumes an activity.
 */
export function IntimacySurface({ storyId, branchId, projectId, active, focusCharacterId = '', onContinueStory, className }: IntimacySurfaceProps) {
  const { t } = useTranslation()
  // The read follows the activity the player is looking at. Empty reads the
  // branch's list, which is what the surface opens on before it knows which
  // activity this container is about.
  const [activityId, setActivityId] = useState('')
  // The list is the player's own choice, so arriving on it must not be undone by
  // the automatic opening below.
  const [browsing, setBrowsing] = useState(false)
  const [draft, setDraft] = useState('')
  const listRef = useRef<HTMLDivElement | null>(null)

  const entry = useIntimacyWindow({ storyId, branchId, activityId, active })

  const all = entry.data?.activities || []
  const scoped = useMemo(
    () => (focusCharacterId
      ? all.filter((activity) => (activity.participants || []).some((participant) => participant.character_id === focusCharacterId))
      : all),
    [all, focusCharacterId],
  )

  // A person's chat is about that person, so it opens on their own activity
  // instead of asking them to pick from a list. A group chat opens on the
  // newest running activity, which is the one the group is actually in.
  useEffect(() => {
    if (!active || browsing || activityId) return
    const open = scoped.filter((activity) => activity.phase !== 'ended')
    const newest = open.length > 0 ? open[open.length - 1] : scoped[scoped.length - 1]
    if (newest) setActivityId(newest.id)
  }, [active, activityId, browsing, scoped])

  const selected = useMemo(
    () => scoped.find((activity) => activity.id === activityId) || null,
    [activityId, scoped],
  )

  const cast = entry.data?.cast || []
  const names = useMemo(() => {
    const map: Record<string, string> = {}
    for (const character of cast) map[character.id] = knownName(character)
    for (const [id, name] of Object.entries(entry.data?.participants || {})) if (!map[id]) map[id] = name
    for (const [id, name] of Object.entries(selected?.participant_names || {})) if (!map[id]) map[id] = name
    return map
  }, [cast, entry.data?.participants, selected?.participant_names])
  const nameOf = (id: string) => names[id] || id
  // The roster draws the same picture the character window does, out of the same
  // cast read, so a person does not change face between the two views.
  const avatarOf = (id: string) => cast.find((character) => character.id === id)?.avatar || ''
  const lorePathOf = (id: string) => cast.find((character) => character.id === id)?.avatar_lore_path || ''

  // A sent exchange lands at the bottom; following it is the whole point.
  useEffect(() => {
    if (!entry.data?.exchanges?.length) return
    const node = listRef.current
    if (node) node.scrollTop = node.scrollHeight
  }, [entry.data?.exchanges?.length, activityId])

  useEffect(() => { setDraft('') }, [activityId])

  const eligible = entry.data?.eligible || []
  const eligibleCast = useMemo(
    () => eligible.map((id) => cast.find((character) => character.id === id)).filter((character): character is CharacterWindowCharacter => Boolean(character)),
    [cast, eligible],
  )
  // In a person's chat the only person to invite is that person, and only when
  // their own authored data says they are an adult. A group chat offers whoever
  // is eligible.
  const inviteTargets = focusCharacterId
    ? eligibleCast.filter((character) => character.id === focusCharacterId)
    : eligibleCast

  const openActivity = selected && (selected.phase === 'active' || selected.phase === 'ended') ? selected : null
  const inProgress = openActivity?.phase === 'active'

  const submit = async () => {
    const text = draft.trim()
    if (!text || entry.sending || !inProgress) return
    const sent = await entry.send(text)
    if (sent) setDraft('')
  }

  const invite = async (characterId: string) => {
    // A window opened for one person asks that person and enters directly when
    // they agree; the window's own entry builds a roster the player then
    // confirms. Which one it is comes from how the window was opened, not from
    // anything the player has to choose again here.
    const id = await entry.invite(characterId, Boolean(focusCharacterId))
    if (id) {
      setBrowsing(false)
      setActivityId(id)
    }
  }

  const continueStory = () => {
    if (!onContinueStory) return
    const roster = (selected?.roster || []).map(nameOf)
    onContinueStory(t('intimacyWindow.returnStoryMessage', { name: roster.join('、') }))
  }

  const model = entry.data?.model

  return (
    <div data-testid="intimacy-surface" className={cn('flex min-h-0 flex-1 flex-col', className)}>
      {model ? <IntimacyModelLine model={model} /> : null}

      {entry.error ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center">
          <p className="text-[12px] text-[var(--nova-danger)]">{t('intimacyWindow.loadFailed')}</p>
          <p className="text-[11px] text-[var(--nova-text-faint)]">{entry.error}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void entry.reload()}>{t('intimacyWindow.retry')}</Button>
        </div>
      ) : entry.loading ? (
        <div className="flex flex-1 items-center justify-center gap-2 text-[12px] text-[var(--nova-text-muted)]">
          <Spinner className="h-3.5 w-3.5" />
          {t('intimacyWindow.loading')}
        </div>
      ) : !selected ? (
        <ActivityList
          activities={scoped}
          nameOf={nameOf}
          onSelect={(id) => { setBrowsing(false); setActivityId(id) }}
          inviteTargets={inviteTargets}
          inviting={entry.inviting}
          invitingCharacterID={entry.invitingCharacterID}
          onInvite={(id) => void invite(id)}
          modelConfigured={Boolean(model?.configured)}
          projectId={projectId}
          emptyHint={focusCharacterId ? t('intimacyWindow.invite.emptyHint') : undefined}
        />
      ) : (
        <>
          <div className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3 py-1.5">
            <PhaseBadge phase={selected.phase} />
            <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--nova-text-faint)]">
              {t('intimacyWindow.roster.count', { count: (selected.roster || []).length, total: (selected.roster || []).length + 1 })}
            </span>
            <button
              type="button"
              className="shrink-0 text-[10px] text-[var(--nova-text-muted)] underline-offset-2 hover:underline"
              onClick={() => { setBrowsing(true); setActivityId('') }}
            >
              {t('intimacyWindow.back')}
            </button>
          </div>
          <RosterBlock
            activity={selected}
            nameOf={nameOf}
            avatarOf={avatarOf}
            lorePathOf={lorePathOf}
            rosterClips={entry.data?.roster_clips}
            projectId={projectId}
          />
          {selected.phase !== 'ended' ? (
            <InviteBlock
              targets={inviteTargets}
              invited={new Set((selected.participants || []).map((participant) => participant.character_id))}
              inviting={entry.inviting}
              invitingCharacterID={entry.invitingCharacterID}
              onInvite={(id) => void invite(id)}
              disabled={!model?.configured}
              projectId={projectId}
            />
          ) : null}
          {entry.inviteError ? <p className="px-3 pb-1 text-[11px] text-[var(--nova-danger)]">{t('intimacyWindow.invite.failed')} {entry.inviteError}</p> : null}

          <div ref={listRef} data-testid="intimacy-surface-transcript" className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-3 py-2">
            {(entry.data?.exchanges || []).length === 0 ? (
              <p className="py-6 text-center text-[12px] text-[var(--nova-text-faint)]">
                {selected.phase === 'inviting' || selected.phase === 'ready'
                  ? t('intimacyWindow.roster.awaiting')
                  : t('intimacyWindow.noHistory')}
              </p>
            ) : (entry.data?.exchanges || []).map((exchange) => (
              <ExchangeRow key={exchange.id} exchange={exchange} nameOf={nameOf} />
            ))}
            {entry.sending ? (
              <p className="flex items-center gap-2 self-start text-[11px] text-[var(--nova-text-muted)]">
                <Spinner className="h-3 w-3" />
                {t('intimacyWindow.sending')}
              </p>
            ) : null}
          </div>

          {(entry.data?.invitations || []).map((thread) => (
            <InvitationThread
              key={thread.character_id}
              thread={thread}
              nameOf={nameOf}
              busy={entry.inviting && entry.invitingCharacterID === thread.character_id}
              disabled={entry.inviting}
              onAnswer={() => void entry.askAgain(thread.character_id)}
            />
          ))}

          {entry.sendError ? <p className="px-3 pb-1 text-[11px] text-[var(--nova-danger)]">{t('intimacyWindow.replyFailed')} {entry.sendError}</p> : null}
          {entry.startError ? <p className="px-3 pb-1 text-[11px] text-[var(--nova-danger)]">{t('intimacyWindow.startFailed')} {entry.startError}</p> : null}
          {entry.endError ? <p className="px-3 pb-1 text-[11px] text-[var(--nova-danger)]">{entry.endError}</p> : null}

          {selected.phase === 'inviting' || selected.phase === 'ready' ? (
            <div className="shrink-0 border-t border-[var(--nova-border)] px-3 py-2">
              <p className="mb-1.5 text-[11px] text-[var(--nova-text-faint)]">{t('intimacyWindow.roster.awaiting')}</p>
              <div className="flex gap-2">
                <Button
                  type="button"
                  size="sm"
                  className="h-7 flex-1 text-[11px]"
                  disabled={entry.starting || (selected.roster || []).length === 0}
                  onClick={() => void entry.start(selected.version)}
                >
                  {entry.starting ? <Spinner className="mr-1 h-3 w-3" /> : <Check className="mr-1 h-3 w-3" />}
                  {entry.starting ? t('intimacyWindow.starting') : t('intimacyWindow.start')}
                </Button>
                <Button type="button" variant="outline" size="sm" className="h-7 text-[11px]" onClick={() => { setBrowsing(true); setActivityId('') }}>
                  {t('intimacyWindow.adjust')}
                </Button>
              </div>
            </div>
          ) : null}

          {inProgress ? (
            <div className="flex shrink-0 items-end gap-2 border-t border-[var(--nova-border)] px-3 py-2">
              <Textarea
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault()
                    void submit()
                  }
                }}
                minRows={1}
                maxRows={5}
                placeholder={t('intimacyWindow.inputPlaceholder', { names: (selected.roster || []).map(nameOf).join('、') })}
                className="min-h-8 flex-1 resize-none bg-[var(--nova-surface-2)] text-[12px]"
                disabled={entry.sending}
              />
              <Button
                type="button"
                size="icon-sm"
                className="h-8 w-8"
                disabled={!draft.trim() || entry.sending}
                onClick={() => void submit()}
                aria-label={t('intimacyWindow.send')}
                title={t('intimacyWindow.send')}
              >
                <Send className="h-4 w-4" />
              </Button>
            </div>
          ) : null}

          {openActivity ? (
            <div className="flex shrink-0 gap-2 border-t border-[var(--nova-border)] px-3 py-2">
              {inProgress ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-7 flex-1 text-[11px]"
                  disabled={entry.ending}
                  onClick={() => void entry.end()}
                >
                  {entry.ending ? t('intimacyWindow.ending') : t('intimacyWindow.end')}
                </Button>
              ) : (
                <p className="flex-1 text-[11px] text-[var(--nova-text-faint)]">{t('intimacyWindow.ended')}</p>
              )}
              {onContinueStory ? (
                <Button type="button" size="sm" className="h-7 text-[11px]" onClick={continueStory}>
                  {t('intimacyWindow.returnStory')}
                </Button>
              ) : null}
            </div>
          ) : null}
        </>
      )}
    </div>
  )
}

function knownName(character: CharacterWindowCharacter): string {
  return character.known_as || character.name || character.id
}

function PhaseBadge({ phase }: { phase: string }) {
  const { t } = useTranslation()
  return (
    <Badge variant="outline" className="h-5 shrink-0 border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 text-[10px] font-normal text-[var(--nova-text-muted)]">
      {t(`intimacyWindow.phase.${phase}`, { defaultValue: phase })}
    </Badge>
  )
}

/**
 * The activities this container holds. Opening one is a plain read: a saved
 * activity shows what was said and never replays an opening.
 */
function ActivityList({ activities, nameOf, onSelect, inviteTargets, inviting, invitingCharacterID, onInvite, modelConfigured, emptyHint, projectId }: {
  activities: IntimacyWindowActivity[]
  nameOf: (id: string) => string
  onSelect: (id: string) => void
  inviteTargets: CharacterWindowCharacter[]
  inviting: boolean
  invitingCharacterID: string
  onInvite: (id: string) => void
  modelConfigured: boolean
  emptyHint?: string
  projectId?: string
}) {
  const { t } = useTranslation()
  if (activities.length === 0) {
    return (
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
        <InviteBlock
          targets={inviteTargets}
          invited={new Set()}
          inviting={inviting}
          invitingCharacterID={invitingCharacterID}
          onInvite={onInvite}
          disabled={!modelConfigured}
          emptyHint={emptyHint}
          projectId={projectId}
        />
        {inviteTargets.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-1 px-6 text-center">
            <p className="text-[12px] text-[var(--nova-text-muted)]">{t('intimacyWindow.invite.empty')}</p>
            <p className="text-[11px] text-[var(--nova-text-faint)]">{emptyHint || t('intimacyWindow.invite.emptyHint')}</p>
          </div>
        ) : null}
      </div>
    )
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-2 py-2">
      <InviteBlock
        targets={inviteTargets}
        invited={new Set()}
        inviting={inviting}
        invitingCharacterID={invitingCharacterID}
        onInvite={onInvite}
        disabled={!modelConfigured}
        projectId={projectId}
      />
      {activities.map((activity) => {
        const roster = (activity.roster || []).map(nameOf)
        return (
          <button
            key={activity.id}
            type="button"
            data-testid="intimacy-surface-activity-row"
            data-activity-id={activity.id}
            onClick={() => onSelect(activity.id)}
            className="flex w-full items-start gap-2 rounded-[var(--nova-radius)] px-2 py-2 text-left transition-colors hover:bg-[var(--nova-hover)]"
          >
            <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
              <HeartHandshake className="h-3.5 w-3.5" />
            </span>
            <span className="min-w-0 flex-1">
              <span className="flex items-center gap-1.5">
                <span className="truncate text-[12px] text-[var(--nova-text)]">
                  {roster.length > 0 ? roster.join('、') : t('intimacyWindow.invite.empty')}
                </span>
                <PhaseBadge phase={activity.phase} />
              </span>
              <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-muted)]">
                {t('intimacyWindow.roster.count', { count: roster.length, total: roster.length + 1 })}
              </span>
            </span>
          </button>
        )
      })}
    </div>
  )
}

/** The roster: who actually takes part, with each person's own answer. */
function RosterBlock({ activity, nameOf, avatarOf, lorePathOf, rosterClips, projectId }: {
  activity: IntimacyWindowActivity
  nameOf: (id: string) => string
  avatarOf: (id: string) => string
  lorePathOf: (id: string) => string
  /** The private clips of the people in the room, when the activity is running. */
  rosterClips?: Record<string, CharacterMediaClip[]>
  projectId?: string
}) {
  const { t } = useTranslation()
  // Which participant's picture is open. A clip is never laid out on the roster
  // itself: opening one person's picture is the player asking to look, and the
  // activity's privacy does not turn a roster row into a screening.
  const [openClip, setOpenClip] = useState('')
  const participants = activity.participants || []
  if (participants.length === 0) return null
  const roster = activity.roster || []
  return (
    <div data-testid="intimacy-surface-roster" className="shrink-0 border-b border-[var(--nova-border)] px-3 py-2">
      <div className="mb-1 flex items-baseline justify-between gap-2">
        <p className="text-[10px] uppercase tracking-wide text-[var(--nova-text-faint)]">{t('intimacyWindow.roster.title')}</p>
        <p className="text-[10px] text-[var(--nova-text-faint)]">
          {t('intimacyWindow.roster.count', { count: roster.length, total: roster.length + 1 })}
        </p>
      </div>
      <ul className="flex flex-col gap-1">
        {participants.map((participant) => {
          // The clip is here only because the server put it here: this person
          // consented and the activity is running. Nothing in the window decides
          // that, so an invitation that has not been answered has no clip.
          const clips = rosterClips?.[participant.character_id] || []
          const name = nameOf(participant.character_id)
          const still = personAvatarSource(avatarOf(participant.character_id), lorePathOf(participant.character_id), projectId)
          const open = openClip === participant.character_id
          return (
            <li key={participant.character_id} className="flex flex-col gap-1 text-[11px]">
              <div className="flex items-center gap-1.5">
                <PersonAvatar
                  avatar={avatarOf(participant.character_id)}
                  lorePath={lorePathOf(participant.character_id)}
                  projectId={projectId}
                  name={name}
                  size="xs"
                />
                <span className="min-w-0 flex-1 truncate text-[var(--nova-text)]">{name}</span>
                <ResponseBadge status={participant.status} />
                {participant.condition ? (
                  <span className="min-w-0 max-w-[50%] truncate text-[10px] text-[var(--nova-text-faint)]" title={participant.condition}>
                    {t('intimacyWindow.roster.condition', { condition: participant.condition })}
                  </span>
                ) : null}
              </div>
              {clips.length > 0 ? (
                <>
                  <button
                    type="button"
                    data-testid={`intimacy-roster-clip-toggle-${participant.character_id}`}
                    aria-expanded={open}
                    onClick={() => setOpenClip(open ? '' : participant.character_id)}
                    className="ml-6 self-start text-[10px] text-[var(--nova-text-muted)] underline-offset-2 hover:underline"
                    title={open ? t('characterWindow.portraitHide') : t('characterWindow.portraitShow', { name })}
                  >
                    {open ? t('characterWindow.portraitHide') : t('characterWindow.portraitShow', { name })}
                  </button>
                  {open ? (
                    <PersonClip
                      clips={clips}
                      name={name}
                      testId={`intimacy-roster-clip-${participant.character_id}`}
                      autoPlayKey={`${activity.id}/${participant.character_id}`}
                      stillSrc={still || undefined}
                      stillAlt={t('characterWindow.portraitAlt', { name })}
                      className="ml-6 self-start"
                    />
                  ) : null}
                </>
              ) : null}
            </li>
          )
        })}
      </ul>
    </div>
  )
}

function ResponseBadge({ status }: { status: IntimacyResponseStatus }) {
  const { t } = useTranslation()
  const key = RESPONSE_STATUSES.includes(status) ? status : 'pending'
  const tone = status === 'accepted'
    ? 'border-[var(--nova-accent-green)]/40 bg-[var(--nova-accent-green)]/10 text-[var(--nova-accent-green)]'
    : status === 'declined'
      ? 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-faint)]'
      : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'
  return (
    <Badge variant="outline" className={cn('h-4 shrink-0 px-1 text-[10px] font-normal', tone)}>
      {t(`intimacyWindow.roster.${key}`)}
    </Badge>
  )
}

/** The people who may be invited right now, each with their own real name. */
function InviteBlock({ targets, invited, inviting, invitingCharacterID, onInvite, disabled = false, emptyHint, projectId }: {
  targets: CharacterWindowCharacter[]
  invited: Set<string>
  inviting: boolean
  invitingCharacterID: string
  onInvite: (id: string) => void
  disabled?: boolean
  emptyHint?: string
  projectId?: string
}) {
  const { t } = useTranslation()
  const open = targets.filter((target) => !invited.has(target.id))
  if (open.length === 0) {
    if (!emptyHint) return null
    return (
      <div className="shrink-0 border-b border-[var(--nova-border)] px-3 py-2">
        <p className="text-[11px] text-[var(--nova-text-faint)]">{emptyHint}</p>
      </div>
    )
  }
  return (
    <div className="shrink-0 border-b border-[var(--nova-border)] px-3 py-2">
      <p className="mb-1 text-[10px] uppercase tracking-wide text-[var(--nova-text-faint)]">{t('intimacyWindow.invite.title')}</p>
      <ul className="flex flex-col gap-1">
        {open.map((target) => {
          const busy = inviting && invitingCharacterID === target.id
          return (
            <li key={target.id} className="flex items-center gap-2">
              <PersonAvatar
                avatar={target.avatar}
                lorePath={target.avatar_lore_path}
                projectId={projectId}
                name={knownName(target)}
                size="xs"
              />
              <span className="min-w-0 flex-1 truncate text-[12px] text-[var(--nova-text)]">
                {knownName(target)}
                {target.role ? <span className="ml-1 text-[10px] text-[var(--nova-text-faint)]">{target.role}</span> : null}
              </span>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-6 shrink-0 px-2 text-[10px]"
                disabled={disabled || inviting}
                onClick={() => onInvite(target.id)}
              >
                {busy ? <Spinner className="mr-1 h-2.5 w-2.5" /> : null}
                {busy ? t('intimacyWindow.invite.asking', { name: knownName(target) }) : t('intimacyWindow.invite.oneOnOne')}
              </Button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

/**
 * One outstanding invitation into a running activity. It is shown apart from
 * the room because this person has not agreed: the room's conversation is not
 * theirs to hear, and only what was said between them and the protagonist is.
 */
function InvitationThread({ thread, nameOf, busy, disabled, onAnswer }: {
  thread: IntimacyInvitationThread
  nameOf: (id: string) => string
  busy: boolean
  disabled: boolean
  onAnswer: () => void
}) {
  const { t } = useTranslation()
  return (
    <div data-testid="intimacy-surface-invitation" className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2">
      <div className="mb-1 flex items-baseline gap-1.5">
        <p className="min-w-0 flex-1 truncate text-[11px] text-[var(--nova-text)]">{nameOf(thread.character_id)}</p>
        <ResponseBadge status={thread.status} />
      </div>
      <p className="mb-1 text-[10px] text-[var(--nova-text-faint)]">{t('intimacyWindow.invitations.hint')}</p>
      <ul className="mb-1.5 flex flex-col gap-1">
        {(thread.exchanges || []).map((exchange) => (
          <li key={exchange.id} className="text-[11px] leading-relaxed text-[var(--nova-text-muted)]">
            <span className="mr-1 text-[10px] text-[var(--nova-text-faint)]">
              {exchange.speaker === 'player' ? t('characterWindow.speaker.you') : nameOf(thread.character_id)}
            </span>
            {exchange.text}
          </li>
        ))}
      </ul>
      <Button type="button" variant="outline" size="sm" className="h-6 px-2 text-[10px]" disabled={disabled} onClick={onAnswer}>
        {busy ? <Spinner className="mr-1 h-2.5 w-2.5" /> : null}
        {t('intimacyWindow.invitations.answer')}
      </Button>
    </div>
  )
}

function ExchangeRow({ exchange, nameOf }: { exchange: CharacterExchange; nameOf: (id: string) => string }) {
  const { t } = useTranslation()
  const mine = exchange.speaker === 'player'
  const speaker = mine ? t('characterWindow.speaker.you') : nameOf(exchange.author_id || exchange.character_id)
  return (
    <div className={cn('flex flex-col gap-0.5', mine ? 'items-end' : 'items-start')}>
      <span className="text-[10px] text-[var(--nova-text-faint)]">
        {speaker}
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
