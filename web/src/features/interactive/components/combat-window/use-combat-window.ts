import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { APIError, createAgentCommandID } from '@/lib/api-client'
import { getCombatWindow, narrateCombatRound, playCombatRound } from '../../api'
import type { CombatActionRequest, CombatWindow, CombatantSpec, PlayerCombatIntent } from '../../types'

export interface CombatWindowState {
  data: CombatWindow | null
  loading: boolean
  /** Non-null when the last read failed. The panel shows it and offers a retry. */
  error: string | null
  /** True while a round is being resolved. */
  playing: boolean
  /** Non-null when the last round failed. The chosen action stays put. */
  playError: string | null
  /**
   * How that failure is to be read, so the panel does not tell the player a round
   * was not settled when nobody knows whether it was.
   *
   * `unknown` — the answer was lost and the round may exist; retrying resends the
   * same one. `conflict` — the identity was already committed against different
   * content, so the old binding is gone and the next choice is a new command.
   * `refused` — the server answered and settled nothing.
   */
  playErrorKind: CombatFailureKind | null
  hasMore: boolean
  loadingEarlier: boolean
  /** True while a missing passage is being written. */
  narrating: boolean
  /**
   * The passage this session asked for and did not get, if any.
   *
   * The journal cannot tell "nobody tried yet" from "the attempt failed": both
   * leave a committed round with no prose. So the failed case is remembered here,
   * against the round it belongs to, and only while the panel is open. A read
   * still reports the durable fact, and a reload honestly returns to pending.
   */
  narrationFailure: { roundEventId: string; message: string } | null
  /**
   * The battle a refusal told us is already running for this actor.
   *
   * R1 keeps one person in one unfinished battle, so a second opening is refused
   * with the running battle's identity. The window reports it rather than only
   * the message, because the player's next step is to enter that fight, and a
   * disabled button would leave them with no way in. The panel offers the choice
   * and clears this when it is taken or dismissed.
   */
  runningBattleId: string | null
  /**
   * The command identity the panel's next submission will carry, when an attempt
   * is outstanding.
   *
   * §4 makes an unconfirmed command visible rather than implicit: the player is
   * told that the last attempt may already have been settled and that the same
   * round is what a retry sends. Null means the next submission is a new round.
   */
  pendingCommandId: string | null
  /** Whether the outstanding attempt carries an action the player could resend. */
  pendingCommandResendable: boolean
  /** The saved intent and target are shown when an unknown round is resent. */
  pendingCommandIntent: PlayerCombatIntent | null
  pendingCommandTargetID: string | null
}

export interface UseCombatWindowOptions {
  storyId: string
  branchId: string
  /** The battle the panel is open on. Empty shows the scene and live battles. */
  sessionId: string
  /** False keeps the panel from reading until it is actually open. */
  active: boolean
}

/** One round or opening as the panel asked for it, without the identity. */
interface CombatCommandRequest {
  sessionId?: string
  combatants?: CombatantSpec[]
  actions: CombatActionRequest[]
  playerIntent?: PlayerCombatIntent
}

/** What the window showed when the command was minted, so a read can tell whether it landed. */
interface CombatCommandBaseline {
  sessionId: string
  round: number
  lastRoundEventId: string
  sessions: string[]
}

/**
 * An attempt whose outcome is not yet known.
 *
 * §4 requires more than an id: the identity is bound to the request it was
 * minted for, so a retry can only resend that request and a different choice
 * cannot ride the same identity to a second settlement. It is kept in local
 * storage as well as in memory, because closing the panel or crossing screens
 * must not turn an unknown result into a fresh round.
 */
interface PendingCombatCommand {
  commandId: string
  request: CombatCommandRequest
  baseline: CombatCommandBaseline
}

const PENDING_COMMAND_PREFIX = 'ta.combatWindow.pending'

function pendingCommandKey(storyId: string, branchId: string, sessionId: string): string {
  return [PENDING_COMMAND_PREFIX, storyId, branchId || '-', sessionId || 'open'].join('.')
}

function loadPendingCommand(key: string): PendingCombatCommand | null {
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<PendingCombatCommand> | null
    if (typeof parsed?.commandId !== 'string' || !parsed.commandId.trim()) return null
    const request = parsed.request
    if (!request || !Array.isArray(request.actions)) return null
    const baseline = parsed.baseline
    if (!baseline || typeof baseline.round !== 'number' || !Array.isArray(baseline.sessions)) return null
    return { commandId: parsed.commandId, request, baseline }
  } catch {
    return null
  }
}

