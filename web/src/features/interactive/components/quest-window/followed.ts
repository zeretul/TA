/**
 * Legacy local attention is retained until the player imports each ID into the
 * canonical Story journal. This file is only a retryable migration source.
 */
const FOLLOWED_KEY_PREFIX = 'ta.questWindow.followed.'

function storageKey(storyId: string, branchId: string) {
  return `${FOLLOWED_KEY_PREFIX}${storyId || 'default'}.${branchId || 'main'}`
}

function read(key: string): string[] {
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((item): item is string => typeof item === 'string')
  } catch {
    return []
  }
}

function write(key: string, taskIds: string[]) {
  try {
    window.localStorage.setItem(key, JSON.stringify(taskIds))
  } catch {
    // Keep the original local record if storage is unavailable; importing the
    // same desired state again is safe because the journal update is idempotent.
  }
}

export function readFollowedTasks(storyId: string, branchId: string): string[] {
  return read(storageKey(storyId, branchId))
}

export function writeFollowedTasks(storyId: string, branchId: string, taskIds: string[]) {
  write(storageKey(storyId, branchId), taskIds)
}
