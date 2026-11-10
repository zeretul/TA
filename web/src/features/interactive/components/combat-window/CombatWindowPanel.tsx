import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useWorldPresentation } from '../../world-presentation-context'
import type { TFunction } from 'i18next'
import { X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import type {
  CombatActionRequest, CombatExchangeOutcome, CombatExchangeStep, CombatWindowEncounter,
  CombatWindowRound, CombatWindowSide, CombatWindowSkill, PlayerCombatIntent,
} from '../../types'
import {
  itemEffectLabel, itemLabel, poolLabel, reasonLabel, realmLabel, resultLabel, sideCount, skillCostLabel, skillEffectLabel, skillLabel, skillRefusalLabel, standingLabel, techniqueLabel,
  variantEffectLabel, variantLabel,
} from './combat-format'
import { useCombatWindow } from './use-combat-window'
import { EntryDetails } from '../entry-details/EntryDetails'
import { combatEquipmentPresentation, combatItemPresentation, combatSkillPresentation } from '../entry-details/combat-presentations'

interface CombatWindowPanelProps {
  storyId: string
  branchId: string
  /** True while the panel is on screen. It is what starts the read. */
  active: boolean
  /** The battle whose window is open, or '' to show the scene and live battles. */
  sessionId: string
  onSelectSession: (sessionId: string) => void
  /**
   * Collapses the panel. It is deliberately the only thing closing does: §七.1
   * requires closing to be a display act that neither flees nor ends the fight,
   * and there is no route for it to call.
   */
  onClose: () => void
  /**
   * Ends this battle and returns to the main story, carrying what the fight
   * actually settled.
   *
   * It is offered only once the battle has reached a result. §3 of the handoff
   * design is explicit that a fight still running may only be collapsed: there
   * is no button that ends a live battle, because ending it is the rules'
   * decision, not a display act. The message travels down the ordinary send
   * chain — the battle's result is read from the journal by the turn rather than
   * restated here — so this is not a second action path.
   */
  onContinueStory?: (message: string) => void
  /**
   * Dock/float/minimize controls, when the stage hosts this panel in a window
   * that can move. They sit in this header rather than in a second title bar.
   */
  windowActions?: ReactNode
  className?: string
}

/**
 * The battle window: both sides with the numbers the fight is actually being
 * fought at, the techniques the player has really learned, and the committed
 * rounds.
 *
 * Reading is free. Playing a round resolves and commits it in one call, so the
 * panel never shows a round the journal does not have, and a retry of a lost
 * response is answered from the committed record rather than resolving again.
 */
export function CombatWindowPanel({ storyId, branchId, active, sessionId, onSelectSession, onClose, onContinueStory, windowActions, className }: CombatWindowPanelProps) {
  const { t, terms } = useWorldPresentation()
  const window = useCombatWindow({ storyId, branchId, sessionId, active })
  const [selected, setSelected] = useState<SelectedAction>({ kind: 'strike', targetIds: [] })
  const [playerIntent, setPlayerIntent] = useState<PlayerCombatIntent>('spare')
  const [starting, setStarting] = useState(false)

  const sides = window.data?.sides || []
  const nameOf = useMemo(() => {
    const byID = new Map(sides.map((combatant) => [combatant.actor_id, combatant.actor_id === 'protagonist' && (!combatant.name || combatant.name === combatant.actor_id) ? t('combatWindow.protagonist') : combatant.name || combatant.actor_id]))
    return (id: string) => byID.get(id) || id
  }, [sides, t])
  // A settled dose step names its definition rather than a technique, so the
  // wording has to come from the item name keys the catalogue published. The
  // map is built from the sides' own stock rather than from a second table, so a
  // definition the window never offered cannot be given a name here either.
  const itemNameOf = useMemo(() => {
    const byDef = new Map<string, string>()
    for (const combatant of sides) {
      for (const item of combatant.items || []) {
        if (item.name_key && !byDef.has(item.def_id)) byDef.set(item.def_id, item.name_key)
      }
    }
    return byDef
  }, [sides])

  // A technique this story generated has no localisation key: its name is the
  // story's own wording, carried beside the definition ID. The map is built from
  // the player's own learned set, which is the only place the window is ever sent
  // such a name.
  const skillNameOf = useMemo(() => {
    const byID = new Map<string, string>()
    for (const combatant of sides) {
      for (const skill of combatant.skills || []) {
        const name = (skill.name || '').trim()
        if (name && !byID.has(skill.id)) byID.set(skill.id, name)
      }
      const prepared = combatant.preparation
      const preparedName = (prepared?.name || '').trim()
      if (prepared && preparedName && !byID.has(prepared.skill_id)) byID.set(prepared.skill_id, preparedName)
    }
    return byID
  }, [sides])

  // Allies stay visible, but player-side affiliation never grants control.
  // Only the protagonist identity explicitly projected by the rules may act.
  const player = useMemo(() => sides.filter((combatant) => combatant.side === 'player'), [sides])
  const enemies = useMemo(() => sides.filter((combatant) => combatant.side === 'enemy' && combatant.standing === 'standing'), [sides])
  // §3 requires a passage that was never attempted to read differently from one
  // whose attempt failed. The journal knows the round has no prose; it cannot
  // know why, so the panel pairs that durable fact with this session's own
  // failure, and reports `failed` only when the failure belongs to this round.
  const recovery = useMemo(() => {
    const round = window.data?.last_round
    if (!round || (round.narration || '').trim()) return null
    const failure = window.narrationFailure
    if (failure && failure.roundEventId === round.event_id) {
      return { state: 'failed' as const, message: failure.message }
    }
    if (!window.data?.narration_error) return null
    // A caller that asked for the prose in the same request is told directly
    // that the attempt failed; the two-phase caller learns it from its own call.
    if (window.data.narration_state === 'failed') {
      return { state: 'failed' as const, message: window.data.narration_error }
    }
    return { state: 'pending' as const, message: window.data.narration_error }
  }, [window.data?.last_round, window.data?.narration_error, window.data?.narration_state, window.narrationFailure])
  const actor = player.find((combatant) => combatant.actor_id === window.data?.player_actor_id && combatant.standing === 'standing')
  const preparationSkillID = actor?.preparation?.skill_id
  // The target is fixed before the round is paid for. With one opponent there is
  // nothing to ask, so it is chosen for the player; with several the window asks
  // rather than guessing, because a preparation binds to the opponent it names
  // and the completion will not pick a different one.
  const enemyIDs = enemies.map((enemy) => enemy.actor_id).join(',')
  useEffect(() => {
    setSelected((current) => {
      if (current.kind !== 'strike' && current.kind !== 'skill') return current
      const reach = reachOf(current, actor?.skills)
      const kept = current.targetIds.filter((id) => enemies.some((enemy) => enemy.actor_id === id)).slice(0, reach)
      if (kept.length) return kept.length === current.targetIds.length ? current : { ...current, targetIds: kept }
      // A single opponent is not a choice, so the window keeps its selection in
      // step with it. Several opponents are a choice, and one the player has not
      // made stays unmade rather than being guessed.
      const targetIds = defaultTargetIds(enemies).slice(0, reach)
      return targetIds.length ? { ...current, targetIds } : current
    })
  }, [enemyIDs])
  useEffect(() => {
    setSelected((current) => {
      if (preparationSkillID && current.kind !== 'continue_preparation' && current.kind !== 'cancel_preparation') {
        return { kind: 'continue_preparation' }
      }
      if (!preparationSkillID && (current.kind === 'continue_preparation' || current.kind === 'cancel_preparation')) {
        return { kind: 'strike', targetIds: defaultTargetIds(enemies) }
      }
      return current
    })
  }, [actor?.actor_id, preparationSkillID])

  const actions = useMemo(() => {
    if (!actor || window.data?.ended) return []
    const built = buildAction(actor, selected, enemies)
    return built ? [built] : []
  }, [actor, selected, enemies, window.data?.ended])
  const selectedActionKey = JSON.stringify(selected)
  useEffect(() => setPlayerIntent('spare'), [selectedActionKey])

  const canPlay = Boolean(actor) && !window.data?.ended && !window.playing && actions.length > 0
  // §七.1: an opened encounter has no exchange behind it yet. The panel says so
  // rather than leaving an empty history to be read as a lost round, and the
  // action selector below is what the player is being asked for.
  const awaitingFirstAction = Boolean(window.data?.session_id) && !window.data?.ended &&
    !window.data?.last_round && (window.data?.history || []).length === 0
  const play = () => {
    if (!canPlay) return
    // An unconfirmed attempt is not a round to replay with whatever is now
    // selected: it is resent exactly as it was minted, because the first attempt
    // may already have settled and a second round beside it would settle twice.
    if (window.pendingCommandResendable && (window.playErrorKind === 'unknown' || window.playErrorKind === 'blocked')) {
      void window.resendPending()
      return
    }
    // A round the server definitely refused keeps its chosen action and retries
    // as a new command under a new identity.
    const round = window.playError
      ? window.retryRound(actions, playerIntent)
      : window.playRound(actions, playerIntent)
    void round.then((eventID) => {
      if (eventID) setPlayerIntent('spare')
    })
  }

  // Entering the battle a refusal named. The panel selects it, because the host
  // owns which battle is open, and the hook drops the offer as the selection
  // changes. The refused opening's command identity is discarded on the way: it
  // was never committed, and the next round is a new one in the existing fight.
  const enterRunningBattle = () => {
    const target = window.runningBattleId
    if (!target) return
    window.dismissRunningBattle()
    onSelectSession(target)
  }

  // Ending the battle and returning to the story is offered only once the fight
  // has actually reached a result. A live battle has no such button: §3 of the
  // handoff design allows only collapse, an actual retreat, a surrender or
  // another legal action, and collapsing must not be able to end the fight.
  const continueStory = () => {
    if (!onContinueStory || window.playing) return
    onContinueStory(t('combatWindow.continueStoryMessage'))
  }

  return (
    <div data-testid="combat-window-panel" className={cn('flex h-full min-h-0 flex-col bg-[var(--nova-surface)] text-[var(--nova-text)]', className)}>
      <header className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2">
        <h2 className="min-w-0 flex-1 truncate text-[13px] font-medium">{terms.combat}</h2>
        {window.data?.round && !window.data.ended ? (
          <Badge variant="outline" className="h-5 shrink-0 border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 text-[10px] font-normal text-[var(--nova-text-muted)]">
            {t('combatWindow.round', { round: window.data.round })}
          </Badge>
        ) : null}
        {windowActions}
        <Button type="button" variant="ghost" size="icon-sm" onClick={onClose} aria-label={t('worldPresentation.closeCombat', terms)} title={t('combatWindow.closeHint')}>
          <X className="h-4 w-4" />
        </Button>
      </header>

      <SceneBlock scene={window.data?.scene} />

      {window.data?.lethal_risk ? (
        <p className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] text-[var(--nova-danger)]">
          {t('combatWindow.lethalRisk')}
        </p>
      ) : null}

      {window.runningBattleId ? (
        <RunningBattleOffer
          sessionId={window.runningBattleId}
          onEnter={enterRunningBattle}
          onDismiss={window.dismissRunningBattle}
        />
      ) : null}

      {window.error ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center">
          <p className="text-[12px] text-[var(--nova-danger)]">{t('combatWindow.loadFailed')}</p>
          <p className="text-[11px] text-[var(--nova-text-faint)]">{window.error}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void window.reload()}>{t('combatWindow.retry')}</Button>
        </div>
      ) : window.loading ? (
        <div className="flex flex-1 items-center justify-center gap-2 text-[12px] text-[var(--nova-text-muted)]">
          <Spinner className="h-3.5 w-3.5" />
          {t('combatWindow.loading')}
        </div>
      ) : !sessionId ? (
        <BattlePicker
          sessions={window.data?.sessions || []}
          encounter={window.data?.encounter}
          starting={starting}
          playing={window.playing}
          // A refusal that named the running battle is answered by the offer
          // above, which says what happened and what to do next. Printing the
          // raw transport error beside it would read as "retry this" when the
          // retry is exactly what cannot succeed.
          playError={window.runningBattleId ? null : window.playError}
          onStart={() => setStarting(true)}
          onCancelStart={() => setStarting(false)}
          onSelect={onSelectSession}
          onBegin={async (combatants) => {
            // Opening establishes the encounter and settles nothing, so the
            // window selects it and the player picks the first action from the
            // selector that then appears.
            const opened = await window.startBattle(combatants)
            // The battle exists now, so the window selects it: the caller owns
            // which battle is open, and without this the picker would offer to
            // start a second fight beside the one that was just opened.
            if (opened) onSelectSession(opened)
          }}
        />
      ) : (
        <>
          <div className="@container flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-3 py-2">
            {window.data?.ended ? (
              <div className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-[11px]">
                <span className="text-[var(--nova-text-muted)]">{t('combatWindow.ended')}</span>
                {window.data.result ? <span className="ml-1 text-[var(--nova-text)]">{resultLabel(window.data.result, t)}</span> : null}
              </div>
            ) : null}
            {/* §2: the two sides sit beside each other when the window is wide
                enough and stack naturally when it is not. A fixed two-column
                grid would squeeze the prose into a narrow column on a narrow
                window, which §5 forbids. */}
            <div className="flex flex-col gap-3 @md:flex-row @md:items-start @md:gap-4">
              <div className="@md:min-w-0 @md:flex-1">
                <SideBlock title={t('combatWindow.side.player')} sides={player} count={sideCount(sides, 'player')} nameOf={nameOf} skillNameOf={skillNameOf} />
              </div>
              <div className="@md:min-w-0 @md:flex-1">
                <SideBlock title={t('combatWindow.side.enemy')} sides={enemies} count={sideCount(sides, 'enemy')} nameOf={nameOf} skillNameOf={skillNameOf} />
              </div>
            </div>
            {awaitingFirstAction ? (
              <p className="rounded border border-dashed border-[var(--nova-border)] px-2 py-1.5 text-[11px] text-[var(--nova-text-muted)]">
                {t('combatWindow.awaitingFirstAction')}
              </p>
            ) : null}
            {window.data?.last_round ? (
              <>
                {window.data.last_round.fatalities?.map((fatality) => (
                  <p key={`${fatality.source_event_id}:${fatality.step_index}:${fatality.actor_id}`} className="rounded border border-[var(--nova-danger)]/40 bg-[var(--nova-surface-2)] px-2 py-1.5 text-[11px] text-[var(--nova-danger)]">
                    {t('combatWindow.fatality', { actor: nameOf(fatality.actor_id), killer: nameOf(fatality.killer_actor_id) })}
                  </p>
                ))}
                <OutcomeBlock
                  outcome={window.data.last_round.outcome}
                  narration={window.data.last_round.narration}
                  nameOf={nameOf}
                  itemNameOf={itemNameOf}
                  skillNameOf={skillNameOf}
                />
              </>
            ) : null}
            {recovery ? (
              <NarrationRecovery
                state={recovery.state}
                message={recovery.message}
                busy={window.narrating}
                onRetry={() => {
                  const round = window.data?.last_round
                  if (round) void window.narrateRound(round.event_id)
                }}
              />
            ) : null}
            <HistoryBlock
              rounds={window.data?.history || []}
              awaitingFirstAction={awaitingFirstAction}
              hasMore={Boolean(window.hasMore)}
              loadingEarlier={window.loadingEarlier}
              narrating={window.narrating}
              nameOf={nameOf}
              itemNameOf={itemNameOf}
              skillNameOf={skillNameOf}
              onLoadEarlier={() => void window.loadEarlier()}
              onNarrate={(roundEventId) => void window.narrateRound(roundEventId)}
            />
          </div>

          {window.data?.ended ? (
            <div className="shrink-0 border-t border-[var(--nova-border)] px-3 py-2">
              <p className="mb-1.5 text-[11px] text-[var(--nova-text-faint)]">{t('combatWindow.continueStoryHint')}</p>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-7 w-full text-[11px]"
                disabled={!onContinueStory || window.playing}
                onClick={continueStory}
              >
                {t('combatWindow.continueStory')}
              </Button>
            </div>
          ) : (
            <ActionBlock
              actor={actor}
              enemies={enemies}
              selected={selected}
              onSelect={(action) => {
                setSelected(action)
                setPlayerIntent('spare')
              }}
              playerIntent={playerIntent}
              onPlayerIntentChange={setPlayerIntent}
              playing={window.playing}
              narrating={window.narrating}
              canPlay={canPlay}
              playError={window.playError}
              playErrorKind={window.playErrorKind}
              pendingResendable={window.pendingCommandResendable}
              pendingIntent={window.pendingCommandIntent}
              pendingTargetID={window.pendingCommandTargetID}
              onPlay={play}
              skillNameOf={skillNameOf}
            />
          )}
        </>
      )}
    </div>
  )
}

