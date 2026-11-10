import { useCallback, useEffect, useRef, useState } from 'react'
import { APIError } from '@/lib/api-client'
import { adoptStoryAssistantDirection, askStoryAssistant } from '../../api'
import type { StoryAssistantAdoptResult, StoryAssistantResult } from '../../types'

/**
 * One advisory answer, plus whether it still describes the scene on screen.
 *
 * `stale` is not an error: the answer arrived correctly, but the branch has
 * moved on since the question was asked. Showing it as current advice would
 * describe a situation the player has already left, so the panel labels it
 * instead of quietly presenting it as the situation.
 */
export interface StoryAssistantAnswer extends StoryAssistantResult {
  stale: boolean
}

export interface StoryAssistantState {
  /** The last answer, whatever turn it was written against. */
  answer: StoryAssistantAnswer | null
  asking: 'analyze' | 'inspire' | null
  /** Non-null when the last question failed. Nothing was committed. */
  error: string | null
  adopting: boolean
  /** Non-null when adopting a direction failed. */
  adoptError: string | null
  /** True when the plan moved under the player: a re-read is needed, not a blind retry. */
  adoptConflict: boolean
  /** The wording last accepted as this branch's direction, for the confirmation line. */
  adopted: string | null
}

export interface UseStoryAssistantOptions {
  storyId: string
  branchId: string
  /** The turn the player is looking at. An answer bound to another turn is stale. */
  turnId: string
  /**
   * The plan revision the player was shown, or undefined when this branch has no
   * plan. Without a plan a direction has nowhere to be recorded, and the panel
   * says so rather than offering a button that cannot work.
   */
  planRevision?: string
  /**
   * Called with the plan the server just wrote. The caller is expected to fold
   * that plan into whatever it is displaying, so the next adoption quotes the
   * revision the branch actually carries.
   */
  onPlanAdvanced?: (result: StoryAssistantAdoptResult) => void
}

export interface UseStoryAssistantResult extends StoryAssistantState {
  canAdopt: boolean
  ask: (input: { action: 'analyze' | 'inspire'; playerInput?: string; chosenOption?: string }) => Promise<void>
  adopt: (direction: string) => Promise<boolean>
  clear: () => void
}

function initialState(): StoryAssistantState {
  return { answer: null, asking: null, error: null, adopting: false, adoptError: null, adoptConflict: false, adopted: null }
}

/**
 * A refused adoption is only a conflict when the branch's plan moved. Other
 * refusals (an ended turn, a branch without a plan) are not fixed by re-reading
 * the plan, so they stay ordinary failures.
 */
function isAdoptConflict(error: unknown) {
  if (!(error instanceof APIError)) return false
  if (error.code) return error.code === 'api.storyAssistant.planConflict'
  return error.status === 409
}

/**
 * Drives the in-story assistant panel.
 *
 * Every call here is either a read or a deliberate write the player asked for:
 * `ask` never touches the world, and `adopt` only runs when the player clicks
 * "adopt". The hook never sends anything to the story on its own, and it never
 * applies a direction by itself.
 *
 * Answers are bound to a turn. A response that lands after the branch moved on
 * is kept but marked stale, because the player may still want to read what they
 * asked for; it is simply not presented as describing the current scene.
 */
