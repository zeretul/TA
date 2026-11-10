import { describe, expect, it } from 'vitest'
import { selectWorkbenchRoute } from './WorkbenchRouteHost'
import type { WorkspaceMode } from '@/stores/workspace-store'

const PLAYER_MODES: WorkspaceMode[] = ['interactive', 'history', 'games', 'lore', 'game-settings', 'assistant', 'books']

/** Stored or programmatic values naming a retired authoring surface. */
const RETIRED_MODES = ['ide', 'writing', 'skills', 'agents', 'automations', 'agentchat', 'trajectory', 'versions', 'presets']

describe('selectWorkbenchRoute', () => {
  it('opens settings above whatever route is current', () => {
    expect(selectWorkbenchRoute({ mode: 'interactive', rightPanel: null, settingsOpen: true })).toBe('settings')
  })

  it('keeps every player destination reachable', () => {
    for (const mode of PLAYER_MODES) {
      expect(selectWorkbenchRoute({ mode, rightPanel: null, settingsOpen: false })).toBe(mode)
    }
  })

  it('does not re-open a retired authoring surface from a stored or programmatic mode', () => {
    for (const mode of RETIRED_MODES) {
      expect(selectWorkbenchRoute({
        mode: mode as WorkspaceMode,
        rightPanel: null,
        settingsOpen: false,
      })).toBe('interactive')
    }
  })
})
