import { useStorySpeech } from '../use-story-speech'
import { resolveWorldPresentation } from '../world-presentation'
import { WorldPresentationProvider } from '../world-presentation-context'
import { useImageModelConfigured } from '@/features/settings/use-image-model-configured'
import { SpeechPlayback } from '@/features/speech/SpeechPlayback'
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { BookOpen, HeartHandshake, MessagesSquare, Swords } from 'lucide-react'
import { EmptyState } from '@/components/common/EmptyState'
import { LoadingState } from '@/components/common/LoadingState'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { AgentTaskControls } from '@/components/Chat/AgentTaskControls'
import { MessageList, type ProseRevealRequest, type TurnScrollRequest } from '@/components/Chat/MessageList'
import { AgentSubAgentSessionPanel } from '@/components/Chat/AgentSubAgentSessionPanel'
import type { ComposerTokenInputHandle, ComposerTokenSpec, ComposerTrigger } from '@/components/Chat/composer-token-input'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { agentMessageDisplayText, createAgentDataMessage } from '@/lib/agent-ui-message'
import { agentSubAgentSessionKey, agentViewContent, agentViewAskID, agentViewAskInteraction, type AgentMessageView } from '@/lib/agent-message-view'
import { useSkillCommands } from '@/hooks/useSkillCommands'
import { useConversationConfig } from '@/features/conversation-config/use-conversation-config'
import type { ConversationConfigBinding, ConversationConfigChanges } from '@/features/conversation-config/types'
import { supportsRuntimeOperation } from '@/features/conversation-config/types'
import { getActiveInteractiveChat, getInteractiveHistoryPage, resolveInteractiveAsk, switchInteractiveTurnVersion, updateInteractiveTurnNarrative } from '../api'
import { sanitizeStoredNarrative } from '../stream-parser'
import { emptyStoryStageRun, useInteractiveStore } from '../stores/interactive-store'
import type { StoryStageRunState } from '../stores/interactive-store'
import { useInteractiveAgentCommands, type StoryStageRuntimeUpdater } from '../use-interactive-agent-commands'
import { DEFAULT_NARRATIVE_STYLE_ID } from '../narrative-style'
import { StoryStageControls } from './story-stage/StoryStageControls'
import { StoryWindow } from './story-stage/story-window'
import { NewStorySetupPanel } from './NewStorySetupPanel'
import { TurnNavigator } from './TurnNavigator'
import { DEFAULT_STORY_STATE_DISPLAY, type StoryStateDisplayPreference } from './story-state/display-preference'
import { StoryStateLedger } from './story-state/StoryStateLedger'
import { buildStoryStateModel } from './story-state/model'
import { storyStageSnapshotKey } from './story-stage/utils'
import { addStagedAction, composeStagedRequest, newStagedActionID, type StagedAction } from './story-stage/staged-actions'
import { useLiveMessageAccumulator } from './story-stage/use-live-message-accumulator'
import { useStoryStageMessages } from './story-stage/use-story-stage-messages'
import { useStoryHistoryWindow } from './story-stage/use-story-history-window'
import { useStoryImages } from './story-stage/use-story-images'
import { useStoryStageRuntime } from './story-stage/use-story-stage-runtime'
import { StoryStageComposer } from './story-stage/StoryStageComposer'
import { useStoryAssistant } from './story-stage/use-story-assistant'
import { StoryStageHeader } from './story-stage/StoryStageHeader'
import { ChatWindowPanel, type ChatTarget } from './chat-window/ChatWindowPanel'
import { CombatWindowPanel } from './combat-window/CombatWindowPanel'
import { AdventureCombatPanel } from './combat-window/AdventureCombatPanel'
import { IntimacyWindowPanel } from './intimacy-window/IntimacyWindowPanel'
import { RealmWindowPanel } from './realm-window/RealmWindowPanel'
import { QuestWindowPanel } from './quest-window/QuestWindowPanel'
import { LoreOverviewPanel } from './lore-overview/LoreOverviewPanel'
import { IncomingMessageHint } from './story-stage/IncomingMessageHint'
import { useStoryCharacterMessages } from './story-stage/use-story-character-messages'
import { buildStoryStageCommandMenu } from './story-stage/story-stage-commands'
import { branchCreationSourceFromMessage, branchCreationSourceFromTurn } from './branching/model'
import type { StoryStageProps } from './story-stage/story-stage-props'
import { useStoryWaitingHintIndex, storyWaitingHintKey } from './story-stage/use-story-waiting-hint'
import { useIsMobile } from '@/hooks/useIsMobile'

const DEFAULT_READING_FONT_SIZE = 18
const EMPTY_STAGE_RUN = emptyStoryStageRun()
const STAGE_LINE_HEIGHT = 1.78

