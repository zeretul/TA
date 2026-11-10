import { describe, expect, it } from 'vitest'
import { createInstance } from 'i18next'
import zhCN from '@/i18n/locales/zh-CN'
import type { BranchSummary } from '../../types'
import { branchDisplayName } from './model'

/**
 * The director console header shows which route the player is on. The stored
 * branch ID is `br_...`, which says nothing about the story, so the header reads
 * the route's own name through this same helper the timeline uses.
 */

function translator() {
  const instance = createInstance()
  instance.init({ lng: 'zh-CN', resources: { 'zh-CN': { translation: zhCN } }, interpolation: { escapeValue: false } })
  return instance.t
}

function labels(t: ReturnType<typeof translator>) {
  return { main: t('branchTimeline.mainBranch'), unknown: t('branchTimeline.unknownBranch') }
}

function branch(overrides: Partial<BranchSummary> = {}): BranchSummary {
  return {
    id: 'br_dmcn0a1b2c',
    head: 'ev_1',
    title: '雨夜改道',
    created_at: '2026-10-24T00:00:00Z',
    current: false,
    ...overrides,
  }
}

describe('the route a console header names', () => {
  it('reads the route title the story gave it, never the stored id', () => {
    const name = branchDisplayName(branch(), labels(translator()))
    expect(name).toBe('雨夜改道')
    expect(name).not.toContain('br_')
  })

  it('calls the main line 主线', () => {
    expect(branchDisplayName(branch({ id: 'main' }), labels(translator()))).toBe('主线')
  })

  it('has a word for a route it was not given', () => {
    const name = branchDisplayName(undefined, labels(translator()))
    expect(name).toBe('未知剧情线')
    expect(name).not.toContain('br_')
  })

  it('still names a route whose title was never filled in', () => {
    // An older branch may carry no title at all. The id is what remains, and a
    // blank header would be worse than storage's own name.
    expect(branchDisplayName(branch({ title: '' }), labels(translator()))).toBe('br_dmcn0a1b2c')
  })
})
