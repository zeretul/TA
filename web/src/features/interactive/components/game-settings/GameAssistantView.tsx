import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { LoadingState } from '@/components/common/LoadingState'
import { SettingsPageFrame } from '@/features/settings/SettingsPageFrame'
import { useInteractiveStore } from '../../stores/interactive-store'
import { getInteractiveStories } from '../../api'
import type { AssistantHandoff } from '../../assistant-handoff'

/** Owns the single game assistant; entry handoffs keep their original story scope. */
export function GameAssistantView({ projectId, visible, assistantHandoff, onAssistantHandoffConsumed }: {
  projectId: string
  visible: boolean
  assistantHandoff?: AssistantHandoff | null
  onAssistantHandoffConsumed: (key: string) => void
}) {
  const { t } = useTranslation()
  const storyId = useInteractiveStore(state => state.currentStoryId)
  const branchId = useInteractiveStore(state => state.currentBranchId)
  const story = useInteractiveStore(state => state.stories.find(item => item.id === storyId))
  const requestEpoch = useRef(0)
  useEffect(() => {
    requestEpoch.current++
    return () => { requestEpoch.current++ }
  }, [projectId])
  const refreshStories = () => {
    const epoch = requestEpoch.current
    void getInteractiveStories().then(index => {
      if (epoch !== requestEpoch.current) return
      const store = useInteractiveStore.getState()
      store.setStories(index.stories, store.currentStoryId)
    }).catch(error => console.error('[game-assistant] Failed to refresh stories after mutation', error))
  }
  const [handoff, setHandoff] = useState<AssistantHandoff | null>(null)
  useEffect(() => {
    if (!visible) { setHandoff(null); return }
    if (assistantHandoff) {
      setHandoff(assistantHandoff)
      onAssistantHandoffConsumed(assistantHandoff.key)
    }
  }, [visible, assistantHandoff, onAssistantHandoffConsumed])
  return <SettingsPageFrame visible={visible} title={t('gameSettings.assistant.title')}>
    <p className="border-b border-border px-4 py-2 text-xs text-muted-foreground">{t('gameSettings.assistant.hint')}</p>
    <div className="min-h-0 flex-1">
      {handoff || !storyId || branchId ? <ConfigManagerChat
        key={projectId}
        projectId={projectId}
        origin={handoff ? 'lore' : 'game-settings'}
        storyId={handoff ? handoff.storyId : storyId || undefined}
        branchId={handoff ? undefined : branchId || undefined}
        resourceId={handoff?.resourceId || storyId || undefined}
        context={handoff?.context || (story ? { active_story_id: story.id, active_story_title: story.title } : undefined)}
        initialInstruction={handoff?.instruction}
        initialInstructionKey={handoff?.key}
        onMutated={refreshStories}
      /> : <LoadingState label={t('common.loading')} />}
    </div>
  </SettingsPageFrame>
}
