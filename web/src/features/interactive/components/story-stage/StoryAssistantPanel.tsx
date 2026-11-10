import { useState } from 'react'
import type { TFunction } from 'i18next'
import { ChevronDown, ChevronUp, Lightbulb, Sparkles, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { StoryAssistantSuggestion } from '../../types'
import type { UseStoryAssistantResult } from './use-story-assistant'

interface StoryAssistantPanelProps {
  assistant: UseStoryAssistantResult
  expanded: boolean
  onToggle: () => void
  onClose: () => void
  /** The unsent text in the composer, if any. It is context, never a submission. */
  input: string
  /** Puts wording in the composer without sending it. */
  onUseWording: (wording: string) => void
  /**
   * Re-reads the branch plan after a conflict, so the next adoption quotes the
   * revision the branch actually carries. The advice on screen is kept.
   */
  onReloadPlan?: () => void
  t: TFunction
}

/**
 * The in-story assistant, inline in the composer.
 *
 * 自制-剧情助手与选项分析 places it here rather than in a floating window: only
 * chat and battle get a gameplay window, and the assistant is part of the choice
 * the player is already making. It is collapsed by default and costs nothing
 * until the player asks it something.
 *
 * Nothing in this panel submits a story action. "Use this" only fills the
 * composer, which the player still has to send; "remember as a preference" is a
 * separate, explicit write that records a direction on the branch's plan.
 */
export function StoryAssistantPanel({ assistant, expanded, onToggle, onClose, input, onUseWording, onReloadPlan, t }: StoryAssistantPanelProps) {
  const [adoptingDirection, setAdoptingDirection] = useState<string | null>(null)
  const { answer, asking, error, adopting, adoptError, adoptConflict, adopted, canAdopt, ask, adopt } = assistant
  const suggestions = answer?.suggestions ?? []
  const busy = asking !== null

  const startAdopt = (suggestion: StoryAssistantSuggestion) => {
    const wording = (suggestion.action || suggestion.label).trim()
    if (!wording) return
    setAdoptingDirection(wording)
    void adopt(wording)
  }

  return (
    <div className="mb-2 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]" data-nova-story-assistant>
      <div className="flex min-h-8 items-center gap-1.5 px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
        <button
          type="button"
          className="nova-nav-item flex min-w-0 flex-1 items-center gap-1.5 rounded-[var(--nova-radius)] px-1.5 py-1 text-left hover:bg-[var(--nova-hover)]"
          onMouseDown={(event) => event.preventDefault()}
          onClick={onToggle}
          aria-expanded={expanded}
        >
          <Lightbulb className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="shrink-0 font-medium text-[var(--nova-text-muted)]">{t('storyStage.assistant.title')}</span>
          <span className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">
            {busy ? t('storyStage.assistant.thinking') : t('storyStage.assistant.subtitle')}
          </span>
          {expanded ? <ChevronUp className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />}
        </button>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="h-6 w-6 shrink-0 text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
          onClick={onClose}
          aria-label={t('storyStage.assistant.close')}
        >
          <X className="h-3.5 w-3.5" />
        </Button>
      </div>
      {expanded ? (
        <div className="border-t border-[var(--nova-border)] px-2.5 py-2">
          <div className="flex flex-wrap gap-1.5">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-7 rounded-[var(--nova-radius)] px-2.5 text-[11px]"
              disabled={busy}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => void ask({ action: 'analyze', playerInput: input })}
            >
              {asking === 'analyze' ? t('storyStage.assistant.analyzing') : t('storyStage.assistant.analyze')}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-7 rounded-[var(--nova-radius)] px-2.5 text-[11px]"
              disabled={busy}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => void ask({ action: 'inspire', playerInput: input })}
            >
              {asking === 'inspire' ? t('storyStage.assistant.inspiring') : t('storyStage.assistant.inspire')}
            </Button>
          </div>
          <p className="mt-1.5 text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('storyStage.assistant.readOnlyNote')}</p>

          {/* Everything the answer adds scrolls inside this box, so a long
              reading or a pile of suggestions can never push the composer and
              its main input out of reach. */}
          <div className="mt-2 max-h-[40vh] overflow-y-auto overscroll-contain pr-1" data-nova-story-assistant-scroll>
            {error ? (
              <p className="text-[11px] leading-4 text-[var(--nova-danger)]">{t('storyStage.assistant.failed', { message: error })}</p>
            ) : null}

            {answer ? (
              <div className="space-y-2">
                {answer.stale ? (
                  <p className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-[11px] leading-4 text-[var(--nova-text-muted)]">
                    {t('storyStage.assistant.stale')}
                  </p>
                ) : null}
                <div className="whitespace-pre-wrap break-words text-xs leading-5 text-[var(--nova-text-muted)]">
                  {answer.read}
                </div>
                {suggestions.length > 0 ? (
                  <div className="space-y-1.5">
                    {suggestions.map((suggestion, index) => (
                      <div key={`${index}-${suggestion.action}`} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5" data-nova-story-assistant-suggestion>
                        <div className="text-xs font-medium text-[var(--nova-text)]">{suggestion.label}</div>
                        {suggestion.detail ? (
                          <div className="mt-0.5 text-[11px] leading-4 text-[var(--nova-text-muted)]">{suggestion.detail}</div>
                        ) : null}
                        <div className="mt-1.5 flex flex-wrap gap-1.5">
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="h-6 rounded-[var(--nova-radius)] px-2 text-[11px]"
                            onMouseDown={(event) => event.preventDefault()}
                            onClick={() => onUseWording(suggestion.action || suggestion.label)}
                          >
                            {t('storyStage.assistant.useThis')}
                          </Button>
                          {canAdopt ? (
                            <Button
                              type="button"
                              variant="ghost"
                              size="sm"
                              className="h-6 rounded-[var(--nova-radius)] px-2 text-[11px]"
                              disabled={adopting}
                              title={t('storyStage.assistant.adoptHint')}
                              onMouseDown={(event) => event.preventDefault()}
                              onClick={() => startAdopt(suggestion)}
                            >
                              <Sparkles className="h-3 w-3" />
                              {adopting && adoptingDirection === (suggestion.action || suggestion.label).trim()
                                ? t('storyStage.assistant.adopting')
                                : t('storyStage.assistant.adopt')}
                            </Button>
                          ) : null}
                        </div>
                      </div>
                    ))}
                  </div>
                ) : null}
              </div>
            ) : null}

            {adoptError ? (
              <p className="mt-2 text-[11px] leading-4 text-[var(--nova-danger)]">{t('storyStage.assistant.adoptFailed', { message: adoptError })}</p>
            ) : null}
            {adoptConflict ? (
              <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                <p className="text-[11px] leading-4 text-[var(--nova-text-muted)]">{t('storyStage.assistant.adoptConflict')}</p>
                {onReloadPlan ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-6 rounded-[var(--nova-radius)] px-2 text-[11px]"
                    onMouseDown={(event) => event.preventDefault()}
                    onClick={onReloadPlan}
                  >
                    {t('storyStage.assistant.reloadPlan')}
                  </Button>
                ) : null}
              </div>
            ) : null}
            {adopted && !adoptError ? (
              <p className="mt-2 text-[11px] leading-4 text-[var(--nova-text-muted)]">{t('storyStage.assistant.adopted')}</p>
            ) : null}
            {!canAdopt ? (
              <p className="mt-2 text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('storyStage.assistant.noPlan')}</p>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  )
}
