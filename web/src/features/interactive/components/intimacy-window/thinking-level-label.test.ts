import { describe, expect, it } from 'vitest'
import { createInstance } from 'i18next'
import zhCN from '@/i18n/locales/zh-CN'
import { thinkingLevelLabel } from './thinking-level-label'

/**
 * The model line names the level the player chose. A stored `high` is a
 * configuration value, not a word a player reads, so every level the selector
 * offers has to come back as its Chinese label — and a value the interface does
 * not know must not be echoed in English either.
 */

function translator() {
  const instance = createInstance()
  instance.init({ lng: 'zh-CN', resources: { 'zh-CN': { translation: zhCN } }, interpolation: { escapeValue: false } })
  return instance.t
}

describe('thinking level wording', () => {
  it('words every level the selector offers', () => {
    const t = translator()
    expect(thinkingLevelLabel('off', t)).toBe('关闭')
    expect(thinkingLevelLabel('low', t)).toBe('低')
    expect(thinkingLevelLabel('medium', t)).toBe('中')
    expect(thinkingLevelLabel('high', t)).toBe('高')
  })

  it('never shows the stored value itself', () => {
    const t = translator()
    for (const level of ['off', 'low', 'medium', 'high']) {
      expect(thinkingLevelLabel(level, t)).not.toBe(level)
    }
  })

  it('names a level it does not know in plain language rather than echoing it', () => {
    const t = translator()
    const label = thinkingLevelLabel('some_future_level', t)
    expect(label).toBe('其他')
    expect(label).not.toContain('some_future_level')
  })
})
