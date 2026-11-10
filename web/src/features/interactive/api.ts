import { createAgentCommandID, fetchAPI, jsonHeaders, parseSSEStream, requestJSON, responseAPIError } from '@/lib/api-client'
import type { AgentCommandReceipt, AgentRuntimeActiveOutput, AgentRuntimeOpenTool, AgentRuntimeOperation, AgentRuntimeQueuedCommand, AgentRuntimeRecoveryAction, AgentRuntimeRecoveryReceipt, ContextAnalysis, InteractiveImage } from '@/lib/api-client'
import { isKnownAgentCommandOutcome } from '@/lib/agent-command'
import type { ChatAttachmentDescriptor, ChatAttachmentUpload } from '@/lib/chat-attachments'
import type { ThinkingLevel } from '@/features/settings/thinking-levels'
import type { ActorStateModule, ActorTraitRollRequest, ActorTraitRollResult, BranchSummary, CharacterChatResult, CharacterPictureImportResult, CharacterPictureState, CharacterWindow, CombatActionRequest, CombatNarration, CombatWindow, CombatantSpec, EventPackageModule, GamePlanningTemplate, GroupRoundResult, GroupWindow, GroupWindowGroup, ImagePreset, InitialActorTraitRoll, IntimacyResponse, IntimacyRound, IntimacyWindow, InteractiveSnapshotResponse, InteractiveSSEEvent, InteractiveStoryUpdateInput, PlacePictureImportResult, PlacePictureState, PlayerCombatIntent, RuleResolution, RuleResolutionRerollInput, RuleSystemModule, StoryAssistantAdoptResult, StoryAssistantResult, StoryCheckSettings, StoryDirectorModuleRefs, StoryHistoryPage, StoryPlanningMode, StoryProtagonist, StoryStateSchemaPolicy, StoryStylePurposes, StyleReference, StyleReferenceFileDocument, StoryImageSettings, StoryIndex, StoryOpeningConfig, StorySummary, Teller, UpdateBranchPlanResult, UpdateTurnNarrativeResult } from './types'
import type { JourneyResult, JourneyState } from './journey-types'
import type { RealmAction, RealmResult, RealmState } from './realm-types'
import type { QuestAction, QuestResult, QuestState } from './quest-types'
import type { PublicEventAction, PublicEventResult, PublicEventState } from './public-event-types'

function presetMutationBody<T extends object>(input: T, baseRevision?: string) {
  return {
    ...input,
    ...(baseRevision ? { base_revision: baseRevision } : {}),
  }
}

export function getInteractiveStories(): Promise<StoryIndex> {
  return requestJSON('/api/interactive/stories')
}

