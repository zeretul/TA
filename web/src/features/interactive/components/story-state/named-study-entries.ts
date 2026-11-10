import type { LedgerFieldEntry } from './model'
import type { StoryDirectorActorStateSystem } from '../../types'
import { isMethodRecordField } from './methods'
import { isSkillRecordField } from './skills'

const record = (value: unknown): value is Record<string, unknown> => Boolean(value) && typeof value === 'object' && !Array.isArray(value)
const text = (value: unknown): string => typeof value === 'string' ? value.trim() : ''

/** Display only, from this snapshot's exact registered definition. Never list
 * the registry, alter a saved record, or enrich another person's private sheet. */
export function namedStudyEntries(entries: LedgerFieldEntry[], state: unknown, actorId: string, unnamed: string, frozen?: { craftMethods: StoryDirectorActorStateSystem['craft_methods']; name: (key: string) => string }): LedgerFieldEntry[] {
  if (actorId !== 'protagonist') return entries
  const actors = record(state) && record(state.actors) ? state.actors : {}
  const story = record(actors.story) && record(actors.story.state) ? actors.story.state : {}
  const registry = record(story['技能定义']) ? story['技能定义'] : {}
  return entries.map(entry => {
    if (entry.withheld || !record(entry.value) || (!isSkillRecordField(entry.field) && !isMethodRecordField(entry.field))) return entry
    const value = Object.fromEntries(Object.entries(entry.value).map(([key, stored]) => {
      if (!record(stored)) return [key, stored]
      const id = text(stored['定义'])
      const definition = registry[id]
      const resolved = record(definition) && text(definition['定义']) === id && definition['版本'] === stored['版本'] ? definition : undefined
      const frozenCraft = frozen?.craftMethods?.find(method => method.id === id && method.version === stored['版本'])
      const translated = frozenCraft ? text(frozen?.name(frozenCraft.name_key)) : ''
      const frozenName = translated && translated !== frozenCraft?.name_key ? translated : ''
      const internalID = id.startsWith('gen.') || id.startsWith('craft.')
      const internalKey = internalID && (key === id || key.includes(`[${id}]`))
      const name = text(stored['名称']) || text(resolved?.['名称']) || frozenName || (internalKey ? unnamed : '')
      return [key, { ...stored, ...(name ? { 名称: name } : {}), ...(resolved?.['类别'] === 'craft' || frozenCraft ? { 类别: 'craft' } : !resolved && !frozenCraft && internalID ? { 类别: 'unverified' } : {}) }]
    }))
    return { ...entry, value }
  })
}