export function useStoryAssistant({ storyId, branchId, turnId, planRevision, onPlanAdvanced }: UseStoryAssistantOptions): UseStoryAssistantResult {
  const [state, setState] = useState<StoryAssistantState>(initialState)
  // The plan revision the server returned from the last adoption. Without it a
  // second adoption would still quote the revision the player was shown first,
  // and the server would refuse it as a conflict with a plan that changed
  // because of us.
  const [localRevision, setLocalRevision] = useState<string | undefined>(undefined)

  // Each request owns a sequence number instead of a boolean lock, so leaving
  // the story, the branch, the turn or the panel frees the buttons at once and
  // a response that lands afterwards is dropped rather than applied.
  const askSeqRef = useRef(0)
  const activeAskRef = useRef(0)
  const adoptSeqRef = useRef(0)
  const activeAdoptRef = useRef(0)
  // The direction being adopted. It survives a failed attempt so a retry asks for
  // the same direction, which the server treats as an idempotent read once it has
  // been recorded.
  const pendingDirectionRef = useRef<string | null>(null)

  const release = useCallback(() => {
    askSeqRef.current += 1
    adoptSeqRef.current += 1
    activeAskRef.current = 0
    activeAdoptRef.current = 0
    pendingDirectionRef.current = null
  }, [])

  // Leaving the story or the branch drops everything: an answer written for one
  // branch must never be shown beside another.
  useEffect(() => {
    release()
    setLocalRevision(undefined)
    setState(initialState())
  }, [branchId, release, storyId])

  // The plan revision on screen is the caller's; once it catches up with a plan
  // we just wrote, there is nothing local left to remember.
  useEffect(() => {
    setLocalRevision(undefined)
  }, [branchId, planRevision, storyId])

  // A new turn keeps the old answer readable but no longer current, and an
  // adoption aimed at the previous turn must not be retried against this one.
  useEffect(() => {
    adoptSeqRef.current += 1
    activeAdoptRef.current = 0
    pendingDirectionRef.current = null
    setState(current =>
      current.error === null && current.adoptError === null && !current.adopting && !current.adoptConflict
        ? current
        : { ...current, error: null, adoptError: null, adoptConflict: false, adopting: false },
    )
  }, [turnId])

  const ask = useCallback(async (input: { action: 'analyze' | 'inspire'; playerInput?: string; chosenOption?: string }) => {
    if (!storyId || !turnId || activeAskRef.current) return
    const seq = ++askSeqRef.current
    activeAskRef.current = seq
    const askedAtTurn = turnId
    setState(current => ({ ...current, asking: input.action, error: null }))
    try {
      const answer = await askStoryAssistant(storyId, {
        branchId, turnId: askedAtTurn, action: input.action,
        playerInput: input.playerInput, chosenOption: input.chosenOption,
      })
      if (seq !== askSeqRef.current) return
      setState(current => ({
        ...current,
        asking: null,
        error: null,
        answer: { ...answer, stale: false },
      }))
    } catch (error) {
      if (seq !== askSeqRef.current) return
      setState(current => ({
        ...current,
        asking: null,
        error: error instanceof Error ? error.message : String(error),
      }))
    } finally {
      if (activeAskRef.current === seq) activeAskRef.current = 0
    }
  }, [branchId, storyId, turnId])

  const adopt = useCallback(async (direction: string) => {
    const wording = direction.trim()
    const baseRevision = localRevision ?? planRevision
    if (!storyId || !turnId || !wording || !baseRevision || activeAdoptRef.current) return false
    const seq = ++adoptSeqRef.current
    activeAdoptRef.current = seq
    pendingDirectionRef.current = wording
    setState(current => ({ ...current, adopting: true, adoptError: null, adoptConflict: false }))
    try {
      const result = await adoptStoryAssistantDirection(storyId, {
        branchId, turnId, direction: wording, baseRevision,
      })
      if (seq !== adoptSeqRef.current) return false
      pendingDirectionRef.current = null
      setLocalRevision(result.branch_plan.revision)
      setState(current => ({ ...current, adopting: false, adoptError: null, adoptConflict: false, adopted: wording }))
      onPlanAdvanced?.(result)
      return result.applied
    } catch (error) {
      if (seq !== adoptSeqRef.current) return false
      // The direction stays pending so a retry is the same request; the server
      // records it at most once.
      setState(current => ({
        ...current,
        adopting: false,
        adoptError: error instanceof Error ? error.message : String(error),
        adoptConflict: isAdoptConflict(error),
      }))
      return false
    } finally {
      if (activeAdoptRef.current === seq) activeAdoptRef.current = 0
    }
  }, [branchId, localRevision, onPlanAdvanced, planRevision, storyId, turnId])

  const clear = useCallback(() => {
    release()
    setState(initialState())
  }, [release])

  // Staleness is a property of the present, not of the response: the same answer
  // becomes stale the moment the branch moves on, without another fetch.
  const answer = state.answer
    ? { ...state.answer, stale: state.answer.turn_id !== turnId }
    : null

  return {
    ...state,
    answer,
    canAdopt: Boolean((localRevision ?? planRevision) && turnId && storyId),
    ask,
    adopt,
    clear,
  }
}