export function createInteractiveStory(input: { title: string; custom_agent_id?: string; profile_id?: string; thinking_level?: ThinkingLevel; origin?: string; protagonist?: StoryProtagonist; story_teller_id: string; style_purposes?: StoryStylePurposes; planning_template_id?: string; planning_mode?: StoryPlanningMode; module_refs?: StoryDirectorModuleRefs; reply_target_chars?: number; choice_count?: number; image_settings?: StoryImageSettings; check_settings?: StoryCheckSettings; opening?: StoryOpeningConfig; initial_trait_rolls?: InitialActorTraitRoll[]; state_schema_policy?: StoryStateSchemaPolicy; narrative_requirements?: string }): Promise<StorySummary> {
  return requestJSON('/api/interactive/stories', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function rollInteractiveActorTraits(input: ActorTraitRollRequest): Promise<ActorTraitRollResult> {
  return requestJSON('/api/interactive/actor-traits/roll', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateInteractiveStory(
  id: string,
  input: InteractiveStoryUpdateInput,
): Promise<StorySummary> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function selectInteractiveStory(id: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(id)}/select`, {
    method: 'POST',
  })
}

export function deleteInteractiveStory(id: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export function getInteractiveSnapshot(storyId: string, branchId?: string): Promise<InteractiveSnapshotResponse> {
  const query = branchId ? `?branch=${encodeURIComponent(branchId)}` : ''
  return requestJSON<InteractiveSnapshotResponse>(`/api/interactive/stories/${encodeURIComponent(storyId)}/snapshot${query}`)
}

export function getInteractiveHistoryPage(storyId: string, branchId: string, before: string, limit = 100): Promise<StoryHistoryPage> {
  const query = new URLSearchParams({ branch: branchId, before, limit: String(limit) })
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/history?${query.toString()}`)
}

/**
 * Reads the character window. It is a plain read with no model call, so opening
 * the window costs no world time and no generation.
 */
export function getCharacterWindow(
  storyId: string,
  options: { branchId?: string; characterId?: string; before?: string; limit?: number } = {},
): Promise<CharacterWindow> {
  const query = new URLSearchParams()
  if (options.branchId) query.set('branch', options.branchId)
  if (options.characterId) query.set('character_id', options.characterId)
  if (options.before) query.set('before', options.before)
  if (options.limit) query.set('limit', String(options.limit))
  const suffix = query.toString()
  return requestJSON<CharacterWindow>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/characters${suffix ? `?${suffix}` : ''}`,
  )
}

/**
 * Reads one person's picture as the window would draw it.
 *
 * The author's editor calls this before and after an import so the preview and
 * the game cannot disagree: both ask the server the same question.
 */
export function getCharacterPicture(
  storyId: string,
  characterId: string,
  options: { branchId?: string } = {},
): Promise<CharacterPictureState> {
  const query = new URLSearchParams()
  if (options.branchId) query.set('branch', options.branchId)
  const suffix = query.toString()
  return requestJSON<CharacterPictureState>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/characters/${encodeURIComponent(characterId)}/picture${suffix ? `?${suffix}` : ''}`,
  )
}

/**
 * Copies one picture the author picked into the media directory and binds it to
 * this person.
 *
 * The request names a person and a purpose. It never names a file path or a
 * binding: the server works out from what the story committed whether this is an
 * author key or a story-invented actor, so a page cannot point a face at somebody
 * else.
 */
export function importCharacterPicture(
  storyId: string,
  characterId: string,
  file: File,
  options: { branchId?: string; purpose?: 'avatar' | 'portrait' } = {},
): Promise<CharacterPictureImportResult> {
  const query = new URLSearchParams()
  if (options.branchId) query.set('branch', options.branchId)
  const suffix = query.toString()
  const form = new FormData()
  form.append('file', file, file.name)
  if (options.purpose) form.append('purpose', options.purpose)
  return requestJSON<CharacterPictureImportResult>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/characters/${encodeURIComponent(characterId)}/picture${suffix ? `?${suffix}` : ''}`,
    { method: 'POST', body: form },
  )
}

/**
 * Removes this person's imported picture and reports what answers instead.
 *
 * The restored picture comes back in the same answer, so the editor does not
 * have to read again to know what to draw.
 */
export function clearCharacterPicture(
  storyId: string,
  characterId: string,
  options: { branchId?: string; purpose?: 'avatar' | 'portrait' } = {},
): Promise<CharacterPictureImportResult> {
  const query = new URLSearchParams()
  if (options.branchId) query.set('branch', options.branchId)
  const suffix = query.toString()
  const form = new FormData()
  if (options.purpose) form.append('purpose', options.purpose)
  return requestJSON<CharacterPictureImportResult>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/characters/${encodeURIComponent(characterId)}/picture/clear${suffix ? `?${suffix}` : ''}`,
    { method: 'POST', body: form },
  )
}

/**
 * One place's picture, as the window would draw it.
 *
 * The place has to be one this story's own frozen catalog registered; the server
 * refuses any other name rather than binding a file nothing will ever draw.
 */
export function getPlacePicture(storyId: string, place: string): Promise<PlacePictureState> {
  const query = new URLSearchParams({ place })
  return requestJSON<PlacePictureState>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/places/picture?${query.toString()}`,
  )
}

/** Copies one picture the author picked into the media directory and binds it to a place. */
export function importPlacePicture(storyId: string, place: string, file: File): Promise<PlacePictureImportResult> {
  const form = new FormData()
  form.append('file', file, file.name)
  form.append('place', place)
  return requestJSON<PlacePictureImportResult>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/places/picture`,
    { method: 'POST', body: form },
  )
}

/** Removes a place's imported picture. */
export function clearPlacePicture(storyId: string, place: string): Promise<PlacePictureImportResult> {
  const form = new FormData()
  form.append('place', place)
  return requestJSON<PlacePictureImportResult>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/places/picture/clear`,
    { method: 'POST', body: form },
  )
}

/**
 * Sends one message in the character window and returns the character's reply.
 *
 * commandId makes a retry safe: a repeated click, a lost response or a recovery
 * re-read returns the exchange the first attempt committed rather than asking
 * the character again.
 */
export function sendCharacterMessage(
  storyId: string,
  input: { characterId: string; message: string; branchId?: string; commandId?: string },
): Promise<CharacterChatResult> {
  return requestJSON<CharacterChatResult>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/characters/messages`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        character_id: input.characterId,
        message: input.message,
      }),
    },
  )
}

/**
 * Reads the group surface. It is a plain read with no model call: opening a
 * group, switching between groups and scrolling back all cost no world time.
 */
export function getGroupWindow(
  storyId: string,
  options: { branchId?: string; groupId?: string; before?: string; limit?: number } = {},
): Promise<GroupWindow> {
  const query = new URLSearchParams()
  if (options.branchId) query.set('branch', options.branchId)
  if (options.groupId) query.set('group_id', options.groupId)
  if (options.before) query.set('before', options.before)
  if (options.limit) query.set('limit', String(options.limit))
  const suffix = query.toString()
  return requestJSON<GroupWindow>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/groups${suffix ? `?${suffix}` : ''}`,
  )
}

/** Builds a self-built group from people the story already knows. */
export function createGroup(
  storyId: string,
  input: { name: string; members: string[]; branchId?: string; commandId?: string },
): Promise<GroupWindowGroup> {
  return requestJSON<GroupWindowGroup>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/groups`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        name: input.name,
        members: input.members,
      }),
    },
  )
}

