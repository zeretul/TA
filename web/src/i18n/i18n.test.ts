import { afterEach, describe, expect, it, vi } from 'vitest'
import zhCN from './locales/zh-CN'
import enUS from './locales/en-US'

describe('i18n', () => {
  afterEach(() => {
    window.localStorage.clear()
    setBrowserLanguage('zh-CN')
  })

  it('resolves auto from browser language', async () => {
    const { resolveLocale } = await import('./index')

    expect(resolveLocale('auto', 'zh-Hans')).toBe('zh-CN')
    expect(resolveLocale('auto', 'en-GB')).toBe('en-US')
  })

  it('keeps locale resource keys aligned', () => {
    expect(Object.keys(enUS).sort()).toEqual(Object.keys(zhCN).sort())
  })

  it('boots from the locally cached configured locale before browser language', async () => {
    vi.resetModules()
    setBrowserLanguage('en-US')
    window.localStorage.setItem('ta.locale.configured', 'zh-CN')

    const { default: i18next, getConfiguredLocale, getResolvedLocale } = await import('./index')

    expect(getConfiguredLocale()).toBe('zh-CN')
    expect(getResolvedLocale()).toBe('zh-CN')
    expect(i18next.language).toBe('zh-CN')
    expect(document.documentElement.lang).toBe('zh-CN')
  })

  it('persists the configured locale after settings are loaded', async () => {
    vi.resetModules()
    const { setConfiguredLocale } = await import('./index')

    setConfiguredLocale('en-US')

    expect(window.localStorage.getItem('ta.locale.configured')).toBe('en-US')
  })

  it('uses the canonical Writing Agent and Game Agent names in both locales', () => {
    expect(zhCN['agents.ide.title']).toBe('写作 Agent')
    expect(zhCN['agents.interactiveStory.title']).toBe('游戏 Agent')
    expect(zhCN['agents.subAgents.parent.ide']).toBe('写作')
    expect(zhCN['agents.subAgents.parent.interactive_story']).toBe('游戏')
    expect(enUS['agents.ide.title']).toBe('Writing Agent')
    expect(enUS['agents.interactiveStory.title']).toBe('Game Agent')
    expect(enUS['agents.subAgents.parent.ide']).toBe('Writing')
    expect(enUS['agents.subAgents.parent.interactive_story']).toBe('Game')
  })

  it('formats user-facing times with a 24-hour clock', async () => {
    vi.resetModules()
    const { formatDateTime, setConfiguredLocale } = await import('./index')
    setConfiguredLocale('en-US')

    const formatted = formatDateTime(new Date(2026, 7, 26, 23, 4))

    expect(formatted).toContain('23:04')
    expect(formatted).not.toMatch(/\b(?:AM|PM)\b/i)
  })

  // A key with no entry in either locale is a defect in the code, not something
  // to show the player: the key name itself used to land in the middle of a
  // sentence. It reads as nothing instead, and the console names it once.
  it('shows nothing rather than the key name for a key no locale defines', async () => {
    vi.resetModules()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { default: i18next } = await import('./index')

    expect(i18next.t('no.such.key.anywhere')).toBe('')
    expect(warn).toHaveBeenCalledWith('[i18n] Missing translation key', 'no.such.key.anywhere')

    // The same key twice must not repeat the warning on every render.
    i18next.t('no.such.key.anywhere')
    expect(warn.mock.calls.filter(([first]) => first === '[i18n] Missing translation key')).toHaveLength(1)
    warn.mockRestore()
  })
})

function setBrowserLanguage(language: string) {
  Object.defineProperty(window.navigator, 'languages', {
    configurable: true,
    value: [language],
  })
  Object.defineProperty(window.navigator, 'language', {
    configurable: true,
    value: language,
  })
}
