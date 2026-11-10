import type { WorldEntity } from './types'

// Newly recorded forts and towns inherit their known parent region's map
// anchor. Unknown or cyclic locations stay unplaced rather than being guessed.
export function resolveMapRegion(location: string | undefined, entities: ReadonlyMap<string, WorldEntity>, regions: ReadonlySet<string>): string | undefined {
  const seen = new Set<string>()
  while (location && !seen.has(location)) {
    if (regions.has(location)) return location
    seen.add(location)
    location = entities.get(location)?.location
  }
  return undefined
}
