import { expect, it } from 'vitest'
import { resolveMapRegion } from './world-map'
import type { WorldEntity } from './types'

it('places new settlements under their recorded parent and follows later movement', () => {
  const entities = new Map([['fort', { id: 'fort', location: 'town' }], ['town', { id: 'town', location: 'north' }]]) as Map<string, WorldEntity>
  const regions = new Set(['north', 'south'])
  expect(resolveMapRegion('fort', entities, regions)).toBe('north')
  entities.set('town', { id: 'town', location: 'south' } as WorldEntity)
  expect(resolveMapRegion('fort', entities, regions)).toBe('south')
  expect(resolveMapRegion('unknown', entities, regions)).toBeUndefined()
})

it('does not loop forever or invent a position for cyclic locations', () => {
  const entities = new Map([['a', { id: 'a', location: 'b' }], ['b', { id: 'b', location: 'a' }]]) as Map<string, WorldEntity>
  expect(resolveMapRegion('a', entities, new Set(['north']))).toBeUndefined()
})