/**
 * One round's prose, or a plain statement that there is none yet.
 *
 * The passage is the round's official description, so it is shown as the
 * story's own text rather than as a caption on the numbers — and at a size meant
 * for reading, not as a footnote under the figures (§1).
 */
function NarrationBlock({ narration, collapsible = false }: { narration?: string; collapsible?: boolean }) {
  const { t } = useWorldPresentation()
  const text = (narration || '').trim()
  if (!text) return null
  if (!collapsible) {
    return (
      <p className="mt-1.5 whitespace-pre-wrap text-[13px] leading-relaxed text-[var(--nova-text)]">{text}</p>
    )
  }
  return (
    <details className="mt-1.5">
      <summary className="cursor-pointer select-none text-[11px] text-[var(--nova-text-muted)]">
        {t('combatWindow.roundNarration')}
      </summary>
      <p className="mt-1 whitespace-pre-wrap text-[13px] leading-relaxed text-[var(--nova-text)]">{text}</p>
    </details>
  )
}

/**
 * The recovery path for a round whose passage is missing.
 *
 * §8 keeps the settled round and its expression apart: the round is committed
 * and stands, and what is missing is only its description. So this offers to
 * write the passage against the same round record, and never to play the round
 * again.
 *
 * §3 requires "待生成" and "失败" to be visibly different. They are: a passage
 * that was never attempted says so and offers nothing to explain, while one
 * whose attempt failed carries the reason it produced.
 */
