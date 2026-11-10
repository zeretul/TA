import { useWorldPresentation } from '../../world-presentation-context'
import { BookOpen, DoorOpen, HeartHandshake, ListChecks, MessagesSquare, PanelRight, SlidersHorizontal, Swords } from 'lucide-react'
import type { RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { StoryPicker, type StoryPickerProps } from '../StoryPicker'
import { TurnNavigator, type TurnNavigatorProps } from '../TurnNavigator'

interface StoryStageControlsProps {
  isMobile: boolean
  picker: StoryPickerProps
  history: TurnNavigatorProps
  directorPanelVisible?: boolean
  onToggleDirectorPanel?: () => void
  chatWindowVisible?: boolean
  onToggleChatWindow?: () => void
  /** Incoming messages the player has not read; shown as a quiet dot, not a badge count. */
  chatWindowUnread?: number
  intimacyWindowVisible?: boolean
  onToggleIntimacyWindow?: () => void
  combatWindowVisible?: boolean
  onToggleCombatWindow?: () => void
  realmWindowVisible?: boolean
  onToggleRealmWindow?: () => void
  questWindowVisible?: boolean
  onToggleQuestWindow?: () => void
  loreOverviewVisible?: boolean
  onToggleLoreOverview?: () => void
  /** Lets the stage put focus back on the button after the panel closes (§2). */
  loreOverviewTriggerRef?: RefObject<HTMLButtonElement | null>
}

export function StoryStageControls({ isMobile, picker, history, directorPanelVisible, onToggleDirectorPanel, chatWindowVisible, onToggleChatWindow, chatWindowUnread = 0, intimacyWindowVisible, onToggleIntimacyWindow, combatWindowVisible, onToggleCombatWindow, realmWindowVisible, onToggleRealmWindow, questWindowVisible, onToggleQuestWindow, loreOverviewVisible, onToggleLoreOverview, loreOverviewTriggerRef }: StoryStageControlsProps) {
  const { t } = useTranslation()
  const { terms } = useWorldPresentation()
  return (
    <>
      {isMobile ? (
        <TurnNavigator {...history} renderTrigger={(openHistory) => <StoryPicker {...picker} onOpenHistory={history.items.length ? openHistory : undefined} />} />
      ) : <StoryPicker {...picker} />}
      {/* One entry for every conversation. Private chat and groups were two
          buttons opening two windows for the same act, which made the toolbar
          describe our records instead of what the player is doing. */}
      {onToggleChatWindow && (
        <Button type="button" variant={isMobile ? 'ghost' : 'outline'} size={isMobile ? 'icon' : 'sm'} className={cn('relative', !isMobile && 'h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)]', !isMobile && (chatWindowVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'))} onClick={onToggleChatWindow} aria-label={chatWindowVisible ? t('chatWindow.close') : t('chatWindow.open')} title={t('chatWindow.title')}>
          {isMobile ? <MessagesSquare /> : <MessagesSquare className="h-3.5 w-3.5" />}
          <span className="max-lg:sr-only">{t('chatWindow.title')}</span>
          {chatWindowUnread > 0 && !chatWindowVisible ? (
            <span className="absolute right-0.5 top-0.5 h-1.5 w-1.5 rounded-full bg-[var(--nova-accent)]" aria-hidden="true" />
          ) : null}
        </Button>
      )}
      {/* Dual cultivation is a separate activity with its own records, so it
          keeps its own entry (T27). The caller only passes the handler when the
          world actually has that activity, so a detective story never grows a
          双修 button just because the toolbar has a slot for it. */}
      {onToggleIntimacyWindow && (
        <Button type="button" variant={isMobile ? 'ghost' : 'outline'} size={isMobile ? 'icon' : 'sm'} className={cn(!isMobile && 'h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)]', !isMobile && (intimacyWindowVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'))} onClick={onToggleIntimacyWindow} aria-label={intimacyWindowVisible ? t('intimacyWindow.close') : t('intimacyWindow.open')} title={t('intimacyWindow.title')}>
          {isMobile ? <HeartHandshake /> : <HeartHandshake className="h-3.5 w-3.5" />}
          <span className="max-lg:sr-only">{t('intimacyWindow.title')}</span>
        </Button>
      )}
      {onToggleCombatWindow && (
        <Button type="button" variant={isMobile ? 'ghost' : 'outline'} size={isMobile ? 'icon' : 'sm'} className={cn(!isMobile && 'h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)]', !isMobile && (combatWindowVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'))} onClick={onToggleCombatWindow} aria-label={combatWindowVisible ? t('worldPresentation.closeCombat', terms) : t('worldPresentation.openCombat', terms)} title={terms.combat}>
          {isMobile ? <Swords /> : <Swords className="h-3.5 w-3.5" />}
          <span className="max-lg:sr-only">{terms.combat}</span>
        </Button>
      )}
      {onToggleRealmWindow && (
        <Button type="button" variant={isMobile ? 'ghost' : 'outline'} size={isMobile ? 'icon' : 'sm'} className={cn(!isMobile && 'h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)]', !isMobile && (realmWindowVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'))} onClick={onToggleRealmWindow} aria-label={realmWindowVisible ? t('realmWindow.close', terms) : t('realmWindow.open', terms)} title={terms.exploration}>
          {isMobile ? <DoorOpen /> : <DoorOpen className="h-3.5 w-3.5" />}
          <span className="max-lg:sr-only">{terms.exploration}</span>
        </Button>
      )}
      {onToggleQuestWindow && (
        <Button type="button" variant={isMobile ? 'ghost' : 'outline'} size={isMobile ? 'icon' : 'sm'} className={cn(!isMobile && 'h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)]', !isMobile && (questWindowVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'))} onClick={onToggleQuestWindow} aria-label={questWindowVisible ? t('questWindow.close') : t('questWindow.open')} title={t('questWindow.title')}>
          {isMobile ? <ListChecks /> : <ListChecks className="h-3.5 w-3.5" />}
          <span className="max-lg:sr-only">{t('questWindow.title')}</span>
        </Button>
      )}
      {onToggleLoreOverview && (
        <Button type="button" ref={loreOverviewTriggerRef} data-lore-overview-trigger variant={isMobile ? 'ghost' : 'outline'} size={isMobile ? 'icon' : 'sm'} className={cn(!isMobile && 'h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)]', !isMobile && (loreOverviewVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'))} onClick={onToggleLoreOverview} aria-label={loreOverviewVisible ? t('loreOverview.close') : t('loreOverview.open')} aria-expanded={loreOverviewVisible} title={t('loreOverview.title')}>
          {isMobile ? <BookOpen /> : <BookOpen className="h-3.5 w-3.5" />}
          <span className="max-lg:sr-only">{t('loreOverview.title')}</span>
        </Button>
      )}
      {onToggleDirectorPanel && (
        <Button type="button" variant={isMobile ? 'ghost' : 'outline'} size={isMobile ? 'icon' : 'sm'} className={cn(!isMobile && 'h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)]', !isMobile && (directorPanelVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'))} onClick={onToggleDirectorPanel} aria-label={directorPanelVisible ? t('storyStage.hideDirectorPanel') : t('storyStage.showDirectorPanel')} title={t('storyStage.directorPanel')}>
          {isMobile ? <SlidersHorizontal /> : <PanelRight className="h-3.5 w-3.5" />}
          <span className="max-lg:sr-only">{t('storyStage.directorPanel')}</span>
        </Button>
      )}
    </>
  )
}