/**
 * Adds and removes group members. A removal ends that tenure; adding them back
 * later opens a new one, so what was said while they were away stays away.
 */
export function changeGroupMembers(
  storyId: string,
  groupId: string,
  input: { add?: string[]; remove?: string[]; branchId?: string; commandId?: string },
): Promise<GroupWindowGroup> {
  return requestJSON<GroupWindowGroup>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/groups/${encodeURIComponent(groupId)}/members`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        add: input.add || [],
        remove: input.remove || [],
      }),
    },
  )
}

/**
 * Sends one message to a group and returns the round the members committed.
 *
 * One round is one model call. commandId makes a retry safe: a repeated click or
 * a lost response returns the committed round rather than asking the group
 * again.
 */
export function sendGroupMessage(
  storyId: string,
  input: { groupId: string; message: string; branchId?: string; commandId?: string },
): Promise<GroupRoundResult> {
  return requestJSON<GroupRoundResult>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/groups/messages`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        group_id: input.groupId,
        message: input.message,
      }),
    },
  )
}

/**
 * Reads the intimacy window. It is a plain read with no model call: opening it,
 * re-reading a saved activity and resuming one all cost no world time and no
 * generation, which is what makes restoring a finished activity read history
 * instead of replaying an opening.
 */
export function getIntimacyWindow(
  storyId: string,
  options: { branchId?: string; activityId?: string; before?: string; limit?: number } = {},
): Promise<IntimacyWindow> {
  const query = new URLSearchParams()
  if (options.branchId) query.set('branch', options.branchId)
  if (options.activityId) query.set('activity_id', options.activityId)
  if (options.before) query.set('before', options.before)
  if (options.limit) query.set('limit', String(options.limit))
  const suffix = query.toString()
  return requestJSON<IntimacyWindow>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/intimacy${suffix ? `?${suffix}` : ''}`,
  )
}

/**
 * Asks one person to join. The person really answers, in their own words, using
 * the story's dedicated model.
 *
 * A one-to-one invitation that is accepted enters the activity directly, so the
 * response may already carry a started activity and its opening. A refusal or a
 * condition stops at the answer; it is never recorded as consent.
 */
export function inviteIntimacy(
  storyId: string,
  input: { characterId: string; activityId?: string; oneOnOne?: boolean; branchId?: string; commandId?: string },
): Promise<IntimacyResponse> {
  return requestJSON<IntimacyResponse>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/intimacy/invitations`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        activity_id: input.activityId || '',
        character_id: input.characterId,
        one_on_one: Boolean(input.oneOnOne),
      }),
    },
  )
}

/** Asks one invited person again, for an invitation that was sent without an answer yet. */
export function respondIntimacy(
  storyId: string,
  activityId: string,
  input: { characterId: string; branchId?: string; commandId?: string },
): Promise<IntimacyResponse> {
  return requestJSON<IntimacyResponse>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/intimacy/${encodeURIComponent(activityId)}/responses`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        character_id: input.characterId,
      }),
    },
  )
}

/**
 * Starts a multi-person activity with the roster the player confirmed.
 *
 * expectedVersion is the roster version the player was shown. A confirmation
 * based on a roster they never saw is refused rather than applied.
 */
export function confirmIntimacy(
  storyId: string,
  activityId: string,
  input: { expectedVersion: number; branchId?: string; commandId?: string },
): Promise<IntimacyResponse> {
  return requestJSON<IntimacyResponse>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/intimacy/${encodeURIComponent(activityId)}/confirm`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        expected_version: input.expectedVersion,
      }),
    },
  )
}

/** Sends one line in an open activity and returns the round the roster committed. */
export function sendIntimacyMessage(
  storyId: string,
  activityId: string,
  input: { message: string; branchId?: string; commandId?: string },
): Promise<IntimacyRound> {
  return requestJSON<IntimacyRound>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/intimacy/${encodeURIComponent(activityId)}/messages`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        message: input.message,
      }),
    },
  )
}

/** Ends an activity. Its conversation stays readable, so ending is not a delete. */
export function endIntimacy(
  storyId: string,
  activityId: string,
  input: { branchId?: string; commandId?: string } = {},
): Promise<IntimacyResponse> {
  return requestJSON<IntimacyResponse>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/intimacy/${encodeURIComponent(activityId)}/end`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
      }),
    },
  )
}

/**
 * Reads the battle window. It is a plain read with no model call, so opening the
 * window costs no world time and no generation. With no sessionId it reports the
 * scene and the branch's live battles, which is what the entry point needs before
 * the player picks one.
 */
