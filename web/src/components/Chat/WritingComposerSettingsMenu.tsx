import { SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { Teller } from '@/features/interactive/types'
import { narrativeStyleName, resolveNarrativeStyle } from '@/features/interactive/narrative-style'
import { PersistedSettingsMenuSub } from './PersistedSettingsMenuSub'

interface WritingComposerSettingsMenuProps {
  enabled: boolean
  tellers: Teller[]
  tellerID: string
  savingTeller?: boolean
  onTellerChange: (value: string) => void | Promise<unknown>
}

/** Non-image Writing options composed from the generic persisted-setting submenu. */
export function WritingComposerSettingsMenu({
  enabled,
  tellers,
  tellerID,
  savingTeller,
  onTellerChange,
}: WritingComposerSettingsMenuProps) {
  const { t } = useTranslation()
  const selectedTeller = resolveNarrativeStyle(tellers, tellerID)
  if (tellers.length === 0) return null

  return (
    <PersistedSettingsMenuSub
      icon={SlidersHorizontal}
      label={t('chat.teller')}
      currentLabel={selectedTeller ? narrativeStyleName(selectedTeller, t) : tellerID}
      value={selectedTeller?.id || tellerID}
      options={tellers.map((item) => ({ id: item.id, label: narrativeStyleName(item, t) }))}
      saving={savingTeller}
      disabled={!enabled}
      onValueChange={onTellerChange}
    />
  )
}