function savePendingCommand(key: string, pending: PendingCombatCommand | null): void {
  try {
    if (pending) window.localStorage.setItem(key, JSON.stringify(pending))
    else window.localStorage.removeItem(key)
  } catch {
    // 隐私模式等场景下 localStorage 不可用，静默降级为会话内状态。
  }
}

/**
 * A stable rendering of one request, used to decide whether a submission is the
 * same round as the outstanding attempt.
 *
 * Order matters for a declared intent list, and an absent optional field is the
 * same request as an empty one, so those are dropped before comparison.
 */
function canonicalRequestValue(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonicalRequestValue).join(',')}]`
  if (value && typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
      .filter(([, item]) => item !== undefined && item !== null && item !== '')
      .sort(([left], [right]) => left.localeCompare(right))
    return `{${entries.map(([name, item]) => `${JSON.stringify(name)}:${canonicalRequestValue(item)}`).join(',')}}`
  }
  return JSON.stringify(value ?? null)
}

function combatRequestFingerprint(request: CombatCommandRequest): string {
  return canonicalRequestValue({
    sessionId: request.sessionId || '',
    combatants: request.combatants || [],
    actions: request.actions || [],
    playerIntent: request.playerIntent || 'spare',
  })
}

/**
 * How a failed submission is to be read.
 *
 * The three cases §4 separates are not the same thing, and the interface used to
 * treat them as one:
 *
 *   - `unknown` — the answer was lost (a transport failure) or the server failed
 *     after possibly committing. The round may exist; the identity must be kept.
 *   - `conflict` — the store found this identity already committed against
 *     different content. The formal battle has to be read before anything else
 *     is sent.
 *   - `refused` — the server answered and committed nothing. The identity is
 *     spent and the player's corrected choice is a new command.
 *   - `blocked` — the player chose something else while an attempt was still
 *     unconfirmed. Nothing was sent: a lost answer may not be answered by
 *     immediately minting a new identity and fighting again (§4).
 *
 * A thrown value that is not the server's answer is treated as unknown: guessing
 * that it committed nothing is exactly the assumption that buys a second round.
 */
type CombatFailureKind = 'unknown' | 'conflict' | 'refused' | 'blocked'

function classifyCombatFailure(error: unknown): CombatFailureKind {
  if (!(error instanceof APIError)) return 'unknown'
  // Status 0 is the transport's own report of a lost answer, not a server reply.
  if (error.status === 0 || error.status >= 500) return 'unknown'
  if (error.status === 409 && error.code === 'api.combatWindow.commandConflict') return 'conflict'
  return 'refused'
}

function baselineFromWindow(data: CombatWindow | null): CombatCommandBaseline {
  return {
    sessionId: data?.session_id || '',
    round: data?.round ?? 0,
    lastRoundEventId: data?.last_round?.event_id || '',
    sessions: data?.sessions || [],
  }
}

/**
 * Whether a read of the formal battle shows that the outstanding attempt was in
 * fact committed.
 *
 * The window carries no command id, so the answer is read from the battle's own
 * progress: a later exchange, a different last round, or — for an opening — a
 * battle that did not exist when the attempt was made.
 */
function pendingOutcomeLanded(baseline: CombatCommandBaseline, fresh: CombatWindow): boolean {
  if (baseline.sessionId) {
    if (fresh.session_id && fresh.session_id !== baseline.sessionId) return true
    const lastRoundEventId = fresh.last_round?.event_id || ''
    if (lastRoundEventId && lastRoundEventId !== baseline.lastRoundEventId) return true
    return (fresh.round ?? 0) > baseline.round
  }
  const before = new Set(baseline.sessions)
  return (fresh.sessions || []).some(id => !before.has(id))
}

export interface UseCombatWindowResult extends CombatWindowState {
  reload: () => Promise<void>
  openBattle: (sessionId: string) => void
  loadEarlier: () => Promise<void>
  /**
   * Opens a battle with the named participants and settles nothing.
   *
   * §七.1 makes the opening the encounter itself: both sides are shown at the
   * numbers they already have and the player is asked for a first action. The
   * first exchange is played afterwards through `playRound`, so this never spends
   * pools or stock. Only the opening needs the roster; later rounds address the
   * battle by id. It returns the id of the battle it opened, so the window can
   * select it: the caller owns which battle is open, and a freshly opened fight
   * the window did not select would leave the player looking at the picker again.
   */
  startBattle: (combatants: CombatantSpec[]) => Promise<string | null>
  /** Plays one exchange of the open battle. */
  playRound: (actions: CombatActionRequest[], playerIntent?: PlayerCombatIntent) => Promise<string | null>
  /**
   * Replays the round that just failed under its original command identity. It
   * exists so a lost response is retried rather than replaced by a second round.
   * A request that is not the one the identity was minted for cannot be replayed:
   * it needs a new command, which is what `playRound` gives it.
   */
  retryRound: (actions: CombatActionRequest[], playerIntent?: PlayerCombatIntent) => Promise<string | null>
  /**
   * Resends the outstanding attempt exactly as it was minted, whatever the player
   * currently has selected. §4 forbids answering a lost answer with a new identity
   * and a fresh fight, so this is the only way past an unconfirmed round.
   */
  resendPending: () => Promise<string | null>
  /**
   * Writes the prose for the round that is already settled. It never plays the
   * round again: the settled result stands, and what is missing is only its
   * description.
   */
  narrateRound: (roundEventId: string) => Promise<boolean>
  /**
   * Drops the running-battle offer.
   *
   * Entering the battle is the panel's act, because only the panel's host owns
   * which battle is open: it selects the id and the window re-reads that fight.
   * What the hook owns is the offer itself, so this clears it once the player has
   * either taken it or waved it away.
   */
  dismissRunningBattle: () => void
}

/**
 * Derives the request identity of one round's passage.
 *
 * The identity is bound to the story, branch, battle and committed round record,
 * so the first automatic attempt, a manual retry and a retry after the panel was
 * closed and reopened all name the same request. The store answers a repeated
 * identity from the passage it already committed, which keeps a lost response
 * from buying a second model call or a second passage. Another round, battle or
 * branch yields another identity, so one round's prose is never served for a
 * different round. A deliberate rewrite sends its own fresh command id through
 * narrateCombatRound and is not routed through this helper.
 */
export function combatNarrationCommandId(
  storyId: string,
  branchId: string,
  sessionId: string,
  roundEventId: string,
): string {
  return ['combat-narration', 'v1', storyId, branchId, sessionId, roundEventId]
    .map(part => encodeURIComponent(part))
    .join(':')
}

/**
 * Reads the battle identity a refusal named, when it named one.
 *
 * Only the two conflict codes that mean "that person is already in a fight" carry
 * it. Any other failure leaves the window to report its message alone, so this
 * cannot silently redirect the player into an unrelated battle.
 */
function runningBattleFrom(error: unknown): string | null {
  if (!(error instanceof APIError) || error.status !== 409) return null
  if (error.code !== 'api.combatWindow.actorAlreadyFighting' && error.code !== 'api.combatWindow.staleBattle') {
    return null
  }
  const value = error.details?.session_id
  return typeof value === 'string' && value.trim() ? value.trim() : null
}

/**
 * Reads and drives the battle window.
 *
 * The read is a plain projection of committed facts, so opening the panel costs
 * no world time and no generation. Playing a round resolves and commits it in one
 * call, so the panel never shows a round the journal does not have.
 *
 * The command identity is held across a failed attempt and cleared only on
 * success. That is what makes a repeated click, a lost response or a recovery a
 * read of the round the first attempt settled rather than a second round: the
 * retry carries the same command id, so the store answers from its committed
 * record instead of drawing again.
 */
export function useCombatWindow({ storyId, branchId, sessionId, active }: UseCombatWindowOptions): UseCombatWindowResult {
  const { i18n } = useTranslation()
  const [state, setState] = useState<CombatWindowState>({
    data: null, loading: false, error: null, playing: false,
    playError: null, hasMore: false, loadingEarlier: false, narrating: false,
    narrationFailure: null, runningBattleId: null, pendingCommandId: null,
    pendingCommandResendable: false, pendingCommandIntent: null,
    pendingCommandTargetID: null, playErrorKind: null,
  })
  const requestSeqRef = useRef(0)
  // The identity of the round currently being attempted, held with the request it
  // was minted for. It survives an unknown outcome so the retry addresses the same
  // round, and is dropped once the round is committed so the next round is a
  // genuinely new command.
  const pendingCommandRef = useRef<PendingCombatCommand | null>(null)
  // A round in flight must not be joined by a second one: two concurrent submits
  // would resolve two exchanges for one click.
  const inFlightRef = useRef(false)
  // The window as it was last read, for the baseline an attempt is compared
  // against. A ref keeps `commit` from depending on the state it also writes.
  const dataRef = useRef<CombatWindow | null>(null)

  const storageKey = pendingCommandKey(storyId, branchId, sessionId)

  const publishPending = useCallback((pending: PendingCombatCommand | null) => {
    pendingCommandRef.current = pending
    savePendingCommand(pendingCommandKey(storyId, branchId, sessionId), pending)
    setState(current => ({
      ...current,
      pendingCommandId: pending?.commandId ?? null,
      pendingCommandResendable: Boolean(pending && pending.request.sessionId && pending.request.actions.length > 0),
      pendingCommandIntent: pending?.request.playerIntent || null,
      pendingCommandTargetID: pending?.request.actions[0]?.target_id || pending?.request.actions[0]?.target_ids?.[0] || null,
    }))
  }, [branchId, sessionId, storyId])

  const read = useCallback(async (before?: string): Promise<CombatWindow | null> => {
    if (!active || !storyId) return null
    const seq = ++requestSeqRef.current
    setState(current => ({
      ...current,
      loading: !before,
      loadingEarlier: Boolean(before),
      error: null,
    }))
    try {
      const data = await getCombatWindow(storyId, {
        branchId: branchId || undefined,
        sessionId: sessionId || undefined,
        before,
      })
      // A slower earlier read must not overwrite a newer one.
      if (seq !== requestSeqRef.current) return null
      if (!before) dataRef.current = data
      setState(current => ({
        ...current,
        data: before && current.data
          ? { ...data, history: [...(data.history || []), ...(current.data.history || [])] }
          : data,
        hasMore: data.has_more,
        loading: false,
        loadingEarlier: false,
        error: null,
      }))
      // §4: a read of the formal battle comes first, and if it shows the
      // outstanding attempt was in fact committed, that round is what the player
      // sees — the old "waiting to retry" state is over rather than lingering.
      if (!before && pendingCommandRef.current && pendingOutcomeLanded(pendingCommandRef.current.baseline, data)) {
        publishPending(null)
        setState(current => ({ ...current, playError: null, playErrorKind: null }))
      }
      return data
    } catch (error) {
      if (seq !== requestSeqRef.current) return null
      setState(current => ({
        ...current, loading: false, loadingEarlier: false,
        error: error instanceof Error ? error.message : String(error),
      }))
      return null
    }
  }, [active, branchId, publishPending, sessionId, storyId, i18n.language])

  useEffect(() => {
    if (!active) return
    void read()
  }, [active, read])

  // Selecting another battle drops the round identity: a command minted for one
  // battle must never be replayed against a different one. It also drops the
  // running-battle offer, because the player has just acted on it. An attempt on
  // another battle keeps its own identity, because it is stored under that
  // battle's key and is restored when the player returns to it.
  useEffect(() => {
    pendingCommandRef.current = loadPendingCommand(storageKey)
    inFlightRef.current = false
    const restored = pendingCommandRef.current
    // A restored attempt is reported as an unknown outcome, not as a fresh round:
    // closing the panel, switching screens or reloading must not turn a lost
    // answer into an invitation to fight again (§4).
    setState(current => ({
      ...current, narrationFailure: null, runningBattleId: null,
      pendingCommandId: restored?.commandId ?? null,
      pendingCommandResendable: Boolean(restored && restored.request.sessionId && restored.request.actions.length > 0),
      pendingCommandIntent: restored?.request.playerIntent || null,
      pendingCommandTargetID: restored?.request.actions[0]?.target_id || restored?.request.actions[0]?.target_ids?.[0] || null,
      playError: restored ? current.playError : null,
      playErrorKind: restored ? (current.playErrorKind ?? 'unknown') : null,
    }))
  }, [storageKey])

  const openBattle = useCallback((nextSessionId: string) => {
    publishPending(null)
    setState(current => ({
      ...current, playError: null, playErrorKind: null,
      data: current.data
        ? { ...current.data, session_id: nextSessionId, sides: [], history: [], last_round: undefined, ended: false, result: undefined }
        : current.data,
    }))
  }, [publishPending])

  const loadEarlier = useCallback(async () => {
    const cursor = state.data?.cursor
    if (!cursor) return
    await read(cursor)
  }, [read, state.data?.cursor])

  // §4's recovery, in the order it requires: read the formal battle first, then
  // decide what the outstanding attempt means.
  //
  // A conflict is the one answer that cannot be retried, because the store holds
  // this identity against different content. The battle is read anyway, so a round
  // the first attempt really committed is shown rather than hidden behind the
  // refusal. Either way the identity is then unbound: the player's next choice is
  // a new command, never this one carrying different content.
  const recoverFromConflict = useCallback(async (message: string): Promise<void> => {
    // The read itself clears an attempt it can see was committed, so the
    // outstanding command is captured before it.
    const outstanding = pendingCommandRef.current
    const fresh = await read()
    const landed = Boolean(fresh && outstanding && pendingOutcomeLanded(outstanding.baseline, fresh))
    publishPending(null)
    // The submission is over either way; leaving `playing` set would keep the
    // panel on "settling" with no way to send the corrected action.
    if (landed) {
      setState(current => ({ ...current, playing: false, playError: null, playErrorKind: null }))
      return
    }
    setState(current => ({ ...current, playing: false, playError: message, playErrorKind: 'conflict' }))
  }, [publishPending, read])

  const commit = useCallback(async (
    input: CombatCommandRequest,
    commandId: string,
  ): Promise<string | null> => {
    if (inFlightRef.current) return null
    inFlightRef.current = true
    setState(current => ({ ...current, playing: true, playError: null }))
    // An opening establishes the encounter and settles nothing. It has no round
    // to describe, so the two-phase settle-then-narrate path below is skipped
    // entirely: asking for prose about a round that does not exist would be a
    // request the store cannot answer.
    const opening = !input.sessionId
    let settled: CombatWindow
    try {
      // Settlement first, prose second. §3 requires the committed round to be
      // visible while its passage is still being written, and §6.6 orders the two
      // the same way, so this call stops at the round.
      settled = await playCombatRound(storyId, {
        sessionId: input.sessionId, combatants: input.combatants,
        actions: input.actions, playerIntent: input.playerIntent || 'spare', branchId, commandId, narrate: false,
      })
    } catch (error) {
      inFlightRef.current = false
      const kind = classifyCombatFailure(error)
      // One person, one unfinished battle: the refusal names the fight that is
      // already running, and the player's next step is to enter it.
      const runningBattleId = runningBattleFrom(error)
      const message = error instanceof Error ? error.message : String(error)
      if (kind === 'conflict') {
        await recoverFromConflict(message)
        return null
      }
      if (kind === 'refused') {
        // The server answered and committed nothing, so this identity is spent and
        // the player's corrected choice must mint a new one.
        publishPending(null)
      }
      // An unknown outcome keeps the identity and the request it was minted for:
      // the round may already exist, and the retry is what reads it back.
      setState(current => ({
        ...current, playing: false, runningBattleId, playError: message, playErrorKind: kind,
      }))
      return null
    }
    // The round is committed, so this command identity is spent. The next round
    // mints a new one rather than reusing an id the store already answered.
    publishPending(null)
    // The settled round is already in the journal, so the panel reads it back
    // rather than inventing a local row. It renders now, before any prose exists.
    await read()
    setState(current => ({ ...current, playing: false, playError: null, playErrorKind: null }))
    const target = settled?.session_id || input.sessionId || null
    if (opening) {
      inFlightRef.current = false
      return target
    }
    const roundEventId = settled?.last_round?.event_id
    // The passage is written under an identity bound to this round record, not to
    // the attempt that produced it, so a lost response is answered from the
    // committed prose and the manual retry below addresses the same request. A
    // failure here leaves the round standing and the window's existing 补写 entry
    // to retry it.
    if (target && roundEventId) {
      setState(current => ({ ...current, narrating: true }))
      try {
        await narrateCombatRound(storyId, {
          sessionId: target, roundEventId, branchId,
          commandId: combatNarrationCommandId(storyId, settled?.branch_id || branchId, target, roundEventId),
        })
        setState(current => ({ ...current, narrationFailure: null }))
        await read()
      } catch (error) {
        // The round stands; only its description is missing. The reason is kept
        // against this round so the panel can say the attempt failed rather than
        // reading as though nobody had asked yet.
        setState(current => ({
          ...current,
          narrationFailure: {
            roundEventId,
            message: error instanceof Error ? error.message : String(error),
          },
        }))
        await read()
      }
      setState(current => ({ ...current, narrating: false }))
    }
    inFlightRef.current = false
    return target
  }, [branchId, publishPending, read, recoverFromConflict, storyId])

  // The request identity a submission carries. A caller whose request is the
  // outstanding one is asking to replay it; any other request is a new command,
  // because one identity can only ever stand for the round it was minted for.
  const mintCommand = useCallback((request: CombatCommandRequest): string => {
    const outstanding = pendingCommandRef.current
    if (outstanding) {
      // Only the outstanding request itself may reuse the identity. A different
      // choice is refused instead of quietly minting a second one: the first
      // attempt may already have been settled, and sending another round beside
      // it would settle twice for one fight the player only saw once.
      if (combatRequestFingerprint(outstanding.request) === combatRequestFingerprint(request)) {
        return outstanding.commandId
      }
      return ''
    }
    const commandId = createAgentCommandID()
    publishPending({ commandId, request, baseline: baselineFromWindow(dataRef.current) })
    return commandId
  }, [publishPending])

  const startBattle = useCallback(async (combatants: CombatantSpec[]) => {
    const request: CombatCommandRequest = { combatants, actions: [] }
    const commandId = mintCommand(request)
    if (!commandId) {
      setState(current => ({ ...current, playError: null, playErrorKind: 'blocked' }))
      return null
    }
    return commit(request, commandId)
  }, [commit, mintCommand])

  const playRound = useCallback(async (actions: CombatActionRequest[], playerIntent: PlayerCombatIntent = 'spare') => {
    const target = state.data?.session_id
    if (!target) return null
    const request: CombatCommandRequest = { sessionId: target, actions, playerIntent }
    const commandId = mintCommand(request)
    if (!commandId) {
      // An unconfirmed attempt stands. Answering it by fighting again is exactly
      // the double settlement §4 rules out, so the player is told to resend or
      // read the result first instead of being given a silent second round.
      setState(current => ({ ...current, playError: null, playErrorKind: 'blocked' }))
      return null
    }
    return commit(request, commandId)
  }, [commit, mintCommand, state.data?.session_id])

  const retryRound = useCallback(async (actions: CombatActionRequest[], playerIntent: PlayerCombatIntent = 'spare') => {
    // A retry of the outstanding request replays it under its own identity. A
    // retry that names a different action is the next round and must not ride an
    // identity minted for other content, so it goes through the same gate as a
    // fresh submission and is refused while the attempt is unconfirmed.
    return playRound(actions, playerIntent)
  }, [playRound])

  // Resends the outstanding attempt exactly as it was minted, whatever is now
  // selected. This is the one path that always answers an unknown outcome.
  const resendPending = useCallback(async () => {
    const outstanding = pendingCommandRef.current
    if (!outstanding) return null
    return commit(outstanding.request, outstanding.commandId)
  }, [commit])

  const narrateRound = useCallback(async (roundEventId: string) => {
    const target = state.data?.session_id
    if (!target || !roundEventId) return false
    if (inFlightRef.current) return false
    inFlightRef.current = true
    setState(current => ({ ...current, narrating: true }))
    try {
      await narrateCombatRound(storyId, {
        sessionId: target, roundEventId, branchId,
        commandId: combatNarrationCommandId(storyId, state.data?.branch_id || branchId, target, roundEventId),
      })
      // The passage is committed, so the panel reads it back from the journal
      // rather than holding a local copy that a reload would not have.
      setState(current => ({ ...current, narrationFailure: null }))
      await read()
      setState(current => ({ ...current, narrating: false }))
      return true
    } catch (error) {
      setState(current => ({
        ...current, narrating: false,
        narrationFailure: {
          roundEventId,
          message: error instanceof Error ? error.message : String(error),
        },
      }))
      return false
    } finally {
      inFlightRef.current = false
    }
  }, [branchId, read, storyId, state.data?.session_id, state.data?.branch_id])

  const dismissRunningBattle = useCallback(() => {
    setState(current => ({ ...current, runningBattleId: null, playError: null, playErrorKind: null }))
  }, [])

  return useMemo(() => ({
    ...state, reload: async () => { await read() }, openBattle, loadEarlier, startBattle, playRound, retryRound, resendPending, narrateRound,
    dismissRunningBattle,
  }), [dismissRunningBattle, loadEarlier, narrateRound, openBattle, read, retryRound, resendPending, playRound, startBattle, state])
}
