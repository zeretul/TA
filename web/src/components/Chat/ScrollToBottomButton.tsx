import { useTranslation } from 'react-i18next'
import { ArrowDown, ChevronDown } from 'lucide-react'

interface ScrollToBottomButtonProps {
  visible: boolean
  onClick: () => void
  bottomOffsetPx?: number
  rightOffsetPx?: number
  /** `new-turn` asks to be taken to the start of the newest prose instead of the end. */
  mode?: 'bottom' | 'new-turn'
}

export function ScrollToBottomButton({ visible, onClick, bottomOffsetPx = 24, rightOffsetPx = 24, mode = 'bottom' }: ScrollToBottomButtonProps) {
  const { t } = useTranslation()
  if (!visible) return null
  const newTurn = mode === 'new-turn'
  const label = newTurn ? t('chat.action.readNewTurn') : t('chat.action.scrollToBottom')
  return (
    <button
      type="button"
      data-nova-scroll-to-bottom
      data-nova-read-new-turn={newTurn ? '' : undefined}
      onPointerDown={(event) => {
        if (event.button !== 0) return
        event.preventDefault()
        onClick()
      }}
      onClick={(event) => {
        if (event.detail === 0) onClick()
      }}
      aria-label={label}
      className={`absolute z-30 flex items-center justify-center gap-1 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] shadow-[var(--nova-shadow)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] ${newTurn ? 'h-8 px-3 text-[11px]' : 'h-8 w-8'}`}
      style={{ bottom: bottomOffsetPx, right: rightOffsetPx }}
    >
      {newTurn ? <ArrowDown className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
      {newTurn ? <span className="whitespace-nowrap">{label}</span> : null}
    </button>
  )
}
