import { Fragment, useLayoutEffect, type CSSProperties, type ReactNode, type RefCallback } from 'react'
import { motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import type { AgentAskAnswer, AgentAskResolution, ChatMessage } from '@/lib/api'
import { agentViewContent, agentViewNavigationAnchor, isAgentTraceView, type AgentExecutionTiming, type AgentMessageView } from '@/lib/agent-message-view'
import { listItem, novaEase, timelineAttachment } from '@/features/motion/motion-tokens'
import { cn } from '@/lib/utils'
import { AgentActivityShimmer, MessageItem, type AssistantMessagePresentation } from './MessageItem'
import { AgentMessageItem } from './AgentMessageItem'
import { AgentExecutionProcess } from './AgentExecutionProcess'
import type { AgentRunPresentationSection } from './agent-run-presentation'
import { AgentRunActions } from './AgentRunActions'

export type AgentChatListItem =
  | { kind: 'empty'; key: string }
  | { kind: 'typing'; key: string }
  | { kind: 'activity'; key: string; content: string; screenReaderLabel?: string; runId?: string }
  | { kind: 'clear'; key: string; createdAt?: string }
  | { kind: 'message'; key: string; view: AgentMessageView; sourceIndex: number }
  | { kind: 'legacy-message'; key: string; message: ChatMessage; sourceIndex: number; openView?: AgentMessageView }
  | { kind: 'trace'; key: string; views: AgentMessageView[]; activeStreamingTrace: boolean }
  | { kind: 'run'; key: string; runId: string; sections: AgentRunPresentationSection[]; sourceIndex: number }
  | { kind: 'attachment'; key: string; runId: string; content: ReactNode }

export function AgentChatListRow({ projectId, item, nextItem, executionTimings, isStreaming, tailFollowActive, activeTraceDisplay, tracePresentation = 'full', subAgentPresentation, highlightDialogue, messageStyle, contentClassName, canMutateMessage, onEditMessage, onEditAssistantReply, onCreateBranch, onRegenerateMessage, onSwitchMessageVersion, onOpenSubAgentSession, onReadAloud, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey, onResolveAsk, onInteractiveCardLayoutChange, streamingRowRef, syncStreamingTailLayout }: {
  projectId?: string
  item: AgentChatListItem
  executionTimings: ReadonlyMap<string, AgentExecutionTiming>
  nextItem?: AgentChatListItem
  isStreaming: boolean
  tailFollowActive: boolean
  activeTraceDisplay: 'expanded' | 'collapsed'
  tracePresentation?: 'full' | 'quiet'
  subAgentPresentation: 'card' | 'content'
  highlightDialogue: boolean
  messageStyle?: CSSProperties
  contentClassName?: string
  canMutateMessage?: (view: AgentMessageView) => boolean
  onEditMessage?: (view: AgentMessageView) => void
  onEditAssistantReply?: (view: AgentMessageView) => void
  onCreateBranch?: (view: AgentMessageView) => void
  onRegenerateMessage?: (view: AgentMessageView) => void
  onSwitchMessageVersion?: (view: AgentMessageView, direction: -1 | 1) => void
  onOpenSubAgentSession?: (view: AgentMessageView) => void
  onReadAloud?: (message: AgentMessageView) => void
  onGenerateInteractiveImage?: (view: AgentMessageView) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
  onResolveAsk?: (view: AgentMessageView, action: { status: 'answered'; answers: AgentAskAnswer[] } | { status: 'cancelled' }) => Promise<AgentAskResolution>
  onInteractiveCardLayoutChange?: (element?: HTMLElement) => void
  streamingRowRef?: RefCallback<HTMLDivElement>
  syncStreamingTailLayout?: () => void
}) {
  const { t } = useTranslation()
  const isLast = !nextItem
  // Consecutive trace rows share the spacing used inside AgentExecutionProcess.
  const continuesTrace = item.kind === 'message' && isAgentTraceView(item.view)
    && nextItem?.kind === 'message' && isAgentTraceView(nextItem.view)
  const turnAnchor = chatListItemNavigationAnchor(item)
  const renderMessageView = (view: AgentMessageView, key?: string, assistantPresentation: AssistantMessagePresentation = 'message') => {
    const mutationsAllowed = canMutateMessage?.(view) !== false
    const rendered = (
      <AgentMessageItem
        projectId={projectId}
        view={view}
        assistantPresentation={assistantPresentation}
        highlightDialogue={highlightDialogue}
        messageStyle={messageStyle}
        onEditMessage={isStreaming || !mutationsAllowed ? undefined : onEditMessage}
        onEditAssistantReply={isStreaming || !mutationsAllowed ? undefined : onEditAssistantReply}
        onCreateBranch={isStreaming ? undefined : onCreateBranch}
        onRegenerateMessage={isStreaming || !mutationsAllowed ? undefined : onRegenerateMessage}
        onSwitchMessageVersion={isStreaming || !mutationsAllowed ? undefined : onSwitchMessageVersion}
        onOpenSubAgentSession={onOpenSubAgentSession}
        onReadAloud={onReadAloud}
        onGenerateInteractiveImage={isStreaming ? undefined : onGenerateInteractiveImage}
        generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
        activeSubAgentSessionKey={activeSubAgentSessionKey}
        subAgentPresentation={subAgentPresentation}
        onResolveAsk={onResolveAsk}
        onInteractiveCardLayoutChange={onInteractiveCardLayoutChange}
      />
    )
    if (!isProseAnchorView(view)) return <Fragment key={key}>{rendered}</Fragment>
    // Only the narrative carries the reading anchor. Marking it here rather than
    // inside MessageItem keeps every other chat surface unchanged.
    return (
      <div key={key} data-nova-chat-prose-anchor className="scroll-mt-3">
        {rendered}
      </div>
    )
  }
  const renderExecutionProcess = (key: string, views: AgentMessageView[], active: boolean, timing?: AgentExecutionTiming) => views.length > 0 ? (
    <AgentExecutionProcess
      projectId={projectId}
      key={key}
      views={views}
      active={active}
      activeSubAgentSessionKey={activeSubAgentSessionKey}
      activeTraceDisplay={activeTraceDisplay}
      presentation={tracePresentation}
      highlightDialogue={highlightDialogue}
      messageStyle={messageStyle}
      onGenerateInteractiveImage={onGenerateInteractiveImage}
      onOpenSubAgentSession={onOpenSubAgentSession}
      onInteractiveCardLayoutChange={onInteractiveCardLayoutChange}
      onResolveAsk={onResolveAsk}
      timing={timing}
    />
  ) : null
  const timedProcessKey = item.kind === 'run'
    ? (item.sections.find(section => section.kind === 'process' && section.active)?.key ||
      item.sections.find(section => section.kind === 'process')?.key)
    : undefined
  // Terminal replies own their actions. Progress/tool-only runs expose their
  // reference after output stops, following the same timing as reply actions.
  const needsRunActions = item.kind === 'run'
    && !isStreaming
    && !item.sections.some(section => section.kind === 'process' && section.active)
    && !item.sections.some(section => section.kind === 'message' && section.view.kind === 'assistant')
    && !(nextItem?.kind === 'message' && nextItem.view.kind === 'error' && nextItem.view.metadata.run_id === item.runId)
  useLayoutEffect(() => {
    if (isLast && tailFollowActive) syncStreamingTailLayout?.()
  }, [isLast, item, syncStreamingTailLayout, tailFollowActive])

  // A translated active tail would visibly move after its bottom anchor is captured.
  return (
    <motion.div
      ref={isLast ? streamingRowRef : undefined}
      data-nova-chat-item={item.kind}
      data-nova-chat-tail-row
      data-nova-chat-row-key={item.key}
      data-nova-chat-turn-anchor={turnAnchor}
      data-nova-chat-prose-turn={chatListItemProseAnchor(item) || undefined}
      className={cn('min-w-0 px-6', contentClassName, isLast ? 'pb-0' : continuesTrace ? 'pb-2' : 'pb-4')}
      variants={item.kind === 'attachment' ? timelineAttachment : listItem}
      initial={isLast && isStreaming ? false : 'initial'}
      animate="animate"
      transition={{ duration: item.kind === 'attachment' ? 0.14 : 0.18, ease: novaEase }}
    >
      {item.kind === 'empty' ? (
        <div className="flex min-h-[240px] items-center justify-center">
          <div className="rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3 text-center text-sm text-[var(--nova-text-muted)] shadow-[0_14px_34px_rgba(0,0,0,0.22)]">
            {t('chat.empty')}
          </div>
        </div>
      ) : item.kind === 'typing' ? (
        <div className="flex justify-start">
          <div className="px-1 py-2">
            <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-[var(--nova-text-muted)]" />
          </div>
        </div>
      ) : item.kind === 'activity' ? (
        <AgentActivityShimmer content={item.content} screenReaderLabel={item.screenReaderLabel} />
      ) : item.kind === 'clear' ? (
        <ContextClearDivider createdAt={item.createdAt} />
      ) : item.kind === 'trace' ? (
        <AgentExecutionProcess
          projectId={projectId}
          views={item.views}
          active={item.activeStreamingTrace}
          activeSubAgentSessionKey={activeSubAgentSessionKey}
          activeTraceDisplay={activeTraceDisplay}
          presentation={tracePresentation}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
            onGenerateInteractiveImage={onGenerateInteractiveImage}
          onOpenSubAgentSession={onOpenSubAgentSession}
          onInteractiveCardLayoutChange={onInteractiveCardLayoutChange}
          onResolveAsk={onResolveAsk}
          timing={executionTimings.get(chatListItemRunID(item))}
        />
      ) : item.kind === 'run' ? (
        <div className="space-y-2">
          {item.sections.map(section => section.kind === 'process'
            ? renderExecutionProcess(
                section.key,
                section.views,
                section.active,
                section.key === timedProcessKey ? executionTimings.get(item.runId) : undefined,
              )
            : renderMessageView(section.view, section.key))}
          {needsRunActions ? (
            <div className="flex flex-wrap items-center gap-2 px-1">
              <AgentRunActions projectId={projectId} runID={item.runId} />
            </div>
          ) : null}
        </div>
      ) : item.kind === 'attachment' ? (
        item.content
      ) : item.kind === 'legacy-message' ? (
        <MessageItem
          projectId={projectId}
          message={item.message}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          onOpenSubAgentSession={item.openView && onOpenSubAgentSession ? () => onOpenSubAgentSession(item.openView as AgentMessageView) : undefined}
          activeSubAgentSessionKey={activeSubAgentSessionKey}
          onResolveAsk={item.openView && onResolveAsk ? (_message, action) => onResolveAsk(item.openView as AgentMessageView, action) : undefined}
        />
      ) : (
        renderMessageView(item.view)
      )}
    </motion.div>
  )
}

export function chatListItemRunID(item: AgentChatListItem): string {
  if (item.kind === 'activity') return item.runId || ''
  if (item.kind === 'message') return item.view.metadata.run_id || ''
  if (item.kind === 'legacy-message') return item.message.run_id || item.openView?.metadata.run_id || ''
  if (item.kind === 'run') return item.runId
  if (item.kind === 'trace') {
    for (let index = item.views.length - 1; index >= 0; index -= 1) {
      const runID = item.views[index]?.metadata.run_id
      if (runID) return runID
    }
  }
  if (item.kind === 'attachment') return item.runId
  return ''
}

/**
 * The settled prose of a turn: what a player who is waiting for the next turn
 * actually came to read. Tool cards, thinking and state cards are not prose.
 */
export function isProseAnchorView(view: AgentMessageView) {
  return view.kind === 'assistant'
    && !view.metadata.subagent
    && view.metadata.display_phase === 'final'
    && agentViewContent(view).trim().length > 0
}

/**
 * The turn whose prose this row renders, or '' when the row is only progress.
 * A collapsed run keeps its prose in a section, so the row that owns it is the
 * run itself.
 */
export function chatListItemProseAnchor(item?: AgentChatListItem) {
  if (!item) return ''
  if (item.kind === 'message') {
    return isProseAnchorView(item.view) ? agentViewNavigationAnchor(item.view) : ''
  }
  if (item.kind === 'legacy-message' && item.openView && isProseAnchorView(item.openView)) {
    return agentViewNavigationAnchor(item.openView)
  }
  if (item.kind === 'run') {
    for (const section of item.sections) {
      if (section.kind === 'message' && isProseAnchorView(section.view)) {
        return agentViewNavigationAnchor(section.view)
      }
    }
  }
  return ''
}

export function chatListItemNavigationAnchor(item?: AgentChatListItem) {
  if (!item) return ''
  if (item.kind === 'message') return agentViewNavigationAnchor(item.view)
  if (item.kind === 'legacy-message') return item.message.navigation_turn_id || item.message.turn_id || ''
  if (item.kind === 'run') {
    for (let sectionIndex = item.sections.length - 1; sectionIndex >= 0; sectionIndex -= 1) {
      const section = item.sections[sectionIndex]
      const views = section.kind === 'process' ? section.views : [section.view]
      for (let viewIndex = views.length - 1; viewIndex >= 0; viewIndex -= 1) {
        const anchor = agentViewNavigationAnchor(views[viewIndex])
        if (anchor) return anchor
      }
    }
  }
  return ''
}

function ContextClearDivider({ createdAt }: { createdAt?: string }) {
  const { t } = useTranslation()
  const timeText = createdAt ? new Date(createdAt).toLocaleString() : ''

  return (
    <div className="flex items-center gap-3 py-1" role="separator" aria-label={t('chat.contextCleared')}>
      <div className="h-px flex-1 bg-[var(--nova-border)]" />
      <div className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1 text-[11px] text-[var(--nova-text-muted)]">
        {t('chat.contextClearedDetail', { time: timeText ? ` · ${timeText}` : '' })}
      </div>
      <div className="h-px flex-1 bg-[var(--nova-border)]" />
    </div>
  )
}
