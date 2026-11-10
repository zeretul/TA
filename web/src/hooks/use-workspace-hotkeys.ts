import { useHotkeys } from 'react-hotkeys-hook'
import { isEditableTarget, isNativeTextEditingShortcut } from '@/lib/keyboard'

interface WorkspaceHotkeysOptions {
  onOpenCommand?: () => void
  onToggleRightPanel?: () => void
}

/** 注册工作台全局快捷键，hook 只分发事件，不直接调用业务 API。 */
export function useWorkspaceHotkeys({
  onOpenCommand,
  onToggleRightPanel,
}: WorkspaceHotkeysOptions) {
  const shouldSkipWorkspaceAction = (event: KeyboardEvent) => {
    return isEditableTarget(event.target) || isNativeTextEditingShortcut(event)
  }

  useHotkeys('meta+k, ctrl+k', (event) => {
    if (shouldSkipWorkspaceAction(event)) return
    event.preventDefault()
    onOpenCommand?.()
  }, { enableOnFormTags: true, enableOnContentEditable: true }, [onOpenCommand])

  useHotkeys('meta+alt+b, ctrl+alt+b', (event) => {
    event.preventDefault()
    onToggleRightPanel?.()
  }, { enableOnFormTags: true, enableOnContentEditable: true }, [onToggleRightPanel])
}
