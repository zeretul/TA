import { beforeEach, describe, expect, it } from 'vitest'
import { migrateLegacyStorage } from './storage-migration'

const MARKER = 'ta.migration.storagePrefix.v2'

describe('migrateLegacyStorage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.sessionStorage.clear()
  })

  it('copies entries from both earlier prefixes', () => {
    window.localStorage.setItem('nova.layout.activityBarExpanded', 'true')
    window.localStorage.setItem('denova.story-window.geometry.character', '{"x":1}')
    window.sessionStorage.setItem('nova.remoteAccess.credentials', 'pair')

    migrateLegacyStorage()

    expect(window.localStorage.getItem('ta.layout.activityBarExpanded')).toBe('true')
    expect(window.localStorage.getItem('ta.story-window.geometry.character')).toBe('{"x":1}')
    expect(window.sessionStorage.getItem('ta.remoteAccess.credentials')).toBe('pair')
    expect(window.localStorage.getItem(MARKER)).toBe('done')
  })

  it('keeps the current prefix value when both exist', () => {
    window.localStorage.setItem('nova.layout.activityBarExpanded', 'false')
    window.localStorage.setItem('ta.layout.activityBarExpanded', 'true')

    migrateLegacyStorage()

    expect(window.localStorage.getItem('ta.layout.activityBarExpanded')).toBe('true')
  })

  it('leaves the old entries in place so a rollback still reads them', () => {
    window.localStorage.setItem('nova.layout.settingsOpen', 'true')

    migrateLegacyStorage()

    expect(window.localStorage.getItem('nova.layout.settingsOpen')).toBe('true')
  })

  it('runs once and does not resurrect a value the user has since cleared', () => {
    window.localStorage.setItem('nova.layout.settingsOpen', 'true')
    migrateLegacyStorage()
    window.localStorage.removeItem('ta.layout.settingsOpen')

    migrateLegacyStorage()

    expect(window.localStorage.getItem('ta.layout.settingsOpen')).toBeNull()
  })
})
