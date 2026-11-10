import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { requestJSON } from '@/lib/api-client'
import type { Snapshot } from '../../types'

/** Names are presentation metadata from the exact bound catalog. Values and
 * differences always remain the supplied snapshot, including historical ones. */
export function useAdventureDisplaySnapshot(snapshot: Snapshot | null): Snapshot | null {
  const { i18n, t } = useTranslation()
  const rules = snapshot?.actor_state_schema?.system.adventure_rules
  const key = rules && i18n.language.startsWith('en') ? `${snapshot?.story_id}:${snapshot?.branch_id}:${i18n.language}` : ''
  const [names, setNames] = useState<{ key: string; values: Record<string, string> }>({ key: '', values: {} })
  useEffect(() => {
    if (!key || !snapshot) return
    let cancelled = false
    void requestJSON<{ skills: { id: string; name: string }[]; items: { id: string; name: string }[] }>(`/api/interactive/stories/${encodeURIComponent(snapshot.story_id)}/adventure-window?branch=${encodeURIComponent(snapshot.branch_id)}`)
      .then(result => { if (!cancelled) setNames({ key, values: Object.fromEntries([...(result.skills || []), ...(result.items || [])].map(item => [item.id, item.name])) }) })
      .catch(error => console.error('[adventure-state] Catalog names unavailable', error))
    return () => { cancelled = true }
  }, [key, snapshot?.story_id, snapshot?.branch_id])
  return useMemo(() => {
    if (!snapshot?.actor_state_schema || !rules || !key) return snapshot
    const values = names.key === key ? names.values : {}
    return { ...snapshot, actor_state_schema: { ...snapshot.actor_state_schema, system: { ...snapshot.actor_state_schema.system, adventure_rules: {
      ...rules,
      progression: rules.progression ? { ...rules.progression, stages: rules.progression.stages.map(stage => ({ ...stage, name: stage.english_name })) } : undefined,
      skills: (rules.skills || []).map(item => ({ ...item, name: values[item.id] || t('storyStage.adventure.unknownSkill') })),
      items: (rules.items || []).map(item => ({ ...item, name: values[item.id] || t('storyStage.adventure.unknownItem') })),
    } } } }
  }, [snapshot, rules, key, names, t])
}