function NarrationRecovery({ state, message, busy, onRetry }: { state?: 'pending' | 'failed'; message?: string; busy: boolean; onRetry: () => void }) {
  const { t } = useWorldPresentation()
  const failed = state === 'failed'
  return (
    <div className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-2">
      <p className="text-[12px] text-[var(--nova-text-muted)]">
        {failed ? t('combatWindow.narrationFailed') : t('combatWindow.narrationPending')}
      </p>
      {failed && message ? (
        <p className="mt-0.5 text-[11px] text-[var(--nova-text-faint)]">{message}</p>
      ) : null}
      <p className="mt-0.5 text-[11px] text-[var(--nova-text-faint)]">
        {failed ? t('combatWindow.narrationFailedHint') : t('combatWindow.narrationPendingHint')}
      </p>
      <Button type="button" variant="outline" size="sm" className="mt-1.5 text-[11px]" disabled={busy} onClick={onRetry}>
        {busy ? t('combatWindow.narrationWriting') : t('combatWindow.narrationRetry')}
      </Button>
    </div>
  )
}

/**
 * The round the player is composing. A plain attack is the zero-cost technique
 * the rules publish, so it is chosen the same way a learned technique is rather
 * than through a separate menu of tactics the design removed.
 */
type SelectedAction =
  | { kind: 'strike'; targetIds: string[] }
  /**
   * variant names a published alternative form of the technique, if the player
   * chose one. It is the action's own `variant` value: §5 makes a variant a way
   * of casting rather than a second registered technique, so it travels beside
   * the skill rather than replacing it.
   */
  | { kind: 'skill'; skillId: string; targetIds: string[]; variant?: string }
  | { kind: 'continue_preparation' }
  | { kind: 'cancel_preparation' }
  | { kind: 'rest' }
  | { kind: 'escape' }
  | { kind: 'surrender' }
  | { kind: 'accept_surrender'; targetId: string }
  | { kind: 'use_item'; instanceId: string }
  /**
   * A spare taken from the pack into its own position. §2 makes the change cost
   * a primary action, so it is a kind of its own rather than a modifier on a
   * technique: the round cannot also cast what was just taken up.
   */
  | { kind: 'swap_equipment'; instanceId: string }

/**
 * The opponents a damaging technique is aimed at, in the order the player chose
 * them. The first is the primary target: a preparation binds to it and the
 * completion strikes it.
 *
 * The list exists because §9 settles a multi-target technique from the actual
 * selection and the cap — 多目标按实际选择和上限结算，不默认打满敌队. A technique that
 * reaches one opponent therefore never carries a second name, and one that
 * reaches three carries only the opponents the player picked.
 */
function defaultTargetIds(enemies: CombatWindowSide[]): string[] {
  return enemies.length === 1 ? [enemies[0].actor_id] : []
}

/** How many opponents the chosen technique reaches. A plain attack reaches one. */
function reachOf(selected: SelectedAction, skills: CombatWindowSkill[] | undefined): number {
  if (selected.kind !== 'skill') return 1
  const skill = (skills || []).find((entry) => entry.id === selected.skillId)
  return Math.max(1, skill?.targets || 1)
}

/**
 * The opponents this declaration is aimed at.
 *
 * Names of opponents who have left the fight are dropped rather than kept, and
 * the list is trimmed to what the technique reaches. With a single standing
 * opponent there is nothing to decide, so it is filled in here rather than
 * waiting for an effect to catch up: the round has to be playable the moment the
 * window is readable.
 */
function resolveTargetIds(selected: SelectedAction, enemies: CombatWindowSide[], reach: number): string[] {
  if (selected.kind !== 'strike' && selected.kind !== 'skill') return []
  const standing = new Set(enemies.map((enemy) => enemy.actor_id))
  const kept = selected.targetIds.filter((id) => standing.has(id))
  if (kept.length > 0) return kept.slice(0, reach)
  return defaultTargetIds(enemies).slice(0, reach)
}

function buildAction(actor: CombatWindowSide, selected: SelectedAction, enemies: CombatWindowSide[]): CombatActionRequest | null {
  const reach = reachOf(selected, actor.skills)
  switch (selected.kind) {
    case 'strike': {
      const targetIds = resolveTargetIds(selected, enemies, reach)
      return targetIds.length
        ? { actor_id: actor.actor_id, kind: 'technique', skill_id: 'strike', target_id: targetIds[0] }
        : null
    }
    case 'skill': {
      const targetIds = resolveTargetIds(selected, enemies, reach)
      if (!targetIds.length) return null
      const skill = (actor.skills || []).find((entry) => entry.id === selected.skillId)
      // A form the rules did not publish for this technique is not sent: the
      // engine would refuse it anyway, and sending it would be the window
      // claiming a capability the player does not have.
      const variant = (skill?.variants || []).some((entry) => entry.id === selected.variant) ? selected.variant : undefined
      const action: CombatActionRequest = { actor_id: actor.actor_id, kind: 'technique', skill_id: selected.skillId, target_id: targetIds[0] }
      if (variant) action.variant = variant
      // The primary target stays in target_id because a preparation binds to it;
      // the rest are the additional opponents the same cast reaches.
      if (targetIds.length > 1) action.target_ids = targetIds.slice(1)
      return action
    }
    case 'continue_preparation':
      return actor.preparation ? { actor_id: actor.actor_id, kind: 'continue_preparation' } : null
    case 'cancel_preparation':
      return actor.preparation ? { actor_id: actor.actor_id, kind: 'cancel_preparation' } : null
    case 'rest':
      return { actor_id: actor.actor_id, kind: 'rest' }
    case 'escape':
      // A withdrawal needs a route that actually exists; without one the rules
      // refuse it, so the button is not offered.
      return actor.route_exists ? { actor_id: actor.actor_id, kind: 'escape' } : null
    case 'surrender':
      return { actor_id: actor.actor_id, kind: 'surrender' }
    case 'accept_surrender':
      return { actor_id: actor.actor_id, kind: 'accept_surrender', target_id: selected.targetId }
    case 'use_item': {
      // Only the copy is named. The effect is priced by the rules from the
      // catalogue, so nothing here can make a dose stronger than it is.
      const item = (actor.items || []).find((entry) => entry.instance_id === selected.instanceId)
      if (!item || !item.usable) return null
      return { actor_id: actor.actor_id, kind: 'use_item', item_instance_id: item.instance_id }
    }
    case 'swap_equipment': {
      // Only the spare is named. The position it goes to is read by the server
      // from the copy's own definition, so a request cannot hang a blade in a
      // treasure row.
      const spare = (actor.spares || []).find((entry) => entry.instance_id === selected.instanceId)
      if (!spare) return null
      return { actor_id: actor.actor_id, kind: 'swap_equipment', swap_instance_id: spare.instance_id }
    }
    default:
      return null
  }
}

/**
 * Why the chosen action cannot be played, when it cannot.
 *
 * The choices are picked before the round is settled, so a choice can go stale:
 * a dose selected last round may be spent by the time the window re-reads the
 * battle. Without this the play button would simply stay dark and the player
 * would have no way to know which of their choices had stopped being legal.
 */
