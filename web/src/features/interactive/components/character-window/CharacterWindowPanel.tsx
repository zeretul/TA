import { useWorldPresentation } from '../../world-presentation-context'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranscriptScroll } from '../chat-window/use-transcript-scroll'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { ArrowLeft, Send, X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import type { CharacterWindowCharacter, CharacterWindowScene, CharacterExchange, CharacterMatter, CharacterWindowMessage } from '../../types'
import { PersonAvatar, personAvatarSource } from '../person-avatar'
import { SummaryBlock } from '../summary-block'
import { PersonClip } from './PersonClip'
import { CharacterPictureControl, useCharacterPicture } from './CharacterPictureControl'
import { PlacePictureControl, usePlacePicture } from './PlacePictureControl'
import { buildNegotiatedAction, composeNegotiatedMessage, type NegotiatedAction } from './negotiated-action'
import { useCharacterWindow } from './use-character-window'
import { usePlayerAttention } from '../use-player-attention'

interface CharacterWindowPanelProps {
  storyId: string
  branchId: string
  /**
   * The project this story's world belongs to. A person's world-book picture is
   * stored as a project-relative path, so the window addresses it through this
   * project instead of the read turning it into a URL.
   */
  projectId?: string
  /** True while the panel is on screen. It is what starts the read. */
  active: boolean
  /** The character whose conversation is open, or '' to show the cast. */
  characterId: string
  onSelectCharacter: (characterId: string) => void
  onClose: () => void
  /**
   * Hands the player's own plan to the main story's existing send chain. It is
   * the same chain the stage composer uses, so acting on an arrangement is a
   * story turn rather than a second, parallel action path.
   */
  onActOnPlan?: (message: string) => void
  /** Carries the conversation already on record into the story without repeating it. */
  onContinueStory?: (message: string, playerLetter?: { character_id: string; body: string }) => void
  /**
   * Dock/float/minimize controls, when the stage hosts this panel in a window
   * that can move. They sit in this header rather than in a second title bar.
   */
  windowActions?: ReactNode
  className?: string
  /**
   * The draft belongs to the person being written to, and it has to outlive the
   * window: closing the window unmounts this panel, and a player who steps away
   * mid-sentence must not come back to an empty box. The host owns it for the
   * same reason it owns which person is open.
   */
  draft: string
  onDraftChange: (value: string) => void
  /**
   * Called after a round commits. The chat shell keeps a list of every
   * conversation beside this one, and that list previews the last line — so it
   * has to be told the last line moved.
   */
  onChanged?: () => void
  /**
   * True when the chat shell hosts this panel behind its own title bar. The
   * shell already says whose conversation is open and how to go back, so a
   * second header inside it would be two bars claiming the same job.
   */
  embedded?: boolean
}

/**
 * The character window: who is in this story, what is unfinished with them, and
 * the conversation so far. Reading is free; only sending calls a model.
 */
export function CharacterWindowPanel({ storyId, branchId, projectId, active, characterId, onSelectCharacter, onClose, onActOnPlan, onContinueStory, windowActions, className, draft, onDraftChange, onChanged, embedded }: CharacterWindowPanelProps) {
  const { t } = useTranslation()
  const window = useCharacterWindow({ storyId, branchId, characterId, active })
  const attention = usePlayerAttention({ storyId, branchId, active })
  const [plan, setPlan] = useState('')
  const { listRef, preserveEarlier } = useTranscriptScroll(JSON.stringify([storyId, branchId, characterId]), window.data?.exchanges, window.loadingEarlier)

  const characters = useMemo(() => {
    const followed = new Set(attention.data?.characters || [])
    return [...(window.data?.characters || [])].sort((a, b) => Number(followed.has(b.id)) - Number(followed.has(a.id)))
  }, [window.data?.characters, attention.data?.characters])
  const selected = useMemo(
    () => characters.find((character) => character.id === characterId) || null,
    [characters, characterId],
  )
  // Missing or stale presence remains a letter. The server rechecks the
  // committed scene before accepting a face-to-face reply.
  const needsLetter = window.data?.communication_mode === 'letter' && !(
    selected?.present === true && window.data?.scene?.present?.includes(selected.id)
  )
  // The affordance is read from committed facts, not from the conversation text,
  // so an invitation or a hesitation cannot be dressed up as a settled plan.
  const negotiated = useMemo(
    () => buildNegotiatedAction(window.data?.matters, window.data?.exchanges),
    [window.data?.matters, window.data?.exchanges],
  )
  const hasConversation = Boolean(window.data?.exchanges?.length)
  // One "sighting" is a person in one scene. A clip may play itself once per
  // sighting and never again for the same one, so the key has to change when the
  // scene actually changes and stay put through every redraw, reply and reopen.
  // It is deliberately not the world time: reading a conversation does not move
  // the clock, and a clip is not tied to a particular minute.
  const sceneKey = useMemo(() => {
    const scene = window.data?.scene
    if (!scene) return ''
    return (scene.location || '').trim()
  }, [window.data?.scene?.location])
  // The prefill is re-seeded only when the conversation changes hands or when
  // the set of agreements itself changes. An ordinary reply leaves it alone, so
  // a plan the player has started typing is not wiped by an unrelated refresh.
  const agreedKey = useMemo(() => (negotiated?.agreed || []).map((item) => item.id).join(','), [negotiated])
  const negotiatedRef = useRef(negotiated)
  negotiatedRef.current = negotiated
  useEffect(() => {
    setPlan(negotiatedRef.current?.plan || '')
  }, [storyId, branchId, characterId, agreedKey])

  // Opening a conversation is what reading its messages means. Reading costs no
  // world time and accepts nothing: it only stops the stage from asking about a
  // message the player has already looked at.
  const messages = window.data?.messages || []
  const markMessage = window.markMessage
  const unreadKey = useMemo(
    () => messages.filter((message) => message.status === 'unread').map((message) => message.id).join(','),
    [messages],
  )
  useEffect(() => {
    if (!active || !characterId || !unreadKey) return
    for (const messageID of unreadKey.split(',')) {
      void markMessage(messageID, 'read')
    }
  }, [active, characterId, markMessage, unreadKey])

  const submit = async () => {
    const text = draft.trim()
    if (!text || window.sending) return
    if (window.data?.communication_mode === 'radio' && !window.data.radio_available) return
    if (needsLetter) {
      if (!onContinueStory || !selected) return
      onContinueStory(t('characterWindow.letterStoryAction', { name: knownName(selected), text }), { character_id: selected.id, body: text })
      onDraftChange('')
      onChanged?.()
      return
    }
    const sent = await window.send(text)
    if (sent) {
      onDraftChange('')
      onChanged?.()
    }
  }

  const openConversation = (id: string) => {
    onDraftChange('')
    onSelectCharacter(id)
  }

  const actOnPlan = () => {
    if (!negotiated || !onActOnPlan) return
    const message = composeNegotiatedMessage(negotiated, plan)
    if (!message) return
    onActOnPlan(message)
  }

  const continueStory = () => {
    if (!onContinueStory) return
    onContinueStory(t('characterWindow.continueStoryMessage', { name: selected ? knownName(selected) : '' }))
  }

  return (
    <div data-testid="character-window-panel" className={cn('flex h-full min-h-0 flex-col bg-[var(--nova-surface)] text-[var(--nova-text)]', className)}>
      {embedded ? null : (
        <header className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2">
          {characterId ? (
            <Button type="button" variant="ghost" size="icon-sm" onClick={() => openConversation('')} aria-label={t('characterWindow.back')} title={t('characterWindow.back')}>
              <ArrowLeft className="h-4 w-4" />
            </Button>
          ) : null}
          <h2 className="min-w-0 flex-1 truncate text-[13px] font-medium">
            {selected ? knownName(selected) : t('characterWindow.title')}
          </h2>
          {/* The header states how a letter gets to this person, and nothing else.
              Ordinary talk is a private conversation, so naming its channel would
              be describing a room the player is not shown; the only channel worth
              labelling is one the world actually gives them, and letters are the
              one the world pack can configure. */}
          {selected && needsLetter ? <LetterBadge /> : null}
          {windowActions}
          <Button type="button" variant="ghost" size="icon-sm" onClick={onClose} aria-label={t('characterWindow.close')} title={t('characterWindow.close')}>
            <X className="h-4 w-4" />
          </Button>
        </header>
      )}

      <SceneBlock storyId={storyId} scene={window.data?.scene} onPictureChanged={() => void window.reload()} />
      {selected ? <AffinityLine character={selected} /> : null}
      {selected ? <div className="flex shrink-0 flex-wrap items-center gap-2 border-b px-3 py-1.5">
        <Button variant="outline" size="sm" disabled={window.loading || attention.busy || !attention.data} onClick={() => void attention.setFollowing('character', selected.id, !attention.data?.characters.includes(selected.id))}>
          {t(attention.data?.characters.includes(selected.id) ? 'characterWindow.unfollow' : 'characterWindow.follow')}
        </Button>
        <span className="text-xs text-muted-foreground">{t('characterWindow.attentionHint')}</span>
      </div> : null}
      {attention.error ? <div role="alert" className="px-3 py-2 text-xs text-destructive">{attention.error}<Button variant="ghost" size="sm" onClick={() => void attention.reload()}>{t('characterWindow.retry')}</Button></div> : null}
      {selected && window.data?.communication_mode === 'radio' ? <p className="border-b px-3 py-2 text-xs text-muted-foreground">{t(window.data.radio_available ? 'characterWindow.radioHint' : 'characterWindow.radioUnavailable')}</p> : null}
      {/* Keyed by the person so opening one conversation cannot leave another
          person's portrait expanded behind it. */}
      {selected ? (
        <PortraitBlock
          key={selected.id}
          storyId={storyId}
          branchId={branchId}
          character={selected}
          projectId={projectId}
          autoPlayKey={selected.clips?.length ? `${storyId}/${branchId}/${sceneKey}/${selected.id}` : undefined}
          onPictureChanged={() => void window.reload()}
        />
      ) : null}
      {window.error ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center">
          <p className="text-[12px] text-[var(--nova-danger)]">{t('characterWindow.loadFailed')}</p>
          <p className="text-[11px] text-[var(--nova-text-faint)]">{window.error}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void window.reload()}>{t('characterWindow.retry')}</Button>
        </div>
      ) : window.loading ? (
        <div className="flex flex-1 items-center justify-center gap-2 text-[12px] text-[var(--nova-text-muted)]">
          <Spinner className="h-3.5 w-3.5" />
          {t('characterWindow.loading')}
        </div>
      ) : !characterId ? (
        <CharacterRoster characters={characters} projectId={projectId} onSelect={openConversation} />
      ) : (
        <>
          {/* Long arrangements and history share the remaining height; the
              composer stays outside this scroll area on narrow screens. */}
          <div ref={listRef} data-testid="character-window-transcript" className="min-h-[12rem] flex-1 overflow-y-auto">
            <MessageList messages={messages} name={selected ? knownName(selected) : ''} onIgnore={(id) => void window.markMessage(id, 'ignored')} />
            <MatterList matters={window.data?.matters || []} clockSeconds={window.data?.clock_seconds || 0} />
            <div className="flex flex-col gap-2 px-3 py-2">
              {window.hasMore ? (
                <Button type="button" variant="ghost" size="sm" className="self-center text-[11px]" disabled={window.loadingEarlier} onClick={() => void preserveEarlier(window.loadEarlier)}>
                  {window.loadingEarlier ? t('characterWindow.loading') : t('characterWindow.loadEarlier')}
                </Button>
              ) : null}
              {(window.data?.exchanges || []).length === 0 ? (
                <p className="py-6 text-center text-[12px] text-[var(--nova-text-faint)]">{t('characterWindow.noHistory')}</p>
              ) : (window.data?.exchanges || []).map((exchange) => (
                <ExchangeRow key={exchange.id} exchange={exchange} name={selected ? knownName(selected) : ''} />
              ))}
              {window.sending ? (
                <p className="flex items-center gap-2 self-start text-[11px] text-[var(--nova-text-muted)]">
                  <Spinner className="h-3 w-3" />
                  {t('characterWindow.sending')}
                </p>
              ) : null}
            </div>
            {window.affinityChange ? (
              <p data-testid="character-window-affinity-change" className="px-3 pb-1 text-[11px] text-[var(--nova-text-muted)]">
                {affinityChangeLine(t, window.affinityChange.delta, selected ? knownName(selected) : '')}
              </p>
            ) : null}
            {window.sendError ? <p className="px-3 pb-1 text-[11px] text-[var(--nova-danger)]">{t('characterWindow.replyFailed')} {window.sendError}</p> : null}
            {window.cancelled ? <p className="px-3 pb-1 text-[11px] text-[var(--nova-text-muted)]">{t('characterWindow.cancelled')}</p> : null}
            {negotiated ? (
              <NegotiatedActionBlock
                action={negotiated}
                plan={plan}
                name={selected ? knownName(selected) : ''}
                disabled={window.sending || !onActOnPlan}
                onPlanChange={setPlan}
                onAct={actOnPlan}
              />
            ) : hasConversation && onContinueStory ? (
              <div className="shrink-0 border-t border-[var(--nova-border)] px-3 py-2">
                <p className="mb-1.5 text-[11px] text-[var(--nova-text-faint)]">{t('characterWindow.continueStoryHint')}</p>
                <Button type="button" variant="outline" size="sm" className="h-7 w-full text-[11px]" onClick={continueStory}>
                  {t('characterWindow.continueStory')}
                </Button>
              </div>
            ) : null}
          </div>
          {needsLetter ? (
            <p className="shrink-0 border-t border-[var(--nova-border)] px-3 pt-2 text-[11px] text-[var(--nova-text-faint)]">
              {t('characterWindow.letterModeHint')}
            </p>
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
              placeholder={t(needsLetter ? 'characterWindow.letterPlaceholder' : 'characterWindow.inputPlaceholder', { name: selected ? knownName(selected) : '' })}
              className="min-h-8 flex-1 resize-none bg-[var(--nova-surface-2)] text-[12px]"
              disabled={window.sending || (window.data?.communication_mode === 'radio' && !window.data.radio_available)}
            />
            {window.sending ? (
              <Button type="button" variant="outline" size="sm" className="h-8" onClick={window.cancel}>{t('characterWindow.cancel')}</Button>
            ) : (
              <Button type="button" size="icon-sm" className="h-8 w-8" disabled={!draft.trim() || (window.data?.communication_mode === 'radio' && !window.data.radio_available) || (needsLetter && !onContinueStory)} onClick={() => void submit()} aria-label={t(needsLetter ? 'characterWindow.sendLetter' : 'characterWindow.send')} title={t(needsLetter ? 'characterWindow.sendLetter' : 'characterWindow.send')}>
                <Send className="h-4 w-4" />
              </Button>
            )}
          </div>
        </>
      )}
    </div>
  )
}

/** The name the player actually knows this character by, falling back to their own name. */
function knownName(character: CharacterWindowCharacter): string {
  return character.known_as || character.name || character.id
}

/**
 * A gender the story settled, or nothing.
 *
 * An unsettled gender draws no label at all. It is not something the player has
 * failed to find out — the author never decided — and a slot reading "unknown"
 * would turn a fact about the record into a gap in the player's knowledge.
 */
function genderLabel(t: TFunction, gender?: string): string {
  if (!gender) return ''
  return t(`characterWindow.gender.${gender}`, { defaultValue: '' })
}

/**
 * Where this character was registered, or nothing.
 *
 * The server's two values are `author_catalog` and `story`, but the caption keys
 * are camelCase. Concatenating the server value straight onto the key is what
 * put the raw string `characterWindow.identitySource.author_catalog` on screen:
 * there is no such key, so i18next echoed the key back. The map is the one place
 * the two spellings are reconciled, and an unknown value draws nothing rather
 * than the key.
 */
const IDENTITY_SOURCES = new Set(['author_catalog', 'story'])

function identitySourceLabel(t: TFunction, source?: string): string {
  if (!source || !IDENTITY_SOURCES.has(source)) return ''
  const camel = source.replace(/_([a-z])/g, (_, letter: string) => letter.toUpperCase())
  return t(`characterWindow.identitySource.${camel}`, { defaultValue: '' })
}

/**
 * The scene line: where the story stands and when. It deliberately does not list
 * who else is here. That roster is the main story's own fact, and repeating it
 * beside a private conversation reads as an audience the conversation does not
 * have — the design says ordinary talk is between the player and this person
 * only, and a "present" list here would say otherwise.
 */
function SceneBlock({ storyId, scene, onPictureChanged }: {
  storyId: string
  scene?: CharacterWindowScene
  onPictureChanged?: () => void
}) {
  const { t } = useTranslation()
  // The place's picture is an author's material, not part of the reading
  // surface, so it stays folded until asked for. The scene's own picture is
  // drawn either way; this only opens the place where one can be added.
  const [open, setOpen] = useState(false)
  // The registered place the story stands in, which is what a picture binds to.
  // `location` is the story's own prose — 青石渡口码头 — and would name a place
  // no picture was ever attached to, so it is only the fallback when the server
  // resolved nothing.
  const place = scene?.place || ''
  const placePicture = usePlacePicture(storyId, place, open && Boolean(place))
  if (!scene || (!scene.location && !scene.world_time)) return null
  return (
    <div className="shrink-0 border-b border-[var(--nova-border)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
      {scene.world_time ? <span>{t('characterWindow.sceneTime')} · {scene.world_time}</span> : null}
      {scene.location ? <span className={cn(scene.world_time && 'ml-2')}>{scene.location}</span> : null}
      {/* The place's own picture, when the author supplied one. It is not a
          background and not a frame: a place with no picture shows the place as
          text alone rather than an empty box, and the picture tells the player
          nothing they were not already told — the story already put them here. */}
      {scene.image ? (
        <img
          src={scene.image}
          alt={scene.location || ''}
          // `object-contain` rather than `object-cover`: a scene picture is not a
          // face that may be cropped to a circle, so it keeps its own proportions
          // and is never stretched to fill the row.
          className="mt-1.5 block max-h-40 w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] object-contain"
          onError={(event) => { event.currentTarget.style.display = 'none' }}
        />
      ) : null}
      {/* Only a place the story stands in can be illustrated, because only a
          registered place has a name the game will ever ask for. */}
      {place ? (
        <>
          <button
            type="button"
            data-testid="scene-picture-toggle"
            aria-expanded={open}
            onClick={() => setOpen((value) => !value)}
            className="mt-1.5 text-[10px] text-[var(--nova-text-faint)] underline-offset-2 hover:text-[var(--nova-text)] hover:underline"
          >
            {t('characterWindow.picture.placeOpen')}
          </button>
          {open ? (
            <PlacePictureControl
              storyId={storyId}
              place={place}
              state={placePicture.state}
              onStateChange={(next) => {
                placePicture.setState(next)
                // The scene's own picture is what the rest of the window draws,
                // so it reads again rather than keeping a place this panel has
                // already replaced.
                onPictureChanged?.()
              }}
            />
          ) : null}
        </>
      ) : null}
    </div>
  )
}

/**
 * The affinity reading for the open conversation: the exact number, the band it
 * falls in, and the formal relationship that stands, if one does.
 *
 * It is one line rather than a meter because the design asks for the figure and
 * a natural name, not a progress bar to fill. A character the story has just
 * introduced reads 0 and "nothing formal", which is the honest starting state
 * rather than a missing value.
 */
function AffinityLine({ character }: { character: CharacterWindowCharacter }) {
  const { t } = useTranslation()
  const { terms } = useWorldPresentation()
  const value = typeof character.affinity === 'number' ? character.affinity : 0
  const bandKey = `characterWindow.band.${character.affinity_band || 'neutral'}`
  const band = t(bandKey, { defaultValue: character.affinity_band || '' })
  const relationshipKey = character.relationship ? `characterWindow.relationship.${character.relationship.kind}` : ''
  const relationship = character.relationship?.kind === 'partner' ? terms.partner : relationshipKey
    ? t(relationshipKey, { defaultValue: character.relationship?.kind || '' })
    : t('characterWindow.affinity.relationshipNone')
  return (
    <div
      data-testid="character-window-affinity"
      className="shrink-0 border-b border-[var(--nova-border)] px-3 py-1.5 text-[11px] text-[var(--nova-text-muted)]"
    >
      <span title={band}>{t('characterWindow.affinity.line', { value, relationship })}</span>
    </div>
  )
}

/**
 * One short sentence for a change of feeling, never a per-line score.
 *
 * The design asks for a plain report of what happened rather than "+4": a large
 * move is described by what it is, and a small one by direction alone. Nothing
 * is shown when the exchange settled nothing, which is the ordinary case.
 */
function affinityChangeLine(t: TFunction, delta: number, name: string): string {
  const magnitude = Math.abs(delta)
  const rose = delta > 0
  if (magnitude >= 9) return t(rose ? 'characterWindow.affinity.roseTrust' : 'characterWindow.affinity.fellGuard', { name })
  return t(rose ? 'characterWindow.affinity.rose' : 'characterWindow.affinity.fell', { name })
}

/**
 * A letter is the one contact route the world pack can actually configure, and
 * it changes what the composer does — the message is handed to the main story to
 * carry rather than answered on the spot. Ordinary talk gets no badge: it is a
 * private conversation, and labelling it "face to face" would claim an audience
 * the design says it does not have.
 */
function LetterBadge() {
  const { t } = useTranslation()
  return (
    <Badge variant="outline" className="h-5 shrink-0 border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 text-[10px] font-normal text-[var(--nova-text-muted)]">
      {t('characterWindow.channel.letter')}
    </Badge>
  )
}

function CharacterRoster({ characters, projectId, onSelect }: { characters: CharacterWindowCharacter[]; projectId?: string; onSelect: (id: string) => void }) {
  const { t } = useTranslation()
  if (characters.length === 0) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-1 px-6 text-center">
        <p className="text-[12px] text-[var(--nova-text-muted)]">{t('characterWindow.empty')}</p>
        <p className="text-[11px] text-[var(--nova-text-faint)]">{t('characterWindow.emptyHint')}</p>
      </div>
    )
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-2 py-2">
      {characters.map((character) => (
        <button
          key={character.id}
          type="button"
          data-testid="character-window-roster-row"
          data-character-id={character.id}
          onClick={() => onSelect(character.id)}
          className="flex w-full items-start gap-2 rounded-[var(--nova-radius)] px-2 py-2 text-left transition-colors hover:bg-[var(--nova-hover)]"
        >
          <CharacterAvatar character={character} projectId={projectId} />
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-1.5">
              <span className="truncate text-[12px] text-[var(--nova-text)]">{knownName(character)}</span>
              {character.known_as && character.known_as !== character.name ? (
                <span className="truncate text-[10px] text-[var(--nova-text-faint)]">（{character.name}）</span>
              ) : null}
            </span>
            {character.role || character.attitude || character.gender || character.lifeform || character.identity_source ? (
              <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-muted)]">
                {[
                  genderLabel(t, character.gender),
                  character.lifeform ? t(`characterWindow.lifeform.${character.lifeform}`) : '',
                  character.cultivation_method_id === 'method.spirit_beast_breathing' ? t('characterWindow.cultivationMethod.spiritBeastBreathing') : '',
                  character.attitude || character.role,
                  identitySourceLabel(t, character.identity_source),
                ].filter(Boolean).join(' · ')}
              </span>
            ) : null}
            <span className="mt-1 flex flex-wrap items-center gap-1.5">
              {character.unread_messages > 0 ? (
                <Badge variant="outline" className="h-4 border-[var(--nova-accent)]/40 bg-[var(--nova-accent)]/10 px-1 text-[10px] font-normal text-[var(--nova-accent)]">
                  {t('characterWindow.unread', { count: character.unread_messages })}
                </Badge>
              ) : null}
              {character.open_matters > 0 ? (
                <Badge variant="outline" className="h-4 border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1 text-[10px] font-normal text-[var(--nova-text-muted)]">
                  {t('characterWindow.openMatters', { count: character.open_matters })}
                </Badge>
              ) : null}
            </span>
          </span>
        </button>
      ))}
    </div>
  )
}