export function getCombatWindow(
  storyId: string,
  options: { branchId?: string; sessionId?: string; before?: string; limit?: number } = {},
): Promise<CombatWindow> {
  const query = new URLSearchParams()
  if (options.branchId) query.set('branch', options.branchId)
  if (options.sessionId) query.set('session_id', options.sessionId)
  if (options.before) query.set('before', options.before)
  if (options.limit) query.set('limit', String(options.limit))
  const suffix = query.toString()
  return requestJSON<CombatWindow>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/combat${suffix ? `?${suffix}` : ''}`,
  )
}

/**
 * Plays one exchange and returns the battle window after it.
 *
 * commandId makes a retry safe: a repeated click, a lost response or a recovery
 * re-read returns the exchange the first attempt settled rather than resolving a
 * second round or drawing again. The caller must reuse the same id for a retry of
 * the same round and mint a new one only for a genuinely new round.
 *
 * A new battle is opened by leaving sessionId empty and naming the combatants.
 */
export function playCombatRound(
  storyId: string,
  input: {
    sessionId?: string
    combatants?: CombatantSpec[]
    actions?: CombatActionRequest[]
    playerIntent?: PlayerCombatIntent
    branchId?: string
    turnId?: string
    commandId?: string
    /**
     * Whether the same call also writes the round's prose. The window sends
     * false so the settled result is on screen while the passage is still being
     * written, then asks for the prose on the narration route under the same
     * command id. Either way the round settled is the same one.
     */
    narrate?: boolean
  },
): Promise<CombatWindow> {
  return requestJSON<CombatWindow>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/combat/rounds`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        session_id: input.sessionId || '',
        turn_id: input.turnId || '',
        combatants: input.combatants || [],
        actions: input.actions || [],
        player_intent: input.playerIntent || 'spare',
        narrate: input.narrate !== false,
      }),
    },
  )
}

/**
 * Writes the prose for a round that is already settled.
 *
 * The round is not played again: the request names a committed round record and
 * the answer is a passage about it. This is the recovery path §8 requires for a
 * round whose passage failed — the settled result stands, and what is missing is
 * only its description.
 */
export function narrateCombatRound(
  storyId: string,
  input: { sessionId: string; roundEventId: string; branchId?: string; commandId?: string },
): Promise<CombatNarration> {
  return requestJSON<CombatNarration>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/combat/narration`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        session_id: input.sessionId,
        round_event_id: input.roundEventId,
      }),
    },
  )
}

/**
 * Reads the journey entry. It is a plain read with no model call, so opening it
 * costs no world time and no generation: an entry that travelled on open would
 * let a player walk a road by opening and closing it.
 */
export function getJourney(
  storyId: string,
  options: { branchId?: string } = {},
): Promise<JourneyState> {
  const query = new URLSearchParams()
  if (options.branchId) query.set('branch', options.branchId)
  const suffix = query.toString()
  return requestJSON<JourneyState>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/journey${suffix ? `?${suffix}` : ''}`,
  )
}

/**
 * Settles one stretch of travel and commits it.
 *
 * commandId makes a retry safe: a repeated click, a lost response or a re-read
 * returns the stretch the first attempt committed rather than walking the same
 * road twice. The content fingerprint is derived by the server, so a caller only
 * has to keep the id stable across a retry.
 *
 * arrive walks the whole route in one commit, which is the ordinary case. A
 * targetMinutes stop is the mid-journey case: it is an absolute point counted
 * from the route's origin, so continuing to the end covers only the remainder.
 */
export function applyJourney(
  storyId: string,
  input: {
    routeId?: string
    journeyId?: string
    targetMinutes?: number
    arrive?: boolean
    branchId?: string
    commandId?: string
  },
): Promise<JourneyResult> {
  return requestJSON<JourneyResult>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/journey`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        route_id: input.routeId || '',
        journey_id: input.journeyId || '',
        target_minutes: input.targetMinutes || 0,
        arrive: Boolean(input.arrive),
      }),
    },
  )
}

/**
 * Reads the realm entry. It is a plain read with no model call, so opening it
 * costs no world time and no generation: a realm entered on open would let a
 * player explore a place by opening and closing the window.
 */
export function getRealm(
  storyId: string,
  options: { branchId?: string } = {},
): Promise<RealmState> {
  const query = new URLSearchParams()
  if (options.branchId) query.set('branch', options.branchId)
  const suffix = query.toString()
  return requestJSON<RealmState>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/realm${suffix ? `?${suffix}` : ''}`,
  )
}

/**
 * Settles one realm action and commits it.
 *
 * commandId makes a retry safe: a repeated click, a lost response or a reopened
 * window returns the action the first attempt committed rather than handing the
 * same reward out twice. The content fingerprint is derived by the server, so a
 * caller only has to keep the id stable across a retry.
 *
 * The reward is named by the realm's own content reference, never by the
 * catalogue entry or the quantity: both are the authored realm's, so a request
 * cannot turn one declared pill into ten.
 */