function selectionBlockReason(actor: CombatWindowSide, selected: SelectedAction, enemies: CombatWindowSide[], t: TFunction): string {
  switch (selected.kind) {
    case 'strike':
    case 'skill': {
      if (enemies.length === 0) return t('combatWindow.targetNone')
      if (!selected.targetIds.length) return t('combatWindow.targetChoose')
      if (selected.kind === 'skill') {
        const skill = (actor.skills || []).find((entry) => entry.id === selected.skillId)
        if (!skill) return t('combatWindow.selectionGone')
        if (!skill.castable) return skillRefusalLabel(skill, actor, t) || t('combatWindow.skillUnavailable')
      }
      return ''
    }
    case 'use_item': {
      const item = (actor.items || []).find((entry) => entry.instance_id === selected.instanceId)
      if (!item) return t('combatWindow.selectionGone')
      if (!item.usable) return reasonLabel(item.reason_key, t) || t('combatWindow.itemUnavailable')
      return ''
    }
    case 'swap_equipment': {
      const spare = (actor.spares || []).find((entry) => entry.instance_id === selected.instanceId)
      return spare ? '' : t('combatWindow.selectionGone')
    }
    case 'escape':
      return actor.route_exists ? '' : t('combatWindow.actionEscapeNoRoute')
    case 'continue_preparation':
    case 'cancel_preparation':
      return actor.preparation ? '' : t('combatWindow.preparation.none')
    default:
      return ''
  }
}

/** One carried consumable, with what the rules would really pay for it. */
function ItemList({ actor, selected, onSelect }: { actor: CombatWindowSide; selected: SelectedAction; onSelect: (action: SelectedAction) => void }) {
  const { t } = useWorldPresentation()
  const items = actor.items || []
  if (items.length === 0) return null
  return (
    <div className="flex flex-col gap-1">
      <p className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.items')}</p>
      {items.map((item) => {
        const chosen = selected.kind === 'use_item' && selected.instanceId === item.instance_id
        const blocked = !item.usable
        return (
          <div key={item.instance_id} className="min-w-0">
          <button
            type="button"
            disabled={blocked}
            onClick={() => onSelect({ kind: 'use_item', instanceId: item.instance_id })}
            className={cn(
              'flex w-full flex-col items-start gap-0.5 rounded border px-2 py-1 text-left text-[11px]',
              chosen ? 'border-[var(--nova-accent)] bg-[var(--nova-hover)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)]',
              blocked ? 'cursor-not-allowed opacity-55' : 'hover:bg-[var(--nova-hover)]',
            )}
          >
            <span className="flex w-full flex-wrap items-baseline gap-x-2">
              <span className="min-w-0 truncate text-[var(--nova-text)]">{itemLabel(item, t)}</span>
              <span className="text-[10px] text-[var(--nova-text-muted)]">{itemEffectLabel(item, t)}</span>
              <span className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.itemRemaining', { count: item.quantity })}</span>
            </span>
            {blocked ? (
              <span className="text-[10px] text-[var(--nova-danger)]">{reasonLabel(item.reason_key, t) || t('combatWindow.itemUnavailable')}</span>
            ) : null}
          </button>
          <EntryDetails entry={combatItemPresentation(item, t)} compact />
          </div>
        )
      })}
    </div>
  )
}

/**
 * What the character is really wearing, position by position.
 *
 * All five rows are drawn whether or not they hold anything: §1 gives the sheet
 * five positions, and showing only the filled ones would misdescribe what the
 * rules allow. A free row says so rather than disappearing.
 *
 * Nothing here is a button. A position is a fact about the fight, and the one
 * action that changes it — taking a spare up — is offered with the spares, where
 * its cost is stated.
 */
function EquipmentList({ actor }: { actor: CombatWindowSide }) {
  const { t } = useWorldPresentation()
  const positions = actor.equipment || []
  if (positions.length === 0) return null
  return (
    <div className="flex flex-col gap-1">
      <p className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.equipment')}</p>
      <div className="grid grid-cols-1 gap-1 sm:grid-cols-2">
        {positions.map((position) => (
          <div
            key={position.slot}
            className="flex min-w-0 flex-col gap-0.5 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[11px]"
          >
            <span className="text-[10px] text-[var(--nova-text-faint)]">{slotLabel(position.slot, t)}</span>
            <span className="flex min-w-0 flex-wrap items-baseline gap-x-2">
              <span className={cn('min-w-0 truncate', position.name ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)]')}>
                {position.name || t('combatWindow.equipmentEmpty')}
              </span>
              {position.operating_mana_cost ? (
                <span className="text-[10px] text-[var(--nova-text-muted)]">
                  {t('combatWindow.equipmentCost', { cost: position.operating_mana_cost })}
                </span>
              ) : null}
            </span>
            {position.instance_id ? <EntryDetails entry={combatEquipmentPresentation(position, t)} compact /> : null}
          </div>
        ))}
      </div>
    </div>
  )
}

/**
 * The spares a mid-fight change could take up.
 *
 * §1 keeps a pack copy from contributing anything, so no number is shown for a
 * spare: what it would do is a fact about its definition, answered once it is
 * really in a position. §2 is stated beside the list, because the choice costs
 * the round's primary action and the player should know that before choosing.
 */
function SpareList({ actor, selected, onSelect }: { actor: CombatWindowSide; selected: SelectedAction; onSelect: (action: SelectedAction) => void }) {
  const { t } = useWorldPresentation()
  const spares = actor.spares || []
  if (spares.length === 0) return null
  return (
    <div className="flex flex-col gap-1">
      <p className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.spares')}</p>
      {spares.map((spare) => {
        const chosen = selected.kind === 'swap_equipment' && selected.instanceId === spare.instance_id
        return (
          <div key={spare.instance_id} className="min-w-0">
          <button
            type="button"
            onClick={() => onSelect({ kind: 'swap_equipment', instanceId: spare.instance_id })}
            className={cn(
              'flex w-full flex-col items-start gap-0.5 rounded border px-2 py-1 text-left text-[11px]',
              chosen ? 'border-[var(--nova-accent)] bg-[var(--nova-hover)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)]',
              'hover:bg-[var(--nova-hover)]',
            )}
          >
            <span className="flex w-full flex-wrap items-baseline gap-x-2">
              <span className="min-w-0 truncate text-[var(--nova-text)]">{spare.name || spare.instance_id}</span>
              <span className="text-[10px] text-[var(--nova-text-muted)]">{slotLabel(spare.slot, t)}</span>
            </span>
            <span className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.spareHint')}</span>
          </button>
          <EntryDetails entry={combatEquipmentPresentation(spare, t)} compact />
          </div>
        )
      })}
    </div>
  )
}

/** The wording for one of the five positions. */
function slotLabel(slot: string, t: TFunction): string {
  const key = `combatWindow.slot.${slot}`
  const label = t(key)
  return label === key ? slot : label
}

function SceneBlock({ scene }: { scene?: { title?: string; location?: string; world_time?: string } }) {
  const { t } = useWorldPresentation()
  if (!scene || (!scene.location && !scene.world_time && !scene.title)) return null
  return (
    <div className="shrink-0 border-b border-[var(--nova-border)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
      {scene.world_time ? <span>{t('combatWindow.sceneTime')} · {scene.world_time}</span> : null}
      {scene.title ? <span className={cn(scene.world_time && 'ml-2')}>{scene.title}</span> : null}
      {scene.location ? <span className={cn((scene.world_time || scene.title) && 'ml-2')}>{scene.location}</span> : null}
    </div>
  )
}

/** One side's combatants with the numbers the fight is being settled against. */
function SideBlock({ title, sides, count, nameOf, skillNameOf }: { title: string; sides: CombatWindowSide[]; count: number; nameOf: (id: string) => string; skillNameOf: Map<string, string> }) {
  const { t } = useWorldPresentation()
  return (
    <section>
      <div className="mb-1 flex items-center gap-2 text-[11px] text-[var(--nova-text-muted)]">
        <span>{title}</span>
        <span className="text-[var(--nova-text-faint)]">{t('combatWindow.count', { count })}</span>
      </div>
      {sides.length === 0 ? <p className="text-[11px] text-[var(--nova-text-faint)]">—</p> : null}
      <div className="flex flex-col gap-1.5">
        {sides.map((combatant) => <CombatantCard key={combatant.actor_id} combatant={combatant} name={nameOf(combatant.actor_id)} skillNameOf={skillNameOf} />)}
      </div>
    </section>
  )
}

