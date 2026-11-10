import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

function browserState(mode?: string, search = '') {
  const values = new Map<string, string>(mode ? [['nova:mode', mode]] : [])
  vi.stubGlobal('window', {
    location: { search },
    localStorage: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key),
    },
  })
  return values
}

describe('merged game destination persistence', () => {
  beforeEach(() => vi.resetModules())
  afterEach(() => vi.unstubAllGlobals())

  it.each(['interactive', 'history', 'games', 'assistant'] as const)(
    'restores the saved %s destination', async (mode) => {
      browserState(mode)
      const { useWorkspaceStore } = await import('./workspace-store')
      expect(useWorkspaceStore.getState().mode).toBe(mode)
    },
  )

  it('honors an explicit history link without replacing the saved project', async () => {
    const values = browserState('assistant', '?game=history')
    const { useWorkspaceStore } = await import('./workspace-store')
    expect(useWorkspaceStore.getState().mode).toBe('history')
    expect(values.get('nova:mode')).toBe('assistant')
    useWorkspaceStore.getState().setSelectedProjectId('adventure-project')
    for (const mode of ['games', 'assistant', 'history', 'interactive'] as const) {
      useWorkspaceStore.getState().setMode(mode)
      expect(useWorkspaceStore.getState().selectedProjectId).toBe('adventure-project')
      expect(values.get('nova:mode')).toBe(mode)
    }
  })

  it('keeps history independent while preserving the shared assistant', async () => {
    browserState()
    const { isSharedWorkspaceMode, workspaceModeRequiresBook } = await import('./workspace-store')
    expect(isSharedWorkspaceMode('assistant')).toBe(true)
    for (const mode of ['history', 'games'] as const) {
      expect(isSharedWorkspaceMode(mode)).toBe(false)
      expect(workspaceModeRequiresBook(mode)).toBe(false)
    }
    expect(workspaceModeRequiresBook('interactive')).toBe(true)
  })

  it('does not restore a retired authoring destination', async () => {
    browserState('writing')
    const { useWorkspaceStore } = await import('./workspace-store')
    expect(useWorkspaceStore.getState().mode).toBe('interactive')
  })
})