function CharacterAvatar({ character, projectId }: { character: CharacterWindowCharacter; projectId?: string }) {
  return (
    <PersonAvatar
      avatar={character.avatar}
      lorePath={character.avatar_lore_path}
      projectId={projectId}
      name={knownName(character)}
    />
  )
}

/**
 * The open conversation's portrait: the same person's picture, at the size of a
 * portrait rather than of a roster row.
 *
 * It starts collapsed. The design keeps pictures out of the reading surface, so
 * this is something the player opens when they want to look at the person, not
 * something that appears because a scene began.
 *
 * When this build has a reviewed clip of the person going about an ordinary day,
 * the open block plays that clip instead of the still picture. It plays itself
 * once per person per scene — the player opened the picture, and this is the
 * first time they have looked at this person here — and then settles back onto
 * the still. A reply, a redraw, a reopened window, a reload or a cold restart
 * all leave it still; only the player's own press, or a scene that has actually
 * changed, starts it again. The private clip is not offered here: it belongs to
 * an activity that has already established the adult, consent and privacy
 * conditions, and a character window is not one.
 *
 * A person with no clip keeps the still picture, and a clip that fails to load
 * falls back to it — the player never gets an empty frame. The picture that
 * fails to load leaves the header's small avatar in place. That is the same
 * fallback the roster uses, so a broken file costs the player a picture and
 * nothing else.
 */
