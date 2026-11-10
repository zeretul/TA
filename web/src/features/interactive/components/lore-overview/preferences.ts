/**
 * How the player last read the overview.
 *
 * These are interface preferences, not world facts: which tab was open, how wide
 * the panel was. They are kept in localStorage so returning to the same story
 * and branch puts the reader back where they were, and they never become a
 * second source for anything the journal owns.
 */
const TAB_KEY_PREFIX = 'ta.loreOverview.tab.'
const WIDTH_KEY = 'ta.loreOverview.width'

/** The width the panel opens at the first time, in pixels. */
export const DEFAULT_LORE_OVERVIEW_WIDTH = 380
export const MIN_LORE_OVERVIEW_WIDTH = 320
export const MAX_LORE_OVERVIEW_WIDTH = 640
/** Prose the panel must leave visible before it stops widening. */
export const MIN_STORY_BODY_WIDTH = 360

function read(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

function write(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value)
  } catch {
    // 隐私模式等场景下 localStorage 不可用，静默降级为会话内状态。
  }
}

export function readLoreOverviewTab(storyId: string, branchId: string): string | null {
  return read(`${TAB_KEY_PREFIX}${storyId || 'default'}.${branchId || 'main'}`)
}

export function writeLoreOverviewTab(storyId: string, branchId: string, tab: string) {
  write(`${TAB_KEY_PREFIX}${storyId || 'default'}.${branchId || 'main'}`, tab)
}

export function readLoreOverviewWidth(): number {
  const raw = read(WIDTH_KEY)
  const parsed = raw ? Number.parseInt(raw, 10) : Number.NaN
  if (!Number.isFinite(parsed)) return DEFAULT_LORE_OVERVIEW_WIDTH
  return Math.min(MAX_LORE_OVERVIEW_WIDTH, Math.max(MIN_LORE_OVERVIEW_WIDTH, parsed))
}

export function writeLoreOverviewWidth(width: number) {
  write(WIDTH_KEY, String(Math.round(width)))
}
