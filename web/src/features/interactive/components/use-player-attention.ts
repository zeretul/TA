import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getPlayerAttention, updatePlayerAttention, type AttentionKind, type PlayerAttention } from '../player-attention-api'

// Each request captures its story and branch. A late response can never replace
// a new target's state, and failed optimistic changes restore the last record.
export function usePlayerAttention({ storyId, branchId, active }: { storyId: string; branchId: string; active: boolean }) {
  const { t } = useTranslation()
  const key = JSON.stringify([storyId, branchId, active])
  const target = useRef(key)
  target.current = key
  const generation = useRef(0)
  const pending = useRef(false)
  const saved = useRef<PlayerAttention | null>(null)
  const [data, setData] = useState<PlayerAttention | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const reload = useCallback(async () => {
    if (!active || !storyId || !branchId || pending.current) return
    const seq = ++generation.current
    setBusy(true)
    setError('')
    try {
      const next = await getPlayerAttention(storyId, branchId)
      if (target.current !== key || seq !== generation.current) return
      saved.current = next
      setData(next)
    } catch {
      if (target.current === key && seq === generation.current) setError(t('api.attention.unavailable'))
    } finally {
      if (target.current === key && seq === generation.current) setBusy(false)
    }
  }, [storyId, branchId, active, key, t])

  useEffect(() => {
    saved.current = null
    pending.current = false
    setData(null)
    setError('')
    void reload()
    return () => { generation.current++ }
  }, [reload])

  const setFollowing = useCallback(async (kind: AttentionKind, id: string, following: boolean): Promise<boolean> => {
    const before = saved.current
    if (!active || !before || pending.current || target.current !== key) return false
    pending.current = true
    const seq = ++generation.current
    const current = () => target.current === key && seq === generation.current
    const field = kind === 'character' ? 'characters' : 'quests'
    setBusy(true)
    setError('')
    setData({ ...before, [field]: following ? [...new Set([...before[field], id])] : before[field].filter((value) => value !== id) })
    try {
      const next = await updatePlayerAttention(storyId, { branch: branchId, expected_head: before.head, kind, id, following })
      if (!current()) return false
      saved.current = next
      setData(next)
      return true
    } catch (cause) {
      if (!current()) return false
      setData(before)
      console.warn('Player attention update failed', { storyId, branchId, kind, id, cause })
      setError(cause instanceof Error ? cause.message : t('api.attention.unavailable'))
      // Recover the latest head after a conflict or a response lost after commit.
      try {
        const latest = await getPlayerAttention(storyId, branchId)
        if (current()) { saved.current = latest; setData(latest) }
      } catch { /* Keep the last confirmed state and expose the retry action. */ }
      return false
    } finally {
      if (current()) { pending.current = false; setBusy(false) }
    }
  }, [storyId, branchId, active, key, t])

  return { data, busy, error, reload, setFollowing }
}