function PortraitBlock({ storyId, branchId, character, projectId, autoPlayKey, onPictureChanged }: {
  storyId: string
  branchId: string
  character: CharacterWindowCharacter
  projectId?: string
  autoPlayKey?: string
  onPictureChanged?: () => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [broken, setBroken] = useState(false)
  const name = knownName(character)
  // The world book carries one picture per person, so a world-book picture is
  // what the open conversation enlarges. A chosen avatar is already a URL.
  const display = personAvatarSource(character.avatar_display, character.avatar_lore_path, projectId)
  useEffect(() => { setBroken(false) }, [display])
  const clips = character.clips || []
  // A person the game has no picture for still gets this block, because adding
  // the first picture is exactly what the author comes here to do. It says so
  // rather than offering to show a picture that does not exist.
  const hasPicture = Boolean(display) || clips.length > 0
  const label = open
    ? t('characterWindow.portraitHide')
    : hasPicture
      ? t('characterWindow.portraitShow', { name })
      : t('characterWindow.portraitAdd')
  // The imported picture is read once, the first time the block is opened. It is
  // what the material control below reports and what a replacement is drawn from.
  const picture = useCharacterPicture(storyId, branchId, character.id, open)
  return (
    <div className="shrink-0 border-b border-[var(--nova-border)] px-3 py-1.5">
      <button
        type="button"
        data-testid="character-window-portrait-toggle"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="flex items-center gap-1.5 text-[11px] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
        title={label}
      >
        <PersonAvatar
          avatar={character.avatar}
          lorePath={character.avatar_lore_path}
          projectId={projectId}
          name={name}
          size="xs"
        />
        <span>{label}</span>
      </button>
      {open ? (
        <>
          {clips.length > 0 ? (
            <div className="mt-1.5 flex flex-col items-start gap-1.5">
              <PersonClip
                clips={clips}
                name={name}
                testId="character-window-clip"
                autoPlayKey={autoPlayKey}
                stillSrc={display && !broken ? display : undefined}
                stillAlt={t('characterWindow.portraitAlt', { name })}
              />
            </div>
          ) : display && !broken ? (
            <img
              src={display}
              alt={t('characterWindow.portraitAlt', { name })}
              onError={() => setBroken(true)}
              className="mt-1.5 block max-h-[288px] w-auto max-w-[192px] rounded-[var(--nova-radius)] border border-[var(--nova-border)] object-contain"
            />
          ) : null}
          {display && !broken && character.image_caption ? (
            <p data-testid="character-window-image-caption" className="mt-1 text-[11px] text-[var(--nova-text-muted)]">{character.image_caption}</p>
          ) : null}
          {/* A person with no picture at all still gets this control: adding the
              first picture is exactly what the author came here to do, and a
              control that only appears once a picture exists would be useless. */}
          <CharacterPictureControl
            storyId={storyId}
            branchId={branchId}
            characterId={character.id}
            name={name}
            state={picture.state}
            onStateChange={(next) => {
              picture.setState(next)
              // The rest of the window draws the same person, so it reads again
              // rather than keeping a face this panel has already replaced.
              setBroken(false)
              onPictureChanged?.()
            }}
          />
        </>
      ) : null}
    </div>
  )
}

/**
 * The incoming messages from this character, newest first.
 *
 * A message is shown with the reason it was sent and its original send time, so
 * an invitation from a month of world time ago reads as old rather than as
 * today's news. Receiving one accepts nothing: the player can answer it in the
 * box below, deal with it later, or dismiss it, and dismissing it changes only
 * what the stage keeps asking about.
 */
function MessageList({ messages, name, onIgnore }: { messages: CharacterWindowMessage[]; name: string; onIgnore: (id: string) => void }) {
  const { t } = useTranslation()
  if (messages.length === 0) return null
  return (
    <div className="shrink-0 border-b border-[var(--nova-border)] px-3 py-2">
      <p className="mb-1 text-[10px] uppercase tracking-wide text-[var(--nova-text-faint)]">{t('characterWindow.messages')}</p>
      <ul className="flex flex-col gap-1.5">
        {messages.map((message) => {
          const handled = message.status === 'handled' || message.status === 'ignored'
          return (
            <li
              key={message.id}
              className={cn(
                'rounded-[var(--nova-radius)] border px-2 py-1.5',
                handled ? 'border-[var(--nova-border)] bg-transparent' : 'border-[var(--nova-accent)]/30 bg-[var(--nova-surface-2)]',
              )}
            >
              <div className="flex items-baseline justify-between gap-2">
                <span className="text-[10px] text-[var(--nova-text-faint)]">
                  {name}
                  {message.world_time ? ` · ${message.world_time}` : ''}
                </span>
                {message.lapsed ? (
                  <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">{t('characterWindow.messageLapsed')}</span>
                ) : message.deadline ? (
                  <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">{t('characterWindow.negotiated.until', { deadline: message.deadline })}</span>
                ) : null}
              </div>
              <p className="mt-0.5 whitespace-pre-wrap break-words text-[12px] leading-relaxed text-[var(--nova-text)]">{message.text}</p>
              <div className="mt-1 flex items-center justify-between gap-2">
                <span className="min-w-0 truncate text-[10px] text-[var(--nova-text-faint)]" title={message.basis}>
                  {t('characterWindow.messageBasis', { basis: message.basis })}
                </span>
                {message.status === 'unread' || message.status === 'read' ? (
                  <button
                    type="button"
                    className="shrink-0 text-[10px] text-[var(--nova-text-muted)] underline-offset-2 hover:underline"
                    onClick={() => onIgnore(message.id)}
                  >
                    {t('characterWindow.messageIgnore')}
                  </button>
                ) : (
                  <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">
                    {message.status === 'handled' ? t('characterWindow.messageHandled') : t('characterWindow.messageIgnored')}
                  </span>
                )}
              </div>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

function MatterList({ matters, clockSeconds }: { matters: CharacterMatter[]; clockSeconds: number }) {
  const { t } = useTranslation()
  if (matters.length === 0) return null
  // The closed line names the first arrangement, so the block still tells the
  // player what is outstanding without being opened.
  const first = matters[0]
  const summary = `${matterKindLabel(t, first.kind)} · ${first.letter ? first.letter.body : first.statement}`
  return (
    <SummaryBlock label={t('characterWindow.matters')} count={matters.length} summary={summary}>
      <ul className="flex flex-col gap-1">
        {matters.map((matter) => (
          <li key={matter.id} className="flex items-start gap-1.5 text-[11px] text-[var(--nova-text-muted)]">
            <span className="shrink-0 text-[var(--nova-text-faint)]">
              {matter.letter ? t('characterWindow.letterSent') : matter.letter_reply ? t('characterWindow.letterReply') : matter.settled_by ? t('characterWindow.letterReceipt') : matterKindLabel(t, matter.kind)}
            </span>
            <span className="min-w-0 flex-1">
              {matter.letter ? matter.letter.body : matter.statement}
              {matter.letter ? (
                <span className="mt-0.5 block text-[10px] text-[var(--nova-text-faint)]">
                  {matter.status === 'settled'
                    ? t('characterWindow.letterReceiptRecorded')
                    : matter.status === 'abandoned'
                      ? t('characterWindow.letterAbandoned')
                      : clockSeconds >= matter.letter.earliest_delivery_seconds
                        ? t('characterWindow.letterArrivalReached')
                        : t('characterWindow.letterInTransit')}
                </span>
              ) : matter.letter_reply ? (
                <span className="mt-0.5 block text-[10px] text-[var(--nova-text-faint)]">
                  {t('characterWindow.letterReplyText', { body: matter.letter_reply.body })}
                </span>
              ) : null}
            </span>
            {matter.deadline ? <span className="shrink-0 text-[var(--nova-text-faint)]">{matter.deadline}</span> : null}
          </li>
        ))}
      </ul>
    </SummaryBlock>
  )
}

const MATTER_KINDS = new Set(['permission', 'action', 'promise', 'disclosure', 'misunderstanding', 'attitude'])

function matterKindLabel(t: TFunction, kind: string): string {
  return MATTER_KINDS.has(kind) ? t(`characterWindow.matter.${kind}`) : kind
}

/**
 * The bottom affordance for an arrangement that was actually made.
 *
 * The other side's commitments are shown as they are recorded and are not
 * editable: an editable copy of them is how a summary edit would hand an NPC an
 * obligation they never accepted. The player's own plan is editable, and it is
 * the only thing sent down the story's send chain.
 */
function NegotiatedActionBlock({ action, plan, name, disabled, onPlanChange, onAct }: {
  action: NegotiatedAction
  plan: string
  name: string
  disabled: boolean
  onPlanChange: (value: string) => void
  onAct: () => void
}) {
  const { t } = useTranslation()
  const first = action.agreed[0]
  return (
    <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] py-2">
      <SummaryBlock
        label={t('characterWindow.negotiated.title')}
        count={action.agreed.length}
        summary={first ? `${matterKindLabel(t, first.kind)} · ${first.statement}` : undefined}
        className="border-b-0 px-3"
      >
        <ul className="flex flex-col gap-0.5">
          {action.agreed.map((matter) => (
            <li key={matter.id} className="flex items-start gap-1.5 text-[11px] text-[var(--nova-text-muted)]">
              <span className="shrink-0 text-[var(--nova-text-faint)]">{matterKindLabel(t, matter.kind)}</span>
              <span className="min-w-0 flex-1">
                {matter.statement}
                {matter.deadline ? <span className="ml-1 text-[var(--nova-text-faint)]">（{t('characterWindow.negotiated.until', { deadline: matter.deadline })}）</span> : null}
              </span>
            </li>
          ))}
        </ul>
      </SummaryBlock>
      <div className="px-3 pt-1.5">
        <Textarea
          value={plan}
          onChange={(event) => onPlanChange(event.target.value)}
          minRows={1}
          maxRows={3}
          placeholder={t('characterWindow.negotiated.planPlaceholder', { name })}
          className="mb-1.5 min-h-8 resize-none bg-[var(--nova-surface)] text-[12px]"
          disabled={disabled}
        />
        <Button type="button" size="sm" className="h-7 w-full text-[11px]" disabled={disabled || !composeNegotiatedMessage(action, plan)} onClick={onAct}>
          {t('characterWindow.negotiated.act')}
        </Button>
      </div>
    </div>
  )
}

function ExchangeRow({ exchange, name }: { exchange: CharacterExchange; name: string }) {
  const { t } = useTranslation()
  const mine = exchange.speaker === 'player'
  return (
    <div className={cn('flex flex-col gap-0.5', mine ? 'items-end' : 'items-start')}>
      <span className="text-[10px] text-[var(--nova-text-faint)]">
        {mine ? t('characterWindow.speaker.you') : name}
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