export function applyRealm(
  storyId: string,
  input: {
    action: RealmAction
    realmId?: string
    instanceId?: string
    connectionId?: string
    destinationAreaId?: string
    contentRefId?: string
    branchId?: string
    commandId?: string
  },
): Promise<RealmResult> {
  return requestJSON<RealmResult>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/realm`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        realm_id: input.realmId || '',
        instance_id: input.instanceId || '',
        action: input.action,
        connection_id: input.connectionId || '',
        destination_area_id: input.destinationAreaId || '',
        content_ref_id: input.contentRefId || '',
      }),
    },
  )
}

/**
 * Reads the task entry. It is a plain read with no model call, so opening it
 * costs no world time and no generation: a task offered on open would let a
 * player take on work by opening and closing the window.
 */
export function getQuests(
  storyId: string,
  options: { branchId?: string } = {},
): Promise<QuestState> {
  const query = new URLSearchParams()
  if (options.branchId) query.set('branch', options.branchId)
  const suffix = query.toString()
  return requestJSON<QuestState>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/quests${suffix ? `?${suffix}` : ''}`,
  )
}

/**
 * Settles one task action and commits it.
 *
 * commandId makes a retry safe: a repeated click, a lost response or a reopened
 * window returns the action the first attempt committed rather than paying the
 * same reward out twice. The content fingerprint is derived by the server, so a
 * caller only has to keep the id stable across a retry.
 *
 * The request names only the task and the action. The objectives, the amount and
 * the item are the authored task's, so a request cannot turn one reward into
 * another.
 */
export function applyQuest(
  storyId: string,
  input: {
    action: QuestAction
    taskId: string
    branchId?: string
    worldTime?: string
    commandId?: string
  },
): Promise<QuestResult> {
  return requestJSON<QuestResult>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/quests`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        world_time: input.worldTime || '',
        task_id: input.taskId,
        action: input.action,
      }),
    },
  )
}

/**
 * Reads the public event entry. It is a plain read with no model call, and each
 * stage is derived from the branch's committed clock, so opening the entry costs
 * no world time and cannot advance the event.
 */
export function getPublicEvents(
  storyId: string,
  options: { branchId?: string } = {},
): Promise<PublicEventState> {
  const query = new URLSearchParams()
  if (options.branchId) query.set('branch', options.branchId)
  const suffix = query.toString()
  return requestJSON<PublicEventState>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/public-events${suffix ? `?${suffix}` : ''}`,
  )
}

/**
 * Settles one participation action and commits it.
 *
 * commandId makes a retry safe: a repeated click, a lost response or a reopened
 * window returns the action the first attempt committed rather than paying the
 * same reward out twice. The request names only the event, the action and how the
 * character takes part; the stage, the lines and the payment are the authored
 * event's and the branch clock's, so a request cannot move the event forward.
 */
