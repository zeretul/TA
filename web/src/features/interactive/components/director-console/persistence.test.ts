import { beforeEach, describe, expect, it } from 'vitest'
import { readStoredDirectorConsoleTab, readStoredDirectorRevealed, writeStoredDirectorConsoleTab, writeStoredDirectorRevealed } from './persistence'

describe('director-console persistence', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('round-trips the director reveal flag per story', () => {
    expect(readStoredDirectorRevealed('story-a')).toBe(false)
    writeStoredDirectorRevealed('story-a', true)
    expect(readStoredDirectorRevealed('story-a')).toBe(true)
    expect(readStoredDirectorRevealed('story-b')).toBe(false)
    writeStoredDirectorRevealed('story-a', false)
    expect(readStoredDirectorRevealed('story-a')).toBe(false)
  })

  it('round-trips every handbook tab per story and rejects unknown values', () => {
    expect(readStoredDirectorConsoleTab('story-a')).toBeNull()
    writeStoredDirectorConsoleTab('story-a', 'recent')
    writeStoredDirectorConsoleTab('story-b', 'history')
    expect(readStoredDirectorConsoleTab('story-a')).toBe('recent')
    expect(readStoredDirectorConsoleTab('story-b')).toBe('history')
    window.localStorage.setItem('ta.directorConsole.tab.story-a', 'bogus')
    expect(readStoredDirectorConsoleTab('story-a')).toBeNull()
  })

  it('migrates former state, branch and tuning tab preferences', () => {
    window.localStorage.setItem('ta.directorConsole.stateTab.story-a', 'world')
    window.localStorage.setItem('ta.directorConsole.tab.story-b', 'branches')
    window.localStorage.setItem('ta.directorConsole.tab.story-c', 'tuning')
    window.localStorage.setItem('ta.directorConsole.tab.story-d', 'controls')
    window.localStorage.setItem('ta.directorConsole.tab.story-e', 'overview')
    window.localStorage.setItem('ta.directorConsole.tab.story-f', 'routes')
    expect(readStoredDirectorConsoleTab('story-a')).toBe('recent')
    expect(readStoredDirectorConsoleTab('story-b')).toBe('history')
    expect(readStoredDirectorConsoleTab('story-c')).toBe('recent')
    expect(readStoredDirectorConsoleTab('story-d')).toBe('recent')
    expect(readStoredDirectorConsoleTab('story-e')).toBe('recent')
    expect(readStoredDirectorConsoleTab('story-f')).toBe('history')
  })
})
