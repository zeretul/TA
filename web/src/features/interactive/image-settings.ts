import type { StoryImageSettings } from './types'

export const DEFAULT_IMAGE_INTERVAL_TURNS = 3

// The style a picture falls back to when a story names none. It matches the
// shipped default the backend seeds, so a story whose stored style was retired
// still opens on a style this build actually has.
export const DEFAULT_IMAGE_PRESET_ID = 'realistic'

export function normalizeStoryImageSettings(value?: Partial<StoryImageSettings> | null): StoryImageSettings {
  const rawMode = typeof value?.mode === 'string' ? String(value.mode) : ''
  return {
    mode: rawMode === 'interval' || rawMode === 'every_turn' ? 'interval' : 'manual',
    interval_turns: rawMode === 'every_turn' ? 1 : normalizeImageIntervalTurns(value?.interval_turns),
    preset_id: typeof value?.preset_id === 'string' && value.preset_id.trim() ? value.preset_id.trim() : DEFAULT_IMAGE_PRESET_ID,
  }
}

export function normalizeImageIntervalTurns(value: unknown): number {
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue) || numberValue <= 0) return DEFAULT_IMAGE_INTERVAL_TURNS
  return Math.min(50, Math.max(1, Math.floor(numberValue)))
}