function CombatantCard({ combatant, name, skillNameOf }: { combatant: CombatWindowSide; name: string; skillNameOf: Map<string, string> }) {
  const { t } = useWorldPresentation()
  const realm = realmLabel(combatant.realm, t)
  const observed = combatant.observed
  return (
    <div className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5">
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <span className="min-w-0 truncate text-[12px] text-[var(--nova-text)]">{name}</span>
        {realm ? <span className="text-[10px] text-[var(--nova-text-muted)]">{realm}</span> : null}
        <span className="text-[10px] text-[var(--nova-text-faint)]">{standingLabel(combatant.standing, t)}</span>
        {combatant.has_implement ? (
          <span className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.implementReady')}</span>
        ) : null}
      </div>
      {combatant.exact ? (
        <>
          <div className="mt-0.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[10px] text-[var(--nova-text-muted)]">
            <span>{t('combatWindow.health')} {poolLabel(combatant.health, combatant.health_max, t)}</span>
            <span>{t('combatWindow.mana')} {poolLabel(combatant.mana, combatant.mana_max, t)}</span>
            <span>{t('combatWindow.burden')} {combatant.burden ?? t('combatWindow.unknownValue')}</span>
            {/* The burden floor is the number the rules hold the burden above,
                which is not something the player acts on. §T21 keeps one word
                for it — 伤势 — and reads it as the fact it stands for. */}
            {combatant.burden_floor !== undefined ? (
              <span>
                {t('combatWindow.burdenFloor')} {combatant.burden_floor > 0 ? t('combatWindow.injured') : t('combatWindow.uninjured')}
              </span>
            ) : null}
          </div>
          {/* The four contest numbers, because a lasting effect is settled
              against them: a slow is only real to the player if they can see
              their own agility fall and come back. */}
          {combatant.attack !== undefined || combatant.agility !== undefined ? (
            <div className="mt-0.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[10px] text-[var(--nova-text-muted)]">
              {combatant.attack !== undefined ? <span>{t('combatWindow.attack')} {combatant.attack}</span> : null}
              {combatant.defense !== undefined ? <span>{t('combatWindow.defense')} {combatant.defense}</span> : null}
              {combatant.agility !== undefined ? <span>{t('combatWindow.agility')} {combatant.agility}</span> : null}
              {combatant.perception !== undefined ? <span>{t('combatWindow.perception')} {combatant.perception}</span> : null}
            </div>
          ) : null}
          {combatant.shield || combatant.burn_ticks || combatant.slow_actions ? (
            <div className="mt-0.5 flex flex-wrap gap-x-3 text-[10px] text-[var(--nova-text-faint)]">
              {combatant.shield ? <span>{t('combatWindow.shield', { amount: combatant.shield })}</span> : null}
              {combatant.burn_ticks ? <span>{t('combatWindow.burn', { count: combatant.burn_ticks })}</span> : null}
              {combatant.slow_actions ? <span>{t('combatWindow.slow', { count: combatant.slow_actions })}</span> : null}
            </div>
          ) : null}
        </>
      ) : (
        // An opponent is shown only as far as the fight has put them in front of
        // the player: a coarse visible condition, the arts that were actually
        // performed, and effects that are plainly in sight. There is no hidden
        // row behind a toggle — the server did not send the numbers at all.
        <div className="mt-0.5 flex flex-col gap-0.5 text-[10px] text-[var(--nova-text-muted)]">
          <div className="flex flex-wrap gap-x-3 gap-y-0.5">
            <span>{observedConditionLabel(observed?.condition, t)}</span>
            {observed?.shield ? <span>{t('combatWindow.observed.shield')}</span> : null}
            {observed?.burn_ticks ? <span>{t('combatWindow.observed.afire')}</span> : null}
            {observed?.slow_actions ? <span>{t('combatWindow.observed.slowed')}</span> : null}
            {observed?.doses ? <span>{t('combatWindow.observed.doses', { count: observed.doses })}</span> : null}
          </div>
          <span className="text-[var(--nova-text-faint)]">
            {t('combatWindow.observed.techniques')}
            {observed?.techniques?.length
              ? `：${observed.techniques.map((seen) => observedTechniqueLabel(seen, t)).join('、')}`
              : `：${t('combatWindow.observed.noTechniques')}`}
          </span>
          <span className="text-[var(--nova-text-faint)]">{t('combatWindow.observed.itemsUnknown')}</span>
        </div>
      )}
      {combatant.offer_text ? (
        <p className="mt-0.5 text-[10px] text-[var(--nova-accent)]">{t('combatWindow.offerPending', { text: combatant.offer_text })}</p>
      ) : null}
      {combatant.preparation ? (
        <p className="mt-0.5 text-[10px] text-[var(--nova-accent)]">
          {t('combatWindow.preparation.pending', { technique: techniqueLabel(combatant.preparation.skill_id, t, combatant.preparation.name || skillNameOf.get(combatant.preparation.skill_id)) })}
        </p>
      ) : null}
    </div>
  )
}

/** The coarse band the player can read off an opponent, or a plain unknown. */
function observedConditionLabel(condition: string | undefined, t: TFunction): string {
  if (!condition) return t('combatWindow.observed.conditionUnknown')
  const label = t(condition)
  return label === condition ? t('combatWindow.observed.conditionUnknown') : label
}

/**
 * One technique the player has watched an opponent perform.
 *
 * An unrecognised art is not given a name, a key or a cost: the player saw
 * something happen and does not know what it was, and inventing a label here
 * would tell them more than the fight did.
 */
function observedTechniqueLabel(seen: { round: number; recognized: boolean; skill_id?: string; name_key?: string; name?: string }, t: TFunction): string {
  if (!seen.recognized) return t('combatWindow.observed.unidentifiedTechnique')
  const ownName = (seen.name || '').trim()
  if (ownName) return ownName
  if (seen.name_key) {
    const translated = t(seen.name_key)
    if (translated !== seen.name_key) return translated
  }
  if (seen.skill_id) return techniqueLabel(seen.skill_id, t)
  return t('combatWindow.observed.unidentifiedTechnique')
}