export function StoryStage({ active = true, projectId, workspace, styleSceneSuggestions = [], stories = [], story, tellers = [], planningTemplates = [], recentNarrativeStyleID = DEFAULT_NARRATIVE_STYLE_ID, narrativeStyleLoading = false, storyId, branchId, snapshot, snapshotLoading = false, loreItems = [], directorPanelVisible = true, stateDisplayPreference = DEFAULT_STORY_STATE_DISPLAY, onStorySelect = noop, onStorySetupUpdate = noop, onStoryDelete = noop, onStoryRename, onRequestLoreInit, onOpenDirectorConfig, onOpenWorldPacks, onToggleDirectorPanel, onOpenDirectorState, onRegisterOpenTurn, onRequestCreateBranch, onStateDisplayPreferenceChange = noopStateDisplayPreferenceChange, onTurnPersisted = noopTurnPersisted, onBranchPlanAdvanced, onDone }: StoryStageProps) {
  const { t, i18n } = useTranslation()
  const imageConfigured = useImageModelConfigured(projectId)
  const conversationBinding = useMemo<ConversationConfigBinding | undefined>(() => storyId ? {
    mode: 'interactive', project_id: projectId, story_id: storyId,
    branch_id: branchId || snapshot?.branch_id || 'main',
  } : undefined, [branchId, projectId, snapshot?.branch_id, storyId])
  const conversationConfig = useConversationConfig(conversationBinding)
  const approvalReady = conversationConfig.initialized && !conversationConfig.saving
  const isMobile = useIsMobile()
  const storyStateModel = useMemo(() => buildStoryStateModel(snapshot), [snapshot])
  const worldPresentation = resolveWorldPresentation({ story, snapshot }, i18n.resolvedLanguage || i18n.language, t)
  const independentCombatAvailable = worldPresentation.combatMode !== 'none'
  const BattlePanel = worldPresentation.combatMode === 'adventure' ? AdventureCombatPanel : CombatWindowPanel
  // Dual cultivation belongs to a world whose state system is the cultivation
  // one. It is a display judgement, not a rule: the activity itself decides
  // whether an invitation is legal, and nothing here may grant or forbid one.
  const dualCultivationAvailable = worldPresentation.intimacyAvailable
  const [input, setInput] = useState('')
  const [styleScenes, setStyleScenes] = useState<string[]>([])
  const [styleSceneQuery, setStyleSceneQuery] = useState<string | null>(null)
  const [showSkillCommands, setShowSkillCommands] = useState(false)
  const [skillCommandQuery, setSkillCommandQuery] = useState<string | null>(null)
  const [activeSkillCommandIndex, setActiveSkillCommandIndex] = useState(0)
  const [inputFloatHeight, setInputFloatHeight] = useState(0)
  const inputRef = useRef<ComposerTokenInputHandle | null>(null)
  const inputFloatRef = useRef<HTMLDivElement | null>(null)
  const skillCommands = useSkillCommands({
    agentKey: 'interactive_story',
    projectId,
  })
  const snapshotKey = storyStageSnapshotKey(storyId, branchId, snapshot)
  const stageKey = `${workspace || 'current'}:${storyId || 'none'}:${branchId || snapshot?.branch_id || 'main'}`
  const { displaySnapshot, historyWindow, prependPage: prependHistoryPage, resetToLatest: resetHistoryToLatest } = useStoryHistoryWindow(stageKey, snapshot)
  const speech = useStorySpeech({ owner: stageKey, story, snapshot: displaySnapshot, active })
  const [historyLoading, setHistoryLoading] = useState(false)
  const stageRun = useInteractiveStore((state) => state.storyStageRuns[stageKey] || EMPTY_STAGE_RUN)
  const setStoryStageRun = useInteractiveStore((state) => state.setStoryStageRun)
  const clearStoryStageRun = useInteractiveStore((state) => state.clearStoryStageRun)
  const streaming = stageRun.streaming
  const activityContent = stageRun.activityContent
  const liveMessages = stageRun.liveMessages
  const rewindTurnId = stageRun.rewindTurnId
  const branchTerminal = snapshot?.current_turn?.terminal_outcome?.terminal === true
  const [replyEditTarget, setReplyEditTarget] = useState<{
    turnId: string
    branchId: string
    initialContent: string
    expectedNarrative: string
  } | null>(null)
  const [editingTurn, setEditingTurn] = useState<{
    id: string
    content: string
  } | null>(null)
  const [switchingVersionTurnId, setSwitchingVersionTurnId] = useState<string | null>(null)
  const [hotChoicesExpanded, setHotChoicesExpanded] = useState(false)
  // The in-story assistant is an inline panel the player opens on demand. It is
  // never called automatically: every question costs a model call and is asked
  // deliberately.
  const [assistantOpen, setAssistantOpen] = useState(false)
  const [assistantExpanded, setAssistantExpanded] = useState(false)
  // A world started from the world page already has its character and its
  // opening, so the stage must generate that opening rather than show the
  // setup panel again. The store carries the handoff because the switch to
  // the game surface happens in a different component.
  const storePendingOpeningStoryId = useInteractiveStore((state) => state.pendingOpeningStoryId)
  const setStorePendingOpeningStoryId = useInteractiveStore((state) => state.setPendingOpeningStoryId)
  const [pendingOpeningStoryId, setPendingOpeningStoryId] = useState('')
  // A story created through character creation already has its protagonist. Its
  // opening is generated without the setup screen, and this records the one
  // attempt so a failure shows a retry instead of looping.
  const identityConfirmed = story?.identity_confirmed === true
  const [confirmedOpeningFailed, setConfirmedOpeningFailed] = useState(false)
  const confirmedOpeningAttemptRef = useRef('')
  const [activeSubAgentSessionKey, setActiveSubAgentSessionKey] = useState('')
  const [activeTurnAnchorId, setActiveTurnAnchorId] = useState('')
  const [turnScrollRequest, setTurnScrollRequest] = useState<TurnScrollRequest>()
  const [proseRevealRequest, setProseRevealRequest] = useState<ProseRevealRequest>()
  const revealRequestIdRef = useRef(0)
  // A turn persisted while the runtime was still streaming; its prose is only
  // projected once the run settles, so the reveal waits for that render.
  const pendingRevealTurnIdRef = useRef('')
  // One window holds every conversation. A private chat and a group are still
  // two different records, but they are not two different products: the window
  // lists both and hosts whichever one is open.
  const [chatWindowOpen, setChatWindowOpen] = useState(false)
  const [chatTarget, setChatTarget] = useState<ChatTarget | null>(null)
  // Kept here rather than inside the panels: closing the window unmounts them,
  // and a half-written message must survive the player stepping away. One entry
  // per conversation, so switching does not overwrite what was typed in another.
  const [chatDrafts, setChatDrafts] = useState<Record<string, string>>({})
  // A person named by something else in the game — an incoming message, a
  // profile — and handed to the overview so it opens on them.
  const [loreOverviewFocusID, setLoreOverviewFocusID] = useState('')
  const [combatWindowOpen, setCombatWindowOpen] = useState(false)
  const [combatWindowSessionID, setCombatWindowSessionID] = useState('')
  // The intimacy activity has its own window again (T27). Opening it from the
  // toolbar is the multi-person form; a person named here is the one the window
  // was opened for, which makes its invitation the one-to-one kind.
  const [intimacyWindowOpen, setIntimacyWindowOpen] = useState(false)
  const [intimacyFocusID, setIntimacyFocusID] = useState('')
  const [realmWindowOpen, setRealmWindowOpen] = useState(false)
  const [questWindowOpen, setQuestWindowOpen] = useState(false)
  // Arrangements made in a window wait here until the player sends. They are
  // never a turn of their own: the story only sees them once the player presses
  // send, together with the rest of that request.
  const [stagedActions, setStagedActions] = useState<StagedAction[]>([])
  // The overview is a read-only reading surface beside the prose. It is closed
  // on first entry (§2), and closing it never touches the draft, the story or
  // an open chat/battle window.
  const [loreOverviewOpen, setLoreOverviewOpen] = useState(false)
  // §2: closing puts focus back on the entry that opened it, so a keyboard
  // reader is not dropped at the top of the page.
  const loreOverviewTriggerRef = useRef<HTMLButtonElement | null>(null)
  const closeLoreOverview = () => {
    setLoreOverviewOpen(false)
    setLoreOverviewFocusID('')
    loreOverviewTriggerRef.current?.focus()
  }
  // The chat can show whose conversation is open without becoming a profile.
  // Opening the overview on that person keeps the two jobs apart: the chat stays
  // the place to talk, and the overview stays the place to read who they are.
  const openProfileFromChat = (characterID: string) => {
    setLoreOverviewFocusID(characterID)
    setLoreOverviewOpen(true)
  }
  // The stage's own read of incoming messages, independent of whether the window
  // is open: the hint has to be visible while the player is reading the story.
  const storyMessages = useStoryCharacterMessages(
    storyId || '', branchId || snapshot?.branch_id || '', active && !chatWindowOpen, snapshotKey,
  )
  const chatWindowUnread = storyMessages.unread

  // The window belongs to one story branch; carrying a selection across is wrong.
  useEffect(() => {
    setChatTarget(null)
    setChatDrafts({})
  }, [stageKey])

  // A battle belongs to one branch too: a session id from another branch names a
  // fight that is not here, and reopening it would report the wrong battle.
  useEffect(() => {
    setCombatWindowSessionID('')
  }, [stageKey])

  useEffect(() => {
    setReplyEditTarget(null)
  }, [stageKey])

  // A staged arrangement belongs to one branch: carrying it across would put
  // words the player agreed to in another story into this one's request.
  useEffect(() => {
    setStagedActions([])
  }, [stageKey])
  useEffect(() => {
    if (streaming && !historyWindow.followLatest) resetHistoryToLatest()
  }, [historyWindow.followLatest, resetHistoryToLatest, streaming])
  const previousSnapshotKeyRef = useRef(snapshotKey)
  const stageTextStyle = useMemo<CSSProperties>(
    () => ({
      fontSize: `var(--nova-reading-font-size, ${DEFAULT_READING_FONT_SIZE}px)`,
      lineHeight: STAGE_LINE_HEIGHT,
      fontFamily: 'var(--nova-reading-font-family)',
    }),
    [],
  )
  const inputTextStyle = useMemo<CSSProperties>(
    () => ({
      fontSize: `min(var(--nova-reading-font-size, ${DEFAULT_READING_FONT_SIZE}px), 16px)`,
      lineHeight: 1.35,
      fontFamily: 'var(--nova-reading-font-family)',
    }),
    [],
  )

  const updateStageRun = useCallback(
    (updater: Partial<StoryStageRunState> | ((current: StoryStageRunState) => StoryStageRunState)) => {
      setStoryStageRun(stageKey, updater)
    },
    [setStoryStageRun, stageKey],
  )

  const setStageRuntime = useCallback(
    (runtime: StoryStageRuntimeUpdater) => updateStageRun((current) => ({
      ...current,
      runtime: typeof runtime === 'function' ? runtime(current.runtime) : runtime,
    })),
    [updateStageRun],
  )
  const readStageRuntime = useCallback(
    () => useInteractiveStore.getState().storyStageRuns[stageKey]?.runtime || EMPTY_STAGE_RUN.runtime,
    [stageKey],
  )
  const interactiveAgentCommands = useInteractiveAgentCommands({
    storyId,
    branchId,
    readRuntime: readStageRuntime,
    onRuntimeChange: setStageRuntime,
  })
  const setStageStreaming = useCallback(
    (value: boolean) => {
      updateStageRun({ streaming: value })
    },
    [updateStageRun],
  )

  const setStageActivityContent = useCallback(
    (value: string) => {
      updateStageRun({ activityContent: value })
    },
    [updateStageRun],
  )

  const setStageLiveMessages = useCallback(
    (updater: AgentUIMessage[] | ((current: AgentUIMessage[]) => AgentUIMessage[])) => {
      updateStageRun((current) => ({
        ...current,
        liveMessages: typeof updater === 'function' ? updater(current.liveMessages) : updater,
      }))
    },
    [updateStageRun],
  )

  const liveAccumulator = useLiveMessageAccumulator({
    setMessages: setStageLiveMessages,
  })
  const storyImages = useStoryImages({
    stageKey,
    storyId,
    branchId,
    snapshot,
    t,
    onDone,
    setActivity: setStageActivityContent,
  })
  const liveTurnNavigationAnchorId = useMemo(() => `live:${stageKey}`, [stageKey])
  // The waiting hint is interface narration for the ordinary wait. It pauses
  // while a real question needs the player and never replaces real status.
  const waitingHintIndex = useStoryWaitingHintIndex(streaming && !stageRun.runtime.pendingAsk && !stageRun.runtime.recoveryPaused)
  const waitingHint = waitingHintIndex >= 0 ? t(storyWaitingHintKey(waitingHintIndex)) : ''
  const {
    agentMessages,
    turnNavigationItems,
    turnsById,
  } = useStoryStageMessages({
    pendingAsk: stageRun.runtime.pendingAsk,
    snapshot: displaySnapshot,
    rewindTurnId,
    liveMessages,
    streaming,
    stageKey,
    liveTurnNavigationAnchorId,
    optimisticInteractiveImages: storyImages.optimisticImages,
    belongsToStage: liveAccumulator.belongsToStage,
    renderKeyFor: liveAccumulator.renderKeyFor,
  })
  const {
    commands: filteredSkillCommands,
    builtInItems: filteredBuiltInCommandItems,
    skillItems: filteredSkillCommandItems,
  } = useMemo(() => buildStoryStageCommandMenu(skillCommandQuery, skillCommands, {
    compactDescription: t('chat.command.compact.desc'),
    compactHint: t('chat.command.compact.hint'),
    skillHint: t('chat.command.skill.hint'),
  }), [skillCommandQuery, skillCommands, t])

  useEffect(() => {
    if (previousSnapshotKeyRef.current === snapshotKey) return
    if (streaming) return
    previousSnapshotKeyRef.current = snapshotKey
    setStageActivityContent('')
    if (liveMessages.length > 0) {
      clearStoryStageRun(stageKey)
    }
  }, [clearStoryStageRun, liveMessages.length, setStageActivityContent, snapshotKey, stageKey, streaming])

  // Switching story, branch or reopening drops a reveal that was still owed:
  // its turn does not exist on the stage the player is now looking at. The key
  // is the stage, not the snapshot: the snapshot's own key also changes the
  // moment a turn is committed, which is exactly when the reveal is owed.
  useEffect(() => {
    pendingRevealTurnIdRef.current = ''
  }, [stageKey])

  // The finished turn only becomes readable prose after the run settles, so the
  // reveal is issued here rather than when the turn was persisted.
  useEffect(() => {
    const turnId = pendingRevealTurnIdRef.current
    if (!turnId || streaming) return
    if (!turnNavigationItems.some((item) => item.anchorId === turnId)) return
    pendingRevealTurnIdRef.current = ''
    revealRequestIdRef.current += 1
    setProseRevealRequest({ anchorId: turnId, requestId: revealRequestIdRef.current })
  }, [streaming, turnNavigationItems])

  useEffect(() => {
    if (activeSkillCommandIndex >= filteredSkillCommands.length) setActiveSkillCommandIndex(0)
  }, [activeSkillCommandIndex, filteredSkillCommands.length])

  const handleTurnNavigationSelect = useCallback((anchorId: string) => {
    setActiveTurnAnchorId(anchorId)
    setTurnScrollRequest((current) => ({
      anchorId,
      requestId: (current?.requestId || 0) + 1,
    }))
  }, [])
  // The handbook beside the stage opens a turn through this same jump, so both
  // surfaces scroll the prose the same way instead of keeping two mechanisms.
  useEffect(() => {
    onRegisterOpenTurn?.(handleTurnNavigationSelect)
    return () => onRegisterOpenTurn?.(null)
  }, [handleTurnNavigationSelect, onRegisterOpenTurn])
  const handleVisibleTurnAnchorChange = useCallback((anchorId: string) => {
    setActiveTurnAnchorId(anchorId)
  }, [])
  useEffect(() => {
    const fallbackAnchorId = turnNavigationItems[turnNavigationItems.length - 1]?.anchorId || ''
    setActiveTurnAnchorId((current) => {
      if (!current) return fallbackAnchorId
      return turnNavigationItems.some((item) => item.anchorId === current) ? current : fallbackAnchorId
    })
  }, [turnNavigationItems])
  const openSubAgentSession = useCallback((view: AgentMessageView) => {
    const key = agentSubAgentSessionKey(view)
    if (key) setActiveSubAgentSessionKey(key)
  }, [])
  const scrollResetKey = `${storyId || 'none'}:${branchId || snapshot?.branch_id || 'main'}`
  const hotChoices = useMemo(
    () =>
      (snapshot?.current_turn?.turn_result?.choices || snapshot?.current_turn?.hot_state?.choices || [])
        .map((choice) => choice.trim())
        .filter(Boolean),
    [snapshot?.current_turn?.hot_state?.choices, snapshot?.current_turn?.turn_result?.choices],
  )
  const canUseHotChoices = hotChoices.length > 0 && !branchTerminal && !streaming && !editingTurn && Boolean(storyId)
  const showHotChoices = canUseHotChoices && hotChoicesExpanded
  // A direction can only be adopted where the branch actually carries a plan.
  // The revision is what makes the write safe: an adopted direction is added to
  // the plan the player was shown, not to a newer one they never saw.
  const assistant = useStoryAssistant({
    storyId,
    branchId: branchId || snapshot?.branch_id || 'main',
    turnId: snapshot?.current_turn?.id || '',
    planRevision: snapshot?.branch_plan?.revision,
    onPlanAdvanced: onBranchPlanAdvanced,
  })
  // A conflicted adoption is fixed by reading the plan the branch actually
  // carries; the advice stays on screen so the player can record it again.
  const reloadAssistantPlan = useCallback(() => {
    void onDone({ silent: true })
  }, [onDone])
  const messageListBottomPadding = inputFloatHeight > 0 ? inputFloatHeight + 20 : undefined
  const loadEarlierMessages = useCallback(async () => {
    if (!storyId || !historyWindow.beforeCursor || historyLoading) return
    setHistoryLoading(true)
    try {
      const page = await getInteractiveHistoryPage(storyId, branchId || snapshot?.branch_id || 'main', historyWindow.beforeCursor)
      prependHistoryPage(page)
    } catch (error) {
      console.error('[interactive-stage] load earlier story history failed', error)
      setStageLiveMessages((current) => [...current, errorMessage(error instanceof Error ? error.message : t('chat.history.loadEarlierFailed'))])
    } finally {
      setHistoryLoading(false)
    }
  }, [branchId, historyLoading, historyWindow.beforeCursor, prependHistoryPage, setStageLiveMessages, snapshot?.branch_id, storyId, t])
  const latestTurnID = snapshot?.current_turn?.id || snapshot?.turns?.at(-1)?.id || ''
  const canMutateStoryView = useCallback((view: AgentMessageView) => {
    const turnID = view.metadata.turn_id
    return !turnID || !latestTurnID || turnID === latestTurnID
  }, [latestTurnID])
  const syncInputFloatHeight = useCallback(() => {
    const element = inputFloatRef.current
    if (!element) return
    const nextHeight = Math.ceil(element.getBoundingClientRect().height)
    setInputFloatHeight((current) => (current === nextHeight ? current : nextHeight))
  }, [])

  useLayoutEffect(() => {
    syncInputFloatHeight()
  }, [editingTurn, hotChoices.length, input, showHotChoices, stageRun.runtime.queue.length, syncInputFloatHeight])

  useEffect(() => {
    const element = inputFloatRef.current
    if (!element || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(syncInputFloatHeight)
    observer.observe(element)
    return () => observer.disconnect()
  }, [syncInputFloatHeight])

  const toggleHotChoices = () => {
    if (!canUseHotChoices) return
    setHotChoicesExpanded((value) => !value)
  }

  const toggleAssistant = () => {
    if (!storyId || branchTerminal) return
    if (!assistantOpen) {
      setAssistantOpen(true)
      setAssistantExpanded(true)
      return
    }
    setAssistantExpanded((value) => !value)
  }

  const closeAssistant = () => {
    setAssistantOpen(false)
    setAssistantExpanded(false)
  }

  useEffect(() => {
    setHotChoicesExpanded(false)
  }, [snapshotKey])

  useEffect(() => {
    // An answer written for one branch must not sit beside another, so the panel
    // closes when the stage switches stories or branches. The stage is the unit,
    // not the snapshot: the snapshot key also moves when a turn is committed,
    // and a player reading an analysis should not have it closed under them by
    // their own turn finishing.
    setAssistantOpen(false)
    setAssistantExpanded(false)
  }, [stageKey])

  function clearSubmittedComposer() {
    speech.stop()
    setInput('')
    setEditingTurn(null)
    setStyleScenes([])
    setStyleSceneQuery(null)
    setShowSkillCommands(false)
    setSkillCommandQuery(null)
    setActiveSkillCommandIndex(0)
  }

  const { commandSubmitting, deleteQueuedCommand, queueActionPendingCommandID, send, steerQueuedCommand, stop, suspend, resumeTask } = useStoryStageRuntime({
    stageKey,
    storyId,
    branchId,
    input,
    editingTurnId: editingTurn?.id,
    styleScenes,
    streaming,
    branchTerminal,
    blocked: !approvalReady || (streaming && !supportsRuntimeOperation(conversationConfig.snapshot, 'queue')),
    stageRun,
    liveTurnNavigationAnchorId,
    t,
    interactiveAgentCommands,
    liveAccumulator,
    storyImages,
    readStageRuntime,
    setStageRuntime,
    updateStageRun,
    setStreaming: setStageStreaming,
    setActivity: setStageActivityContent,
    setMessages: setStageLiveMessages,
    clearComposer: clearSubmittedComposer,
    onTurnPersisted: (event, options) => {
      speech.onPersisted(event, options)
      // A replay only restores history and must not move the page.
      if (!options?.replayed && event.turn?.id) pendingRevealTurnIdRef.current = event.turn.id
      return onTurnPersisted(event, options)
    },
    onDone,
  })

  useEffect(() => {
    if (!storePendingOpeningStoryId || storePendingOpeningStoryId !== storyId) return
    setStorePendingOpeningStoryId('')
    setPendingOpeningStoryId(storePendingOpeningStoryId)
  }, [storePendingOpeningStoryId, storyId, setStorePendingOpeningStoryId])

  // The opening is generated once per stage. Two things ask for it: the handoff
  // from the world page, which lives in memory and is gone after a reload, and a
  // story whose protagonist came out of character creation, which is recorded on
  // the story itself and so still applies after a reload. Both are the same
  // request, and the backend refuses a second one while the first is running, so
  // one attempt ref covers them: whichever notices first sends, the other stands
  // down instead of colliding.
  useEffect(() => {
    if (!storyId) return
    const handoff = pendingOpeningStoryId === storyId
    if (!handoff && !identityConfirmed) return
    if (snapshot?.story_id !== storyId || snapshotLoading || streaming || !approvalReady) return
    if ((snapshot.turn_count || 0) > 0 || agentMessages.length > 0) {
      if (handoff) setPendingOpeningStoryId('')
      return
    }
    if (confirmedOpeningAttemptRef.current === stageKey) return
    confirmedOpeningAttemptRef.current = stageKey
    if (handoff) setPendingOpeningStoryId('')
    void send({ startOpening: true }).then((started) => {
      if (started) return
      // A confirmed character has nothing else to show, so the failure becomes a
      // retry the player can press; the handoff path still has its setup screen.
      if (identityConfirmed) setConfirmedOpeningFailed(true)
      else console.warn('[story-setup] Story was created but opening generation did not start', { storyId })
    })
  }, [agentMessages.length, approvalReady, identityConfirmed, pendingOpeningStoryId, send, snapshot?.story_id, snapshot?.turn_count, snapshotLoading, stageKey, storyId, streaming])

  const switchMessageVersion = async (view: AgentMessageView, direction: -1 | 1) => {
    const turnId = view.metadata.turn_id
    if (!turnId || !storyId || streaming || switchingVersionTurnId) return
    const versions = view.metadata.turn_versions || []
    const currentIndex = view.metadata.turn_version_index ?? versions.findIndex((version) => version.current)
    const nextVersion = versions[currentIndex + direction]
    if (!nextVersion) return
    speech.stop()
    setSwitchingVersionTurnId(turnId)
    setStageActivityContent(direction > 0 ? t('storyStage.activity.switchNewer') : t('storyStage.activity.switchOlder'))
    try {
      await switchInteractiveTurnVersion(storyId, {
        branch_id: branchId,
        turn_id: turnId,
        version_turn_id: nextVersion.turn_id,
      })
      clearStoryStageRun(stageKey)
      await onDone()
    } catch (error) {
      setStageLiveMessages((prev) => [
        ...prev,
        errorMessage(error instanceof Error ? error.message : t('storyStage.activity.switchFailed')),
      ])
    } finally {
      setSwitchingVersionTurnId(null)
      setStageActivityContent('')
    }
  }

  const startEditingView = (view: AgentMessageView) => {
    const turnId = view.metadata.turn_id
    if (!turnId || streaming) return
    const content = agentViewContent(view)
    setEditingTurn({ id: turnId, content })
    setInput(content)
    setShowSkillCommands(false)
    setActiveSkillCommandIndex(0)
    window.requestAnimationFrame(() => {
      inputRef.current?.focus()
      inputRef.current?.setSelectionRange(content.length, content.length)
    })
  }

  const startEditingAssistantReply = (view: AgentMessageView) => {
    if (streaming || storyImages.generatingTurnId || switchingVersionTurnId) return
    const turnId = view.metadata.turn_id
    if (!turnId) return
    const turn = turnsById.get(turnId)
    if (!turn) return
    speech.stop()
    setReplyEditTarget({
      turnId: turn.id,
      branchId: turn.branch_id || branchId,
      initialContent: sanitizeStoredNarrative(turn.narrative),
      expectedNarrative: turn.narrative,
    })
  }

  const startCreatingBranchFromView = (view: AgentMessageView) => {
    const turnId = view.metadata.turn_id
    if (!turnId || !onRequestCreateBranch) return
    const turn = turnsById.get(turnId)
    onRequestCreateBranch(turn
      ? branchCreationSourceFromTurn(turn, t('branchTimeline.nodeFallback'))
      : branchCreationSourceFromMessage(turnId, agentViewContent(view), t('branchTimeline.nodeFallback')))
  }

  const regenerateView = (view: AgentMessageView) => {
    if (streaming) return
    const turnId = view.metadata.turn_id
    if (!turnId) {
      if (stageRun.runtime.phase === 'suspended') {
        // Retry resumes the original operation, including its regeneration target.
        void resumeTask()
        return
      }
      const liveUserMessage = [...liveMessages].reverse().find((item) => item.role === 'user')
      const source = stageRun.retryMessage || (liveUserMessage ? agentMessageDisplayText(liveUserMessage) : '')
      if (source.trim()) void send({ message: source, rewindTurnId: stageRun.rewindTurnId })
      return
    }
    const source = turnsById.get(turnId)?.user || agentViewContent(view)
    void send({ message: source, rewindTurnId: turnId })
  }

  const switchViewVersion = (view: AgentMessageView, direction: -1 | 1) => {
    void switchMessageVersion(view, direction)
  }

  const generateImageForView = (view: AgentMessageView) => {
    void storyImages.generateForTurn(view.metadata.turn_id || '', 'manual', true)
  }

  const cancelEditing = () => {
    setEditingTurn(null)
    setInput('')
    setStyleSceneQuery(null)
    setShowSkillCommands(false)
    setSkillCommandQuery(null)
    setActiveSkillCommandIndex(0)
  }

  const handleInputChange = (nextValue: string) => {
    setInput(nextValue)
  }

  // 见闻与事务 only puts words in the player's own box. It never sends: the
  // player can edit them, and accepting or handing in still goes through the
  // main story's formal path.
  const fillQuestIntent = (intent: string) => {
    setInput(intent)
    setShowSkillCommands(false)
    setSkillCommandQuery(null)
    setActiveSkillCommandIndex(0)
    window.requestAnimationFrame(() => {
      inputRef.current?.focus()
      inputRef.current?.setSelectionRange(intent.length, intent.length)
    })
  }

  const handleInputTriggerChange = (trigger: ComposerTrigger | null) => {
    if (trigger?.kind === 'slash') {
      setSkillCommandQuery(trigger.query)
      setShowSkillCommands(true)
      setActiveSkillCommandIndex(0)
    } else {
      setSkillCommandQuery(null)
      setShowSkillCommands(false)
      setActiveSkillCommandIndex(0)
    }
    setStyleSceneQuery(trigger?.kind === 'style' ? trigger.query : null)
  }

  const selectSkillCommand = (name: string) => {
    const command = filteredSkillCommands.find((item) => item.name === name)
    if (command?.builtIn) {
      inputRef.current?.replaceActiveTriggerText(`/${name} `)
    } else {
      inputRef.current?.replaceActiveTriggerWithToken({ kind: 'skill', value: name, label: name })
    }
    setShowSkillCommands(false)
    setSkillCommandQuery(null)
    setActiveSkillCommandIndex(0)
    inputRef.current?.focus()
  }

  const selectStyleScene = (scene: string) => {
    inputRef.current?.replaceActiveTriggerWithToken({ kind: 'style', value: scene, label: scene })
    setStyleScenes((current) => Array.from(new Set([...current, scene])))
    setStyleSceneQuery(null)
    inputRef.current?.focus()
  }

  const removeStyleScene = (scene: string) => {
    setStyleScenes((current) => current.filter((item) => item !== scene))
  }

  const handleTokenRemove = (token: ComposerTokenSpec) => {
    if (token.kind === 'style' && styleScenes.includes(token.value)) removeStyleScene(token.value)
  }

  const selectHotChoice = (choice: string) => {
    setInput(choice)
    setShowSkillCommands(false)
    setSkillCommandQuery(null)
    setActiveSkillCommandIndex(0)
    setHotChoicesExpanded(false)
    window.requestAnimationFrame(() => {
      inputRef.current?.focus()
      inputRef.current?.setSelectionRange(choice.length, choice.length)
    })
  }

  const saveEditedReply = async (narrative: string) => {
    if (!replyEditTarget) return
    await updateInteractiveTurnNarrative(storyId, replyEditTarget.turnId, {
      branch_id: replyEditTarget.branchId,
      narrative,
      expected_narrative: replyEditTarget.expectedNarrative,
    })
    await onDone({ silent: true })
  }

  // An arrangement made in a window joins the main draft rather than becoming a
  // turn of its own. Adding one moves nothing in the world: no time passes, no
  // coin is spent, no item changes hands, and the other side's permission is not
  // read as the player's decision to act. Only the player's send does that.
  const stageAction = (action: Omit<StagedAction, 'id'>) => {
    if (streaming || commandSubmitting) return
    setStagedActions((current) => addStagedAction(current, { ...action, id: newStagedActionID() }))
  }

  // Editing a staged arrangement puts it back in the player's own box, where it
  // is theirs to rewrite. The entry is removed only because its words are now
  // in the box; leaving both would send the same plan twice.
  const editStagedAction = (id: string) => {
    const target = stagedActions.find((item) => item.id === id)
    if (!target) return
    setStagedActions((current) => current.filter((item) => item.id !== id))
    fillQuestIntent(target.message)
  }

  const removeStagedAction = (id: string) => {
    setStagedActions((current) => current.filter((item) => item.id !== id))
  }

  const actFromChatWindow = (message: string, playerLetter?: { character_id: string; body: string }) => {
    if (streaming || commandSubmitting) return
    stageAction({ message, ...(playerLetter ? { playerLetter } : {}) })
    setChatWindowOpen(false)
  }

  // A settled battle is read from the journal by the next turn, so continuing
  // from the window only puts the player's line into the draft. Nothing about
  // the fight is restated or re-decided here.
  const actFromCombatWindow = (message: string) => {
    if (streaming || commandSubmitting) return
    stageAction({ message })
    setCombatWindowOpen(false)
  }

  const actFromRealmWindow = (message: string) => {
    if (streaming || commandSubmitting) return
    stageAction({ message })
    setRealmWindowOpen(false)
  }

  // Same contract as the battle window: what happened is already recorded, so
  // this only carries the player's line into the next turn.
  const actFromIntimacyWindow = (message: string) => {
    if (streaming || commandSubmitting) return
    stageAction({ message })
    setIntimacyWindowOpen(false)
  }

  // The player's own send is what turns staged arrangements into a request. The
  // staged words and whatever they typed travel as one turn, so the story never
  // sees the same plan twice and never acts on one the player did not send.
  //
  // A refused send leaves the list untouched: the words are still the player's
  // to retry, and a retried request must not double up what the first attempt
  // already carried.
  //
  // A regenerate or an opening is not the player sending a new line, so neither
  // carries staged words: they would be appended to a message the player did
  // not write this time.
  const sendFromComposer = async (options?: Parameters<typeof send>[0]) => {
    const composing = !options?.startOpening && !options?.rewindTurnId
    if (!stagedActions.length || !composing) return send(options)
    const composed = composeStagedRequest(stagedActions, options?.message ?? input)
    const accepted = await send({ ...options, message: composed.message, playerLetter: composed.playerLetter })
    if (accepted) setStagedActions([])
    return accepted
  }

  const stageControls = (
    <StoryStageControls
      isMobile={isMobile}
      picker={{
        stories, currentStoryId: storyId,
        onSelect: (id) => { setPendingOpeningStoryId(''); onStorySelect(id) },
        onDeleteStories: onStoryDelete, onRenameStory: onStoryRename,
      }}
      history={{ items: turnNavigationItems, activeAnchorId: activeTurnAnchorId, onSelect: handleTurnNavigationSelect }}
      directorPanelVisible={directorPanelVisible}
      onToggleDirectorPanel={onToggleDirectorPanel}
      chatWindowVisible={chatWindowOpen}
      onToggleChatWindow={storyId ? () => setChatWindowOpen((open) => !open) : undefined}
      chatWindowUnread={chatWindowUnread}
      combatWindowVisible={combatWindowOpen}
      onToggleCombatWindow={storyId && independentCombatAvailable ? () => setCombatWindowOpen((open) => !open) : undefined}
      intimacyWindowVisible={intimacyWindowOpen}
      onToggleIntimacyWindow={storyId && dualCultivationAvailable ? () => {
        // The toolbar entry is the multi-person form, so it never keeps a
        // selection made earlier from a person's conversation.
        setIntimacyFocusID('')
        setIntimacyWindowOpen((open) => !open)
      } : undefined}
      realmWindowVisible={realmWindowOpen}
      onToggleRealmWindow={storyId ? () => setRealmWindowOpen((open) => !open) : undefined}
      questWindowVisible={questWindowOpen}
      onToggleQuestWindow={storyId ? () => setQuestWindowOpen((open) => !open) : undefined}
      loreOverviewVisible={loreOverviewOpen}
      onToggleLoreOverview={storyId ? () => setLoreOverviewOpen((open) => !open) : undefined}
      loreOverviewTriggerRef={loreOverviewTriggerRef}
    />
  )
  // Both sources count, so the handoff from the world page never flashes the
  // setup panel for a story that was already created with its character. The
  // id check is what keeps "no story at all" from matching two empty strings.
  const waitingToStartOpening = Boolean(storyId)
    && (pendingOpeningStoryId === storyId || storePendingOpeningStoryId === storyId)
  const committedTurnCount = Math.max(story?.turn_count || 0, snapshot?.turn_count || 0, snapshot?.turns?.length || 0)
  const openingRuntimeActive = streaming
    || Boolean(stageRun.runtime.operationId)
    || Boolean(stageRun.runtime.pendingInterruptionId)
    || stageRun.runtime.recoveryPaused
    || stageRun.runtime.queue.length > 0
    || Boolean(stageRun.retryMessage)
    || liveMessages.length > 0
  const storySetupVisible = !identityConfirmed
    && !waitingToStartOpening
    && !snapshotLoading
    && committedTurnCount === 0
    && !openingRuntimeActive
  // A confirmed story with no opening yet is waiting on generation, not on the
  // player. It shows the ordinary loading state, or a retry if that generation
  // already failed once on this stage.
  const confirmedOpeningPending = identityConfirmed
    && !snapshotLoading
    && committedTurnCount === 0
    && agentMessages.length === 0
    && !streaming

  // §3: the branch ends, the story does not. The ending turn is the branch head,
  // so the fork has to be cut from the turn before it — forking the ending turn
  // would put the new branch at the same terminal head and refuse the very first
  // action, which is the store's rule and not a limitation of this panel.
  const endingTurn = branchTerminal ? snapshot?.current_turn : undefined
  const branchEnding = useMemo(() => {
    if (!endingTurn?.terminal_outcome) return null
    const turns = snapshot?.turns || []
    const endingIndex = turns.findIndex((turn) => turn.id === endingTurn.id)
    const beforeEnding = endingIndex > 0
      ? turns[endingIndex - 1]
      : turns.length > 1 ? turns[turns.length - 2] : undefined
    return {
      outcome: endingTurn.terminal_outcome,
      canFork: Boolean(beforeEnding && onRequestCreateBranch),
      hasEarlier: historyWindow.stageKey === stageKey && historyWindow.hasMore,
      onReviewStory: () => {
        if (historyWindow.stageKey === stageKey && historyWindow.hasMore) {
          void loadEarlierMessages()
          return
        }
        const first = turnNavigationItems[0]?.anchorId
        if (first) handleTurnNavigationSelect(first)
      },
      onForkBeforeEnding: () => {
        if (beforeEnding) onRequestCreateBranch?.(branchCreationSourceFromTurn(beforeEnding, t('branchTimeline.nodeFallback')))
      },
      onStartNewStory: () => {
        setPendingOpeningStoryId('')
        onOpenWorldPacks?.()
      },
    }
  }, [branchTerminal, endingTurn, historyWindow.hasMore, historyWindow.stageKey, loadEarlierMessages, onOpenWorldPacks, onRequestCreateBranch, snapshot?.turns, stageKey, t, turnNavigationItems])

  return (
    <WorldPresentationProvider story={story} snapshot={snapshot}>
    <main className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-[var(--nova-surface-2)]">
      <div data-testid="story-stage-card" className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-surface-2)]">
        <StoryStageHeader isMobile={isMobile} controls={stageControls} />
        <div className="shrink-0 px-3"><SpeechPlayback owner={stageKey} /></div>

        <div data-story-stage-content className="nova-story-stage-content flex min-h-0 flex-1 overflow-hidden bg-[var(--nova-surface-2)]">
          {!isMobile && <TurnNavigator items={turnNavigationItems} activeAnchorId={activeTurnAnchorId} onSelect={handleTurnNavigationSelect} />}
          <section data-story-stage-body className="relative flex min-h-0 min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            {historyWindow.stageKey === stageKey && !historyWindow.followLatest ? (
              <Button type="button" variant="secondary" size="sm" className="absolute right-4 top-3 z-30 shadow-md" onClick={resetHistoryToLatest}>
                {t('storyStage.history.backToLatest')}
              </Button>
            ) : null}
            {storySetupVisible && story ? (
              <NewStorySetupPanel
                key={story.id}
                projectId={projectId}
                tellers={tellers}
                planningTemplates={planningTemplates}
                loreItems={loreItems}
                recentNarrativeStyleID={recentNarrativeStyleID}
                narrativeStyleLoading={narrativeStyleLoading}
                conversationConfig={conversationConfig}
                story={story}
                onRequestLoreInit={onRequestLoreInit}
                onOpenPresets={onOpenDirectorConfig}
                onCreate={async (input) => {
                  await onStorySetupUpdate(input)
                  const runtimeChanges: ConversationConfigChanges = {}
                  if (conversationConfig.snapshot?.profile_id !== input.profile_id) runtimeChanges.profile_id = input.profile_id
                  if (conversationConfig.snapshot?.thinking_level !== input.thinking_level) runtimeChanges.thinking_level = input.thinking_level
                  if (Object.keys(runtimeChanges).length > 0) {
                    const saved = await conversationConfig.patch(runtimeChanges)
                    if (!saved) throw new Error(t('storyPicker.setup.model.saveFailed'))
                  }
                  const started = await send({ startOpening: true })
                  if (!started) throw new Error(t('storyPicker.setup.startFailed'))
                }}
              />
            ) : storySetupVisible && !story ? (
              <EmptyState
                icon={BookOpen}
                variant="page"
                title={t('storyStage.noStory.title')}
                description={t('storyStage.noStory.description')}
                action={onOpenWorldPacks ? { label: t('storyStage.noStory.action'), onClick: onOpenWorldPacks } : undefined}
                className="bg-[var(--nova-surface-2)]"
              />
            ) : confirmedOpeningPending && confirmedOpeningFailed ? (
              <EmptyState
                icon={BookOpen}
                variant="page"
                title={t('storyStage.opening.failedTitle')}
                description={t('storyStage.opening.failedDescription')}
                action={{ label: t('storyStage.opening.retry'), onClick: () => { setConfirmedOpeningFailed(false); confirmedOpeningAttemptRef.current = '' } }}
                className="bg-[var(--nova-surface-2)]"
              />
            ) : (snapshotLoading || waitingToStartOpening || confirmedOpeningPending) && agentMessages.length === 0 && !streaming ? (
              <LoadingState
                label={t('common.loading')}
                layout="conversation"
                className="h-full min-h-0 flex-1 bg-[var(--nova-surface-2)]"
              />
            ) : (
              <MessageList
                projectId={projectId}
                attachmentScope={storyId ? { kind: 'story', id: storyId } : undefined}
                messages={agentMessages}
                onResolveAsk={async (view, action) => {
                  const askID = agentViewAskID(view)
                  const result = await resolveInteractiveAsk(storyId, branchId, askID, action)
                  const previous = agentViewAskInteraction(view)
                  if (previous && result.status !== 'pending') {
                    setStageLiveMessages(current => [...current, createAgentDataMessage({ id: `ask-${askID}`, type: 'agent-ask', data: { ...previous, ...result } })])
                  }
                  interactiveAgentCommands.project(await getActiveInteractiveChat(storyId, branchId))
                  return result
                }}
                isStreaming={streaming}
                activityContent={stageRun.runtime.recoveryPaused ? t('storyStage.activity.recoveryPaused') : activityContent}
                waitingHint={waitingHint}
                waitingHintScreenReaderLabel={t('storyStage.waiting.screenReader')}
                highlightDialogue
                scrollResetKey={scrollResetKey}
                bottomPaddingClassName="pb-36"
                bottomPaddingPx={messageListBottomPadding}
                hasEarlierMessages={historyWindow.stageKey === stageKey && historyWindow.hasMore}
                isLoadingEarlierMessages={historyLoading}
                onLoadEarlierMessages={loadEarlierMessages}
                afterContent={historyWindow.followLatest && !streaming && storyStateModel.hasState && stateDisplayPreference !== 'director-only' ? (
                  <StoryStateLedger
                    snapshot={snapshot}
                    displayPreference={stateDisplayPreference}
                    onDisplayPreferenceChange={onStateDisplayPreferenceChange}
                    onOpenDirectorState={onOpenDirectorState}
                  />
                ) : undefined}
                afterContentKey={snapshot?.current_turn?.id || ''}
                messageStyle={stageTextStyle}
                collapseTraceGroups
                activeTraceDisplay="collapsed"
                tracePresentation="quiet"
                turnScrollRequest={turnScrollRequest}
                proseRevealRequest={proseRevealRequest}
                onVisibleTurnAnchorChange={handleVisibleTurnAnchorChange}
                canMutateMessage={canMutateStoryView}
                onEditMessage={startEditingView}
                onEditAssistantReply={storyImages.generatingTurnId || switchingVersionTurnId ? undefined : startEditingAssistantReply}
                onCreateBranch={onRequestCreateBranch ? startCreatingBranchFromView : undefined}
                onRegenerateMessage={regenerateView}
                onSwitchMessageVersion={switchViewVersion}
                onReadAloud={speech.configured ? (view) => {
                  const turn = turnsById.get(view.metadata.turn_id || '')
                  if (turn) speech.read(turn)
                } : undefined}
                onGenerateInteractiveImage={imageConfigured ? generateImageForView : undefined}
                generatingInteractiveImageTurnId={storyImages.generatingTurnId || undefined}
                onOpenSubAgentSession={openSubAgentSession}
                activeRunId={stageRun.runtime.operationId}
                activeSubAgentSessionKey={activeSubAgentSessionKey}
              />
            )}
            {activeSubAgentSessionKey && (
              <div className="absolute inset-y-0 right-0 z-30 w-[min(420px,92vw)] border-l border-[var(--nova-border)] shadow-[var(--nova-shadow)]">
                <AgentSubAgentSessionPanel
                  projectId={projectId}
                  messages={agentMessages}
                  sessionKey={activeSubAgentSessionKey}
                  onClose={() => setActiveSubAgentSessionKey('')}
                  highlightDialogue
                  messageStyle={stageTextStyle}
                />
              </div>
            )}
            {chatWindowOpen && storyId && !isMobile ? (
              <StoryWindow label={t('chatWindow.title')} icon={<MessagesSquare className="h-3.5 w-3.5" />} minimizedOrder={0} cascadeStep={0} dockedWidth={640} windowKey="chat">
                {({ windowActions }) => (
                  <ChatWindowPanel
                    storyId={storyId}
                    branchId={branchId || snapshot?.branch_id || ''}
                    projectId={projectId}
                    active={active && chatWindowOpen}
                    target={chatTarget}
                    onTargetChange={setChatTarget}
                    onActOnPlan={actFromChatWindow}
                    onContinueStory={actFromChatWindow}
                    onOpenProfile={openProfileFromChat}
                    onClose={() => setChatWindowOpen(false)}
                    windowActions={windowActions}
                    drafts={chatDrafts}
                    onDraftChange={(key, value) => setChatDrafts((current) => ({ ...current, [key]: value }))}
                  />
                )}
              </StoryWindow>
            ) : null}
            {chatWindowOpen && storyId && isMobile ? (
              <Sheet open onOpenChange={(open) => { if (!open) setChatWindowOpen(false) }}>
                <SheetContent side="right" className="w-[92vw] max-w-none gap-0 p-0" showCloseButton={false}>
                  {/* The window draws its own visible title; the sheet still needs
                      one of its own so the overlay is announced as the chat
                      window rather than as an unnamed dialog. */}
                  <SheetTitle className="sr-only">{t('chatWindow.title')}</SheetTitle>
                  <ChatWindowPanel
                    storyId={storyId}
                    branchId={branchId || snapshot?.branch_id || ''}
                    projectId={projectId}
                    active={active && chatWindowOpen}
                    target={chatTarget}
                    onTargetChange={setChatTarget}
                    onActOnPlan={actFromChatWindow}
                    onContinueStory={actFromChatWindow}
                    onOpenProfile={openProfileFromChat}
                    onClose={() => setChatWindowOpen(false)}
                    drafts={chatDrafts}
                    onDraftChange={(key, value) => setChatDrafts((current) => ({ ...current, [key]: value }))}
                  />
                </SheetContent>
              </Sheet>
            ) : null}
            {combatWindowOpen && independentCombatAvailable && storyId && !isMobile ? (
              <StoryWindow label={worldPresentation.terms.combat} icon={<Swords className="h-3.5 w-3.5" />} minimizedOrder={chatWindowOpen ? 1 : 0} cascadeStep={chatWindowOpen ? 1 : 0} dockedWidth={560} windowKey="combat">
                {({ windowActions }) => (
                  <BattlePanel
                    storyId={storyId}
                    branchId={branchId || snapshot?.branch_id || ''}
                    active={active && combatWindowOpen}
                    sessionId={combatWindowSessionID}
                    onSelectSession={setCombatWindowSessionID}
                    onContinueStory={actFromCombatWindow}
                    onClose={() => setCombatWindowOpen(false)}
                    windowActions={windowActions}
                  />
                )}
              </StoryWindow>
            ) : null}
            {combatWindowOpen && independentCombatAvailable && storyId && isMobile ? (
              <Sheet open onOpenChange={(open) => { if (!open) setCombatWindowOpen(false) }}>
                <SheetContent side="right" className="w-[92vw] max-w-none gap-0 p-0" showCloseButton={false}>
                  {/* The panel draws its own visible header; the sheet still needs a
                      title of its own so the overlay is announced as the battle
                      window rather than as an unnamed dialog. */}
                  <SheetTitle className="sr-only">{worldPresentation.terms.combat}</SheetTitle>
                  <BattlePanel
                    storyId={storyId}
                    branchId={branchId || snapshot?.branch_id || ''}
                    active={active && combatWindowOpen}
                    sessionId={combatWindowSessionID}
                    onSelectSession={setCombatWindowSessionID}
                    onContinueStory={actFromCombatWindow}
                    onClose={() => setCombatWindowOpen(false)}
                  />
                </SheetContent>
              </Sheet>
            ) : null}
            {intimacyWindowOpen && dualCultivationAvailable && storyId && !isMobile ? (
              <StoryWindow label={t('intimacyWindow.title')} icon={<HeartHandshake className="h-3.5 w-3.5" />} minimizedOrder={combatWindowOpen ? 2 : chatWindowOpen ? 1 : 0} cascadeStep={combatWindowOpen ? 2 : chatWindowOpen ? 1 : 0} dockedWidth={560} windowKey="intimacy">
                {({ windowActions }) => (
                  <IntimacyWindowPanel
                    storyId={storyId}
                    branchId={branchId || snapshot?.branch_id || ''}
                    projectId={projectId}
                    active={active && intimacyWindowOpen}
                    focusCharacterId={intimacyFocusID}
                    onContinueStory={actFromIntimacyWindow}
                    onClose={() => setIntimacyWindowOpen(false)}
                    windowActions={windowActions}
                  />
                )}
              </StoryWindow>
            ) : null}
            {intimacyWindowOpen && dualCultivationAvailable && storyId && isMobile ? (
              <Sheet open onOpenChange={(open) => { if (!open) setIntimacyWindowOpen(false) }}>
                <SheetContent side="right" className="w-[92vw] max-w-none gap-0 p-0" showCloseButton={false}>
                  {/* The panel draws its own visible header; the sheet still needs
                      a title of its own so the overlay is announced as 双修
                      rather than as an unnamed dialog. */}
                  <SheetTitle className="sr-only">{t('intimacyWindow.title')}</SheetTitle>
                  <IntimacyWindowPanel
                    storyId={storyId}
                    branchId={branchId || snapshot?.branch_id || ''}
                    projectId={projectId}
                    active={active && intimacyWindowOpen}
                    focusCharacterId={intimacyFocusID}
                    onContinueStory={actFromIntimacyWindow}
                    onClose={() => setIntimacyWindowOpen(false)}
                  />
                </SheetContent>
              </Sheet>
            ) : null}
            {questWindowOpen && storyId ? (
              <StoryReferenceDialog title={t('questWindow.title')} onClose={() => setQuestWindowOpen(false)}>
                <QuestWindowPanel
                  storyId={storyId}
                  branchId={branchId || snapshot?.branch_id || ''}
                  active={active && questWindowOpen}
                  onFillIntent={(intent) => { fillQuestIntent(intent); setQuestWindowOpen(false) }}
                  onClose={() => setQuestWindowOpen(false)}
                />
              </StoryReferenceDialog>
            ) : null}
            {realmWindowOpen && storyId ? (
              <StoryReferenceDialog title={worldPresentation.terms.exploration} onClose={() => setRealmWindowOpen(false)}>
                <RealmWindowPanel
                  storyId={storyId}
                  branchId={branchId || snapshot?.branch_id || ''}
                  active={active && realmWindowOpen}
                  onDraftAction={actFromRealmWindow}
                  draftingDisabled={streaming || commandSubmitting}
                  onClose={() => setRealmWindowOpen(false)}
                />
              </StoryReferenceDialog>
            ) : null}
            {loreOverviewOpen && storyId ? (
              <StoryReferenceDialog title={t('loreOverview.title')} onClose={closeLoreOverview}>
                <LoreOverviewPanel
                  storyId={storyId}
                  branchId={branchId || snapshot?.branch_id || ''}
                  projectId={projectId}
                  active={active && loreOverviewOpen}
                  snapshot={snapshot}
                  origin={story?.origin}
                  worldName={story?.pack_binding?.pack_name}
                  resizable={false}
                  focusEntryId={loreOverviewFocusID || undefined}
                  onClose={closeLoreOverview}
                />
              </StoryReferenceDialog>
            ) : null}
          </section>
        </div>
        {!chatWindowOpen && storyId ? (
          <IncomingMessageHint
            messages={storyMessages.pending}
            onOpen={(characterID) => {
              setChatTarget({ kind: 'person', id: characterID })
              setChatWindowOpen(true)
            }}
          />
        ) : null}
      </div>
      <StoryStageComposer
        taskControls={<AgentTaskControls suspended={stageRun.runtime.phase === 'suspended'} pending={commandSubmitting || stageRun.runtime.abortPending} onResume={() => void resumeTask()} onAbort={() => void stop()} />}
        layout={{ projectId, creatingStory: storySetupVisible || (waitingToStartOpening && (!isMobile || !streaming)), isMobile, inputTextStyle, workspace, inputFloatRef, inputRef, t, attachmentDraftKey: stageKey }}
        editor={{ input, editingTurn, styleScenes, styleSceneQuery, styleSceneSuggestions, showSkillCommands, activeSkillCommandIndex, skillCommands, filteredSkillCommands, filteredBuiltInCommandItems, filteredSkillCommandItems, setStyleSceneQuery, setShowSkillCommands, setSkillCommandQuery, setActiveSkillCommandIndex }}
        story={{ storyId, branchTerminal, branchEnding, hotChoices, hotChoicesExpanded, showHotChoices, canUseHotChoices, setHotChoicesExpanded, assistantOpen, assistantExpanded, toggleAssistant, closeAssistant, assistant, reloadPlan: reloadAssistantPlan }}
        staged={{ actions: stagedActions, onEdit: editStagedAction, onRemove: removeStagedAction }}
        runtime={{ streaming, approvalReady, conversationConfig, abortPending: stageRun.runtime.abortPending, recoveryPaused: stageRun.runtime.recoveryPaused, recoveryAbortAvailable: stageRun.runtime.recoveryAbortAvailable, pendingInterruptionId: stageRun.runtime.pendingInterruptionId, operationId: stageRun.runtime.operationId, connection: stageRun.runtime.connection, commandSubmitting, queue: stageRun.runtime.queue, queueActionPendingCommandID }}
        dialogs={{ replyEditTarget, closeReplyEditor: () => setReplyEditTarget(null), saveReply: saveEditedReply }}
        actions={{ cancelEditing, selectHotChoice, selectStyleScene, selectSkillCommand, handleInputChange, handleInputTriggerChange, handleTokenRemove, toggleHotChoices, send: sendFromComposer, steerQueuedCommand, deleteQueuedCommand, stop: stageRun.runtime.recoveryPaused || stageRun.runtime.connection !== 'connected' || !supportsRuntimeOperation(conversationConfig.snapshot, 'pause') ? stop : suspend }}
      />
    </main>
    </WorldPresentationProvider>
  )

}
function StoryReferenceDialog({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose() }}>
      <DialogContent showCloseButton={false} aria-describedby={undefined} className="grid h-[min(84dvh,760px)] w-[min(760px,calc(100vw-1rem))] max-w-none grid-rows-[minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-none">
        <DialogTitle className="sr-only">{title}</DialogTitle>
        {children}
      </DialogContent>
    </Dialog>
  )
}

function noop() {}
function noopStateDisplayPreferenceChange(_value: StoryStateDisplayPreference) {}

function noopTurnPersisted() {
  return undefined
}

function errorMessage(content: string) {
  return createAgentDataMessage({ type: 'agent-error', data: { role: 'error', content } })
}
