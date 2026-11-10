import { requestJSON } from '@/lib/api-client'
import type { StorySummary } from './types'

export interface GameSave {
  project_id: string
  world_name: string
  workspace: string
  story: StorySummary
}
export interface GameSaveCatalog {
  saves: GameSave[]
  unavailable: string[]
}
export function getGameSaves(): Promise<GameSaveCatalog> {
  return requestJSON('/api/game-saves')
}