export function applyPublicEvent(
  storyId: string,
  input: {
    eventId: string
    action: PublicEventAction
    participationAction?: string
    branchId?: string
    worldTime?: string
    commandId?: string
  },
): Promise<PublicEventResult> {
  return requestJSON<PublicEventResult>(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/public-events`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        command_id: input.commandId || createAgentCommandID(),
        branch: input.branchId || '',
        world_time: input.worldTime || '',
        event_id: input.eventId,
        action: input.action,
        participation_action: input.participationAction || '',
      }),
    },
  )
}

/**
 * Records what the player did with one incoming message. Marking a message
 * handled never accepts whatever it proposed; only a committed reply can do that.
 */
export function setCharacterMessageStatus(
  storyId: string,
  input: { characterId: string; messageId: string; status: 'read' | 'handled' | 'ignored'; branchId?: string },
): Promise<void> {
  return requestJSON(
    `/api/interactive/stories/${encodeURIComponent(storyId)}/characters/${encodeURIComponent(input.characterId)}/messages/${encodeURIComponent(input.messageId)}/status`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ branch: input.branchId || '', status: input.status }),
    },
  )
}

export function rerollInteractiveRuleResolution(storyId: string, resolutionId: string, input: RuleResolutionRerollInput = {}): Promise<RuleResolution> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/rules/resolutions/${encodeURIComponent(resolutionId)}/reroll`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function getInteractiveTellers(): Promise<Teller[]> {
  const data = await requestJSON<{ tellers: Teller[] }>('/api/interactive/tellers')
  return data.tellers || []
}

/**
 * One preset by id.
 *
 * The list endpoint only reports the preset files that exist on disk, so an id
 * the server can still resolve — a style the recommended set absorbed, kept for
 * stories that already saved it — has no entry there. This endpoint answers for
 * every builtin id.
 */
export function getInteractiveTeller(id: string): Promise<Teller> {
  return requestJSON<Teller>(`/api/interactive/tellers/${encodeURIComponent(id)}`)
}

export function createInteractiveTeller(input: Partial<Teller>): Promise<Teller> {
  return requestJSON('/api/interactive/tellers', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateInteractiveTeller(id: string, input: Partial<Teller>, baseRevision?: string): Promise<Teller> {
  return requestJSON(`/api/interactive/tellers/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision)),
  })
}

export function deleteInteractiveTeller(id: string): Promise<void> {
  return requestJSON(`/api/interactive/tellers/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getStyleReferences(): Promise<StyleReference[]> {
  const data = await requestJSON<{ styles: StyleReference[] }>('/api/styles')
  return data.styles || []
}

export function saveStyleReference(input: { name: string; description?: string; filename?: string; content: string }): Promise<StyleReference> {
  return requestJSON('/api/styles', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function readStyleReferenceFile(path: string): Promise<StyleReferenceFileDocument> {
  return requestJSON(`/api/styles/file?path=${encodeURIComponent(path)}`)
}

export function updateStyleReferenceFile(input: { path: string; content: string; base_revision?: string }): Promise<StyleReferenceFileDocument> {
  return requestJSON('/api/styles/file', {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function getGamePlanningTemplates(): Promise<GamePlanningTemplate[]> {
  const data = await requestJSON<{ game_planning_templates: GamePlanningTemplate[] }>('/api/game-planning-templates')
  return data.game_planning_templates || []
}

export function createGamePlanningTemplate(input: Partial<GamePlanningTemplate>): Promise<GamePlanningTemplate> {
  return requestJSON('/api/game-planning-templates', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateGamePlanningTemplate(id: string, input: Partial<GamePlanningTemplate>, baseRevision?: string): Promise<GamePlanningTemplate> {
  return requestJSON(`/api/game-planning-templates/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision)),
  })
}

export function deleteGamePlanningTemplate(id: string): Promise<void> {
  return requestJSON(`/api/game-planning-templates/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getEventPackages(): Promise<EventPackageModule[]> {
  const data = await requestJSON<{ event_packages: EventPackageModule[] }>('/api/event-packages')
  return data.event_packages || []
}

export function createEventPackage(input: Partial<EventPackageModule>): Promise<EventPackageModule> {
  return requestJSON('/api/event-packages', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateEventPackage(id: string, input: Partial<EventPackageModule>, baseRevision?: string): Promise<EventPackageModule> {
  return requestJSON(`/api/event-packages/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision)),
  })
}

export function deleteEventPackage(id: string): Promise<void> {
  return requestJSON(`/api/event-packages/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getRuleSystems(): Promise<RuleSystemModule[]> {
  const data = await requestJSON<{ rule_systems: RuleSystemModule[] }>('/api/rule-systems')
  return data.rule_systems || []
}

export function createRuleSystem(input: Partial<RuleSystemModule>): Promise<RuleSystemModule> {
  return requestJSON('/api/rule-systems', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateRuleSystem(id: string, input: Partial<RuleSystemModule>, baseRevision?: string): Promise<RuleSystemModule> {
  return requestJSON(`/api/rule-systems/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision)),
  })
}

export function deleteRuleSystem(id: string): Promise<void> {
  return requestJSON(`/api/rule-systems/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getActorStates(): Promise<ActorStateModule[]> {
  const data = await requestJSON<{ actor_states: ActorStateModule[] }>('/api/actor-states')
  return data.actor_states || []
}

export function createActorState(input: Partial<ActorStateModule>): Promise<ActorStateModule> {
  return requestJSON('/api/actor-states', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateActorState(id: string, input: Partial<ActorStateModule>, baseRevision?: string): Promise<ActorStateModule> {
  return requestJSON(`/api/actor-states/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision)),
  })
}

export function deleteActorState(id: string): Promise<void> {
  return requestJSON(`/api/actor-states/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getImagePresets(): Promise<ImagePreset[]> {
  const data = await requestJSON<{ presets: ImagePreset[] }>('/api/image-presets')
  return data.presets || []
}

export function createImagePreset(input: Partial<ImagePreset>): Promise<ImagePreset> {
  return requestJSON('/api/image-presets', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateImagePreset(id: string, input: Partial<ImagePreset>, baseRevision?: string): Promise<ImagePreset> {
  return requestJSON(`/api/image-presets/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision)),
  })
}

export function deleteImagePreset(id: string): Promise<void> {
  return requestJSON(`/api/image-presets/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getInteractiveBranches(storyId: string): Promise<BranchSummary[]> {
  const data = await requestJSON<{ branches: BranchSummary[] }>(`/api/interactive/stories/${encodeURIComponent(storyId)}/branches`)
  return data.branches || []
}

export function createInteractiveBranch(storyId: string, input: { parent_event_id: string; title: string; custom_agent_id?: string }): Promise<BranchSummary> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/branches`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function deleteInteractiveBranch(storyId: string, branchId: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/branches/${encodeURIComponent(branchId)}`, { method: 'DELETE' })
}

export function updateInteractiveBranchPlan(storyId: string, branchId: string, input: { markdown: string; base_revision: string }): Promise<UpdateBranchPlanResult> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/branches/${encodeURIComponent(branchId)}/plan`, {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

/**
 * Asks the in-story assistant one read-only question.
 *
 * The request is bound to the turn the player is looking at. When the branch has
 * moved on, the server refuses rather than answering about a scene that is over.
 */
export function askStoryAssistant(storyId: string, input: {
  branchId: string
  turnId: string
  action: 'analyze' | 'inspire'
  playerInput?: string
  chosenOption?: string
}): Promise<StoryAssistantResult> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/assistant`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      branch_id: input.branchId,
      turn_id: input.turnId,
      action: input.action,
      player_input: input.playerInput || '',
      chosen_option: input.chosenOption || '',
    }),
  })
}

/**
 * Records a direction the player explicitly adopted as this branch's forward
 * intent.
 *
 * This is the only write in the assistant flow. Reading an analysis or an
 * inspiration never calls it; the player has to accept a direction first.
 */
export function adoptStoryAssistantDirection(storyId: string, input: {
  branchId: string
  turnId: string
  direction: string
  baseRevision: string
}): Promise<StoryAssistantAdoptResult> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/assistant/adopt`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      branch_id: input.branchId,
      turn_id: input.turnId,
      direction: input.direction,
      base_revision: input.baseRevision,
    }),
  })
}

export function switchInteractiveBranch(storyId: string, branchId: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/switch-branch`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ branch_id: branchId }),
  })
}

export function switchInteractiveTurnVersion(storyId: string, input: { branch_id: string; turn_id: string; version_turn_id: string }): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/switch-turn-version`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateInteractiveTurnNarrative(storyId: string, turnId: string, input: { branch_id: string; narrative: string; expected_narrative: string }): Promise<UpdateTurnNarrativeResult> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/turns/${encodeURIComponent(turnId)}/narrative`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function generateInteractiveImage(storyId: string, input: { command_id: string; branch_id?: string; turn_id: string; source: 'manual' | 'auto'; force?: boolean }): Promise<{ enabled?: boolean; skipped?: boolean; skipped_reason?: string; image?: InteractiveImage }> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/images/generate`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export interface InteractiveStartInput {
  command_id: string
  mode: 'story' | 'setting'
  story_id: string
  branch?: string
  message?: string
  start_opening?: boolean
  resume_interruption_id?: string
  style_scenes?: string[]
  attachments?: ChatAttachmentUpload[]
  regenerate_from_turn_id?: string
  player_letter?: { character_id: string; body: string }
  signal?: AbortSignal
}

export async function sendInteractiveMessage(input: InteractiveStartInput): Promise<ReadableStream<InteractiveSSEEvent>> {
  const { signal, ...body } = input
  const res = await fetchAPI('/api/interactive/chat', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(body),
    signal,
  })
  if (!res.ok) throw await responseAPIError(res)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export interface ActiveInteractiveChat {
  active: boolean
  status?: 'running' | 'done' | 'aborted' | 'error' | 'suspended'
  pending_ask?: import('@/lib/api').AgentAskInteraction
  task_id?: string
  command_id?: string
  story_id?: string
  branch_id?: string
  message?: string
  attachments?: ChatAttachmentDescriptor[]
  regenerate_from_turn_id?: string
  /** Diagnostic-only latest raw SSE cursor; runtime `cursor` is durable state. */
  stream_cursor?: number
  cursor?: number
  phase?: 'idle' | 'running' | string
  recovery_paused?: boolean
  runtime_recoverable?: boolean
  stream_attached?: boolean
  recovery_actions?: AgentRuntimeRecoveryAction[]
  active_operation_id?: string
  active_cycle?: number
  active_output?: AgentRuntimeActiveOutput
  queue?: AgentRuntimeQueuedCommand[]
  open_tools?: AgentRuntimeOpenTool[]
  last_operation?: AgentRuntimeOperation
  /** Exact durable pause checkpoint available to the next idle turn. */
  pending_interruption_id?: string
}

export function resolveInteractiveAsk(storyId: string, branchId: string, askId: string, action: import('@/lib/agent-ask').AgentAskResolveAction): Promise<import('@/lib/api').AgentAskResolution> {
  return requestJSON(`/api/interactive/chat/asks/${encodeURIComponent(askId)}/${action.status === 'answered' ? 'answer' : 'cancel'}`, {
    method: 'POST', headers: jsonHeaders,
    body: JSON.stringify({ story_id: storyId, branch_id: branchId, ...(action.status === 'answered' ? { answers: action.answers } : { reason: action.reason }) }),
  })
}

interface InteractiveAgentCommandBase {
  commandId: string
  targetOperationId: string
  storyId: string
  branchId?: string
}

export type InteractiveAgentCommand = InteractiveAgentCommandBase & (
  | {
    type: 'follow_up'
    input: {
      message: string
      styleScenes?: string[]
      attachments?: ChatAttachmentUpload[]
    }
  }
  | {
    type: 'steer_queued' | 'cancel_queued'
    targetCommandId: string
    reason?: string
  }
  | {
    type: 'abort' | 'suspend'
    reason?: string
  }
)

/** Submit a targeted command to the exact durable game operation in view. */
export function submitInteractiveAgentCommand(command: InteractiveAgentCommand): Promise<AgentCommandReceipt> {
  const input = 'input' in command
    ? {
        message: command.input.message,
        ...(command.input.styleScenes?.length ? { style_scenes: command.input.styleScenes } : {}),
        ...(command.input.attachments?.length ? { attachments: command.input.attachments } : {}),
      }
    : undefined
  const targetCommandId = 'targetCommandId' in command ? command.targetCommandId : ''
  const reason = 'reason' in command ? command.reason : undefined
  return requestJSON('/api/interactive/chat/commands', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      type: command.type,
      command_id: command.commandId,
      target_operation_id: command.targetOperationId,
      story_id: command.storyId,
      ...(command.branchId ? { branch_id: command.branchId } : {}),
      ...(targetCommandId ? { target_command_id: targetCommandId } : {}),
      ...(input ? { input } : {}),
      ...(reason ? { reason } : {}),
    }),
  })
}

export function getActiveInteractiveChat(storyId: string, branchId?: string): Promise<ActiveInteractiveChat> {
  return requestJSON(`/api/interactive/chat/active?${interactiveChatQuery(storyId, branchId)}`)
}

/** Recover accepted game work by server-projected identity, never by browser-cached input. */
export function recoverInteractiveAgentRuntime(input: {
  storyId: string
  branchId?: string
  action: AgentRuntimeRecoveryAction
}): Promise<AgentRuntimeRecoveryReceipt> {
  return requestJSON('/api/interactive/chat/recovery', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      action: input.action,
      story_id: input.storyId,
      ...(input.branchId ? { branch: input.branchId } : {}),
    }),
  })
}

export async function streamActiveInteractiveChat(input: { storyId: string; branchId?: string; taskId?: string; after?: string; signal?: AbortSignal }): Promise<ReadableStream<InteractiveSSEEvent>> {
  const taskId = input.taskId?.trim()
  if (!taskId) throw new Error('Cannot reconnect without an exact Agent stream task')
  const res = await fetchAPI(`/api/interactive/chat/stream?${interactiveChatQuery(input.storyId, input.branchId, taskId, input.after)}`, { signal: input.signal })
  if (!res.ok) throw await responseAPIError(res)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

function interactiveChatQuery(storyId: string, branchId?: string, taskId?: string, after?: string) {
  const params = new URLSearchParams({ story_id: storyId })
  if (branchId) params.set('branch', branchId)
  if (taskId) params.set('task_id', taskId)
  if (after) params.set('after', after)
  return params.toString()
}

export function analyzeInteractiveContext(input: { mode: 'story'; story_id: string; branch?: string; message: string; style_scenes?: string[] }): Promise<ContextAnalysis> {
  return requestJSON('/api/interactive/chat/context-analysis', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

const interactiveStructuralCommandIDs = new Map<string, string>()

export async function compactInteractiveContext(storyId: string, branchId?: string): Promise<void> {
  const key = `compact:${storyId}:${branchId ?? ''}`
  const commandId = interactiveStructuralCommandIDs.get(key) ?? createAgentCommandID()
  interactiveStructuralCommandIDs.set(key, commandId)
  try {
    await requestJSON<Record<string, unknown>>(`/api/interactive/stories/${encodeURIComponent(storyId)}/context-compaction`, {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ command_id: commandId, branch_id: branchId }),
    })
    interactiveStructuralCommandIDs.delete(key)
  } catch (error) {
    if (isKnownAgentCommandOutcome(error)) interactiveStructuralCommandIDs.delete(key)
    throw error
  }
}

export async function removeInteractiveContextCompaction(storyId: string, branchId?: string): Promise<boolean> {
  const key = `remove:${storyId}:${branchId ?? ''}`
  const commandId = interactiveStructuralCommandIDs.get(key) ?? createAgentCommandID()
  interactiveStructuralCommandIDs.set(key, commandId)
  const query = new URLSearchParams({ command_id: commandId })
  if (branchId) query.set('branch', branchId)
  try {
    const data = await requestJSON<{ removed?: boolean }>(`/api/interactive/stories/${encodeURIComponent(storyId)}/context-compaction/active?${query}`, {
      method: 'DELETE',
    })
    interactiveStructuralCommandIDs.delete(key)
    return Boolean(data.removed)
  } catch (error) {
    if (isKnownAgentCommandOutcome(error)) interactiveStructuralCommandIDs.delete(key)
    throw error
  }
}

// All views, including the player snapshot, belong to one journal read lease.
export function getLoreOverview(storyId: string, options: { branchId?: string } = {}): Promise<{ snapshot: InteractiveSnapshotResponse; characters: CharacterWindow; journey: JourneyState; quests: QuestState; events: PublicEventState }> {
  const query = options.branchId ? `?branch=${encodeURIComponent(options.branchId)}` : ''
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/lore-overview${query}`)
}