/** The techniques this combatant has actually learned, with their real costs. */
function SkillList({ actor, enemies, selected, onSelect }: { actor: CombatWindowSide; enemies: CombatWindowSide[]; selected: SelectedAction; onSelect: (action: SelectedAction) => void }) {
  const { t } = useWorldPresentation()
  const skills = (actor.skills || []).filter((skill) => skill.id !== 'strike')
  const defaultTargets = defaultTargetIds(enemies)
  return (
    <div className="flex flex-col gap-1">
      <p className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.skills')}</p>
      {skills.length === 0 ? <p className="text-[11px] text-[var(--nova-text-faint)]">{t('combatWindow.noSkills')}</p> : null}
      {skills.map((skill) => {
        const chosen = selected.kind === 'skill' && selected.skillId === skill.id
        const blocked = !skill.castable
        const effect = skillEffectLabel(skill, t)
        // A technique that reaches more than one opponent starts from the ones
        // the player has already chosen, trimmed to what it can reach; one that
        // reaches a single opponent keeps the single choice it had.
        const reach = Math.max(1, skill.targets || 1)
        const targetIds = selected.kind === 'skill' || selected.kind === 'strike'
          ? resolveTargetIds(selected, enemies, reach)
          : defaultTargets.slice(0, reach)
        return (
          <div
            key={`${skill.id}@${skill.version}`}
            className={cn(
              'flex flex-col items-stretch gap-0.5 rounded border px-2 py-1.5 text-[11px]',
              chosen ? 'border-[var(--nova-accent)] bg-[var(--nova-hover)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)]',
            )}
          >
            <button
              type="button"
              disabled={blocked}
              onClick={() => onSelect({
                kind: 'skill', skillId: skill.id, targetIds,
                // Choosing the technique again keeps the form already chosen;
                // choosing a different one starts from its base form.
                variant: selected.kind === 'skill' && selected.skillId === skill.id ? selected.variant : undefined,
              })}
              className={cn(
                'flex flex-col items-start gap-0.5 text-left',
                blocked ? 'cursor-not-allowed opacity-55' : '',
              )}
            >
              <span className="flex w-full flex-wrap items-baseline gap-x-2">
                <span className="min-w-0 break-words text-[13px] text-[var(--nova-text)]">{skillLabel(skill, t)}</span>
                <span className="text-[11px] text-[var(--nova-text-muted)]">{skillCostLabel(skill, t)}</span>
                {reach > 1 ? <span className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.skillReach', { count: reach })}</span> : null}
              </span>
              {effect ? <span className="text-[11px] leading-snug text-[var(--nova-text-muted)]">{effect}</span> : null}
              {blocked ? (
                <span className="text-[11px] text-[var(--nova-danger)]">{skillRefusalLabel(skill, actor, t) || t('combatWindow.skillUnavailable')}</span>
              ) : null}
            </button>
            <EntryDetails entry={combatSkillPresentation(skill, t)} compact combat />
            {chosen && (skill.variants || []).length > 0 ? (
              // The published forms sit inside the chosen technique rather than
              // beside it, because a form is a way of casting this technique and
              // not a second technique the player has learned. Each form states
              // what it trades in visible text: §3 forbids leaving the effect on
              // a hover title, which a phone cannot show at all.
              <div className="mt-1 flex flex-col gap-1" role="group" aria-label={t('combatWindow.variantChoose')}>
                <button
                  type="button"
                  onClick={() => onSelect({ ...selected, variant: undefined })}
                  className={cn(
                    'flex flex-col items-start rounded border px-1.5 py-1 text-left text-[11px]',
                    !selected.variant ? 'border-[var(--nova-accent)] bg-[var(--nova-surface)]' : 'border-[var(--nova-border)]',
                  )}
                >
                  <span>{t('combatWindow.variantBase')}</span>
                </button>
                {(skill.variants || []).map((variant) => (
                  <button
                    key={variant.id}
                    type="button"
                    onClick={() => onSelect({ ...selected, variant: variant.id })}
                    className={cn(
                      'flex flex-col items-start rounded border px-1.5 py-1 text-left text-[11px]',
                      selected.variant === variant.id ? 'border-[var(--nova-accent)] bg-[var(--nova-surface)]' : 'border-[var(--nova-border)]',
                    )}
                  >
                    <span>{variantLabel(variant, t)}</span>
                    <span className="text-[10px] text-[var(--nova-text-muted)]">{variantEffectLabel(variant, skill.power, t)}</span>
                  </button>
                ))}
              </div>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

/** What the player does this round. */
function ActionBlock({
  actor, enemies, selected, onSelect, playerIntent, onPlayerIntentChange, playing, narrating, canPlay, playError, playErrorKind, pendingResendable, pendingIntent, pendingTargetID, onPlay, skillNameOf,
}: {
  actor?: CombatWindowSide
  enemies: CombatWindowSide[]
  selected: SelectedAction
  onSelect: (action: SelectedAction) => void
  playerIntent: PlayerCombatIntent
  onPlayerIntentChange: (intent: PlayerCombatIntent) => void
  playing: boolean
  /** True while the settled round's prose is still being written. */
  narrating: boolean
  canPlay: boolean
  playError: string | null
  /** How to read that failure. See `CombatWindowState.playErrorKind`. */
  playErrorKind: 'unknown' | 'conflict' | 'refused' | 'blocked' | null
  /** Whether an unconfirmed attempt is still holding an action to resend. */
  pendingResendable: boolean
  pendingIntent: PlayerCombatIntent | null
  pendingTargetID: string | null
  onPlay: () => void
  skillNameOf: Map<string, string>
}) {
  const { t } = useWorldPresentation()
  if (!actor) {
    return (
      <div className="shrink-0 border-t border-[var(--nova-border)] px-3 py-2 text-[11px] text-[var(--nova-text-faint)]">
        {t('combatWindow.noPlayerAction')}
      </div>
    )
  }
  const offering = enemies.filter((enemy) => enemy.offer_text)
  const blockReason = selectionBlockReason(actor, selected, enemies, t)
  const needsTarget = selected.kind === 'strike' || selected.kind === 'skill'
  const reach = reachOf(selected, actor.skills)
  // The picker appears whenever there is more than one opponent to choose
  // between — or when the chosen technique reaches more than one, because then
  // even a single standing opponent is one of several possible picks.
  const chooseTargets = needsTarget && (enemies.length > 1 || reach > 1)
  const chosenTargets = needsTarget ? resolveTargetIds(selected, enemies, reach) : []
  const lethalTargetID = needsTarget && chosenTargets.length === 1
    ? chosenTargets[0]
    : selected.kind === 'continue_preparation'
      ? actor.preparation?.target_id || ''
      : ''
  const lethalTarget = enemies.find((enemy) => enemy.actor_id === lethalTargetID)
  const toggleTarget = (enemyID: string) => {
    if (!needsTarget) return
    const already = chosenTargets.includes(enemyID)
    // A technique that reaches one opponent replaces its choice; one that reaches
    // several adds to it until the reach is full, and removing a name is how a
    // player says they no longer want that opponent struck.
    const next = already ? chosenTargets.filter((id) => id !== enemyID) : [...chosenTargets, enemyID].slice(0, reach)
    if (selected.kind === 'strike' || selected.kind === 'skill') onSelect({ ...selected, targetIds: next })
  }
  return (
    <div className="flex shrink-0 flex-col gap-2 border-t border-[var(--nova-border)] px-3 py-2">
      <SkillList actor={actor} enemies={enemies} selected={selected} onSelect={onSelect} />
      <EquipmentList actor={actor} />
      <SpareList actor={actor} selected={selected} onSelect={onSelect} />
      <ItemList actor={actor} selected={selected} onSelect={onSelect} />
      {chooseTargets ? (
        <div className="flex flex-col gap-1">
          <p className="text-[10px] text-[var(--nova-text-faint)]">
            {reach > 1 ? t('combatWindow.targetUpTo', { count: reach }) : t('combatWindow.target')}
          </p>
          <div className="flex flex-wrap gap-1.5">
            {enemies.map((enemy) => {
              const order = chosenTargets.indexOf(enemy.actor_id)
              return (
                <ActionChoice
                  key={enemy.actor_id}
                  label={order >= 0 ? `${order + 1}. ${enemy.name || enemy.actor_id}` : enemy.name || enemy.actor_id}
                  chosen={order >= 0}
                  onClick={() => toggleTarget(enemy.actor_id)}
                />
              )
            })}
          </div>
        </div>
      ) : null}
      {lethalTarget ? (
        <div className="flex flex-col gap-1">
          <p className="text-[10px] text-[var(--nova-text-faint)]">
            {t('combatWindow.playerIntent')} · {t('combatWindow.playerIntent.target', { target: lethalTarget.name || lethalTarget.actor_id })}
          </p>
          <div className="flex flex-wrap gap-1.5">
            <ActionChoice
              label={t('combatWindow.playerIntent.spare')}
              hint={t('combatWindow.playerIntent.spareHint')}
              chosen={(pendingResendable ? pendingIntent : playerIntent) === 'spare'}
              disabled={pendingResendable}
              onClick={() => onPlayerIntentChange('spare')}
            />
            <ActionChoice
              label={t('combatWindow.playerIntent.takeLife')}
              hint={t('combatWindow.playerIntent.takeLifeHint')}
              chosen={(pendingResendable ? pendingIntent : playerIntent) === 'take_life'}
              disabled={pendingResendable}
              onClick={() => onPlayerIntentChange('take_life')}
            />
          </div>
        </div>
      ) : null}
      {actor.preparation ? (
        <div className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5">
          <p className="text-[10px] text-[var(--nova-text-muted)]">
            {t('combatWindow.preparation.pending', { technique: techniqueLabel(actor.preparation.skill_id, t, actor.preparation.name || skillNameOf.get(actor.preparation.skill_id)) })}
          </p>
          <p className="mt-0.5 text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.preparation.costSettled')}</p>
        </div>
      ) : null}
      <div className="flex flex-col gap-1">
        <p className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.action')}</p>
        <div className="flex flex-wrap gap-1.5">
          {actor.preparation ? (
            <>
              <ActionChoice
                label={t('combatWindow.preparation.continue')}
                hint={t('combatWindow.preparation.continueHint')}
                chosen={selected.kind === 'continue_preparation'}
                onClick={() => onSelect({ kind: 'continue_preparation' })}
              />
              <ActionChoice
                label={t('combatWindow.preparation.cancel')}
                hint={t('combatWindow.preparation.cancelHint')}
                chosen={selected.kind === 'cancel_preparation'}
                onClick={() => onSelect({ kind: 'cancel_preparation' })}
              />
            </>
          ) : null}
          <ActionChoice
            label={t('combatWindow.actionTechnique')}
            hint={t('skill.strike.name')}
            chosen={selected.kind === 'strike'}
            onClick={() => onSelect({ kind: 'strike', targetIds: defaultTargetIds(enemies) })}
          />
          <ActionChoice
            label={t('combatWindow.actionRest')}
            hint={t('combatWindow.actionRestHint')}
            chosen={selected.kind === 'rest'}
            onClick={() => onSelect({ kind: 'rest' })}
          />
          <ActionChoice
            label={t('combatWindow.actionEscape')}
            hint={actor.route_exists ? t('combatWindow.actionEscapeHint') : t('combatWindow.actionEscapeNoRoute')}
            chosen={selected.kind === 'escape'}
            disabled={!actor.route_exists}
            onClick={() => onSelect({ kind: 'escape' })}
          />
          <ActionChoice
            label={t('combatWindow.actionSurrender')}
            hint={t('combatWindow.actionSurrenderHint')}
            chosen={selected.kind === 'surrender'}
            onClick={() => onSelect({ kind: 'surrender' })}
          />
          {offering.map((enemy) => (
            <ActionChoice
              key={enemy.actor_id}
              label={t('combatWindow.actionAcceptSurrender')}
              hint={t('combatWindow.actionAcceptSurrenderHint')}
              chosen={selected.kind === 'accept_surrender' && selected.targetId === enemy.actor_id}
              onClick={() => onSelect({ kind: 'accept_surrender', targetId: enemy.actor_id })}
            />
          ))}
        </div>
      </div>
      {blockReason ? (
        <p className="text-[11px] text-[var(--nova-text-muted)]">{blockReason}</p>
      ) : null}
      {playError || playErrorKind === 'unknown' || playErrorKind === 'blocked' ? (
        <div className="text-[11px] text-[var(--nova-danger)]">
          {pendingResendable && pendingIntent ? (
            <p className="mb-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[var(--nova-text)]">
              {t('combatWindow.pendingIntent', {
                intent: t(pendingIntent === 'take_life' ? 'combatWindow.playerIntent.takeLife' : 'combatWindow.playerIntent.spare'),
                target: enemies.find((enemy) => enemy.actor_id === pendingTargetID)?.name || pendingTargetID || t('combatWindow.unknownTarget'),
              })}
            </p>
          ) : null}
          {/* §4 forbids telling the player a round was not settled when the
              answer was merely lost: it may already be on the record. The
              wording follows what is actually known, and the retry hint says
              that a resend reads the same round rather than rolling again. The
              same wording covers a restored attempt, where no message survives
              the reload but the unknown outcome does. */}
          <p>
            {playErrorKind === 'unknown' || playErrorKind === 'blocked'
              ? t('combatWindow.playUnknown')
              : t('combatWindow.playFailed')} {playError}
          </p>
          {(playErrorKind === 'unknown' || playErrorKind === 'blocked') && pendingResendable ? (
            <p className="text-[var(--nova-text-faint)]">{t('combatWindow.playUnknownHint')}</p>
          ) : null}
          {playErrorKind === 'conflict' ? (
            <p className="text-[var(--nova-text-faint)]">{t('combatWindow.playConflictHint')}</p>
          ) : null}
        </div>
      ) : null}
      <Button type="button" size="sm" className="h-8 w-full text-[12px]" disabled={!canPlay} onClick={onPlay}>
        {playing
          ? t('combatWindow.playing')
          : narrating
            ? t('combatWindow.narrationWriting')
            : playError || playErrorKind === 'unknown' || playErrorKind === 'blocked' || playErrorKind === 'conflict'
              ? t('combatWindow.retryPlay')
              : t('combatWindow.play')}
      </Button>
    </div>
  )
}

/**
 * One choice in the action row.
 *
 * The hint is written under the label rather than into a `title`: §3 requires an
 * unavailable choice to explain itself on the spot, and a phone has no hover to
 * reveal a tooltip with. The same reason applies to a choice that is available —
 * what 求饶 or 调息 really does belongs beside the button, not behind a pointer.
 */
function ActionChoice({ label, hint, chosen, disabled, onClick }: { label: string; hint?: string; chosen: boolean; disabled?: boolean; onClick: () => void }) {
  const text = (hint || '').trim()
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={cn(
        'flex flex-col items-start rounded border px-2 py-1 text-left text-[12px]',
        chosen ? 'border-[var(--nova-accent)] bg-[var(--nova-hover)] text-[var(--nova-text)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]',
        disabled ? 'cursor-not-allowed opacity-55' : 'hover:bg-[var(--nova-hover)]',
      )}
    >
      <span>{label}</span>
      {text ? <span className="text-[10px] leading-snug text-[var(--nova-text-faint)]">{text}</span> : null}
    </button>
  )
}

/**
 * What one settled exchange actually produced.
 *
 * The settled result comes first and the prose after it (§3: "已提交结算先可见").
 * That order is not cosmetic — it is what makes the two independently readable:
 * the numbers are the journal's committed facts, and the passage is the
 * description written about them. Reading the prose first would hide the round
 * behind a call that may still be in flight.
 */
function OutcomeBlock({ outcome, narration, nameOf, itemNameOf, skillNameOf }: { outcome: CombatExchangeOutcome; narration?: string; nameOf: (id: string) => string; itemNameOf: Map<string, string>; skillNameOf: Map<string, string> }) {
  const { t } = useWorldPresentation()
  const steps = outcome.steps || []
  return (
    <section className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-2">
      <p className="text-[12px] font-medium text-[var(--nova-text)]">{t('combatWindow.round', { round: outcome.round })}</p>
      {outcome.order?.length ? (
        <p className="mt-0.5 text-[11px] text-[var(--nova-text-faint)]">
          {t('combatWindow.order')} · {outcome.order.map(nameOf).join(' → ')}
        </p>
      ) : null}
      <div className="mt-1 flex flex-col gap-0.5">
        {steps.map((step, index) => <StepRow key={`${step.actor_id}-${index}`} step={step} nameOf={nameOf} itemNameOf={itemNameOf} skillNameOf={skillNameOf} />)}
      </div>
      {narration?.trim() ? (
        <>
          <p className="mt-2 border-t border-[var(--nova-border)] pt-1.5 text-[10px] text-[var(--nova-text-faint)]">
            {t('combatWindow.roundNarration')}
          </p>
          <NarrationBlock narration={narration} />
        </>
      ) : null}
    </section>
  )
}

/**
 * The name of what a step did, in the player's language.
 *
 * A use_item step names the pill's definition in skill_id, because a settled
 * round has to say what was attempted and a definition is the stable identity.
 * It is worded from the item catalogue's own keys rather than through the
 * technique keys, so a dose never reads as an internal definition ID.
 */
function stepActionLabel(step: CombatExchangeStep, itemNameOf: Map<string, string>, skillNameOf: Map<string, string>, t: TFunction): string {
  const id = (step.skill_id || '').trim()
  if (!id) return ''
  if (step.kind === 'use_item') {
    const key = itemNameOf.get(id)
    if (key) {
      const translated = t(key)
      if (translated !== key) return translated
    }
    return t('combatWindow.unknownItem')
  }
  return techniqueLabel(id, t, skillNameOf.get(id))
}

function StepRow({ step, nameOf, itemNameOf, skillNameOf }: { step: CombatExchangeStep; nameOf: (id: string) => string; itemNameOf: Map<string, string>; skillNameOf: Map<string, string> }) {
  const { t } = useWorldPresentation()
  const lines: string[] = []
  // The technique comes first: a round has to say what was attempted before it
  // says what came of it, and a refused cast is only meaningful with it.
  const technique = stepActionLabel(step, itemNameOf, skillNameOf, t)
  if (technique) lines.push(technique)
  if (step.preparation_abandoned && step.reason_key !== 'reason.preparation_abandoned') {
    lines.push(t('combatWindow.preparation.abandoned'))
  }
  if (step.reason_key) {
    const key = `combatWindow.${step.reason_key}`
    const translated = t(key)
    lines.push(translated === key ? reasonLabel(step.reason_key, t) || t('combatWindow.reasonUnknown') : translated)
  }
  if (step.resolution) {
    if (step.resolution.hit) {
      lines.push(t('combatWindow.step.hit', { damage: step.resolution.damage ?? 0 }))
      if (step.resolution.shield_absorbed) lines.push(t('combatWindow.step.shieldAbsorbed', { amount: step.resolution.shield_absorbed }))
      if (step.resolution.health_lost) lines.push(t('combatWindow.step.healthLost', { amount: step.resolution.health_lost }))
    } else {
      lines.push(t('combatWindow.step.miss'))
    }
  }
  if (step.mana_recovered) lines.push(t('combatWindow.step.manaRecovered', { amount: step.mana_recovered }))
  // A multi-target cast is one action with several struck opponents, so each
  // further target is reported on the same row: §9 settles them from the
  // player's own choice, and a player who picked two opponents has to be able to
  // see both outcomes.
  for (const extra of step.extra_targets || []) {
    const label = nameOf(extra.target_id)
    if (!extra.resolution.hit) {
      lines.push(t('combatWindow.step.extraMiss', { target: label }))
      continue
    }
    lines.push(t('combatWindow.step.extraHit', { target: label, damage: extra.resolution.damage ?? 0 }))
    if (extra.defeated) lines.push(t('combatWindow.step.extraDefeated', { target: label }))
  }
  // A dose that restored health is reported apart from a rest, because the two
  // are different acts with the same pool. A dose that took hold without moving
  // a pool is reported too, since §4.2's boost is a real effect.
  if (step.health_recovered) lines.push(t('combatWindow.step.healthRecovered', { amount: step.health_recovered }))
  if (step.item_applied && !step.health_recovered && !step.mana_recovered) lines.push(t('combatWindow.step.itemTookHold'))
  if (step.burn_applied) lines.push(t('combatWindow.step.burnApplied'))
  if (step.slow_applied) lines.push(t('combatWindow.step.slowApplied'))
  if (step.escaped) lines.push(t('combatWindow.step.escaped'))
  if (step.surrendered) lines.push(t('combatWindow.step.surrendered'))
  if (step.defeated) lines.push(t('combatWindow.step.defeated'))
  const target = step.target_id ? nameOf(step.target_id) : ''
  return (
    <p className="text-[10px] text-[var(--nova-text-muted)]">
      <span className="text-[var(--nova-text)]">{nameOf(step.actor_id)}</span>
      {target ? <span className="text-[var(--nova-text-faint)]"> → {target}</span> : null}
      {lines.length ? <span> · {lines.join('、')}</span> : null}
    </p>
  )
}

/** The committed rounds, oldest first. */
function HistoryBlock({
  rounds, awaitingFirstAction, hasMore, loadingEarlier, narrating, nameOf, itemNameOf, skillNameOf, onLoadEarlier, onNarrate,
}: {
  rounds: CombatWindowRound[]
  /** The encounter is on screen but no exchange has been played yet. */
  awaitingFirstAction: boolean
  hasMore: boolean
  loadingEarlier: boolean
  narrating: boolean
  nameOf: (id: string) => string
  itemNameOf: Map<string, string>
  skillNameOf: Map<string, string>
  onLoadEarlier: () => void
  onNarrate: (roundEventId: string) => void
}) {
  const { t } = useWorldPresentation()
  return (
    <section className="flex flex-col gap-1.5">
      <p className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.history')}</p>
      {hasMore ? (
        <Button type="button" variant="ghost" size="sm" className="self-center text-[11px]" disabled={loadingEarlier} onClick={onLoadEarlier}>
          {loadingEarlier ? t('combatWindow.loading') : t('combatWindow.loadEarlier')}
        </Button>
      ) : null}
      {rounds.length === 0 ? (
        // An encounter that is waiting for its first action already says so
        // above; repeating it as an empty record would read as a round that went
        // missing rather than as one that has not been fought.
        awaitingFirstAction ? null : (
          <p className="py-2 text-center text-[11px] text-[var(--nova-text-faint)]">{t('combatWindow.noHistory')}</p>
        )
      ) : rounds.map((round, index) => (
        <div key={round.event_id} className="rounded border border-[var(--nova-border)] px-2.5 py-2">
          <p className="flex flex-wrap items-baseline gap-x-2 text-[12px] text-[var(--nova-text-muted)]">
            <span>{t('combatWindow.round', { round: round.outcome.round })}</span>
            {round.world_time ? <span className="text-[10px] text-[var(--nova-text-faint)]">{round.world_time}</span> : null}
          </p>
          <div className="mt-0.5 flex flex-col gap-0.5">
            {(round.outcome.steps || []).map((step, index) => <StepRow key={`${step.actor_id}-${index}`} step={step} nameOf={nameOf} itemNameOf={itemNameOf} skillNameOf={skillNameOf} />)}
          </div>
          <NarrationBlock narration={round.narration} collapsible />
          {/*
            A round whose passage was never written keeps its recovery entry even
            after later rounds have been played. The prose belongs to the round,
            not to the fight, so an earlier round must stay fillable — otherwise a
            round lost to a failed call becomes permanently mute as soon as the
            player carries on.

            The newest round is left to the recovery block above, which carries
            the same button plus the explanation; offering it here as well would
            put two identical controls on one round.
          */}
          {(round.narration || '').trim() === '' && index !== rounds.length - 1 ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="mt-1 text-[11px]"
              disabled={narrating}
              onClick={() => onNarrate(round.event_id)}
            >
              {narrating ? t('combatWindow.narrationWriting') : t('combatWindow.narrationRetry')}
            </Button>
          ) : null}
        </div>
      ))}
    </section>
  )
}

/**
 * The way into the fight a refusal named.
 *
 * R1 keeps one person in one unfinished battle, so a second opening is refused.
 * A refusal alone would leave the player outside their own fight with nothing to
 * press, so the panel offers to enter the battle that already holds them: the
 * same encounter, read back from the journal, continued rather than restarted.
 */
function RunningBattleOffer({ sessionId, onEnter, onDismiss }: { sessionId: string; onEnter: () => void; onDismiss: () => void }) {
  const { t } = useWorldPresentation()
  return (
    <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2">
      <p className="text-[11px] text-[var(--nova-text-muted)]">{t('combatWindow.runningBattleHint')}</p>
      <p className="mt-0.5 break-all text-[10px] text-[var(--nova-text-faint)]">{sessionId}</p>
      <div className="mt-1.5 flex gap-1.5">
        <Button type="button" size="sm" className="h-7 flex-1 text-[11px]" onClick={onEnter}>
          {t('combatWindow.runningBattleEnter')}
        </Button>
        <Button type="button" variant="outline" size="sm" className="h-7 text-[11px]" onClick={onDismiss}>
          {t('combatWindow.runningBattleDismiss')}
        </Button>
      </div>
    </div>
  )
}

/**
 * The entry point before a battle is chosen: the branch's live battles, and the
 * encounter the scene already settles.
 *
 * Who fights and on which side is read from the scene's own facts — who is
 * present, and which actors are hostile by construction — so the player is never
 * asked to side the whole cast. A bystander standing in the room is shown as
 * exactly that, not turned into an enemy.
 */
function BattlePicker({
  sessions, encounter, starting, playing, playError, onStart, onCancelStart, onSelect, onBegin,
}: {
  sessions: string[]
  encounter: CombatWindowEncounter | undefined
  starting: boolean
  playing: boolean
  playError: string | null
  onStart: () => void
  onCancelStart: () => void
  onSelect: (sessionId: string) => void
  onBegin: (combatants: Array<{ actor_id: string; side: 'player' | 'enemy'; route_exists?: boolean; retreat?: string }>) => void
}) {
  const { t } = useWorldPresentation()
  const combatants = encounter?.combatants || []
  const bystanders = encounter?.bystanders || []
  // A reason key is the server saying no fight can be opened here. It is shown in
  // the panel's own words rather than the key, and it disables the button rather
  // than letting the player open an empty battle.
  const reason = encounter?.reason_key
    ? t(encounter.reason_key as never, { defaultValue: t('combatWindow.encounter.noEnemy') })
    : null
  const canBegin = combatants.length > 0 && !reason

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-3 py-2">
      {sessions.length ? (
        <section className="flex flex-col gap-1">
          <p className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.sessions')}</p>
          {sessions.map((id) => (
            <button
              key={id}
              type="button"
              onClick={() => onSelect(id)}
              className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-left text-[11px] hover:bg-[var(--nova-hover)]"
            >
              {id}
            </button>
          ))}
        </section>
      ) : (
        <div className="flex flex-1 flex-col items-center justify-center gap-1 text-center">
          <p className="text-[12px] text-[var(--nova-text-muted)]">{t('combatWindow.empty')}</p>
          <p className="text-[11px] text-[var(--nova-text-faint)]">{t('combatWindow.emptyHint')}</p>
        </div>
      )}

      {starting ? (
        <section className="flex flex-col gap-1.5">
          <p className="text-[10px] text-[var(--nova-text-faint)]">{t('combatWindow.startHint')}</p>
          {reason ? <p className="text-[11px] text-[var(--nova-text-faint)]">{reason}</p> : null}
          {combatants.map((actor) => (
            <div key={actor.actor_id} className="flex items-center gap-2 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1">
              <span className="min-w-0 flex-1 truncate text-[11px]">{actor.name || actor.actor_id}</span>
              <span className="text-[10px] text-[var(--nova-text-faint)]">
                {actor.side === 'player' ? t('combatWindow.side.player') : t('combatWindow.side.enemy')}
              </span>
            </div>
          ))}
          {bystanders.length ? (
            <p className="text-[10px] text-[var(--nova-text-faint)]">
              {t('combatWindow.encounter.bystanders')} {bystanders.join(t('combatWindow.encounter.bystanderSeparator'))}
            </p>
          ) : null}
          {playError ? <p className="text-[11px] text-[var(--nova-danger)]">{t('combatWindow.startFailed')} {playError}</p> : null}
          <div className="flex gap-1.5">
            <Button
              type="button" size="sm" className="h-7 flex-1 text-[11px]" disabled={playing || !canBegin}
              onClick={() => onBegin(
                // The scene settled each participant's route; sending back only
                // the ids would turn the protagonist's way out into a default
                // false, and the rules would then refuse the escape.
                combatants.map((actor) => ({
                  actor_id: actor.actor_id,
                  side: actor.side,
                  route_exists: actor.route_exists,
                  retreat: actor.retreat,
                })),
              )}
            >
              {playing ? t('combatWindow.playing') : t('combatWindow.startConfirm')}
            </Button>
            <Button type="button" variant="outline" size="sm" className="h-7 text-[11px]" onClick={onCancelStart}>
              {t('combatWindow.startCancel')}
            </Button>
          </div>
        </section>
      ) : (
        <Button type="button" variant="outline" size="sm" className="h-7 shrink-0 text-[11px]" onClick={onStart}>
          {t('combatWindow.startBattle')}
        </Button>
      )}
    </div>
  )
}
