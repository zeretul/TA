import { expect, it } from 'vitest'
import { buildLoreOverview } from './model'
it('distinguishes two registered routes to the same place without changing the unnamed legacy route', () => {
  const route = { route_id: 'legacy', route_version: '1', origin_location_id: 'a', origin_label: 'Here', destination_location_id: 'b', destination_label: 'There', total_minutes: 20, movement_method: 'walk' }
  const model = buildLoreOverview({ snapshot: null, characters: null, quests: null, events: null,
    journey: { story_id: 'story', branch_id: 'main', clock_seconds: 0, location: 'a', routes: [route, { ...route, route_id: 'shortcut', total_minutes: 10, name: '药窟背路', name_key: 'game.realm.shortcuts.herbCave' }] },
  })
  const rows = model.entries.find(entry => entry.id === 'journey:routes')?.rows
  expect(rows).toEqual([
    { id: 'legacy', label: 'Here', labelKey: undefined, value: 'There · 20' },
    { id: 'shortcut', label: '药窟背路', labelKey: 'game.realm.shortcuts.herbCave', value: 'There · 10' },
  ])
})
it('keeps localized spirit routes when committed location is an English alias', () => {
  const model = buildLoreOverview({ snapshot: null, characters: null, quests: null, events: null,
    journey: { story_id: 'story', branch_id: 'main', clock_seconds: 0,
      location: 'Sunset City workshops', location_id: 'spirit.sunset.market', routes: [{
        route_id: 'spirit-world.tianyuan-road:out', route_version: '1', origin_location_id: 'spirit.sunset.market',
        origin_label: '灵界落日城百工坊', origin_name_key: 'game.spirit.place.spirit.sunset.market',
        destination_location_id: 'spirit.tianyuan.market', destination_label: '灵界天渊城南坊', destination_name_key: 'game.spirit.place.spirit.tianyuan.market',
        total_minutes: 43200, movement_method: 'walk',
      }] },
  })
  expect(model.entries.find(entry => entry.id === 'journey:routes')?.rows).toEqual([
    { id: 'spirit-world.tianyuan-road:out', label: '灵界落日城百工坊', labelKey: 'game.spirit.place.spirit.sunset.market',
      value: '灵界天渊城南坊 · 43200', valueKey: 'game.spirit.place.spirit.tianyuan.market', valueSuffix: ' · 43200' },
  ])
})
